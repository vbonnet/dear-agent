package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RunStatus is the read-side projection used by `workflow status`. It joins
// runs + workflows + nodes into one structure suitable for human display
// or JSON output.
type RunStatus struct {
	RunID        string            `json:"run_id"`
	Workflow     string            `json:"workflow"`
	State        RunState          `json:"state"`
	Inputs       map[string]string `json:"inputs"`
	StartedAt    time.Time         `json:"started_at"`
	FinishedAt   *time.Time        `json:"finished_at,omitempty"`
	TotalTokens  int               `json:"total_tokens"`
	TotalDollars float64           `json:"total_dollars"`
	Error        string            `json:"error,omitempty"`
	Trigger      string            `json:"trigger,omitempty"`
	TriggeredBy  string            `json:"triggered_by,omitempty"`
	Nodes        []NodeStatus      `json:"nodes"`
}

// NodeStatus is one row in RunStatus.Nodes.
type NodeStatus struct {
	NodeID       string     `json:"node_id"`
	State        NodeState  `json:"state"`
	Attempts     int        `json:"attempts"`
	ModelUsed    string     `json:"model_used,omitempty"`
	TokensUsed   int        `json:"tokens_used"`
	DollarsSpent float64    `json:"dollars_spent"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Output       string     `json:"output,omitempty"`
	Error        string     `json:"error,omitempty"`
}

// RunSummary is one entry in `workflow list` — enough to identify and
// triage a run without fetching its node rows.
type RunSummary struct {
	RunID      string     `json:"run_id"`
	Workflow   string     `json:"workflow"`
	State      RunState   `json:"state"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Trigger    string     `json:"trigger,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// ListOptions filters/limits a `workflow list` query.
type ListOptions struct {
	State RunState // empty = any state
	Limit int      // 0 → 50
}

// ErrRunNotFound is returned by Status, Cancel, and Logs when the run_id
// is unknown to the DB. Callers can compare with errors.Is.
var ErrRunNotFound = errors.New("workflow: run not found")

// Status returns a full read-side projection for one run. Errors with
// ErrRunNotFound when the id is unknown.
func Status(ctx context.Context, db *sql.DB, runID string) (*RunStatus, error) {
	if runID == "" {
		return nil, fmt.Errorf("workflow: Status: runID required")
	}
	st := &RunStatus{}
	var (
		inputsJSON  string
		finishedAt  sql.NullTime
		errCol      sql.NullString
		triggerCol  sql.NullString
		triggeredBy sql.NullString
	)
	err := db.QueryRowContext(ctx, `
		SELECT r.run_id, w.name, r.state, r.inputs_json, r.started_at,
		       r.finished_at, r.total_tokens, r.total_dollars,
		       r.error, r.trigger, r.triggered_by
		  FROM runs r
		  JOIN workflows w ON r.workflow_id = w.workflow_id
		 WHERE r.run_id = ?
	`, runID).Scan(
		&st.RunID, &st.Workflow, &st.State, &inputsJSON, &st.StartedAt,
		&finishedAt, &st.TotalTokens, &st.TotalDollars,
		&errCol, &triggerCol, &triggeredBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrRunNotFound, runID)
	}
	if err != nil {
		return nil, fmt.Errorf("workflow: Status: select run: %w", err)
	}
	if finishedAt.Valid {
		st.FinishedAt = &finishedAt.Time
	}
	st.Error = errCol.String
	st.Trigger = triggerCol.String
	st.TriggeredBy = triggeredBy.String
	st.Inputs = map[string]string{}
	if inputsJSON != "" {
		_ = json.Unmarshal([]byte(inputsJSON), &st.Inputs)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT node_id, state, attempts, COALESCE(model_used,''),
		       tokens_used, dollars_spent, started_at, finished_at,
		       output, COALESCE(error,'')
		  FROM nodes
		 WHERE run_id = ?
		 ORDER BY COALESCE(started_at, finished_at), node_id
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("workflow: Status: select nodes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var n NodeStatus
		var startedAt, finishedAt sql.NullTime
		if err := rows.Scan(
			&n.NodeID, &n.State, &n.Attempts, &n.ModelUsed,
			&n.TokensUsed, &n.DollarsSpent, &startedAt, &finishedAt,
			&n.Output, &n.Error,
		); err != nil {
			return nil, fmt.Errorf("workflow: Status: scan node: %w", err)
		}
		if startedAt.Valid {
			t := startedAt.Time
			n.StartedAt = &t
		}
		if finishedAt.Valid {
			t := finishedAt.Time
			n.FinishedAt = &t
		}
		st.Nodes = append(st.Nodes, n)
	}
	return st, rows.Err()
}

// List returns recent runs, optionally filtered by state. Default ordering
// is most-recent-first.
func List(ctx context.Context, db *sql.DB, opts ListOptions) ([]RunSummary, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT r.run_id, w.name, r.state, r.started_at, r.finished_at,
		       COALESCE(r.trigger,''), COALESCE(r.error,'')
		  FROM runs r
		  JOIN workflows w ON r.workflow_id = w.workflow_id
	`
	args := []any{}
	if opts.State != "" {
		q += " WHERE r.state = ?"
		args = append(args, string(opts.State))
	}
	q += " ORDER BY r.started_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("workflow: List: query: %w", err)
	}
	defer rows.Close()
	var out []RunSummary
	for rows.Next() {
		var s RunSummary
		var finished sql.NullTime
		if err := rows.Scan(&s.RunID, &s.Workflow, &s.State, &s.StartedAt, &finished, &s.Trigger, &s.Error); err != nil {
			return nil, fmt.Errorf("workflow: List: scan: %w", err)
		}
		if finished.Valid {
			t := finished.Time
			s.FinishedAt = &t
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Cancel marks a run cancelled. Phase 0 is "set the column"; the runner
// observes cancellation via its context. Future tickets will add an
// active signaling mechanism (cmd/workflow-cancel + an in-process broker).
func Cancel(ctx context.Context, db *sql.DB, runID, reason, actor string) error {
	if runID == "" {
		return fmt.Errorf("workflow: Cancel: runID required")
	}
	if actor == "" {
		actor = "human"
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("workflow: Cancel: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	res, err := tx.ExecContext(ctx, `
		UPDATE runs SET state = 'cancelled', finished_at = ?, error = ?
		 WHERE run_id = ? AND state IN ('pending','running','awaiting_hitl')
	`, now, reason, runID)
	if err != nil {
		return fmt.Errorf("workflow: Cancel: update run: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		// Either the run doesn't exist or it's already terminal.
		var existing string
		if err := tx.QueryRowContext(ctx, `SELECT state FROM runs WHERE run_id = ?`, runID).Scan(&existing); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: %s", ErrRunNotFound, runID)
			}
			return fmt.Errorf("workflow: Cancel: lookup state: %w", err)
		}
		return fmt.Errorf("workflow: Cancel: run %s already in terminal state %s", runID, existing)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_events (
		    event_id, run_id, node_id, attempt_no, from_state, to_state,
		    reason, actor, occurred_at, payload_json
		) VALUES (?, ?, NULL, NULL, 'running', 'cancelled', ?, ?, ?, '')
	`, uuid.NewString(), runID, reason, actor, now); err != nil {
		return fmt.Errorf("workflow: Cancel: emit audit: %w", err)
	}
	return tx.Commit()
}

// LogsOptions filter Logs results.
type LogsOptions struct {
	NodeID string // empty = all nodes (and run-level events)
	Limit  int    // 0 = unbounded
}

// Logs returns audit_events for a run, oldest first.
func Logs(ctx context.Context, db *sql.DB, runID string, opts LogsOptions) ([]AuditEvent, error) {
	if runID == "" {
		return nil, fmt.Errorf("workflow: Logs: runID required")
	}
	q := `
		SELECT event_id, run_id, COALESCE(node_id,''), COALESCE(attempt_no,0),
		       COALESCE(from_state,''), to_state, COALESCE(reason,''),
		       actor, occurred_at, COALESCE(payload_json,'')
		  FROM audit_events
		 WHERE run_id = ?
	`
	args := []any{runID}
	if opts.NodeID != "" {
		q += " AND node_id = ?"
		args = append(args, opts.NodeID)
	}
	q += " ORDER BY occurred_at, event_id"
	if opts.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, opts.Limit)
	}

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("workflow: Logs: query: %w", err)
	}
	defer rows.Close()
	var out []AuditEvent
	for rows.Next() {
		var ev AuditEvent
		var payload string
		if err := rows.Scan(
			&ev.EventID, &ev.RunID, &ev.NodeID, &ev.AttemptNo,
			&ev.FromState, &ev.ToState, &ev.Reason, &ev.Actor, &ev.OccurredAt, &payload,
		); err != nil {
			return nil, fmt.Errorf("workflow: Logs: scan: %w", err)
		}
		if payload != "" {
			ev.Payload = map[string]any{}
			if err := json.Unmarshal([]byte(payload), &ev.Payload); err != nil {
				ev.Payload = map[string]any{"raw": payload}
			}
		}
		out = append(out, ev)
	}

	// Verify the run exists when there are zero events — otherwise an
	// empty result set looks identical to "unknown run id".
	if len(out) == 0 {
		var ok int
		if err := db.QueryRowContext(ctx, `SELECT 1 FROM runs WHERE run_id = ?`, runID).Scan(&ok); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("%w: %s", ErrRunNotFound, runID)
			}
			return nil, fmt.Errorf("workflow: Logs: verify run: %w", err)
		}
	}
	return out, rows.Err()
}

// FormatRunStatusText produces the human-readable display used by
// `workflow status` (without --json).
func FormatRunStatusText(s *RunStatus) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "run %s\n", s.RunID)
	fmt.Fprintf(&sb, "  workflow:    %s\n", s.Workflow)
	fmt.Fprintf(&sb, "  state:       %s\n", s.State)
	fmt.Fprintf(&sb, "  started_at:  %s\n", s.StartedAt.Format(time.RFC3339))
	if s.FinishedAt != nil {
		fmt.Fprintf(&sb, "  finished_at: %s (%s)\n",
			s.FinishedAt.Format(time.RFC3339),
			s.FinishedAt.Sub(s.StartedAt).Round(time.Millisecond))
	}
	if s.Trigger != "" {
		fmt.Fprintf(&sb, "  trigger:     %s\n", s.Trigger)
	}
	if s.Error != "" {
		fmt.Fprintf(&sb, "  error:       %s\n", s.Error)
	}
	if len(s.Nodes) == 0 {
		fmt.Fprintln(&sb, "  no nodes recorded")
		return sb.String()
	}
	fmt.Fprintln(&sb, "  nodes:")
	for _, n := range s.Nodes {
		fmt.Fprintf(&sb, "    %-20s %-10s attempts=%d", n.NodeID, n.State, n.Attempts)
		if n.Error != "" {
			fmt.Fprintf(&sb, " error=%q", n.Error)
		}
		fmt.Fprintln(&sb)
	}
	return sb.String()
}
