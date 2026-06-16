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

type errorProbe struct{ err error }

func (e *errorProbe) Snapshot(context.Context) (ResourceSnapshot, error) {
	return ResourceSnapshot{}, e.err
}
