package supervisor

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPressureLevel_String(t *testing.T) {
	cases := map[PressureLevel]string{
		PressureNone:      "none",
		PressureWarn:      "warn",
		PressureCritical:  "critical",
		PressureEmergency: "emergency",
		PressureLevel(99): "PressureLevel(99)",
	}
	for level, want := range cases {
		if got := level.String(); got != want {
			t.Errorf("PressureLevel(%d).String() = %q, want %q", int(level), got, want)
		}
	}
}

func TestMemoryPressureMonitor_Classify(t *testing.T) {
	mon := NewMemoryPressureMonitor(MemoryPressureThresholds{}) // defaults

	tests := []struct {
		name string
		snap ResourceSnapshot
		want PressureLevel
	}{
		{"calm", ResourceSnapshot{SwapUsedFraction: 0.40}, PressureNone},
		{"swap warn", ResourceSnapshot{SwapUsedFraction: 0.78}, PressureWarn},
		{"swap critical", ResourceSnapshot{SwapUsedFraction: 0.88}, PressureCritical},
		{"swap emergency", ResourceSnapshot{SwapUsedFraction: 0.95}, PressureEmergency},
		{"swap exactly warn boundary", ResourceSnapshot{SwapUsedFraction: 0.75}, PressureWarn},
		{"swap exactly critical boundary", ResourceSnapshot{SwapUsedFraction: 0.85}, PressureCritical},
		{"swap exactly emergency boundary", ResourceSnapshot{SwapUsedFraction: 0.92}, PressureEmergency},
		{
			"free memory floor → emergency despite calm swap",
			ResourceSnapshot{SwapUsedFraction: 0.10, FreePhysicalMemoryBytes: 100 * MiB},
			PressureEmergency,
		},
		{
			"free memory zero is unknown, not emergency",
			ResourceSnapshot{SwapUsedFraction: 0.10, FreePhysicalMemoryBytes: 0},
			PressureNone,
		},
		{
			"free memory above floor is fine",
			ResourceSnapshot{SwapUsedFraction: 0.10, FreePhysicalMemoryBytes: 512 * MiB},
			PressureNone,
		},
		{
			"FD exhaustion forces at least critical",
			ResourceSnapshot{SwapUsedFraction: 0.10, OpenFDFraction: 0.95},
			PressureCritical,
		},
		{
			"max of swap-warn and FD-critical wins",
			ResourceSnapshot{SwapUsedFraction: 0.78, OpenFDFraction: 0.92},
			PressureCritical,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mon.Classify(tt.snap); got != tt.want {
				t.Errorf("Classify(%+v) = %v, want %v", tt.snap, got, tt.want)
			}
		})
	}
}

func TestMemoryPressureThresholds_Defaults(t *testing.T) {
	mon := NewMemoryPressureMonitor(MemoryPressureThresholds{})
	got := mon.Thresholds()
	want := DefaultMemoryPressureThresholds
	if got != want {
		t.Errorf("defaulted thresholds = %+v, want %+v", got, want)
	}

	// Partial override keeps the rest defaulted.
	mon2 := NewMemoryPressureMonitor(MemoryPressureThresholds{SwapWarn: 0.60})
	if mon2.Thresholds().SwapWarn != 0.60 {
		t.Errorf("SwapWarn override = %v, want 0.60", mon2.Thresholds().SwapWarn)
	}
	if mon2.Thresholds().SwapEmergency != DefaultMemoryPressureThresholds.SwapEmergency {
		t.Errorf("SwapEmergency = %v, want default", mon2.Thresholds().SwapEmergency)
	}
}

func TestNewAutoResourceReaper_RequiresTrail(t *testing.T) {
	if _, err := NewAutoResourceReaper(nil); err == nil {
		t.Error("nil trail accepted")
	}
}

func TestAutoResourceReaper_React_NoneIsNoop(t *testing.T) {
	trail, buf := newBufferTrail()
	reaper, err := NewAutoResourceReaper(trail)
	if err != nil {
		t.Fatalf("NewAutoResourceReaper: %v", err)
	}
	out := reaper.React(context.Background(), PressureNone, ResourceSnapshot{})
	if out.Level != PressureNone {
		t.Errorf("level = %v, want none", out.Level)
	}
	if len(parseTrail(t, buf)) != 0 {
		t.Error("PressureNone should record nothing")
	}
}

func TestAutoResourceReaper_React_WarnLogsOnly(t *testing.T) {
	trail, buf := newBufferTrail()
	reclaimer := NewInMemoryReclaimer()
	archiver := NewInMemorySessionArchiver()
	gate := NewInMemorySpawnGate()
	reaper, _ := NewAutoResourceReaper(trail,
		WithReapReclaimer(reclaimer),
		WithSessionArchiver(archiver),
		WithSpawnGate(gate),
	)

	out := reaper.React(context.Background(), PressureWarn, ResourceSnapshot{SwapUsedFraction: 0.78})

	if reclaimer.Calls() != 0 {
		t.Error("warn must not reclaim")
	}
	if archiver.Calls() != 0 {
		t.Error("warn must not archive")
	}
	if out.SpawnsPaused {
		t.Error("warn must not pause spawns")
	}
	// But it must record the observation.
	recs := recordsOfKind(t, buf, "supervisor.over.memory_pressure")
	if len(recs) != 1 {
		t.Fatalf("want 1 memory_pressure record, got %d", len(recs))
	}
	if recs[0]["payload"].(map[string]any)["level"] != "warn" {
		t.Errorf("recorded level = %v, want warn", recs[0]["payload"].(map[string]any)["level"])
	}
}

func TestAutoResourceReaper_React_CriticalReapsAndArchives(t *testing.T) {
	trail, buf := newBufferTrail()
	reclaimer := NewInMemoryReclaimer()
	reclaimer.SetResult(ReclaimResult{OrphansFound: 3, OrphansKilled: 3}, nil)
	archiver := NewInMemorySessionArchiver()
	archiver.SetResult(ArchiveResult{Found: 2, Archived: 2}, nil)
	gate := NewInMemorySpawnGate()
	esc := NewInMemoryPressureEscalator()
	reaper, _ := NewAutoResourceReaper(trail,
		WithReapReclaimer(reclaimer),
		WithSessionArchiver(archiver),
		WithSpawnGate(gate),
		WithPressureEscalator(esc),
	)

	out := reaper.React(context.Background(), PressureCritical, ResourceSnapshot{SwapUsedFraction: 0.88})

	if reclaimer.Calls() != 1 {
		t.Error("critical must reclaim")
	}
	if archiver.Calls() != 1 {
		t.Error("critical must archive")
	}
	if gate.PauseCalls() != 0 {
		t.Error("critical must NOT pause spawns (that's emergency only)")
	}
	if len(esc.Calls()) != 0 {
		t.Error("critical must NOT escalate (that's emergency only)")
	}
	if out.Reclaimed == nil || out.Reclaimed.OrphansKilled != 3 {
		t.Errorf("outcome.Reclaimed = %+v, want 3 killed", out.Reclaimed)
	}
	if out.Archived == nil || out.Archived.Archived != 2 {
		t.Errorf("outcome.Archived = %+v, want 2 archived", out.Archived)
	}
	if len(recordsOfKind(t, buf, "supervisor.over.memory_pressure")) != 1 {
		t.Error("want one memory_pressure record")
	}
}

func TestAutoResourceReaper_React_EmergencySubsumesCritical(t *testing.T) {
	trail, _ := newBufferTrail()
	reclaimer := NewInMemoryReclaimer()
	reclaimer.SetResult(ReclaimResult{OrphansKilled: 1}, nil)
	archiver := NewInMemorySessionArchiver()
	gate := NewInMemorySpawnGate()
	esc := NewInMemoryPressureEscalator()
	reaper, _ := NewAutoResourceReaper(trail,
		WithReapReclaimer(reclaimer),
		WithSessionArchiver(archiver),
		WithSpawnGate(gate),
		WithPressureEscalator(esc),
	)

	snap := ResourceSnapshot{SwapUsedFraction: 0.95, FreePhysicalMemoryBytes: 80 * MiB}
	out := reaper.React(context.Background(), PressureEmergency, snap)

	if reclaimer.Calls() != 1 || archiver.Calls() != 1 {
		t.Error("emergency must also run the critical actions")
	}
	if paused, reason := gate.Paused(); !paused || !strings.Contains(reason, "emergency") {
		t.Errorf("emergency must pause spawns, got paused=%v reason=%q", paused, reason)
	}
	if !out.SpawnsPaused {
		t.Error("outcome.SpawnsPaused must be true")
	}
	calls := esc.Calls()
	if len(calls) != 1 || calls[0].Level != PressureEmergency {
		t.Errorf("emergency must escalate once at emergency level, got %+v", calls)
	}
	if !out.Escalated {
		t.Error("outcome.Escalated must be true")
	}
}

func TestAutoResourceReaper_React_SeamErrorsAreBestEffort(t *testing.T) {
	trail, buf := newBufferTrail()
	reclaimer := NewInMemoryReclaimer()
	reclaimer.SetResult(ReclaimResult{}, errors.New("boom"))
	gate := NewInMemorySpawnGate()
	gate.SetError(errors.New("gate down"))
	esc := NewInMemoryPressureEscalator()
	reaper, _ := NewAutoResourceReaper(trail,
		WithReapReclaimer(reclaimer),
		WithSpawnGate(gate),
		WithPressureEscalator(esc),
	)

	out := reaper.React(context.Background(), PressureEmergency, ResourceSnapshot{SwapUsedFraction: 0.95})

	// A reclaim error must not stop the escalation from running.
	if len(esc.Calls()) != 1 {
		t.Error("escalation must run even after earlier seam errors")
	}
	if out.SpawnsPaused {
		t.Error("a failing gate must leave SpawnsPaused false")
	}
	if len(out.Errors) < 2 {
		t.Errorf("want at least 2 recorded errors, got %v", out.Errors)
	}
	rec := recordsOfKind(t, buf, "supervisor.over.memory_pressure")
	if len(rec) != 1 {
		t.Fatalf("want one record, got %d", len(rec))
	}
	if _, ok := rec[0]["payload"].(map[string]any)["errors"]; !ok {
		t.Error("record payload must include errors")
	}
}

func TestAutoResourceReaper_React_NilSeamsDegradeGracefully(t *testing.T) {
	trail, _ := newBufferTrail()
	reaper, _ := NewAutoResourceReaper(trail) // no seams wired
	out := reaper.React(context.Background(), PressureEmergency, ResourceSnapshot{SwapUsedFraction: 0.99})
	if out.Reclaimed != nil || out.Archived != nil || out.SpawnsPaused || out.Escalated {
		t.Errorf("nil seams must produce a no-action outcome, got %+v", out)
	}
}

func TestResourceIncidentRetro_DEARFormat(t *testing.T) {
	inc := ResourceIncident{
		Level:  PressureCritical,
		Before: ResourceSnapshot{SwapUsedFraction: 0.88, OpenFDFraction: 0.91, GoplsProcesses: 7, FreePhysicalMemoryBytes: 300 * MiB},
		After:  ResourceSnapshot{SwapUsedFraction: 0.70, OpenFDFraction: 0.60, GoplsProcesses: 1, FreePhysicalMemoryBytes: 900 * MiB},
		Outcome: ReapOutcome{
			Level:     PressureCritical,
			Reclaimed: &ReclaimResult{OrphansKilled: 6},
			Archived:  &ArchiveResult{Archived: 2},
		},
	}
	rec := ResourceIncidentRetro(inc)

	if rec.Kind != "supervisor.over.dear_retro" {
		t.Errorf("kind = %q, want supervisor.over.dear_retro", rec.Kind)
	}
	if rec.Role != string(RoleOverseer) {
		t.Errorf("role = %q, want overseer", rec.Role)
	}
	for _, key := range []string{"define", "execute", "audit", "retro"} {
		v, ok := rec.Payload[key].(string)
		if !ok || v == "" {
			t.Errorf("DEAR payload missing non-empty %q: %v", key, rec.Payload[key])
		}
	}
	// Pressure eased → resolved true.
	if rec.Payload["resolved"] != true {
		t.Errorf("resolved = %v, want true (pressure eased)", rec.Payload["resolved"])
	}
	if !strings.Contains(rec.Payload["execute"].(string), "reaped 6 orphan") {
		t.Errorf("execute phase missing reap detail: %q", rec.Payload["execute"])
	}
	if !strings.Contains(rec.Payload["audit"].(string), "eased") {
		t.Errorf("audit phase should say pressure eased: %q", rec.Payload["audit"])
	}
}

func TestResourceIncidentRetro_UnresolvedEmergency(t *testing.T) {
	inc := ResourceIncident{
		Level:   PressureEmergency,
		Before:  ResourceSnapshot{SwapUsedFraction: 0.95, GoplsProcesses: 2},
		After:   ResourceSnapshot{SwapUsedFraction: 0.95, GoplsProcesses: 2}, // unchanged
		Outcome: ReapOutcome{Level: PressureEmergency, SpawnsPaused: true, Escalated: true},
	}
	rec := ResourceIncidentRetro(inc)
	if rec.Payload["resolved"] != false {
		t.Errorf("resolved = %v, want false (pressure unchanged)", rec.Payload["resolved"])
	}
	if !strings.Contains(rec.Payload["retro"].(string), "did not relieve") {
		t.Errorf("retro should recommend escalation: %q", rec.Payload["retro"])
	}
}

func TestOverseer_Tick_MemoryPressure_EmergencyDrivesReaperAndRetro(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{SwapUsedFraction: 0.95, FreePhysicalMemoryBytes: 80 * MiB})

	trail, buf := newBufferTrail()
	o, err := NewOverseer(trail, probe, EscalationThreshold{})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}

	reclaimer := NewInMemoryReclaimer()
	reclaimer.SetResult(ReclaimResult{OrphansKilled: 2}, nil)
	// Simulate pressure dropping after the reap so the retro reads "eased".
	reclaimer.OnReclaim(func() {
		probe.Set(ResourceSnapshot{SwapUsedFraction: 0.50, FreePhysicalMemoryBytes: 2048 * MiB})
	})
	gate := NewInMemorySpawnGate()
	esc := NewInMemoryPressureEscalator()
	reaper, _ := NewAutoResourceReaper(trail,
		WithReapReclaimer(reclaimer),
		WithSessionArchiver(NewInMemorySessionArchiver()),
		WithSpawnGate(gate),
		WithPressureEscalator(esc),
	)
	o.WithMemoryPressure(NewMemoryPressureMonitor(MemoryPressureThresholds{}), reaper)

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if paused, _ := gate.Paused(); !paused {
		t.Error("emergency tick must pause spawns")
	}
	if len(esc.Calls()) != 1 {
		t.Error("emergency tick must escalate to Meta-O")
	}
	if len(recordsOfKind(t, buf, "supervisor.over.memory_pressure")) != 1 {
		t.Error("want a memory_pressure record")
	}
	retros := recordsOfKind(t, buf, "supervisor.over.dear_retro")
	if len(retros) != 1 {
		t.Fatalf("want one DEAR retro, got %d", len(retros))
	}
	if retros[0]["payload"].(map[string]any)["resolved"] != true {
		t.Error("retro should report pressure eased after reap")
	}
}

func TestOverseer_Tick_MemoryPressure_WarnEmitsNoRetro(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{SwapUsedFraction: 0.78})
	trail, buf := newBufferTrail()
	o, _ := NewOverseer(trail, probe, EscalationThreshold{})
	reaper, _ := NewAutoResourceReaper(trail)
	o.WithMemoryPressure(NewMemoryPressureMonitor(MemoryPressureThresholds{}), reaper)

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(recordsOfKind(t, buf, "supervisor.over.memory_pressure")) != 1 {
		t.Error("warn should record the observation")
	}
	if len(recordsOfKind(t, buf, "supervisor.over.dear_retro")) != 0 {
		t.Error("warn must NOT emit a DEAR retro")
	}
}

func TestOverseer_Tick_MemoryPressure_DisabledByDefault(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{SwapUsedFraction: 0.99, FreePhysicalMemoryBytes: 10 * MiB})
	trail, buf := newBufferTrail()
	o, _ := NewOverseer(trail, probe, EscalationThreshold{})
	// No WithMemoryPressure call.
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(recordsOfKind(t, buf, "supervisor.over.memory_pressure")) != 0 {
		t.Error("graduated handling must be off until WithMemoryPressure is called")
	}
}

// recordsOfKind returns the parsed trail records whose "kind" matches.
func recordsOfKind(t *testing.T, buf *bytes.Buffer, kind string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, r := range parseTrail(t, buf) {
		if r["kind"] == kind {
			out = append(out, r)
		}
	}
	return out
}
