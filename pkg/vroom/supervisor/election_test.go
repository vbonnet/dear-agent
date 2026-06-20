package supervisor

import (
	"context"
	"testing"
	"time"
)

// staleSet builds an isStale closure from a set of dark roles.
func staleSet(dark ...Role) func(Role) bool {
	m := map[Role]bool{}
	for _, r := range dark {
		m[r] = true
	}
	return func(r Role) bool { return m[r] }
}

func newElectionForTest(t *testing.T, self Role, store ClaimStore, clock func() time.Time, window time.Duration) *Election {
	t.Helper()
	trail, _ := newBufferTrail()
	e, err := NewElection(ElectionConfig{
		Self:         self,
		Store:        store,
		Trail:        trail,
		VerifyWindow: window,
		Clock:        clock,
	})
	if err != nil {
		t.Fatalf("NewElection: %v", err)
	}
	return e
}

func TestElection_SecondaryClaimsVerifiesAssumes(t *testing.T) {
	now := time.Unix(1000, 0)
	clock := func() time.Time { return now }
	store := NewInMemoryClaimStore()
	// Overseer is the Secondary for Meta-O.
	e := newElectionForTest(t, RoleOverseer, store, clock, 2*time.Second)
	ctx := context.Background()
	isStale := staleSet(RoleMetaOrchestrator)

	// Step 1: claim.
	out := e.Step(ctx, RoleMetaOrchestrator, "primary dark", isStale)
	if !out.Claimed || out.Phase != PhaseClaimed {
		t.Fatalf("step1: want claimed, got %+v", out)
	}
	if _, ok, _ := store.Get(ctx, RoleMetaOrchestrator); !ok {
		t.Fatal("step1: claim not persisted to store")
	}

	// Step 2: still inside verify window — no assume yet.
	now = now.Add(1 * time.Second)
	out = e.Step(ctx, RoleMetaOrchestrator, "primary dark", isStale)
	if out.Assumed || out.Phase != PhaseClaimed {
		t.Fatalf("step2: want still claimed (verify window open), got %+v", out)
	}

	// Step 3: verify window elapsed, unchallenged → assume.
	now = now.Add(2 * time.Second)
	out = e.Step(ctx, RoleMetaOrchestrator, "primary dark", isStale)
	if !out.Assumed || out.Phase != PhaseAssumed {
		t.Fatalf("step3: want assumed, got %+v", out)
	}
	if e.Phase(RoleMetaOrchestrator) != PhaseAssumed {
		t.Error("step3: Phase should be assumed")
	}

	// Step 4: steady state — idempotent.
	out = e.Step(ctx, RoleMetaOrchestrator, "primary dark", isStale)
	if out.Phase != PhaseAssumed || out.Assumed {
		t.Fatalf("step4: want steady assumed (no re-assume), got %+v", out)
	}
}

func TestElection_TertiaryDefersToLiveSecondary(t *testing.T) {
	now := time.Unix(2000, 0)
	clock := func() time.Time { return now }
	store := NewInMemoryClaimStore()
	// Orchestrator is the Tertiary for Meta-O (SecondaryFor(Meta-O)=Overseer).
	e := newElectionForTest(t, RoleOrchestrator, store, clock, 2*time.Second)
	ctx := context.Background()
	// Meta-O dark but Overseer (its Secondary) alive → Orchestrator defers.
	isStale := staleSet(RoleMetaOrchestrator)

	out := e.Step(ctx, RoleMetaOrchestrator, "primary dark", isStale)
	if out.Eligible {
		t.Fatalf("tertiary should defer to live secondary, got %+v", out)
	}
	if out.Claimed || out.Phase == PhaseClaimed {
		t.Fatal("tertiary should not claim while secondary alive")
	}
	if _, ok, _ := store.Get(ctx, RoleMetaOrchestrator); ok {
		t.Fatal("tertiary wrote a claim despite deferring")
	}
}

func TestElection_TertiaryPromotesWhenSecondaryAlsoDark(t *testing.T) {
	now := time.Unix(3000, 0)
	clock := func() time.Time { return now }
	store := NewInMemoryClaimStore()
	e := newElectionForTest(t, RoleOrchestrator, store, clock, 2*time.Second)
	ctx := context.Background()
	// Both Meta-O and its Secondary (Overseer) dark → Orchestrator promotes.
	isStale := staleSet(RoleMetaOrchestrator, RoleOverseer)

	out := e.Step(ctx, RoleMetaOrchestrator, "both dark", isStale)
	if !out.Eligible || !out.Claimed {
		t.Fatalf("tertiary should claim when secondary also dark, got %+v", out)
	}
	now = now.Add(3 * time.Second)
	out = e.Step(ctx, RoleMetaOrchestrator, "both dark", isStale)
	if !out.Assumed {
		t.Fatalf("tertiary should assume after verify window, got %+v", out)
	}
}

func TestElection_RelinquishOnRecovery(t *testing.T) {
	now := time.Unix(4000, 0)
	clock := func() time.Time { return now }
	store := NewInMemoryClaimStore()
	e := newElectionForTest(t, RoleOverseer, store, clock, 2*time.Second)
	ctx := context.Background()

	// Claim + assume while Meta-O dark.
	dark := staleSet(RoleMetaOrchestrator)
	e.Step(ctx, RoleMetaOrchestrator, "dark", dark)
	now = now.Add(3 * time.Second)
	if out := e.Step(ctx, RoleMetaOrchestrator, "dark", dark); !out.Assumed {
		t.Fatalf("want assumed, got %+v", out)
	}

	// Meta-O recovers → relinquish.
	alive := staleSet() // nothing dark
	out := e.Step(ctx, RoleMetaOrchestrator, "dark", alive)
	if !out.Relinquished || out.Phase != PhaseIdle {
		t.Fatalf("want relinquished+idle on recovery, got %+v", out)
	}
	if _, ok, _ := store.Get(ctx, RoleMetaOrchestrator); ok {
		t.Error("claim should be deleted from store after relinquish")
	}
	if e.Active(RoleMetaOrchestrator) {
		t.Error("election should be inactive after relinquish")
	}
}

func TestElection_YieldsToCompetingClaim(t *testing.T) {
	now := time.Unix(5000, 0)
	clock := func() time.Time { return now }
	store := NewInMemoryClaimStore()
	ctx := context.Background()

	// A competing supervisor (Orchestrator) already holds an EARLIER claim on
	// Meta-O. Our Overseer claims later, so on verify it must yield.
	_ = store.Put(ctx, Claim{
		Target:    RoleMetaOrchestrator,
		By:        RoleOrchestrator,
		Reason:    "got there first",
		Timestamp: now.Add(-10 * time.Second),
	})

	e := newElectionForTest(t, RoleOverseer, store, clock, 2*time.Second)
	dark := staleSet(RoleMetaOrchestrator)

	out := e.Step(ctx, RoleMetaOrchestrator, "dark", dark) // claim
	if !out.Claimed {
		t.Fatalf("want claim, got %+v", out)
	}
	now = now.Add(3 * time.Second)
	out = e.Step(ctx, RoleMetaOrchestrator, "dark", dark) // verify → yield
	if !out.Yielded || out.Phase != PhaseIdle {
		t.Fatalf("want yield to earlier competing claim, got %+v", out)
	}
}

func TestClaimWins(t *testing.T) {
	base := time.Unix(6000, 0)
	earlier := Claim{By: RoleOverseer, Timestamp: base}
	later := Claim{By: RoleOrchestrator, Timestamp: base.Add(time.Second)}
	if !claimWins(earlier, later) {
		t.Error("earlier timestamp should win")
	}
	if claimWins(later, earlier) {
		t.Error("later timestamp should lose")
	}

	// Tie broken by canonical role order: meta-orchestrator(0) < orchestrator(1) < overseer(2).
	tieMeta := Claim{By: RoleMetaOrchestrator, Timestamp: base}
	tieOver := Claim{By: RoleOverseer, Timestamp: base}
	if !claimWins(tieMeta, tieOver) {
		t.Error("tie: meta-orchestrator should win over overseer by role order")
	}
}

func TestNewElection_Validation(t *testing.T) {
	trail, _ := newBufferTrail()
	store := NewInMemoryClaimStore()
	if _, err := NewElection(ElectionConfig{Self: "bogus", Store: store, Trail: trail}); err == nil {
		t.Error("want error on invalid self")
	}
	if _, err := NewElection(ElectionConfig{Self: RoleOverseer, Store: nil, Trail: trail}); err == nil {
		t.Error("want error on nil store")
	}
	if _, err := NewElection(ElectionConfig{Self: RoleOverseer, Store: store, Trail: nil}); err == nil {
		t.Error("want error on nil trail")
	}
}
