package supervisor

import (
	"context"
	"testing"
	"time"
)

func TestDefaultBurndownPolicy(t *testing.T) {
	p := defaultBurndownPolicy(BurndownPolicy{})
	if p.Target != 3 {
		t.Errorf("Target: want 3, got %d", p.Target)
	}
	if p.StaleAfter != 20*time.Minute {
		t.Errorf("StaleAfter: want 20m, got %v", p.StaleAfter)
	}
	if len(p.Models) == 0 {
		t.Error("Models: want non-empty default")
	}
}

func TestInMemoryBurndownController_Live(t *testing.T) {
	ctrl := &InMemoryBurndownController{}
	ctrl.AddSession(BurndownSession{SessionID: "s1", LastHeartbeat: time.Now()})
	ctrl.AddSession(BurndownSession{SessionID: "s2", LastHeartbeat: time.Now()})
	live, err := ctrl.Live(context.Background())
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	if len(live) != 2 {
		t.Errorf("want 2 live sessions, got %d", len(live))
	}
}

func TestInMemoryBurndownController_Reconcile_StaleBeads(t *testing.T) {
	ctrl := &InMemoryBurndownController{}
	// One live session, one dead session.
	ctrl.AddSession(BurndownSession{SessionID: "alive"})
	ctrl.ClaimBead("bead-A", "alive", time.Now().Add(-30*time.Minute))  // alive session — not stale
	ctrl.ClaimBead("bead-B", "dead-1", time.Now().Add(-30*time.Minute)) // dead session — stale
	ctrl.ClaimBead("bead-C", "dead-2", time.Now().Add(-5*time.Minute))  // too recent — not stale yet

	stale, err := ctrl.Reconcile(context.Background(), 20*time.Minute)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("want 1 stale bead, got %d: %+v", len(stale), stale)
	}
	if stale[0].BeadID != "bead-B" {
		t.Errorf("want stale bead bead-B, got %s", stale[0].BeadID)
	}
	if stale[0].Reason != "no-live-session" {
		t.Errorf("want reason no-live-session, got %s", stale[0].Reason)
	}
}

func TestInMemoryBurndownController_Reclaim(t *testing.T) {
	ctrl := &InMemoryBurndownController{}
	ctrl.ClaimBead("bead-X", "dead", time.Now().Add(-1*time.Hour))
	if err := ctrl.Reclaim(context.Background(), "bead-X"); err != nil {
		t.Fatalf("Reclaim: %v", err)
	}
	stale, _ := ctrl.Reconcile(context.Background(), time.Millisecond)
	for _, s := range stale {
		if s.BeadID == "bead-X" {
			t.Error("reclaimed bead should not appear in Reconcile")
		}
	}
}

func TestInMemoryBurndownController_Spawn_Valid(t *testing.T) {
	ctrl := &InMemoryBurndownController{}
	sid, err := ctrl.Spawn(context.Background(), WorkerSpec{
		Workspace: "~/worktrees/dear-agent-burndown-1",
		Model:     "claude-opus-4-8",
		Priority:  "P1",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if sid == "" {
		t.Error("Spawn returned empty session id")
	}
	live, _ := ctrl.Live(context.Background())
	if len(live) != 1 {
		t.Errorf("want 1 live session after Spawn, got %d", len(live))
	}
}

func TestInMemoryBurndownController_Spawn_RejectsSrcWorkspace(t *testing.T) {
	ctrl := &InMemoryBurndownController{}
	_, err := ctrl.Spawn(context.Background(), WorkerSpec{
		Workspace: "~/src/dear-agent",
		Model:     "claude-opus-4-8",
	})
	if err == nil {
		t.Error("Spawn with ~/src workspace should fail")
	}
}

func TestOverseer_WithBurndown_TickReclaims(t *testing.T) {
	trail, buf := newBufferTrail()
	probe := NewInMemoryResourceProbe()
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}
	ctrl := &InMemoryBurndownController{}
	// One dead session that claimed a bead 30 minutes ago.
	ctrl.ClaimBead("bead-Z", "session-dead", time.Now().Add(-30*time.Minute))
	o.WithBurndown(ctrl, BurndownPolicy{Target: 1, StaleAfter: 20 * time.Minute})

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	// Expect a reclaim record in the trail.
	var found bool
	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "burndown.reclaim" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected burndown.reclaim in trail; got records=%s", buf.String())
	}
}

func TestOverseer_WithBurndown_SpawnBlockedWhenSaturated(t *testing.T) {
	trail, buf := newBufferTrail()
	// Disk fully saturated.
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{DiskUsedFraction: 0.95})
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}
	ctrl := &InMemoryBurndownController{} // 0 live sessions → deficit = Target
	o.WithBurndown(ctrl, BurndownPolicy{Target: 3, StaleAfter: 20 * time.Minute})

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	records := parseTrail(t, buf)
	for _, r := range records {
		if r["kind"] == "burndown.spawn" {
			t.Errorf("spawn should be blocked when disk saturated; got record %+v", r)
		}
	}
	var blocked bool
	for _, r := range records {
		if r["kind"] == "burndown.spawn_blocked" {
			blocked = true
		}
	}
	if !blocked {
		t.Errorf("expected burndown.spawn_blocked record when resources saturated; got=%s", buf.String())
	}
}

func TestOverseer_WithBurndown_SpawnsToTarget(t *testing.T) {
	trail, buf := newBufferTrail()
	probe := NewInMemoryResourceProbe()
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}
	ctrl := &InMemoryBurndownController{} // 0 live sessions
	o.WithBurndown(ctrl, BurndownPolicy{Target: 2, StaleAfter: 20 * time.Minute, Models: []string{"claude-opus-4-8"}})

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	var spawnCount int
	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "burndown.spawn" {
			spawnCount++
		}
	}
	if spawnCount != 2 {
		t.Errorf("want 2 burndown.spawn events (target=2), got %d; trail=%s", spawnCount, buf.String())
	}
}

func TestOverseer_WithBurndown_Disabled(t *testing.T) {
	trail, buf := newBufferTrail()
	probe := NewInMemoryResourceProbe()
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}
	// No WithBurndown call — burndown phase must not run.
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	for _, r := range parseTrail(t, buf) {
		if k, _ := r["kind"].(string); len(k) >= 8 && k[:8] == "burndown" {
			t.Errorf("unexpected burndown trail record when controller not wired: %+v", r)
		}
	}
}

func TestResourcesSaturated(t *testing.T) {
	thresh := EscalationThreshold{Fraction: 0.9}
	cases := []struct {
		name    string
		snap    ResourceSnapshot
		wantSat bool
	}{
		{"below all", ResourceSnapshot{DiskUsedFraction: 0.5, MemoryUsedFraction: 0.5, CPUUsedFraction: 0.5}, false},
		{"disk saturated", ResourceSnapshot{DiskUsedFraction: 0.95}, true},
		{"memory saturated", ResourceSnapshot{MemoryUsedFraction: 0.9}, true},
		{"cpu saturated", ResourceSnapshot{CPUUsedFraction: 1.0}, true},
		{"at threshold", ResourceSnapshot{DiskUsedFraction: 0.9}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resourcesSaturated(tc.snap, thresh)
			if got != tc.wantSat {
				t.Errorf("resourcesSaturated=%v, want %v", got, tc.wantSat)
			}
		})
	}
}
