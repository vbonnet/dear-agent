package supervisor

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

// Roadmap is the seam the Meta-Orchestrator drives. Per CONTEXT.md §"The
// three supervisors":
//
//	Meta-Orchestrator | Owns: Roadmap, prioritization, technology consistency,
//	                          not reinventing the wheel.
//	Roadmap authority | Only the Meta-Orchestrator may add items to the roadmap.
//
// PR 1 ships an in-memory implementation (InMemoryRoadmap). Follow-ups will
// add adapters to the beads epic store and/or other roadmap substrates.
type Roadmap interface {
	// PendingProposals returns Work Orders awaiting Meta-Orchestrator
	// decision. The returned slice is read-only from the supervisor's
	// perspective; callers must not mutate it.
	PendingProposals(ctx context.Context) ([]WorkProposal, error)

	// Accept records that the Meta-Orchestrator has admitted this
	// proposal to the roadmap.
	Accept(ctx context.Context, proposalID string) error

	// Reject records that the Meta-Orchestrator has rejected this
	// proposal, with a short rationale.
	Reject(ctx context.Context, proposalID, reason string) error
}

// ScopeHint classifies the breadth and depth of a Work Order, giving the
// Meta-Orchestrator a signal for sequencing and resource allocation.
type ScopeHint string

// Recognised ScopeHint values. Unknown strings are tolerated by the
// Meta-Orchestrator and treated as ScopeUnspecified.
const (
	ScopeUnspecified ScopeHint = ""
	ScopeFix         ScopeHint = "fix"      // targeted correction with narrow blast radius
	ScopeRefactor    ScopeHint = "refactor" // internal restructuring, no new capability
	ScopeFeature     ScopeHint = "feature"  // new end-user capability
	ScopeResearch    ScopeHint = "research" // open-ended exploration / spike
	ScopeInfra       ScopeHint = "infra"    // plumbing, CI, toolchain
)

// WorkProposal is the formal Work Order artifact (CONTEXT.md §"Work Order").
// Required fields: ID, Title, Reason. Call Validate to surface all violations.
type WorkProposal struct {
	// ID uniquely identifies the proposal within a Roadmap. Required.
	ID string

	// Title is a short human-readable summary. Required.
	Title string

	// Reason is the proposer's justification. Required; CONTEXT.md §"Work Order"
	// mandates at least one non-empty reason.
	Reason string

	// SubmittedBy is the role + session identifier of the submitting agent.
	// Recommended format: "<role>/<session-id>" (e.g. "worker/sess-abc123").
	SubmittedBy string

	// SubmitterChain is the ordered delegation chain from the original
	// requester to the immediate submitter. Entry 0 is the originator.
	SubmitterChain []string

	// ScopeHint classifies the work's breadth to aid Meta-Orchestrator sequencing.
	ScopeHint ScopeHint

	// Tags are free-form labels for categorization (e.g. "vroom", "agm", "ci").
	Tags []string
}

// Validate returns a combined error describing all schema violations, or nil
// when the proposal satisfies every required-field invariant.
func (p WorkProposal) Validate() error {
	var errs []string
	if p.ID == "" {
		errs = append(errs, "ID required")
	}
	if p.Title == "" {
		errs = append(errs, "Title required")
	}
	if p.Reason == "" {
		errs = append(errs, "Reason required (CONTEXT.md §Work Order)")
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.New("WorkProposal: " + strings.Join(errs, "; "))
}

// metaoIdleEscalationThreshold is the number of consecutive empty-roadmap ticks
// after which the Meta-Orchestrator emits supervisor.metao.idle_escalation.
// Mirrors the Orchestrator's idleEscalationThreshold (ce-6as.7 / ce-6as.61):
// a supervisor that silently does nothing for 7 ticks is either misconfigured
// or waiting for input it never signals — that must surface.
const metaoIdleEscalationThreshold = 7

// MetaOrchestrator is the CTO-analogue supervisor. Its Tick scans the
// roadmap for pending Work Order proposals and decides which to admit.
//
// Admission policy: accept every proposal that passes Validate (required
// fields present). Anti-duplication and scope-expansion logic are
// follow-up work that wires the MetaOrchestrator to the actual roadmap.
//
// Idle escalation (ce-6as.61): when the roadmap is empty for 7 consecutive
// ticks the Meta-Orchestrator emits supervisor.metao.idle_escalation so an
// operator or the Meta-Orchestrator itself can investigate.
type MetaOrchestrator struct {
	trail           decisiontrail.Trail
	roadmap         Roadmap
	consecutiveIdle int
}

// NewMetaOrchestrator constructs the Meta-Orchestrator supervisor.
func NewMetaOrchestrator(trail decisiontrail.Trail, roadmap Roadmap) (*MetaOrchestrator, error) {
	if trail == nil {
		return nil, errors.New("supervisor: MetaOrchestrator requires a Trail")
	}
	if roadmap == nil {
		return nil, errors.New("supervisor: MetaOrchestrator requires a Roadmap")
	}
	return &MetaOrchestrator{trail: trail, roadmap: roadmap}, nil
}

// Role implements Supervisor.
func (m *MetaOrchestrator) Role() Role { return RoleMetaOrchestrator }

// Tick scans pending proposals and applies the PR-1 policy. When no
// proposals are pending it emits supervisor.metao.no_work and, after
// metaoIdleEscalationThreshold consecutive idle ticks, additionally emits
// supervisor.metao.idle_escalation (ce-6as.61).
func (m *MetaOrchestrator) Tick(ctx context.Context) error {
	props, err := m.roadmap.PendingProposals(ctx)
	if err != nil {
		return fmt.Errorf("meta-orchestrator: list proposals: %w", err)
	}

	evaluated := 0
	for _, p := range props {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		accepted, reason := m.evaluate(p)
		rec := decisiontrail.Record{
			Role: string(RoleMetaOrchestrator),
			Kind: "supervisor.metao.roadmap.evaluated",
			Payload: map[string]any{
				"proposal_id": p.ID,
				"title":       p.Title,
				"accepted":    accepted,
				"reason":      reason,
			},
		}
		_ = m.trail.Append(ctx, rec)
		if accepted {
			if err := m.roadmap.Accept(ctx, p.ID); err != nil {
				_ = m.trail.Append(ctx, decisiontrail.Record{
					Role: string(RoleMetaOrchestrator),
					Kind: "supervisor.metao.roadmap.accept_failed",
					Payload: map[string]any{
						"proposal_id": p.ID,
						"error":       err.Error(),
					},
				})
			}
			evaluated++
			continue
		}
		if err := m.roadmap.Reject(ctx, p.ID, reason); err != nil {
			_ = m.trail.Append(ctx, decisiontrail.Record{
				Role: string(RoleMetaOrchestrator),
				Kind: "supervisor.metao.roadmap.reject_failed",
				Payload: map[string]any{
					"proposal_id": p.ID,
					"error":       err.Error(),
				},
			})
		}
		evaluated++
	}

	if evaluated > 0 {
		m.consecutiveIdle = 0
		return nil
	}

	// Empty roadmap tick.
	m.consecutiveIdle++
	_ = m.trail.Append(ctx, decisiontrail.Record{
		Role: string(RoleMetaOrchestrator),
		Kind: "supervisor.metao.no_work",
		Payload: map[string]any{
			"consecutive_idle": m.consecutiveIdle,
		},
	})

	if m.consecutiveIdle >= metaoIdleEscalationThreshold {
		_ = m.trail.Append(ctx, decisiontrail.Record{
			Role: string(RoleMetaOrchestrator),
			Kind: "supervisor.metao.idle_escalation",
			Payload: map[string]any{
				"consecutive_idle": m.consecutiveIdle,
				"threshold":        metaoIdleEscalationThreshold,
				"action":           "investigate: roadmap empty or no proposals being submitted",
			},
		})
	}
	return nil
}

// evaluate returns (accepted, reason). A proposal passes iff it satisfies
// the full WorkProposal schema (Validate returns nil). The reason field on
// success is the proposal's own Reason; on rejection it is the validation error.
func (m *MetaOrchestrator) evaluate(p WorkProposal) (bool, string) {
	if err := p.Validate(); err != nil {
		return false, err.Error()
	}
	return true, p.Reason
}
