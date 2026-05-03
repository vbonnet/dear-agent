package workflow

import (
	"context"
	"fmt"
	"time"
)

// Hooks is the DEAR (Define/Enforce/Audit/Resolve) extension surface for the
// workflow runner. The four hooks line up one-to-one with the substrate
// properties from ADR-009: callers plug in their own logic to participate in
// each phase without forking the runner.
//
//   - OnDefine fires once per run, after Validate has accepted the workflow
//     and before any node executes. Use it to inspect or annotate the
//     definition (custom lint rules, schema policies).
//   - OnEnforce fires before a node body runs, after permissions and budget
//     pre-flight checks. The hook can short-circuit the node with a
//     non-nil error which is recorded as an enforcement denial in the
//     audit log.
//   - OnAudit fires for every state transition, alongside the AuditSink
//     write. Sinks store events; OnAudit lets callers react to them
//     (notifications, metrics, custom logs) without writing a sink.
//   - OnResolve fires when a node enters a terminal failure state. Returning
//     an error is informational — the run is already failing — but lets the
//     hook record its own follow-up actions.
//
// All four are optional. A nil hook is skipped silently. Hook errors are
// audit-logged but never panic the runner.
type Hooks struct {
	OnDefine  func(ctx context.Context, p DefinePayload) error
	OnEnforce func(ctx context.Context, p EnforcePayload) error
	OnAudit   func(ctx context.Context, p AuditPayload) error
	OnResolve func(ctx context.Context, p ResolvePayload) error
}

// DefinePayload is the OnDefine input. Workflow is the validated workflow;
// RunID is the id assigned to the upcoming run.
type DefinePayload struct {
	RunID    string
	Workflow *Workflow
	Inputs   map[string]string
}

// EnforcePayload is the OnEnforce input. The runner has resolved the node's
// kind-specific body (rendered templates, applied retry policy) but has not
// yet dispatched the executor. Returning a non-nil error fails the node.
type EnforcePayload struct {
	RunID   string
	Node    *Node
	Inputs  map[string]string
	Outputs map[string]string
	// Attempt is the 1-based attempt number this enforcement check is for.
	// 1 on first try; ≥ 2 on a retry. Hooks may want to relax checks across
	// retries (e.g. only enforce a token cap on attempt 1).
	Attempt int
}

// AuditPayload is the OnAudit input — a copy of the AuditEvent that was just
// emitted to the configured AuditSink. Hooks read; they do not amend the
// event in place. Mutating Payload.Event in a hook is undefined behaviour.
type AuditPayload struct {
	Event AuditEvent
}

// ResolvePayload is the OnResolve input. Triggered when a node transitions
// to a terminal failure state. ErrorClass is the same short label the
// runner stores on node_attempts.error_class.
type ResolvePayload struct {
	RunID      string
	Node       *Node
	Result     *Result
	ErrorClass string
	OccurredAt time.Time
}

// callDefine invokes OnDefine if set. Hook errors are wrapped with a
// stable prefix so audit tooling can grep them.
func (h *Hooks) callDefine(ctx context.Context, p DefinePayload) error {
	if h == nil || h.OnDefine == nil {
		return nil
	}
	if err := h.OnDefine(ctx, p); err != nil {
		return fmt.Errorf("hook OnDefine: %w", err)
	}
	return nil
}

// callEnforce invokes OnEnforce if set. A non-nil return propagates back to
// the runner as the node's failure cause.
func (h *Hooks) callEnforce(ctx context.Context, p EnforcePayload) error {
	if h == nil || h.OnEnforce == nil {
		return nil
	}
	if err := h.OnEnforce(ctx, p); err != nil {
		return fmt.Errorf("hook OnEnforce: %w", err)
	}
	return nil
}

// callAudit invokes OnAudit if set. Errors are returned to the runner so
// they can be logged; they never block the run.
func (h *Hooks) callAudit(ctx context.Context, p AuditPayload) error {
	if h == nil || h.OnAudit == nil {
		return nil
	}
	if err := h.OnAudit(ctx, p); err != nil {
		return fmt.Errorf("hook OnAudit: %w", err)
	}
	return nil
}

// callResolve invokes OnResolve if set.
func (h *Hooks) callResolve(ctx context.Context, p ResolvePayload) error {
	if h == nil || h.OnResolve == nil {
		return nil
	}
	if err := h.OnResolve(ctx, p); err != nil {
		return fmt.Errorf("hook OnResolve: %w", err)
	}
	return nil
}
