package supervisor

import (
	"context"
	"errors"
	"testing"
)

// fakeAuthority is a hand-controlled AuthorityHolder.
type fakeAuthority struct {
	held map[Role]bool
}

func (f *fakeAuthority) HoldsAuthority(target Role) bool { return f.held[target] }

func TestDelegatedRoadmap_DeniedWithoutAuthority(t *testing.T) {
	trail, _ := newBufferTrail()
	rm := NewInMemoryRoadmap()
	auth := &fakeAuthority{held: map[Role]bool{}} // holds nothing
	d, err := NewDelegatedRoadmap(RoleOverseer, auth, rm, trail)
	if err != nil {
		t.Fatal(err)
	}
	p := WorkProposal{ID: "wp-1", Title: "t", Reason: "r"}
	if err := d.Add(context.Background(), p); !errors.Is(err, ErrNoFailoverAuthority) {
		t.Fatalf("want ErrNoFailoverAuthority, got %v", err)
	}
	if len(rm.FailoverAdded()) != 0 {
		t.Error("nothing should be added without authority")
	}
}

func TestDelegatedRoadmap_AddsUnderAuthority(t *testing.T) {
	trail, buf := newBufferTrail()
	rm := NewInMemoryRoadmap()
	auth := &fakeAuthority{held: map[Role]bool{RoleMetaOrchestrator: true}}
	d, err := NewDelegatedRoadmap(RoleOverseer, auth, rm, trail)
	if err != nil {
		t.Fatal(err)
	}
	p := WorkProposal{ID: "wp-7", Title: "degraded work", Reason: "needed while meta-o dark"}
	if err := d.Add(context.Background(), p); err != nil {
		t.Fatalf("Add under authority: %v", err)
	}
	added := rm.FailoverAdded()
	if len(added) != 1 || added[0] != "wp-7" {
		t.Fatalf("want wp-7 in failover-added review queue, got %v", added)
	}
	accepted := rm.Accepted()
	if len(accepted) != 1 || accepted[0] != "wp-7" {
		t.Fatalf("want wp-7 accepted, got %v", accepted)
	}
	if !hasKind(trailKinds(t, buf), "supervisor.failover.roadmap_added") {
		t.Error("want supervisor.failover.roadmap_added trail record")
	}
}

func TestDelegatedRoadmap_RejectsInvalidProposal(t *testing.T) {
	trail, _ := newBufferTrail()
	rm := NewInMemoryRoadmap()
	auth := &fakeAuthority{held: map[Role]bool{RoleMetaOrchestrator: true}}
	d, _ := NewDelegatedRoadmap(RoleOverseer, auth, rm, trail)
	// Missing Reason → validation error, even with authority.
	err := d.Add(context.Background(), WorkProposal{ID: "x", Title: "t"})
	if err == nil {
		t.Fatal("want validation error for malformed proposal")
	}
	if len(rm.FailoverAdded()) != 0 {
		t.Error("invalid proposal must not be added")
	}
}

// roadmapWithoutFailover implements Roadmap but NOT FailoverAuthority.
type roadmapWithoutFailover struct{}

func (roadmapWithoutFailover) PendingProposals(context.Context) ([]WorkProposal, error) {
	return nil, nil
}
func (roadmapWithoutFailover) Accept(context.Context, string) error         { return nil }
func (roadmapWithoutFailover) Reject(context.Context, string, string) error { return nil }

func TestDelegatedRoadmap_UnsupportedRoadmap(t *testing.T) {
	trail, _ := newBufferTrail()
	auth := &fakeAuthority{held: map[Role]bool{RoleMetaOrchestrator: true}}
	d, _ := NewDelegatedRoadmap(RoleOverseer, auth, roadmapWithoutFailover{}, trail)
	err := d.Add(context.Background(), WorkProposal{ID: "x", Title: "t", Reason: "r"})
	if !errors.Is(err, ErrFailoverUnsupported) {
		t.Fatalf("want ErrFailoverUnsupported, got %v", err)
	}
}

func TestNewDelegatedRoadmap_Validation(t *testing.T) {
	trail, _ := newBufferTrail()
	rm := NewInMemoryRoadmap()
	auth := &fakeAuthority{held: map[Role]bool{}}
	if _, err := NewDelegatedRoadmap("bad", auth, rm, trail); err == nil {
		t.Error("want error on invalid self")
	}
	if _, err := NewDelegatedRoadmap(RoleOverseer, nil, rm, trail); err == nil {
		t.Error("want error on nil authority")
	}
	if _, err := NewDelegatedRoadmap(RoleOverseer, auth, nil, trail); err == nil {
		t.Error("want error on nil roadmap")
	}
	if _, err := NewDelegatedRoadmap(RoleOverseer, auth, rm, nil); err == nil {
		t.Error("want error on nil trail")
	}
}
