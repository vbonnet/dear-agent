package supervisor

import (
	"context"
	"errors"
	"testing"
)

func TestOverseer_Role(t *testing.T) {
	trail, _ := newBufferTrail()
	o, err := NewOverseer(trail, NewInMemoryResourceProbe(), EscalationThreshold{})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}
	if o.Role() != RoleOverseer {
		t.Errorf("Role = %q, want %q", o.Role(), RoleOverseer)
	}
}

func TestNewOverseer_RejectsNilDeps(t *testing.T) {
	trail, _ := newBufferTrail()
	if _, err := NewOverseer(nil, NewInMemoryResourceProbe(), EscalationThreshold{}); err == nil {
		t.Error("nil trail accepted")
	}
	if _, err := NewOverseer(trail, nil, EscalationThreshold{}); err == nil {
		t.Error("nil probe accepted")
	}
}

func TestNewOverseer_DefaultThresholdAppliesWhenZero(t *testing.T) {
	trail, _ := newBufferTrail()
	o, err := NewOverseer(trail, NewInMemoryResourceProbe(), EscalationThreshold{Fraction: 0})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}
	if o.threshold.Fraction != DefaultEscalationThreshold.Fraction {
		t.Errorf("threshold.Fraction = %v, want default %v", o.threshold.Fraction, DefaultEscalationThreshold.Fraction)
	}
}

func TestOverseer_Tick_NoEscalationBelowThreshold(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{
		DiskUsedFraction:   0.5,
		MemoryUsedFraction: 0.4,
		CPUUsedFraction:    0.2,
	})
	trail, buf := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "supervisor.over.escalated" {
			t.Errorf("escalation record present below threshold: %+v", r)
		}
	}
}

func TestOverseer_Tick_EscalatesEachOverThresholdMetric(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{
		DiskUsedFraction:   0.95, // over
		MemoryUsedFraction: 0.5,  // under
		CPUUsedFraction:    0.99, // over
		StrandedWorktrees:  3,    // > 0 → escalates
		OrphanedSessions:   0,    // not escalated
	})
	trail, buf := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	saw := map[string]bool{}
	for _, r := range parseTrail(t, buf) {
		if r["kind"] != "supervisor.over.escalated" {
			continue
		}
		p := r["payload"].(map[string]any)
		saw[p["metric"].(string)] = true
	}
	wantEscalated := []string{"disk", "cpu", "stranded_worktrees"}
	for _, m := range wantEscalated {
		if !saw[m] {
			t.Errorf("no escalation for metric %q", m)
		}
	}
	if saw["memory"] {
		t.Error("memory escalated unexpectedly")
	}
	if saw["orphaned_sessions"] {
		t.Error("orphaned_sessions escalated unexpectedly (count was 0)")
	}
}

func TestOverseer_Tick_AlwaysRecordsSnapshot(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{DiskUsedFraction: 0.1}) // nothing to escalate
	trail, buf := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	saw := false
	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "supervisor.over.resource_snapshot" {
			saw = true
		}
	}
	if !saw {
		t.Error("no resource_snapshot record — operators need a heartbeat-of-OK")
	}
}

func TestOverseer_Tick_ProbeErrorPropagates(t *testing.T) {
	probe := &errorProbe{err: errors.New("boom")}
	trail, _ := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}
	if err := o.Tick(context.Background()); err == nil {
		t.Error("Tick returned nil when probe errored")
	}
}

func TestOverseer_Tick_EscalatesGoplsWhenOverThreshold(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{
		GoplsProcesses: 10, // over default threshold of 5
	})
	trail, buf := newBufferTrail()
	// Use a zero-fraction threshold so only gopls fires.
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9, GoplsProcesses: 5})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	saw := false
	for _, r := range parseTrail(t, buf) {
		if r["kind"] != "supervisor.over.escalated" {
			continue
		}
		p := r["payload"].(map[string]any)
		if p["metric"] == "gopls_processes" {
			saw = true
		}
	}
	if !saw {
		t.Error("no escalation for gopls_processes above threshold")
	}
}

func TestOverseer_Tick_NoGoplsEscalationAtOrBelowThreshold(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{
		GoplsProcesses: 5, // exactly at threshold — should NOT escalate (threshold is exclusive)
	})
	trail, buf := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9, GoplsProcesses: 5})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	for _, r := range parseTrail(t, buf) {
		if r["kind"] != "supervisor.over.escalated" {
			continue
		}
		p := r["payload"].(map[string]any)
		if p["metric"] == "gopls_processes" {
			t.Error("gopls_processes escalated at threshold — want >threshold only")
		}
	}
}

func TestOverseer_Tick_EscalatesFDAndVnodePressure(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{
		OpenFDFraction:    0.95, // over 0.9 threshold
		VnodeUsedFraction: 1.0,  // at full saturation
	})
	trail, buf := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	saw := map[string]bool{}
	for _, r := range parseTrail(t, buf) {
		if r["kind"] != "supervisor.over.escalated" {
			continue
		}
		p := r["payload"].(map[string]any)
		saw[p["metric"].(string)] = true
	}
	if !saw["open_fds"] {
		t.Error("no escalation for open_fds above threshold")
	}
	if !saw["vnodes"] {
		t.Error("no escalation for vnodes at 100% saturation")
	}
}

// --- Reclaim tests ---

func TestOverseer_Tick_ReclaimsOnFDPressure(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{
		OpenFDFraction: 0.95,
		GoplsProcesses: 8,
	})
	trail, buf := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9, GoplsProcesses: 5})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}

	reclaimer := NewInMemoryReclaimer()
	reclaimer.SetResult(ReclaimResult{OrphansFound: 3, OrphansKilled: 3}, nil)
	reclaimer.OnReclaim(func() {
		probe.Set(ResourceSnapshot{OpenFDFraction: 0.4, GoplsProcesses: 2})
	})
	o.WithReclaimer(reclaimer)

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if reclaimer.Calls() != 1 {
		t.Errorf("Reclaim called %d times, want 1", reclaimer.Calls())
	}

	sawReclaim := false
	sawVerify := false
	for _, r := range parseTrail(t, buf) {
		switch r["kind"] {
		case "supervisor.over.reclaim":
			sawReclaim = true
			p := r["payload"].(map[string]any)
			if int(p["orphans_killed"].(float64)) != 3 {
				t.Errorf("orphans_killed = %v, want 3", p["orphans_killed"])
			}
		case "supervisor.over.reclaim_verify":
			sawVerify = true
			p := r["payload"].(map[string]any)
			if p["pressure_down"] != true {
				t.Error("pressure_down should be true after reclaim")
			}
		}
	}
	if !sawReclaim {
		t.Error("no reclaim trail record")
	}
	if !sawVerify {
		t.Error("no reclaim_verify trail record")
	}
}

func TestOverseer_Tick_NoReclaimBelowThreshold(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{
		OpenFDFraction: 0.5,
		GoplsProcesses: 3,
	})
	trail, _ := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9, GoplsProcesses: 5})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}

	reclaimer := NewInMemoryReclaimer()
	o.WithReclaimer(reclaimer)

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if reclaimer.Calls() != 0 {
		t.Errorf("Reclaim called %d times, want 0 (below threshold)", reclaimer.Calls())
	}
}

func TestOverseer_Tick_ReclaimOnGoplsOnly(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{
		OpenFDFraction: 0.5, // below threshold
		GoplsProcesses: 10,  // above threshold
	})
	trail, buf := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9, GoplsProcesses: 5})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}

	reclaimer := NewInMemoryReclaimer()
	reclaimer.SetResult(ReclaimResult{OrphansFound: 5, OrphansKilled: 5}, nil)
	reclaimer.OnReclaim(func() {
		probe.Set(ResourceSnapshot{GoplsProcesses: 3})
	})
	o.WithReclaimer(reclaimer)

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if reclaimer.Calls() != 1 {
		t.Errorf("Reclaim called %d times, want 1", reclaimer.Calls())
	}

	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "supervisor.over.reclaim_verify" {
			p := r["payload"].(map[string]any)
			if int(p["gopls_before"].(float64)) != 10 {
				t.Errorf("gopls_before = %v, want 10", p["gopls_before"])
			}
			if int(p["gopls_after"].(float64)) != 3 {
				t.Errorf("gopls_after = %v, want 3", p["gopls_after"])
			}
		}
	}
}

func TestOverseer_Tick_ReclaimOnVnodePressure(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{VnodeUsedFraction: 0.98})
	trail, _ := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}

	reclaimer := NewInMemoryReclaimer()
	reclaimer.SetResult(ReclaimResult{OrphansFound: 2, OrphansKilled: 2}, nil)
	reclaimer.OnReclaim(func() {
		probe.Set(ResourceSnapshot{VnodeUsedFraction: 0.6})
	})
	o.WithReclaimer(reclaimer)

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if reclaimer.Calls() != 1 {
		t.Errorf("Reclaim called %d times, want 1", reclaimer.Calls())
	}
}

func TestOverseer_Tick_NoReclaimWithoutReclaimer(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{OpenFDFraction: 0.99, GoplsProcesses: 20})
	trail, buf := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9, GoplsProcesses: 5})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}
	// No reclaimer wired — should escalate but not reclaim.
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "supervisor.over.reclaim" {
			t.Error("reclaim record present without a reclaimer wired")
		}
	}
}

func TestOverseer_Tick_ReclaimErrorRecorded(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{OpenFDFraction: 0.95})
	trail, buf := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{Fraction: 0.9})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}

	reclaimer := NewInMemoryReclaimer()
	reclaimer.SetResult(ReclaimResult{}, errors.New("ps failed"))
	o.WithReclaimer(reclaimer)

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick should not propagate reclaim errors: %v", err)
	}

	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "supervisor.over.reclaim" {
			p := r["payload"].(map[string]any)
			if p["error"] == nil {
				t.Error("reclaim error not recorded in trail")
			}
		}
	}
}

type errorProbe struct{ err error }

func (e *errorProbe) Snapshot(context.Context) (ResourceSnapshot, error) {
	return ResourceSnapshot{}, e.err
}
