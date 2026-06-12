package supervisor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

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
// Evaluation policy (CONTEXT.md §"Roadmap authority"):
//   - A proposal without a Reason is rejected immediately (Work Order
//     invariant).
//   - A proposal whose normalized title duplicates an already-admitted
//     proposal is rejected to prevent duplicate work. The check is
//     in-session only: the admitted-title set is not persisted across
//     restarts. (Persistent de-dup requires a roadmap adapter that tracks
//     history — a follow-up once the real Roadmap adapter lands.)
//   - All other proposals are accepted.
//
// The admitted-title set is maintained by metaTitleKey, which folds to
// lowercase and strips punctuation/whitespace so minor wording variations
// ("Fix lint" vs "fix lint." vs "Fix Lint") are caught.
type MetaOrchestrator struct {
	trail          decisiontrail.Trail
	roadmap        Roadmap
	admittedTitles map[string]string // normalized title → first-admitted proposal ID
}

// NewMetaOrchestrator constructs the Meta-Orchestrator supervisor.
func NewMetaOrchestrator(trail decisiontrail.Trail, roadmap Roadmap) (*MetaOrchestrator, error) {
	if trail == nil {
		return nil, errors.New("supervisor: MetaOrchestrator requires a Trail")
	}
	if roadmap == nil {
		return nil, errors.New("supervisor: MetaOrchestrator requires a Roadmap")
	}
	return &MetaOrchestrator{
		trail:          trail,
		roadmap:        roadmap,
		admittedTitles: make(map[string]string),
	}, nil
}

// Role implements Supervisor.
func (m *MetaOrchestrator) Role() Role { return RoleMetaOrchestrator }

// Tick scans pending proposals and applies the evaluation policy.
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
				_ = m.trail.Append(ctx, decisiontrail.Record{
					Role: string(RoleMetaOrchestrator),
					Kind: "supervisor.metao.roadmap.accept_failed",
					Payload: map[string]any{
						"proposal_id": p.ID,
						"error":       err.Error(),
					},
				})
				continue
			}
			// Record the admitted title for in-session de-duplication
			// (ce-6as.80 Phase 2b). Only update after a successful Accept
			// so a failed Accept does not poison the de-dup set.
			m.admittedTitles[metaTitleKey(p.Title)] = p.ID
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

// evaluate returns (accepted, reason). Policy order:
//  1. Missing Reason → reject.
//  2. Duplicate title (in-session) → reject with the conflicting ID.
//  3. Otherwise → accept.
func (m *MetaOrchestrator) evaluate(p WorkProposal) (bool, string) {
	if p.Reason == "" {
		return false, "Work Order missing required Reason field (CONTEXT.md §Work Order)"
	}
	if prevID, dup := m.admittedTitles[metaTitleKey(p.Title)]; dup {
		return false, fmt.Sprintf("duplicate of already-admitted proposal %q (CONTEXT.md §Roadmap authority)", prevID)
	}
	return true, p.Reason
}

// metaTitleKey normalizes a proposal title for duplicate detection:
// lowercase, strip all non-alphanumeric runes, collapse whitespace.
// "Fix lint." and "fix lint" and "Fix  Lint" all map to the same key.
func metaTitleKey(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if unicode.IsSpace(r) {
			b.WriteByte(' ')
		}
	}
	// Collapse repeated spaces and trim.
	fields := strings.Fields(b.String())
	return strings.Join(fields, " ")
}
