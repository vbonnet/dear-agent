package workflow

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite" // pure-Go SQLite driver; matches agm/internal/db
)

//go:embed schema.sql
var sqliteSchema string

// sqliteOpenOptions are the pragma values every runs.db needs:
//   - journal_mode=WAL: lets readers and one writer make progress concurrently
//   - busy_timeout=5000: each writer retries for 5s rather than failing
//     immediately with SQLITE_BUSY when another writer holds the lock
//   - foreign_keys=on: enforce the cascade rules declared in schema.sql
//
// Syntax follows modernc.org/sqlite (each pragma is its own _pragma= arg).
const sqliteOpenOptions = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)"

// SQLiteState is a SQLite-backed implementation of the State interface and
// the substrate-quality storage layer for the workflow engine. A single
// SQLiteState binds to one run_id; the underlying database may hold many
// runs, queryable via CLI tooling that opens the same file.
//
// Construction:
//
//	ss, err := workflow.OpenSQLiteState("runs.db")
//	defer ss.Close()
//	r.State = ss
//
// To resume a specific run:
//
//	ss, err := workflow.ResumeSQLiteState("runs.db", runID)
//
// SQLiteState satisfies State, RunRecorder, and AuditSink — wiring it as
// all three is a one-liner via Runner.UseSQLiteState.
type SQLiteState struct {
	db     *sql.DB
	ownsDB bool

	// runID is empty until the first Save (or set explicitly by Resume).
	// Generated as a UUID on first use; surfaced via RunID for callers
	// that want to record or query against the run later.
	runID string

	// now is overridable in tests so timestamps are deterministic.
	now func() time.Time

	// idGen produces UUIDs for child rows (events, attempts). Overridable
	// for tests that want stable golden output.
	idGen func() string
}

// OpenSQLiteState opens (or creates) a runs.db at path, applies the schema,
// and returns a SQLiteState ready to host a fresh run. The first Save
// generates a run_id (accessible via RunID); subsequent Saves update that
// same run.
//
// WAL mode is enabled so multiple SQLiteStates may write concurrently to
// the same file (the engine targets ~10 concurrent writers — see ADR-010).
func OpenSQLiteState(path string) (*SQLiteState, error) {
	db, err := openSQLiteDB(path)
	if err != nil {
		return nil, err
	}
	return &SQLiteState{
		db:     db,
		ownsDB: true,
		now:    time.Now,
		idGen:  uuid.NewString,
	}, nil
}

// ResumeSQLiteState opens path and binds to an existing run_id. Save on
// the returned state updates that run; Load returns its snapshot. Returns
// an error if runID is empty or does not exist in the DB.
func ResumeSQLiteState(path, runID string) (*SQLiteState, error) {
	if runID == "" {
		return nil, fmt.Errorf("workflow: ResumeSQLiteState: runID is empty")
	}
	db, err := openSQLiteDB(path)
	if err != nil {
		return nil, err
	}
	ss := &SQLiteState{
		db:     db,
		ownsDB: true,
		runID:  runID,
		now:    time.Now,
		idGen:  uuid.NewString,
	}
	// Verify the run exists so misspelled IDs surface immediately rather
	// than silently producing an empty Load.
	var exists int
	if err := db.QueryRowContext(context.Background(), `SELECT 1 FROM runs WHERE run_id = ?`, runID).Scan(&exists); err != nil {
		_ = db.Close()
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("workflow: ResumeSQLiteState: run_id %q not found in %s", runID, path)
		}
		return nil, fmt.Errorf("workflow: ResumeSQLiteState: lookup run: %w", err)
	}
	return ss, nil
}

// openSQLiteDB is the internal connector. Centralized so ttl/WAL/foreign-key
// pragmas stay consistent everywhere.
func openSQLiteDB(path string) (*sql.DB, error) {
	dsn := path + sqliteOpenOptions
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("workflow: open %s: %w", path, err)
	}
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("workflow: ping %s: %w", path, err)
	}
	if _, err := db.ExecContext(ctx, sqliteSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("workflow: apply schema: %w", err)
	}
	return db, nil
}

// Close releases the underlying connection if SQLiteState owns it.
// Calling Close on a SQLiteState that wraps a caller-provided *sql.DB
// (via NewSQLiteStateWithDB) is a no-op.
func (s *SQLiteState) Close() error {
	if s == nil || s.db == nil || !s.ownsDB {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

// DB returns the underlying *sql.DB so callers (CLI tooling, test setup)
// can issue ad-hoc queries. The caller MUST NOT close it — Close on the
// SQLiteState owns the lifecycle.
func (s *SQLiteState) DB() *sql.DB { return s.db }

// RunID returns the run identifier this state is bound to. Empty until
// the first Save (for a freshly opened state).
func (s *SQLiteState) RunID() string { return s.runID }

// Save persists snap. On the first call (when no run exists yet) it
// inserts a workflows row keyed by hash(name+version) and a runs row keyed
// by a freshly generated run_id; subsequent calls update the same run.
//
// Per-node aggregate state (nodes table) is updated for every node whose
// id appears in snap.Outputs OR snap.Completed: any node mentioned in the
// snapshot exists, and is marked 'succeeded' if Completed is true.
func (s *SQLiteState) Save(ctx context.Context, snap Snapshot) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("workflow: SQLiteState.Save: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if s.runID == "" {
		if err := s.initRun(ctx, tx, snap); err != nil {
			return err
		}
	}

	now := s.now()
	if _, err := tx.ExecContext(ctx,
		`UPDATE runs SET inputs_json = ?, finished_at = ? WHERE run_id = ?`,
		mustMarshalJSON(snap.Inputs), nullTime(snap.UpdatedAt, now), s.runID,
	); err != nil {
		return fmt.Errorf("workflow: SQLiteState.Save: update runs: %w", err)
	}

	// Walk every node mentioned in either Outputs or Completed so we
	// don't lose pending entries (Outputs without Completed) or completed
	// entries with empty outputs.
	seen := make(map[string]struct{}, len(snap.Outputs)+len(snap.Completed))
	for id := range snap.Outputs {
		seen[id] = struct{}{}
	}
	for id := range snap.Completed {
		seen[id] = struct{}{}
	}
	for nodeID := range seen {
		state := "pending"
		if snap.Completed[nodeID] {
			state = "succeeded"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO nodes (run_id, node_id, state, output, finished_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (run_id, node_id) DO UPDATE SET
			    state       = excluded.state,
			    output      = excluded.output,
			    finished_at = excluded.finished_at
		`, s.runID, nodeID, state, snap.Outputs[nodeID], nullTime(snap.UpdatedAt, now)); err != nil {
			return fmt.Errorf("workflow: SQLiteState.Save: upsert node %q: %w", nodeID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("workflow: SQLiteState.Save: commit: %w", err)
	}
	return nil
}

// initRun inserts the workflows + runs rows on the first Save. Caller
// must hold an open transaction. This is the legacy path used when only
// State (not RunRecorder) is wired; the substrate path goes through
// BeginRun, which records s.runID before any Save runs.
func (s *SQLiteState) initRun(ctx context.Context, tx *sql.Tx, snap Snapshot) error {
	wfID := workflowIDForName(snap.Workflow)
	now := s.now()
	started := snap.Started
	if started.IsZero() {
		started = now
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO workflows (workflow_id, name, version, yaml_canonical, registered_at)
		VALUES (?, ?, '', '', ?)
		ON CONFLICT (workflow_id) DO NOTHING
	`, wfID, snap.Workflow, now); err != nil {
		return fmt.Errorf("workflow: SQLiteState.Save: upsert workflow: %w", err)
	}

	s.runID = s.idGen()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO runs (run_id, workflow_id, state, inputs_json, started_at, trigger)
		VALUES (?, ?, 'running', ?, ?, 'sdk')
	`, s.runID, wfID, mustMarshalJSON(snap.Inputs), started); err != nil {
		return fmt.Errorf("workflow: SQLiteState.Save: insert run: %w", err)
	}
	return nil
}

// ----- RunRecorder implementation -----

// BeginRun inserts the workflows + runs rows for a new run. The runner
// calls this before executing any node so subsequent audit events and
// attempt records have a parent row to reference. Idempotent: if a run
// with the same ID already exists, BeginRun returns nil without changes.
func (s *SQLiteState) BeginRun(ctx context.Context, rec RunRecord) error {
	if rec.RunID == "" {
		return fmt.Errorf("workflow: BeginRun: RunID required")
	}
	wfID := rec.WorkflowID
	if wfID == "" {
		wfID = workflowIDForName(rec.WorkflowName)
	}
	state := string(rec.State)
	if state == "" {
		state = string(RunStateRunning)
	}
	trigger := rec.Trigger
	if trigger == "" {
		trigger = "sdk"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("workflow: BeginRun: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO workflows (workflow_id, name, version, yaml_canonical, registered_at)
		VALUES (?, ?, '', '', ?)
		ON CONFLICT (workflow_id) DO NOTHING
	`, wfID, rec.WorkflowName, s.now()); err != nil {
		return fmt.Errorf("workflow: BeginRun: upsert workflow: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO runs (run_id, workflow_id, state, inputs_json, started_at, trigger, triggered_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (run_id) DO NOTHING
	`, rec.RunID, wfID, state, rec.InputsJSON, rec.StartedAt, trigger, rec.TriggeredBy); err != nil {
		return fmt.Errorf("workflow: BeginRun: insert run: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("workflow: BeginRun: commit: %w", err)
	}
	s.runID = rec.RunID
	return nil
}

// UpsertNode writes (or updates) one row in the nodes table. Called on
// every node-state transition so the per-run, per-node aggregate stays
// fresh — readers (CLI, dashboards) join against this table.
func (s *SQLiteState) UpsertNode(ctx context.Context, rec NodeRecord) error {
	if rec.RunID == "" || rec.NodeID == "" {
		return fmt.Errorf("workflow: UpsertNode: RunID and NodeID required")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO nodes (
		    run_id, node_id, state, attempts, role_used, model_used,
		    tokens_used, dollars_spent, output, started_at, finished_at, error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (run_id, node_id) DO UPDATE SET
		    state         = excluded.state,
		    attempts      = excluded.attempts,
		    role_used     = excluded.role_used,
		    model_used    = excluded.model_used,
		    tokens_used   = excluded.tokens_used,
		    dollars_spent = excluded.dollars_spent,
		    output        = excluded.output,
		    started_at    = COALESCE(nodes.started_at, excluded.started_at),
		    finished_at   = excluded.finished_at,
		    error         = excluded.error
	`,
		rec.RunID, rec.NodeID, string(rec.State), rec.Attempts, rec.RoleUsed, rec.ModelUsed,
		rec.TokensUsed, rec.DollarsSpent, rec.Output,
		nullableTime(rec.StartedAt), nullableTime(rec.FinishedAt), rec.Error,
	)
	if err != nil {
		return fmt.Errorf("workflow: UpsertNode %s/%s: %w", rec.RunID, rec.NodeID, err)
	}
	return nil
}

// RecordAttempt inserts a row into node_attempts. One row per attempt;
// retries appear as separate rows with incrementing attempt_no.
func (s *SQLiteState) RecordAttempt(ctx context.Context, rec AttemptRecord) error {
	if rec.RunID == "" || rec.NodeID == "" {
		return fmt.Errorf("workflow: RecordAttempt: RunID and NodeID required")
	}
	if rec.AttemptID == "" {
		rec.AttemptID = s.idGen()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO node_attempts (
		    attempt_id, run_id, node_id, attempt_no, state, model_used,
		    prompt_hash, response_hash, tokens_used, dollars_spent,
		    started_at, finished_at, error_class, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		rec.AttemptID, rec.RunID, rec.NodeID, rec.AttemptNo, string(rec.State), rec.ModelUsed,
		rec.PromptHash, rec.ResponseHash, rec.TokensUsed, rec.DollarsSpent,
		rec.StartedAt, nullableTime(rec.FinishedAt), rec.ErrorClass, rec.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("workflow: RecordAttempt %s/%s#%d: %w",
			rec.RunID, rec.NodeID, rec.AttemptNo, err)
	}
	return nil
}

// FinishRun marks the run terminal — sets state, finished_at, and the
// optional error column. The runner calls this exactly once at the end of
// a run, regardless of outcome.
func (s *SQLiteState) FinishRun(ctx context.Context, runID string, state RunState, finishedAt time.Time, errMsg string) error {
	if runID == "" {
		return fmt.Errorf("workflow: FinishRun: runID required")
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE runs SET state = ?, finished_at = ?, error = ? WHERE run_id = ?
	`, string(state), nullableTime(finishedAt), errMsg, runID)
	if err != nil {
		return fmt.Errorf("workflow: FinishRun %s: %w", runID, err)
	}
	return nil
}

// ----- AuditSink implementation -----

// Emit writes one row to audit_events. The runner calls this on every
// state transition; the row is the canonical "what happened" record.
func (s *SQLiteState) Emit(ctx context.Context, ev AuditEvent) error {
	if ev.RunID == "" {
		return fmt.Errorf("workflow: Emit: RunID required")
	}
	if ev.EventID == "" {
		ev.EventID = s.idGen()
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = s.now()
	}
	if ev.Actor == "" {
		ev.Actor = "system"
	}
	var payloadJSON string
	if len(ev.Payload) > 0 {
		payloadJSON = mustMarshalJSON(ev.Payload)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_events (
		    event_id, run_id, node_id, attempt_no, from_state, to_state,
		    reason, actor, occurred_at, payload_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		ev.EventID, ev.RunID, nullableString(ev.NodeID), nullableInt(ev.AttemptNo),
		nullableString(ev.FromState), ev.ToState, ev.Reason, ev.Actor,
		ev.OccurredAt, payloadJSON,
	)
	if err != nil {
		return fmt.Errorf("workflow: Emit: %w", err)
	}
	return nil
}

// nullableTime returns nil for the zero time so the column stores NULL
// rather than 0001-01-01. SQLite's TIMESTAMP type doesn't enforce this
// distinction, but downstream tools (Go's time.Time scan, JSON encoding)
// behave more predictably with explicit NULLs.
func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// nullableString stores NULL for the empty string so callers can
// distinguish "unset" from "explicitly empty". Used for nullable text
// columns like nodes.error and audit_events.from_state.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullableInt stores NULL for zero values. attempt_no=0 is the documented
// sentinel for "not attempt-specific" (run-level events use it).
func nullableInt(i int) any {
	if i == 0 {
		return nil
	}
	return i
}

// Load returns the snapshot for this state's run. Returns (nil, nil) if
// no run has been saved yet — matching FileState semantics for "no
// checkpoint exists".
func (s *SQLiteState) Load(ctx context.Context) (*Snapshot, error) {
	if s.runID == "" {
		return nil, nil
	}
	var (
		wfName     string
		inputsJSON string
		startedAt  time.Time
		updatedAt  sql.NullTime
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT w.name, r.inputs_json, r.started_at, r.finished_at
		FROM runs r
		JOIN workflows w ON r.workflow_id = w.workflow_id
		WHERE r.run_id = ?
	`, s.runID).Scan(&wfName, &inputsJSON, &startedAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("workflow: SQLiteState.Load: select run: %w", err)
	}

	snap := &Snapshot{
		Workflow:  wfName,
		Inputs:    map[string]string{},
		Outputs:   map[string]string{},
		Completed: map[string]bool{},
		Started:   startedAt,
	}
	if updatedAt.Valid {
		snap.UpdatedAt = updatedAt.Time
	}
	if err := json.Unmarshal([]byte(inputsJSON), &snap.Inputs); err != nil {
		return nil, fmt.Errorf("workflow: SQLiteState.Load: unmarshal inputs: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT node_id, state, output FROM nodes WHERE run_id = ?
	`, s.runID)
	if err != nil {
		return nil, fmt.Errorf("workflow: SQLiteState.Load: select nodes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, state, output string
		if err := rows.Scan(&id, &state, &output); err != nil {
			return nil, fmt.Errorf("workflow: SQLiteState.Load: scan node: %w", err)
		}
		snap.Outputs[id] = output
		if state == "succeeded" {
			snap.Completed[id] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workflow: SQLiteState.Load: iterate nodes: %w", err)
	}
	return snap, nil
}

// workflowIDForName produces a stable workflow_id for a given name. Until
// the loader passes canonical YAML through, the engine identifies a
// workflow by name alone. A future ticket (0.* or 1.1) can swap this for
// hash(canonical-yaml) without rewriting any caller.
func workflowIDForName(name string) string {
	sum := sha256.Sum256([]byte("name:" + name))
	return hex.EncodeToString(sum[:16])
}

// mustMarshalJSON panics on encode error. The map[string]string and
// map[string]any inputs we marshal here cannot fail to encode in practice.
func mustMarshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("workflow: marshal json: %v", err))
	}
	return string(b)
}

// nullTime returns the snapshot timestamp if set, falling back to fallback
// (typically time.Now()). SQLite tolerates the zero time but loses semantic
// meaning, so callers should always pass a meaningful fallback.
func nullTime(t, fallback time.Time) time.Time {
	if t.IsZero() {
		return fallback
	}
	return t
}
