package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// NodeState is the canonical lifecycle state of a single node within a
// run. Mirrors the CHECK constraint on the nodes table in schema.sql.
//
// State machine (see ROADMAP.md):
//
//	pending → running → succeeded
//	                  → failed
//	                  → awaiting_hitl → succeeded | failed   (Phase 2)
//	pending → skipped (upstream failed | when=false)
//
// Skipped is terminal in Phase 0; awaiting_hitl is unused until Phase 2.
type NodeState string

// Canonical node states. Mirror schema.sql's CHECK constraint.
const (
	NodeStatePending      NodeState = "pending"
	NodeStateRunning      NodeState = "running"
	NodeStateAwaitingHITL NodeState = "awaiting_hitl"
	NodeStateSucceeded    NodeState = "succeeded"
	NodeStateFailed       NodeState = "failed"
	NodeStateSkipped      NodeState = "skipped"
)

// RunState is the lifecycle state of an entire run. Mirrors runs.state.
type RunState string

// Canonical run states. Mirror schema.sql's CHECK constraint.
const (
	RunStatePending      RunState = "pending"
	RunStateRunning      RunState = "running"
	RunStateAwaitingHITL RunState = "awaiting_hitl"
	RunStateSucceeded    RunState = "succeeded"
	RunStateFailed       RunState = "failed"
	RunStateCancelled    RunState = "cancelled"
)

// AuditEvent is one row of the audit_events table — produced by the
// runner on every state transition (run-level OR node-level).
//
// Field semantics:
//   - RunID is always set.
//   - NodeID is empty for run-level events (e.g. run started, run finished).
//   - AttemptNo is 0 unless the transition is tied to a specific attempt
//     (running → succeeded | failed). Attempt 1 is the first try.
//   - FromState is empty for the initial transition into a state.
//   - Reason is a short free-form string (e.g. "upstream-failed",
//     "context-cancelled", retry message); it's the human-readable column
//     in `workflow logs`.
//   - Actor identifies who caused the transition. Format: "system",
//     "human:<name>", "role:<role>", "mcp:<client-id>".
//   - Payload carries kind-specific extras (model, tokens, dollars,
//     error_class). It's serialized into the audit_events.payload_json
//     column as canonical JSON.
type AuditEvent struct {
	EventID    string
	RunID      string
	NodeID     string
	AttemptNo  int
	FromState  string
	ToState    string
	Reason     string
	Actor      string
	OccurredAt time.Time
	Payload    map[string]any
}

// AuditSink consumes audit events. Implementations:
//   - *SQLiteState writes to the audit_events table (the canonical sink).
//   - StdoutAuditSink mirrors events to a writer (Phase 2 will add JSONL,
//     Engram, OpenTelemetry).
//
// Sinks must NOT block the runner indefinitely — the runner cancels its
// context on shutdown and sinks should respect that.
type AuditSink interface {
	Emit(ctx context.Context, event AuditEvent) error
}

// MultiAuditSink fans out one event to many sinks. A failure on one sink
// is logged via the optional OnError hook but does not block the rest —
// per ADR-010 §D3, "failure of one sink doesn't break the run".
type MultiAuditSink struct {
	Sinks   []AuditSink
	OnError func(sink AuditSink, event AuditEvent, err error)
}

// Emit forwards the event to every sink. Returns nil — sink-level errors
// are routed through OnError, never surfaced to the runner.
func (m *MultiAuditSink) Emit(ctx context.Context, ev AuditEvent) error {
	for _, s := range m.Sinks {
		if err := s.Emit(ctx, ev); err != nil && m.OnError != nil {
			m.OnError(s, ev, err)
		}
	}
	return nil
}

// AttemptRecord is the persisted shape of one entry in node_attempts —
// see schema.sql. The runner builds one of these per attempt and hands it
// to RunRecorder.RecordAttempt.
type AttemptRecord struct {
	AttemptID    string
	RunID        string
	NodeID       string
	AttemptNo    int
	State        NodeState
	ModelUsed    string
	PromptHash   string
	ResponseHash string
	TokensUsed   int
	DollarsSpent float64
	StartedAt    time.Time
	FinishedAt   time.Time
	ErrorClass   string
	ErrorMessage string
}

// NodeRecord is the persisted shape of a row in the nodes table.
type NodeRecord struct {
	RunID        string
	NodeID       string
	State        NodeState
	Attempts     int
	RoleUsed     string
	ModelUsed    string
	TokensUsed   int
	DollarsSpent float64
	Output       string
	StartedAt    time.Time
	FinishedAt   time.Time
	Error        string
}

// RunRecord is the persisted shape of a row in the runs table.
type RunRecord struct {
	RunID        string
	WorkflowID   string
	WorkflowName string
	State        RunState
	InputsJSON   string
	StartedAt    time.Time
	FinishedAt   time.Time
	TotalTokens  int
	TotalDollars float64
	Error        string
	Trigger      string
	TriggeredBy  string
}

// RunRecorder is the substrate-grade hook for per-run, per-node, and
// per-attempt detail beyond the simple Snapshot. SQLiteState implements
// this; FileState does not (it stays JSON-only). The runner uses it
// (when Runner.RunRecorder is set) to:
//
//  1. Insert the runs row when a run starts.
//  2. Upsert a nodes row each time a node enters a new state.
//  3. Insert a node_attempts row per attempt.
//  4. Mark the run finished (succeeded | failed | cancelled).
//
// Why a separate interface from State? State is the legacy snapshot
// surface (still used by FileState). RunRecorder captures structure that
// snapshots can't — retries, per-attempt cost, error classification —
// without forcing every state backend to grow.
type RunRecorder interface {
	BeginRun(ctx context.Context, rec RunRecord) error
	UpsertNode(ctx context.Context, rec NodeRecord) error
	RecordAttempt(ctx context.Context, rec AttemptRecord) error
	FinishRun(ctx context.Context, runID string, state RunState, finishedAt time.Time, errMsg string) error
}

// noopRunRecorder is the default when none is configured. Lets the runner
// call recorder methods unconditionally without nil checks.
type noopRunRecorder struct{}

func (noopRunRecorder) BeginRun(context.Context, RunRecord) error           { return nil }
func (noopRunRecorder) UpsertNode(context.Context, NodeRecord) error        { return nil }
func (noopRunRecorder) RecordAttempt(context.Context, AttemptRecord) error  { return nil }
func (noopRunRecorder) FinishRun(context.Context, string, RunState, time.Time, string) error {
	return nil
}

// noopAuditSink mirrors noopRunRecorder for the AuditSink path.
type noopAuditSink struct{}

func (noopAuditSink) Emit(context.Context, AuditEvent) error { return nil }

// classifyError maps an error to a short string for the
// node_attempts.error_class column. Phase 0 is intentionally crude — it
// only distinguishes context-cancelled errors from everything else; future
// phases (retry-on filter, ticket 1.*) will recognize transient,
// rate_limit, context_overflow.
func classifyError(err error) string {
	if err == nil {
		return ""
	}
	if ctxErr := err.Error(); strings.Contains(ctxErr, "context canceled") ||
		strings.Contains(ctxErr, "context deadline exceeded") {
		return "context_cancelled"
	}
	return "error"
}

// formatActor produces the canonical actor string for a system-driven
// event. Human/MCP actors come in via explicit overrides.
func formatActor(component string) string {
	if component == "" {
		return "system"
	}
	return fmt.Sprintf("system:%s", component)
}
