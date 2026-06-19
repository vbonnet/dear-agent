package supervisor

import (
	"context"
	"errors"
	"fmt"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

// Roadmap-delegation errors (retro R.5).
var (
	// ErrNoFailoverAuthority means the supervisor attempted a delegated
	// roadmap write without having assumed Meta-Orchestrator authority.
	ErrNoFailoverAuthority = errors.New("supervisor: no assumed Meta-Orchestrator authority for delegated roadmap write")

	// ErrFailoverUnsupported means the underlying Roadmap does not implement
	// FailoverAuthority, so delegated writes are impossible.
	ErrFailoverUnsupported = errors.New("supervisor: roadmap does not support failover delegation")
)

// FailoverAuthority is the optional capability a Roadmap implements to accept
// items admitted by a supervisor operating under assumed Meta-Orchestrator
// authority (retro R.5). Items added this way are flagged so the
// Meta-Orchestrator can review them on recovery — they entered the roadmap
// without its normal admission decision.
type FailoverAuthority interface {
	// AddUnderFailover admits p to the roadmap on behalf of by, who has
	// assumed Meta-O authority. Implementations must record that the item was
	// added under failover authority and is subject to Meta-O review.
	AddUnderFailover(ctx context.Context, p WorkProposal, by Role) error
}

// AuthorityHolder reports whether a supervisor currently holds an assumed role.
// *Failover implements it; defining the interface keeps DelegatedRoadmap
// testable without constructing a full Failover.
type AuthorityHolder interface {
	HoldsAuthority(target Role) bool
}

// DelegatedRoadmap is the seam a non-Meta-O supervisor uses to admit roadmap
// items while holding assumed Meta-O authority during degraded operation
// (retro R.5: "Allow the Overseer (as Secondary) to add items to the roadmap
// in degraded mode, with a flag indicating they were added under failover
// authority"). It gates every write on a live assumed-authority check so it
// can never admit work after the Meta-Orchestrator recovers.
type DelegatedRoadmap struct {
	self      Role
	authority AuthorityHolder
	roadmap   Roadmap
	trail     decisiontrail.Trail
}

// NewDelegatedRoadmap constructs a DelegatedRoadmap. All arguments are
// required. The roadmap must implement FailoverAuthority for Add to succeed;
// the check is deferred to Add so a mesh can wire delegation optimistically.
func NewDelegatedRoadmap(self Role, authority AuthorityHolder, roadmap Roadmap, trail decisiontrail.Trail) (*DelegatedRoadmap, error) {
	if !self.Valid() {
		return nil, errors.New("supervisor: DelegatedRoadmap.self is not a valid role")
	}
	if authority == nil {
		return nil, errors.New("supervisor: DelegatedRoadmap requires an AuthorityHolder")
	}
	if roadmap == nil {
		return nil, errors.New("supervisor: DelegatedRoadmap requires a Roadmap")
	}
	if trail == nil {
		return nil, errors.New("supervisor: DelegatedRoadmap requires a Trail")
	}
	return &DelegatedRoadmap{self: self, authority: authority, roadmap: roadmap, trail: trail}, nil
}

// Add admits p to the roadmap under failover authority. It returns
// ErrNoFailoverAuthority unless this supervisor has assumed Meta-O authority,
// ErrFailoverUnsupported if the roadmap cannot accept failover items, and the
// validation error if p is malformed. On success it records
// supervisor.failover.roadmap_added to the decision trail.
func (d *DelegatedRoadmap) Add(ctx context.Context, p WorkProposal) error {
	if !d.authority.HoldsAuthority(RoleMetaOrchestrator) {
		_ = d.trail.Append(ctx, decisiontrail.Record{
			Role: string(d.self),
			Kind: "supervisor.failover.roadmap_denied",
			Payload: map[string]any{
				"proposal_id": p.ID,
				"reason":      "no assumed Meta-O authority",
			},
		})
		return ErrNoFailoverAuthority
	}
	if err := p.Validate(); err != nil {
		return err
	}
	fa, ok := d.roadmap.(FailoverAuthority)
	if !ok {
		return ErrFailoverUnsupported
	}
	if err := fa.AddUnderFailover(ctx, p, d.self); err != nil {
		_ = d.trail.Append(ctx, decisiontrail.Record{
			Role: string(d.self),
			Kind: "supervisor.failover.roadmap_add_failed",
			Payload: map[string]any{
				"proposal_id": p.ID,
				"error":       err.Error(),
			},
		})
		return fmt.Errorf("delegated roadmap add: %w", err)
	}
	_ = d.trail.Append(ctx, decisiontrail.Record{
		Role: string(d.self),
		Kind: "supervisor.failover.roadmap_added",
		Payload: map[string]any{
			"proposal_id":       p.ID,
			"title":             p.Title,
			"by":                string(d.self),
			"under_authority":   string(RoleMetaOrchestrator),
			"subject_to_review": true,
		},
	})
	return nil
}
