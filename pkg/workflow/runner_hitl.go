package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// handleHITL drives the awaiting_hitl portion of the node lifecycle.
//
// Flow:
//   1. Build a HITLRequest from the node's policy + result.
//   2. Persist it (via the recorder, if it can store approvals) and emit
//      the running → awaiting_hitl audit row.
//   3. Notify the backend so it can surface the request to humans.
//   4. Wait for a decision, applying the OnTimeout policy when no backend
//      resolution arrives in time.
//   5. Translate the decision back into a node outcome (resume, fail, or
//      escalate-as-fail in v1).
//
// Returning a non-nil error from handleHITL fails the node (the caller
// stores the error in res.Error). Returning nil means "approved" and the
// caller continues to output materialisation + the success transition.
func (r *Runner) handleHITL(nc *nodeContext, node *Node, res *Result, reason string) error {
	policy := node.HITL
	if policy == nil {
		return nil
	}

	requestedAt := time.Now()
	approvalID, persistErr := r.persistHITL(nc.ctx, nc.runID, node.ID, policy.ApproverRole, reason, requestedAt)
	if persistErr != nil {
		// Best-effort: even if we failed to persist, keep the run flowing.
		r.Logger.Warn("hitl persist failed; falling back to in-memory", "node", node.ID, "err", persistErr)
		approvalID = "ephemeral:" + node.ID
	}

	if err := r.emitAudit(nc.ctx, AuditEvent{
		RunID:      nc.runID,
		NodeID:     node.ID,
		FromState:  string(NodeStateRunning),
		ToState:    string(NodeStateAwaitingHITL),
		Reason:     reason,
		Actor:      "system",
		OccurredAt: requestedAt,
	}); err != nil {
		r.Logger.Warn("audit emit(awaiting_hitl) failed", "node", node.ID, "err", err)
	}

	if r.HITLBackend != nil {
		req := HITLRequest{
			ApprovalID:   approvalID,
			RunID:        nc.runID,
			NodeID:       node.ID,
			ApproverRole: policy.ApproverRole,
			Reason:       reason,
			RequestedAt:  requestedAt,
			Timeout:      policy.Timeout,
			OnTimeout:    policy.OnTimeout,
			NodeOutput:   res.Output,
		}
		if conf, ok := extractConfidence(res); ok {
			req.Confidence = conf
		}
		if err := r.HITLBackend.Request(nc.ctx, req); err != nil {
			r.Logger.Warn("hitl backend request failed", "node", node.ID, "err", err)
		}
	}

	resolution, waitErr := r.waitForHITL(nc.ctx, approvalID, policy)
	if waitErr != nil {
		return waitErr
	}

	switch resolution.Decision {
	case HITLDecisionApprove:
		// Restore running state in audit. The approvals row already shows
		// the human's identity; the audit_events row records the
		// awaiting_hitl → running transition.
		if err := r.emitAudit(nc.ctx, AuditEvent{
			RunID:      nc.runID,
			NodeID:     node.ID,
			FromState:  string(NodeStateAwaitingHITL),
			ToState:    string(NodeStateRunning),
			Reason:     "hitl-approved",
			Actor:      hitlActor(resolution.Approver, resolution.Role),
			OccurredAt: resolution.ResolvedAt,
		}); err != nil {
			r.Logger.Warn("audit emit(approved) failed", "node", node.ID, "err", err)
		}
		return nil
	case HITLDecisionReject:
		return fmt.Errorf("hitl: rejected by %s", hitlActor(resolution.Approver, resolution.Role))
	case HITLDecisionTimeout:
		// Apply on_timeout policy. "approve" means resume despite the
		// human absence; "escalate" is treated as "fail" in v1 with an
		// audit reason that flags it for follow-up tooling. "reject"
		// (and the default) fail the node.
		switch policy.OnTimeout {
		case "approve":
			if err := r.emitAudit(nc.ctx, AuditEvent{
				RunID:      nc.runID,
				NodeID:     node.ID,
				FromState:  string(NodeStateAwaitingHITL),
				ToState:    string(NodeStateRunning),
				Reason:     "hitl-timeout-approved",
				Actor:      "system",
				OccurredAt: resolution.ResolvedAt,
			}); err != nil {
				r.Logger.Warn("audit emit(timeout-approved) failed", "node", node.ID, "err", err)
			}
			return nil
		case "escalate":
			return fmt.Errorf("hitl: timeout (escalation requested but no escalation backend configured)")
		default:
			return fmt.Errorf("hitl: timeout (no approver responded)")
		}
	}
	return fmt.Errorf("hitl: unknown decision %q", resolution.Decision)
}

// persistHITL writes the approvals row using the runner's recorder when it
// is a SQLiteState (the only recorder that owns a *sql.DB). Other recorders
// — including the noop one — return ErrHITLNoBackend so the caller falls
// back to an ephemeral approval id.
func (r *Runner) persistHITL(ctx context.Context, runID, nodeID, approverRole, reason string, requestedAt time.Time) (string, error) {
	ss, ok := r.Recorder.(*SQLiteState)
	if !ok || ss == nil || ss.DB() == nil {
		return "", ErrHITLNoBackend
	}
	return CreateHITLRequest(ctx, ss.DB(), runID, nodeID, approverRole, reason, requestedAt)
}

// waitForHITL waits for the configured backend to resolve the approval. A
// nil backend means "no human in the loop is wired" — we synthesize an
// immediate timeout so the OnTimeout policy decides what happens. The
// per-node Timeout caps the wait independently of the backend; if it
// fires we still synthesize a Timeout decision rather than returning
// ctx.Err() so the OnTimeout policy applies uniformly.
func (r *Runner) waitForHITL(ctx context.Context, approvalID string, policy *HITLPolicy) (HITLResolution, error) {
	if r.HITLBackend == nil {
		return HITLResolution{
			ApprovalID: approvalID,
			Decision:   HITLDecisionTimeout,
			ResolvedAt: time.Now(),
			Reason:     "no-hitl-backend-configured",
		}, nil
	}
	waitCtx := ctx
	if policy.Timeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, policy.Timeout)
		defer cancel()
	}
	res, err := r.HITLBackend.Wait(waitCtx, approvalID)
	if err == nil {
		return res, nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return HITLResolution{
			ApprovalID: approvalID,
			Decision:   HITLDecisionTimeout,
			ResolvedAt: time.Now(),
			Reason:     "policy-timeout",
		}, nil
	}
	return HITLResolution{}, err
}

// hitlActor formats the approver + role into the audit_events.actor string
// shape ("human:<name>" or "role:<role>" when role is the only identifier).
func hitlActor(approver, role string) string {
	switch {
	case approver != "" && role != "":
		return fmt.Sprintf("human:%s/%s", approver, role)
	case approver != "":
		return "human:" + approver
	case role != "":
		return "role:" + role
	default:
		return "human"
	}
}
