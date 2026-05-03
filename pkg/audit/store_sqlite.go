package audit

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite" // pure-Go SQLite driver; matches pkg/workflow
)

//go:embed schema.sql
var sqliteSchema string

// SQLiteOpenOptions are the pragmas every audits-on-runs.db database
// needs. Mirrors pkg/workflow's openSQLiteDB for consistency: the two
// schemas live in the same file, so they must agree on WAL,
// busy_timeout, and foreign_keys.
const SQLiteOpenOptions = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)"

// ErrNotFound is returned by Get* methods when the key is absent.
// Wraps sql.ErrNoRows for callers that want to use errors.Is.
var ErrNotFound = errors.New("audit: not found")

// SQLiteStore implements Store on top of a sql.DB. Construct with
// OpenSQLiteStore (which owns the connection lifecycle) or
// AttachSQLiteStore (which takes an externally-owned *sql.DB and does
// not close it on Close()). The two variants exist so the audit
// subsystem can either share the workflow engine's DB handle or open
// its own; both forms apply the schema on first call.
type SQLiteStore struct {
	db     *sql.DB
	ownsDB bool
	now    func() time.Time
	idGen  func() string
}

// OpenSQLiteStore opens (or creates) a SQLite database at path,
// applies the audit schema (additive), and returns the store. The
// database file may already contain workflow engine tables — those
// remain untouched. WAL mode is required so the workflow engine and
// the audit subsystem can write concurrently.
func OpenSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path+SQLiteOpenOptions)
	if err != nil {
		return nil, fmt.Errorf("audit: open %s: %w", path, err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("audit: ping %s: %w", path, err)
	}
	if err := ApplySchema(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &SQLiteStore{db: db, ownsDB: true, now: time.Now, idGen: uuid.NewString}, nil
}

// AttachSQLiteStore wraps an existing sql.DB. The caller retains
// ownership; Close on the returned store will not close db. Useful
// when the audit store shares a connection with pkg/workflow.
func AttachSQLiteStore(ctx context.Context, db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, errors.New("audit: AttachSQLiteStore: db is nil")
	}
	if err := ApplySchema(ctx, db); err != nil {
		return nil, err
	}
	return &SQLiteStore{db: db, ownsDB: false, now: time.Now, idGen: uuid.NewString}, nil
}

// ApplySchema applies the embedded schema to db. Safe to call
// multiple times — every statement is IF NOT EXISTS. Exported so
// callers that own the *sql.DB can apply the schema before wiring
// other consumers.
func ApplySchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, sqliteSchema); err != nil {
		return fmt.Errorf("audit: apply schema: %w", err)
	}
	return nil
}

// DB returns the underlying *sql.DB. Exported for callers that need
// to JOIN audit tables against workflow tables in the same file.
// Test code should prefer the typed store interface.
func (s *SQLiteStore) DB() *sql.DB { return s.db }

// Close releases the connection if this store owns it. Safe to call
// multiple times.
func (s *SQLiteStore) Close() error {
	if !s.ownsDB || s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

// BeginAuditRun inserts the audit_runs row in state running.
func (s *SQLiteStore) BeginAuditRun(ctx context.Context, rec AuditRunRecord) error {
	if rec.AuditRunID == "" {
		return errors.New("audit: BeginAuditRun: AuditRunID is empty")
	}
	if rec.Repo == "" {
		return errors.New("audit: BeginAuditRun: Repo is empty")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_runs (audit_run_id, repo, cadence, started_at, state, triggered_by)
		VALUES (?, ?, ?, ?, ?, ?)
	`, rec.AuditRunID, rec.Repo, string(rec.Cadence), rec.StartedAt, string(AuditRunRunning), rec.TriggeredBy)
	if err != nil {
		return fmt.Errorf("audit: BeginAuditRun: %w", err)
	}
	return nil
}

// FinishAuditRun updates the row with final state + counts.
func (s *SQLiteStore) FinishAuditRun(ctx context.Context, rec AuditRunRecord) error {
	if rec.AuditRunID == "" {
		return errors.New("audit: FinishAuditRun: AuditRunID is empty")
	}
	if !rec.State.IsValid() {
		return fmt.Errorf("audit: FinishAuditRun: invalid state %q", rec.State)
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE audit_runs
		   SET finished_at       = ?,
		       state             = ?,
		       findings_new      = ?,
		       findings_resolved = ?,
		       findings_open     = ?
		 WHERE audit_run_id = ?
	`, rec.FinishedAt, string(rec.State), rec.FindingsNew, rec.FindingsResolved, rec.FindingsOpen, rec.AuditRunID)
	if err != nil {
		return fmt.Errorf("audit: FinishAuditRun: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("audit: FinishAuditRun: no row for %s", rec.AuditRunID)
	}
	return nil
}

// UpsertFinding implements the lifecycle logic from the Store
// contract: insert or update by (repo, fingerprint), bump last_seen,
// reopen if previously resolved.
func (s *SQLiteStore) UpsertFinding(ctx context.Context, f Finding) (Finding, error) {
	if err := f.Validate(); err != nil {
		return Finding{}, err
	}
	now := s.now()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Finding{}, fmt.Errorf("audit: UpsertFinding: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		existingID    string
		existingState string
		firstSeen     time.Time
	)
	err = tx.QueryRowContext(ctx, `
		SELECT finding_id, state, first_seen
		  FROM audit_findings
		 WHERE repo = ? AND fingerprint = ?
	`, f.Repo, f.Fingerprint).Scan(&existingID, &existingState, &firstSeen)

	evidenceJSON := "{}"
	if len(f.Evidence) > 0 {
		b, mErr := json.Marshal(f.Evidence)
		if mErr != nil {
			return Finding{}, fmt.Errorf("audit: marshal evidence: %w", mErr)
		}
		evidenceJSON = string(b)
	}

	switch {
	case errors.Is(err, sql.ErrNoRows):
		// New row.
		f.FindingID = s.idGen()
		f.State = FindingOpen
		f.FirstSeen = now
		f.LastSeen = now
		_, err = tx.ExecContext(ctx, `
			INSERT INTO audit_findings (
				finding_id, repo, fingerprint, check_id, severity, state, title, detail,
				path, line, first_seen, last_seen, state_note,
				suggested_strategy, suggested_command, suggested_patch,
				suggested_title, suggested_body, evidence_json
			) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		`, f.FindingID, f.Repo, f.Fingerprint, f.CheckID, string(f.Severity), string(f.State),
			f.Title, f.Detail, f.Path, f.Line, f.FirstSeen, f.LastSeen, "",
			string(f.Suggested.Strategy), f.Suggested.Command, f.Suggested.Patch,
			f.Suggested.Title, f.Suggested.Body, evidenceJSON)
		if err != nil {
			return Finding{}, fmt.Errorf("audit: insert finding: %w", err)
		}
	case err != nil:
		return Finding{}, fmt.Errorf("audit: lookup finding: %w", err)
	default:
		f.FindingID = existingID
		f.FirstSeen = firstSeen
		f.LastSeen = now
		newState := FindingState(existingState)
		if newState == FindingResolved {
			newState = FindingReopened
		}
		f.State = newState
		_, err = tx.ExecContext(ctx, `
			UPDATE audit_findings
			   SET severity     = ?,
			       state        = ?,
			       title        = ?,
			       detail       = ?,
			       path         = ?,
			       line         = ?,
			       last_seen    = ?,
			       suggested_strategy = ?,
			       suggested_command  = ?,
			       suggested_patch    = ?,
			       suggested_title    = ?,
			       suggested_body     = ?,
			       evidence_json = ?
			 WHERE finding_id = ?
		`, string(f.Severity), string(f.State), f.Title, f.Detail, f.Path, f.Line, f.LastSeen,
			string(f.Suggested.Strategy), f.Suggested.Command, f.Suggested.Patch,
			f.Suggested.Title, f.Suggested.Body, evidenceJSON, f.FindingID)
		if err != nil {
			return Finding{}, fmt.Errorf("audit: update finding: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Finding{}, fmt.Errorf("audit: UpsertFinding commit: %w", err)
	}
	return f, nil
}

// SetFindingState transitions a finding to state. Returns ErrNotFound
// if the row does not exist; returns an error for illegal transitions
// (e.g. acknowledged → reopened — that's the runner's prerogative,
// not a manual operation).
func (s *SQLiteStore) SetFindingState(ctx context.Context, findingID string, state FindingState, note string) (Finding, error) {
	if !state.IsValid() {
		return Finding{}, fmt.Errorf("audit: SetFindingState: invalid state %q", state)
	}
	now := s.now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Finding{}, fmt.Errorf("audit: SetFindingState: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var current string
	err = tx.QueryRowContext(ctx, `SELECT state FROM audit_findings WHERE finding_id = ?`, findingID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return Finding{}, ErrNotFound
	}
	if err != nil {
		return Finding{}, fmt.Errorf("audit: SetFindingState lookup: %w", err)
	}
	from := FindingState(current)
	if !legalManualTransition(from, state) {
		return Finding{}, fmt.Errorf("audit: SetFindingState: illegal transition %s → %s", from, state)
	}

	if state == FindingResolved {
		_, err = tx.ExecContext(ctx, `
			UPDATE audit_findings
			   SET state = ?, state_note = ?, resolved_at = ?
			 WHERE finding_id = ?`, string(state), note, now, findingID)
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE audit_findings
			   SET state = ?, state_note = ?
			 WHERE finding_id = ?`, string(state), note, findingID)
	}
	if err != nil {
		return Finding{}, fmt.Errorf("audit: SetFindingState update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Finding{}, fmt.Errorf("audit: SetFindingState commit: %w", err)
	}
	return s.GetFinding(ctx, findingID)
}

// legalManualTransition encodes the operator-driven lifecycle. The
// runner has its own automatic rules (open → reopened on re-emission)
// and is allowed to perform any transition; this gate is for manual
// CLI-driven changes only.
func legalManualTransition(from, to FindingState) bool {
	switch from {
	case FindingOpen:
		return to == FindingAcknowledged || to == FindingResolved
	case FindingAcknowledged:
		return to == FindingResolved || to == FindingOpen
	case FindingResolved:
		return to == FindingReopened
	case FindingReopened:
		return to == FindingAcknowledged || to == FindingResolved
	}
	return false
}

// CountFindings returns aggregate counts. New is approximated as
// "first_seen within the last 24h" (see Store contract).
func (s *SQLiteStore) CountFindings(ctx context.Context, repo string) (FindingCounts, error) {
	now := s.now()
	cutoff := now.Add(-24 * time.Hour)
	var counts FindingCounts
	row := s.db.QueryRowContext(ctx, `
		SELECT
			SUM(CASE WHEN state IN ('open','reopened','acknowledged') THEN 1 ELSE 0 END),
			SUM(CASE WHEN state = 'resolved' THEN 1 ELSE 0 END),
			SUM(CASE WHEN first_seen >= ? THEN 1 ELSE 0 END)
		  FROM audit_findings
		 WHERE repo = ?
	`, cutoff, repo)
	var open, resolved, fresh sql.NullInt64
	if err := row.Scan(&open, &resolved, &fresh); err != nil {
		return counts, fmt.Errorf("audit: CountFindings: %w", err)
	}
	counts.Open = int(open.Int64)
	counts.Resolved = int(resolved.Int64)
	counts.New = int(fresh.Int64)
	return counts, nil
}

// ListFindings returns matching rows. Sort is fixed at (severity asc,
// last_seen desc) for stable CLI rendering; callers that need
// alternate ordering can post-sort.
func (s *SQLiteStore) ListFindings(ctx context.Context, filter FindingFilter) ([]Finding, error) {
	q := strings.Builder{}
	q.WriteString(`SELECT finding_id, repo, fingerprint, check_id, severity, state,
		title, detail, path, line, first_seen, last_seen, resolved_at, state_note,
		suggested_strategy, suggested_command, suggested_patch, suggested_title,
		suggested_body, evidence_json
		FROM audit_findings WHERE 1=1 `)
	args := []any{}
	if filter.Repo != "" {
		q.WriteString(` AND repo = ?`)
		args = append(args, filter.Repo)
	}
	if filter.State != "" {
		q.WriteString(` AND state = ?`)
		args = append(args, string(filter.State))
	}
	if filter.Severity != "" {
		q.WriteString(` AND severity = ?`)
		args = append(args, string(filter.Severity))
	}
	if filter.CheckID != "" {
		q.WriteString(` AND check_id = ?`)
		args = append(args, filter.CheckID)
	}
	q.WriteString(` ORDER BY severity ASC, last_seen DESC`)
	if filter.Limit > 0 {
		q.WriteString(` LIMIT ?`)
		args = append(args, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, q.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("audit: ListFindings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []Finding{}
	for rows.Next() {
		f, err := scanFinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit: ListFindings rows: %w", err)
	}
	return out, nil
}

// GetFinding returns one row by id, or ErrNotFound.
func (s *SQLiteStore) GetFinding(ctx context.Context, findingID string) (Finding, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT finding_id, repo, fingerprint, check_id, severity, state,
		       title, detail, path, line, first_seen, last_seen, resolved_at, state_note,
		       suggested_strategy, suggested_command, suggested_patch, suggested_title,
		       suggested_body, evidence_json
		  FROM audit_findings WHERE finding_id = ?`, findingID)
	f, err := scanFinding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Finding{}, ErrNotFound
	}
	return f, err
}

// scannable is the common interface for *sql.Row and *sql.Rows so
// scanFinding can serve both list and get paths without duplication.
type scannable interface {
	Scan(dest ...any) error
}

func scanFinding(s scannable) (Finding, error) {
	var (
		f         Finding
		state     string
		severity  string
		strategy  string
		evidence  string
		resolved  sql.NullTime
		stateNote string
	)
	if err := s.Scan(
		&f.FindingID, &f.Repo, &f.Fingerprint, &f.CheckID, &severity, &state,
		&f.Title, &f.Detail, &f.Path, &f.Line, &f.FirstSeen, &f.LastSeen, &resolved, &stateNote,
		&strategy, &f.Suggested.Command, &f.Suggested.Patch, &f.Suggested.Title,
		&f.Suggested.Body, &evidence,
	); err != nil {
		return Finding{}, err
	}
	f.Severity = Severity(severity)
	f.State = FindingState(state)
	f.Suggested.Strategy = Strategy(strategy)
	if resolved.Valid {
		f.ResolvedAt = resolved.Time
	}
	if evidence != "" && evidence != "{}" {
		f.Evidence = make(map[string]any)
		if err := json.Unmarshal([]byte(evidence), &f.Evidence); err != nil {
			return Finding{}, fmt.Errorf("audit: scan evidence: %w", err)
		}
	}
	_ = stateNote
	return f, nil
}

// UpsertProposal inserts a new proposal row or returns the existing
// id when (audit_run_id, layer, title) collides — refiners may emit
// the same proposal across reruns and we want one row per unique
// proposal per audit-run.
func (s *SQLiteStore) UpsertProposal(ctx context.Context, p Proposal) (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	if p.AuditRunID == "" {
		return "", errors.New("audit: UpsertProposal: AuditRunID is empty")
	}

	var existing string
	err := s.db.QueryRowContext(ctx, `
		SELECT proposal_id FROM audit_proposals
		 WHERE audit_run_id = ? AND target_layer = ? AND title = ?
	`, p.AuditRunID, string(p.Layer), p.Title).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("audit: UpsertProposal lookup: %w", err)
	}

	id := s.idGen()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO audit_proposals (
			proposal_id, audit_run_id, target_layer, title, rationale, patch,
			state, proposed_at
		) VALUES (?,?,?,?,?,?,?,?)
	`, id, p.AuditRunID, string(p.Layer), p.Title, p.Rationale, p.Patch,
		string(ProposalProposed), p.ProposedAt)
	if err != nil {
		return "", fmt.Errorf("audit: UpsertProposal insert: %w", err)
	}
	return id, nil
}

// ListProposals returns matching rows, sorted by proposed_at desc.
func (s *SQLiteStore) ListProposals(ctx context.Context, filter ProposalFilter) ([]Proposal, error) {
	q := strings.Builder{}
	q.WriteString(`SELECT proposal_id, audit_run_id, target_layer, title, rationale,
		patch, state, proposed_at, decided_at, decided_by, decision_note
		FROM audit_proposals WHERE 1=1 `)
	args := []any{}
	if filter.AuditRunID != "" {
		q.WriteString(` AND audit_run_id = ?`)
		args = append(args, filter.AuditRunID)
	}
	if filter.Layer != "" {
		q.WriteString(` AND target_layer = ?`)
		args = append(args, string(filter.Layer))
	}
	if filter.State != "" {
		q.WriteString(` AND state = ?`)
		args = append(args, string(filter.State))
	}
	q.WriteString(` ORDER BY proposed_at DESC`)
	if filter.Limit > 0 {
		q.WriteString(` LIMIT ?`)
		args = append(args, filter.Limit)
	}
	rows, err := s.db.QueryContext(ctx, q.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("audit: ListProposals: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []Proposal{}
	for rows.Next() {
		var (
			p        Proposal
			layer    string
			state    string
			decided  sql.NullTime
		)
		if err := rows.Scan(&p.ProposalID, &p.AuditRunID, &layer, &p.Title, &p.Rationale,
			&p.Patch, &state, &p.ProposedAt, &decided, &p.DecidedBy, &p.Decision); err != nil {
			return nil, fmt.Errorf("audit: ListProposals scan: %w", err)
		}
		p.Layer = ProposalLayer(layer)
		p.State = ProposalState(state)
		if decided.Valid {
			p.DecidedAt = decided.Time
		}
		out = append(out, p)
	}
	return out, nil
}

// SetProposalState records a decision.
func (s *SQLiteStore) SetProposalState(ctx context.Context, proposalID string, state ProposalState, decidedBy, note string) error {
	if !state.IsValid() {
		return fmt.Errorf("audit: SetProposalState: invalid state %q", state)
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE audit_proposals
		   SET state = ?, decided_at = ?, decided_by = ?, decision_note = ?
		 WHERE proposal_id = ?
	`, string(state), s.now(), decidedBy, note, proposalID)
	if err != nil {
		return fmt.Errorf("audit: SetProposalState: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
