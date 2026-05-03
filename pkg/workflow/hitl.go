package workflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// HITLDecision is the outcome of a human-in-the-loop adjudication. The
// canonical values mirror the CHECK constraint on `approvals.decision`.
type HITLDecision string

// Canonical HITL decisions.
const (
	HITLDecisionApprove HITLDecision = "approve"
	HITLDecisionReject  HITLDecision = "reject"
	HITLDecisionTimeout HITLDecision = "timeout"
)

// HITLRequest is the substrate-level shape of a pending approval — what the
// runner emits when a node enters awaiting_hitl, and what HITL backends
// surface to humans (CLI, Discord, MCP, …).
//
// The same struct is the read-side projection of an `approvals` row joined
// with the node's policy: backends that subscribe via a poll/notify loop
// can reconstruct the human-facing message from this alone.
type HITLRequest struct {
	ApprovalID    string
	RunID         string
	NodeID        string
	WorkflowName  string
	ApproverRole  string
	Reason        string
	RequestedAt   time.Time
	Timeout       time.Duration
	OnTimeout     string
	Confidence    float64 // populated when policy was on_low_confidence
	NodeOutput    string
}

// HITLResolution is what a backend hands back when a human (or a timeout)
// decides the request. The runner translates Approve into a node continuation,
// Reject/Timeout into a node failure (per OnTimeout policy).
type HITLResolution struct {
	ApprovalID string
	Decision   HITLDecision
	Approver   string
	Role       string
	Reason     string
	ResolvedAt time.Time
}

// HITLBackend abstracts the adjudicator. Implementations:
//
//   - CLIBackend (the default): blocks until `workflow approve` or
//     `workflow reject` writes a row. SQLite-backed; no live process needed.
//   - DiscordBackend (Phase 2.6): forwards the request to a configured
//     channel; replies become decisions.
//   - MCPBackend (Phase 2.5): exposes the request as an MCP tool; whatever
//     the client decides becomes the resolution.
//
// Request notifies the backend that a node has entered awaiting_hitl and
// the backend should make it visible to the right human(s). Backends MUST
// be non-blocking — the runner waits via Wait, not Request.
//
// Wait blocks until a decision is recorded for approvalID. Returning a
// nil-error HITLResolution is the success path; returning ctx.Err() is
// the cancellation path. A backend that times out returns a Timeout
// decision rather than an error so the runner can apply the OnTimeout
// policy uniformly.
type HITLBackend interface {
	Request(ctx context.Context, req HITLRequest) error
	Wait(ctx context.Context, approvalID string) (HITLResolution, error)
}

// ErrHITLNoBackend is returned by helpers that need a backend but find one
// is not configured. Wrappable via errors.Is for callers that want to
// fall back to a default behaviour.
var ErrHITLNoBackend = errors.New("workflow: HITL backend not configured")

// ErrApprovalNotFound is returned by RecordHITLDecision when the approval
// id is not present in the approvals table.
var ErrApprovalNotFound = errors.New("workflow: approval not found")

// ErrApprovalAlreadyResolved is returned by RecordHITLDecision when the
// approval row is already terminal.
var ErrApprovalAlreadyResolved = errors.New("workflow: approval already resolved")

// ErrApproverRoleMismatch is returned by RecordHITLDecision when the actor
// role does not match the required approver_role.
var ErrApproverRoleMismatch = errors.New("workflow: approver role mismatch")

// CreateHITLRequest inserts a row into approvals and emits an audit event
// recording the running → awaiting_hitl transition. Returns the freshly
// generated approval id so the runner can pass it to the backend.
//
// Idempotent on (run_id, node_id) for the open approval: if a pending row
// already exists for the same node it is reused — re-entering awaiting_hitl
// after a runner restart should not multiply approval rows.
func CreateHITLRequest(ctx context.Context, db *sql.DB, runID, nodeID, approverRole, reason string, requestedAt time.Time) (string, error) {
	if runID == "" || nodeID == "" {
		return "", fmt.Errorf("workflow: CreateHITLRequest: runID and nodeID required")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("workflow: CreateHITLRequest: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existing string
	err = tx.QueryRowContext(ctx, `
		SELECT approval_id FROM approvals
		 WHERE run_id = ? AND node_id = ? AND resolved_at IS NULL
		 LIMIT 1
	`, runID, nodeID).Scan(&existing)
	switch {
	case err == nil:
		if commitErr := tx.Commit(); commitErr != nil {
			return "", fmt.Errorf("workflow: CreateHITLRequest: commit: %w", commitErr)
		}
		return existing, nil
	case errors.Is(err, sql.ErrNoRows):
		// fallthrough: insert below
	default:
		return "", fmt.Errorf("workflow: CreateHITLRequest: lookup: %w", err)
	}

	approvalID := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO approvals (
		    approval_id, run_id, node_id, requested_at, approver_role, reason
		) VALUES (?, ?, ?, ?, ?, ?)
	`, approvalID, runID, nodeID, requestedAt, nullableString(approverRole), nullableString(reason)); err != nil {
		return "", fmt.Errorf("workflow: CreateHITLRequest: insert: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE nodes SET state = 'awaiting_hitl' WHERE run_id = ? AND node_id = ?
	`, runID, nodeID); err != nil {
		return "", fmt.Errorf("workflow: CreateHITLRequest: update node state: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE runs SET state = 'awaiting_hitl' WHERE run_id = ? AND state = 'running'
	`, runID); err != nil {
		return "", fmt.Errorf("workflow: CreateHITLRequest: update run state: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_events (
		    event_id, run_id, node_id, attempt_no, from_state, to_state,
		    reason, actor, occurred_at, payload_json
		) VALUES (?, ?, ?, NULL, 'running', 'awaiting_hitl', ?, 'system', ?, '')
	`, uuid.NewString(), runID, nodeID, reason, requestedAt); err != nil {
		return "", fmt.Errorf("workflow: CreateHITLRequest: emit audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("workflow: CreateHITLRequest: commit: %w", err)
	}
	return approvalID, nil
}

// RecordHITLDecision writes the human's verdict to approvals + audit_events.
// approverRole, when non-empty, must match the row's required approver_role
// (set when the request was created). The function is the canonical write
// path for `workflow approve` / `workflow reject` and any backend that
// commits a decision.
//
// On approve → node returns to running (state restored from audit log).
// On reject → node transitions to failed.
// On timeout → node transitions per OnTimeout policy; the runner reads the
// row and applies the policy. RecordHITLDecision only stores the row.
//
//nolint:gocyclo // sequential update of approvals + nodes + audit; splitting hurts locality
func RecordHITLDecision(ctx context.Context, db *sql.DB, approvalID string, dec HITLDecision, approver, role, reason string, resolvedAt time.Time) error {
	if approvalID == "" {
		return fmt.Errorf("workflow: RecordHITLDecision: approvalID required")
	}
	if dec != HITLDecisionApprove && dec != HITLDecisionReject && dec != HITLDecisionTimeout {
		return fmt.Errorf("workflow: RecordHITLDecision: invalid decision %q", dec)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("workflow: RecordHITLDecision: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		runID, nodeID string
		requiredRole  sql.NullString
		resolvedCol   sql.NullTime
	)
	err = tx.QueryRowContext(ctx, `
		SELECT run_id, node_id, approver_role, resolved_at FROM approvals WHERE approval_id = ?
	`, approvalID).Scan(&runID, &nodeID, &requiredRole, &resolvedCol)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrApprovalNotFound, approvalID)
		}
		return fmt.Errorf("workflow: RecordHITLDecision: lookup: %w", err)
	}
	if resolvedCol.Valid {
		return fmt.Errorf("%w: %s", ErrApprovalAlreadyResolved, approvalID)
	}
	if requiredRole.Valid && requiredRole.String != "" && role != "" && requiredRole.String != role {
		return fmt.Errorf("%w: required %q, got %q", ErrApproverRoleMismatch, requiredRole.String, role)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE approvals
		   SET decision = ?, approver = ?, approver_role = COALESCE(?, approver_role),
		       reason = COALESCE(?, reason), resolved_at = ?
		 WHERE approval_id = ?
	`,
		string(dec),
		nullableString(approver),
		nullableString(role),
		nullableString(reason),
		resolvedAt,
		approvalID,
	); err != nil {
		return fmt.Errorf("workflow: RecordHITLDecision: update: %w", err)
	}

	// On approve, the node returns to running (and the run too, if no
	// other approvals are open). On reject/timeout, the node fails.
	switch dec {
	case HITLDecisionApprove:
		if _, err := tx.ExecContext(ctx, `
			UPDATE nodes SET state = 'running' WHERE run_id = ? AND node_id = ? AND state = 'awaiting_hitl'
		`, runID, nodeID); err != nil {
			return fmt.Errorf("workflow: RecordHITLDecision: restore node: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE runs SET state = 'running'
			 WHERE run_id = ? AND state = 'awaiting_hitl'
			   AND NOT EXISTS (SELECT 1 FROM approvals WHERE run_id = ? AND resolved_at IS NULL AND approval_id != ?)
		`, runID, runID, approvalID); err != nil {
			return fmt.Errorf("workflow: RecordHITLDecision: restore run: %w", err)
		}
	case HITLDecisionReject, HITLDecisionTimeout:
		if _, err := tx.ExecContext(ctx, `
			UPDATE nodes SET state = 'failed', error = ?, finished_at = ? WHERE run_id = ? AND node_id = ?
		`, "hitl-"+string(dec), resolvedAt, runID, nodeID); err != nil {
			return fmt.Errorf("workflow: RecordHITLDecision: fail node: %w", err)
		}
	}

	actor := approver
	if actor == "" {
		actor = "human"
	} else if !strings.Contains(actor, ":") {
		actor = "human:" + actor
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_events (
		    event_id, run_id, node_id, attempt_no, from_state, to_state,
		    reason, actor, occurred_at, payload_json
		) VALUES (?, ?, ?, NULL, 'awaiting_hitl', ?, ?, ?, ?, '')
	`, uuid.NewString(), runID, nodeID, hitlDecisionToState(dec), reason, actor, resolvedAt); err != nil {
		return fmt.Errorf("workflow: RecordHITLDecision: emit audit: %w", err)
	}
	return tx.Commit()
}

// hitlDecisionToState maps the human's decision back into the canonical
// node state machine. Approve resumes the node's body so the audit shows
// awaiting_hitl → running; reject/timeout terminate it.
func hitlDecisionToState(d HITLDecision) string {
	switch d {
	case HITLDecisionApprove:
		return string(NodeStateRunning)
	case HITLDecisionReject, HITLDecisionTimeout:
		return string(NodeStateFailed)
	}
	return string(NodeStateFailed)
}

// LoadHITLRequest reads one approval row by id. Backends use it to render
// the human-facing message (NodeID + ApproverRole + Reason). Returns
// ErrApprovalNotFound when the id is unknown.
func LoadHITLRequest(ctx context.Context, db *sql.DB, approvalID string) (*HITLRequest, error) {
	if approvalID == "" {
		return nil, fmt.Errorf("workflow: LoadHITLRequest: approvalID required")
	}
	var (
		req           HITLRequest
		approverRole  sql.NullString
		reason        sql.NullString
		workflowName  sql.NullString
		nodeOutput    sql.NullString
	)
	err := db.QueryRowContext(ctx, `
		SELECT a.approval_id, a.run_id, a.node_id, a.approver_role, a.reason, a.requested_at,
		       w.name, n.output
		  FROM approvals a
		  JOIN runs r      ON r.run_id      = a.run_id
		  JOIN workflows w ON w.workflow_id = r.workflow_id
		  LEFT JOIN nodes n ON n.run_id = a.run_id AND n.node_id = a.node_id
		 WHERE a.approval_id = ?
	`, approvalID).Scan(
		&req.ApprovalID, &req.RunID, &req.NodeID, &approverRole, &reason, &req.RequestedAt,
		&workflowName, &nodeOutput,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrApprovalNotFound, approvalID)
		}
		return nil, fmt.Errorf("workflow: LoadHITLRequest: %w", err)
	}
	req.ApproverRole = approverRole.String
	req.Reason = reason.String
	req.WorkflowName = workflowName.String
	req.NodeOutput = nodeOutput.String
	return &req, nil
}

// ListPendingHITLRequests returns every unresolved approval, oldest first.
// Used by the CLI list command and by Discord/MCP backends that poll for
// new work.
func ListPendingHITLRequests(ctx context.Context, db *sql.DB) ([]HITLRequest, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT a.approval_id, a.run_id, a.node_id,
		       COALESCE(a.approver_role,''), COALESCE(a.reason,''), a.requested_at,
		       COALESCE(w.name,''), COALESCE(n.output,'')
		  FROM approvals a
		  LEFT JOIN runs r      ON r.run_id      = a.run_id
		  LEFT JOIN workflows w ON w.workflow_id = r.workflow_id
		  LEFT JOIN nodes n     ON n.run_id = a.run_id AND n.node_id = a.node_id
		 WHERE a.resolved_at IS NULL
		 ORDER BY a.requested_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("workflow: ListPendingHITLRequests: query: %w", err)
	}
	defer rows.Close()
	var out []HITLRequest
	for rows.Next() {
		var r HITLRequest
		if err := rows.Scan(
			&r.ApprovalID, &r.RunID, &r.NodeID,
			&r.ApproverRole, &r.Reason, &r.RequestedAt,
			&r.WorkflowName, &r.NodeOutput,
		); err != nil {
			return nil, fmt.Errorf("workflow: ListPendingHITLRequests: scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SQLiteHITLBackend is the default backend: it persists requests in the
// approvals table and waits for `workflow approve` / `workflow reject` to
// flip the row. Polling cadence is configurable; the default 250ms balances
// a sub-second response with negligible CPU on idle.
type SQLiteHITLBackend struct {
	DB           *sql.DB
	PollInterval time.Duration
}

// NewSQLiteHITLBackend returns a SQLiteHITLBackend ready to plug into a
// Runner. PollInterval=0 picks the 250ms default.
func NewSQLiteHITLBackend(db *sql.DB) *SQLiteHITLBackend {
	return &SQLiteHITLBackend{DB: db, PollInterval: 250 * time.Millisecond}
}

// Request is a no-op for the SQLite backend — the audit_events row + the
// approvals row inserted by CreateHITLRequest are the public notification.
// External backends override this to push to Discord/MCP/etc.
func (b *SQLiteHITLBackend) Request(_ context.Context, _ HITLRequest) error {
	return nil
}

// Wait polls the approvals row until decision is non-null or ctx fires.
// Returns the resolution; ctx cancellation surfaces ctx.Err() so the runner
// can short-circuit cleanly during shutdown.
func (b *SQLiteHITLBackend) Wait(ctx context.Context, approvalID string) (HITLResolution, error) {
	if b == nil || b.DB == nil {
		return HITLResolution{}, ErrHITLNoBackend
	}
	interval := b.PollInterval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		var (
			decision sql.NullString
			approver sql.NullString
			role     sql.NullString
			reason   sql.NullString
			resolved sql.NullTime
		)
		err := b.DB.QueryRowContext(ctx, `
			SELECT decision, approver, approver_role, reason, resolved_at
			  FROM approvals WHERE approval_id = ?
		`, approvalID).Scan(&decision, &approver, &role, &reason, &resolved)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return HITLResolution{}, fmt.Errorf("workflow: SQLiteHITLBackend.Wait: %w", err)
		}
		if decision.Valid && resolved.Valid {
			return HITLResolution{
				ApprovalID: approvalID,
				Decision:   HITLDecision(decision.String),
				Approver:   approver.String,
				Role:       role.String,
				Reason:     reason.String,
				ResolvedAt: resolved.Time,
			}, nil
		}
		select {
		case <-ctx.Done():
			return HITLResolution{}, ctx.Err()
		case <-t.C:
		}
	}
}

// shouldBlockOnHITL evaluates the node's HITLPolicy against the result so
// far. It is the gate that turns "node body succeeded" into "but a human
// must sign off first". Returns false (no block) when:
//
//   - HITL is unset
//   - block_policy is "" or "never"
//   - block_policy is "on_low_confidence" but the confidence is ≥ threshold
//
// reason is a short string included in the audit row for context.
func shouldBlockOnHITL(node *Node, res *Result) (bool, string) {
	if node == nil || node.HITL == nil {
		return false, ""
	}
	switch strings.ToLower(node.HITL.BlockPolicy) {
	case "", "never":
		return false, ""
	case "always":
		return true, "block_policy=always"
	case "on_low_confidence":
		conf, ok := extractConfidence(res)
		if !ok {
			// No confidence reported — fall through and block, on the
			// principle "absent evidence is not evidence of high confidence".
			return true, "no-confidence-reported"
		}
		if conf < node.HITL.ConfidenceThreshold {
			return true, fmt.Sprintf("confidence=%.2f<%.2f", conf, node.HITL.ConfidenceThreshold)
		}
		return false, ""
	}
	return false, ""
}

// extractConfidence reads a numeric confidence from the result's metadata.
// Convention: AI executors that self-report a confidence write it as
// res.Meta["confidence"] (float64). Bash nodes typically don't set one and
// will fall through to the "no-confidence-reported" branch above.
func extractConfidence(res *Result) (float64, bool) {
	if res == nil || res.Meta == nil {
		return 0, false
	}
	v, ok := res.Meta["confidence"]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	}
	return 0, false
}
