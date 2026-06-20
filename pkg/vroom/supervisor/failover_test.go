package supervisor

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"
)

// hasKind reports whether want appears among the trail record kinds (helper
// trailKinds is defined in loop_test.go).
func hasKind(kinds []string, want string) bool {
	return slices.Contains(kinds, want)
}

func newFailoverForTest(t *testing.T, self Role, clock func() time.Time, window time.Duration, promote int, notifier PeerNotifier) (*Failover, *bytes.Buffer) {
	t.Helper()
	trail, buf := newBufferTrail()
	store := NewInMemoryClaimStore()
	e, err := NewElection(ElectionConfig{Self: self, Store: store, Trail: trail, VerifyWindow: window, Clock: clock})
	if err != nil {
		t.Fatalf("NewElection: %v", err)
	}
	f, err := NewFailover(FailoverConfig{
		Self:             self,
		Trail:            trail,
		Election:         e,
		Notifier:         notifier,
		PromoteThreshold: promote,
		Clock:            clock,
	})
	if err != nil {
		t.Fatalf("NewFailover: %v", err)
	}
	return f, buf
}

func TestFailover_ModeTransitions(t *testing.T) {
	now := time.Unix(1000, 0)
	clock := func() time.Time { return now }
	f, buf := newFailoverForTest(t, RoleOverseer, clock, 2*time.Second, 3, nil)
	ctx := context.Background()

	allAlive := map[Role]bool{RoleMetaOrchestrator: false, RoleOrchestrator: false}
	f.Observe(ctx, allAlive)
	if f.Mode() != ModeNormal {
		t.Fatalf("want ModeNormal, got %v", f.Mode())
	}

	// One peer dark → degraded.
	oneDark := map[Role]bool{RoleMetaOrchestrator: true, RoleOrchestrator: false}
	f.Observe(ctx, oneDark)
	if f.Mode() != ModeDegraded {
		t.Fatalf("want ModeDegraded, got %v", f.Mode())
	}

	// Both dark → emergency.
	bothDark := map[Role]bool{RoleMetaOrchestrator: true, RoleOrchestrator: true}
	f.Observe(ctx, bothDark)
	if f.Mode() != ModeEmergency {
		t.Fatalf("want ModeEmergency, got %v", f.Mode())
	}

	kinds := trailKinds(t, buf)
	transitions := 0
	for _, k := range kinds {
		if k == "supervisor.mode.transition" {
			transitions++
		}
	}
	if transitions != 3 {
		t.Errorf("want 3 mode transitions (normal→degraded→emergency incl. initial), got %d", transitions)
	}

	// Emergency capabilities should demand self-termination.
	if !f.Capabilities().SelfTerminate {
		t.Error("emergency mode should report SelfTerminate")
	}
}

func TestFailover_NoTransitionRecordWhenModeStable(t *testing.T) {
	now := time.Unix(1000, 0)
	clock := func() time.Time { return now }
	f, buf := newFailoverForTest(t, RoleOverseer, clock, 2*time.Second, 3, nil)
	ctx := context.Background()
	allAlive := map[Role]bool{RoleMetaOrchestrator: false, RoleOrchestrator: false}
	f.Observe(ctx, allAlive)
	f.Observe(ctx, allAlive)
	f.Observe(ctx, allAlive)
	transitions := 0
	for _, k := range trailKinds(t, buf) {
		if k == "supervisor.mode.transition" {
			transitions++
		}
	}
	if transitions != 1 {
		t.Errorf("want exactly 1 transition record for stable mode, got %d", transitions)
	}
}

func TestFailover_SecondaryAssumesAfterThreshold(t *testing.T) {
	now := time.Unix(1000, 0)
	clock := func() time.Time { return now }
	var notes []string
	notifier := PeerNotifierFunc(func(_ context.Context, peer, target, by Role, reason string) error {
		notes = append(notes, string(peer))
		return nil
	})
	// Overseer is Secondary for Meta-O. promote threshold 3, verify window 2s.
	f, buf := newFailoverForTest(t, RoleOverseer, clock, 2*time.Second, 3, notifier)
	ctx := context.Background()
	metaDark := map[Role]bool{RoleMetaOrchestrator: true, RoleOrchestrator: false}

	// Below threshold: no claim yet.
	f.Observe(ctx, metaDark) // staleRun=1
	f.Observe(ctx, metaDark) // staleRun=2
	if hasKind(trailKinds(t, buf), "supervisor.failover.claim") {
		t.Fatal("claim fired before promote threshold")
	}

	// Reach threshold (=3) → claim.
	f.Observe(ctx, metaDark) // staleRun=3 → claim
	if !hasKind(trailKinds(t, buf), "supervisor.failover.claim") {
		t.Fatal("want claim at threshold")
	}
	if f.HoldsAuthority(RoleMetaOrchestrator) {
		t.Fatal("should not hold authority before verify window elapses")
	}

	// Advance past verify window and observe again → assume + announce.
	now = now.Add(3 * time.Second)
	f.Observe(ctx, metaDark)
	if !f.HoldsAuthority(RoleMetaOrchestrator) {
		t.Fatal("want assumed authority after verify window")
	}
	kinds := trailKinds(t, buf)
	if !hasKind(kinds, "supervisor.failover.assumed") {
		t.Error("want supervisor.failover.assumed")
	}
	if !hasKind(kinds, "supervisor.failover.announced") {
		t.Error("want supervisor.failover.announced")
	}
	// Announce should notify the one surviving peer (Orchestrator), not self or target.
	if len(notes) != 1 || notes[0] != string(RoleOrchestrator) {
		t.Errorf("want announce to orchestrator only, got %v", notes)
	}

	// Holding Meta-O authority flips AddRoadmapItems on in degraded mode (R.5).
	if !f.Capabilities().AddRoadmapItems {
		t.Error("assumed Meta-O authority should grant AddRoadmapItems in degraded mode")
	}
}

func TestFailover_ReleasesAuthorityOnRecovery(t *testing.T) {
	now := time.Unix(1000, 0)
	clock := func() time.Time { return now }
	f, _ := newFailoverForTest(t, RoleOverseer, clock, 2*time.Second, 1, nil)
	ctx := context.Background()
	metaDark := map[Role]bool{RoleMetaOrchestrator: true, RoleOrchestrator: false}

	f.Observe(ctx, metaDark) // threshold 1 → claim
	now = now.Add(3 * time.Second)
	f.Observe(ctx, metaDark) // assume
	if !f.HoldsAuthority(RoleMetaOrchestrator) {
		t.Fatal("want assumed authority")
	}

	// Meta-O recovers.
	allAlive := map[Role]bool{RoleMetaOrchestrator: false, RoleOrchestrator: false}
	f.Observe(ctx, allAlive)
	if f.HoldsAuthority(RoleMetaOrchestrator) {
		t.Error("authority should be released after Meta-O recovers")
	}
}

func TestNewFailover_Validation(t *testing.T) {
	trail, _ := newBufferTrail()
	store := NewInMemoryClaimStore()
	e, _ := NewElection(ElectionConfig{Self: RoleOverseer, Store: store, Trail: trail})
	if _, err := NewFailover(FailoverConfig{Self: "bad", Trail: trail, Election: e}); err == nil {
		t.Error("want error on invalid self")
	}
	if _, err := NewFailover(FailoverConfig{Self: RoleOverseer, Trail: nil, Election: e}); err == nil {
		t.Error("want error on nil trail")
	}
	if _, err := NewFailover(FailoverConfig{Self: RoleOverseer, Trail: trail, Election: nil}); err == nil {
		t.Error("want error on nil election")
	}
}
