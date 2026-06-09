package supervisor

import (
	"context"
	"errors"
	"fmt"

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

// WorkProposal is one Work Order awaiting Meta-Orchestrator decision. The
// shape is deliberately small for PR 1; the real Work Order schema (with
// reason, submitter chain, scope hints, etc.) lands in follow-up PRs.
type WorkProposal struct {
	// ID uniquely identifies the proposal within a Roadmap.
	ID string

	// Title is a short human-readable summary.
	Title string

	// Reason is the proposer's justification (required by CONTEXT.md
	// §"Work Order").
	Reason string

	// SubmittedBy names the agent that submitted the proposal. Free-form
	// for now; eventually a role + session identifier.
	SubmittedBy string
}

// MetaOrchestrator is the CTO-analogue supervisor. Its Tick scans the
// roadmap for pending Work Order proposals and decides which to admit.
//
// PR-1 policy is deliberately trivial: accept everything that has a
// non-empty Reason (the only Work Order invariant CONTEXT.md mentions
// explicitly), reject the rest. Real anti-duplication / scope-expansion
// logic lands in the follow-up that wires this to the actual roadmap.
type MetaOrchestrator struct {
	trail   decisiontrail.Trail
	roadmap Roadmap
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

// Tick scans pending proposals and applies the PR-1 policy.
func (m *MetaOrchestrator) Tick(ctx context.Context) error {
	props, err := m.roadmap.PendingProposals(ctx)
	if err != nil {
		return fmt.Errorf("meta-orchestrator: list proposals: %w", err)
	}
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
				// Don't fail the whole tick on one proposal; record and continue.
				_ = m.trail.Append(ctx, decisiontrail.Record{
					Role: string(RoleMetaOrchestrator),
					Kind: "supervisor.metao.roadmap.accept_failed",
					Payload: map[string]any{
						"proposal_id": p.ID,
						"error":       err.Error(),
					},
				})
			}
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
	}
	return nil
}

// evaluate returns (accepted, reason). The PR-1 policy: accept if Reason is
// non-empty; reject otherwise. The reason field in the return is either the
// proposal's own Reason (on accept) or the rejection cause.
func (m *MetaOrchestrator) evaluate(p WorkProposal) (bool, string) {
	if p.Reason == "" {
		return false, "Work Order missing required Reason field (CONTEXT.md §Work Order)"
	}
	return true, p.Reason
}
