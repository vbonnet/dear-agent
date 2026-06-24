package supervisor

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMemoryAlertThresholds_Classify(t *testing.T) {
	tests := []struct {
		name string
		snap ResourceSnapshot
		want PressureLevel
	}{
		{"healthy", ResourceSnapshot{FreePhysicalMemoryBytes: 4096 * MiB, SwapUsedFraction: 0.10}, PressureNone},
		{"free unknown stays none", ResourceSnapshot{FreePhysicalMemoryBytes: 0, SwapUsedFraction: 0.10}, PressureNone},

		// Free-RAM floor.
		{"free warn", ResourceSnapshot{FreePhysicalMemoryBytes: 400 * MiB}, PressureWarn},
		{"free critical", ResourceSnapshot{FreePhysicalMemoryBytes: 150 * MiB}, PressureCritical},
		{"free exactly warn boundary stays none", ResourceSnapshot{FreePhysicalMemoryBytes: 500 * MiB}, PressureNone},
		{"free just under warn", ResourceSnapshot{FreePhysicalMemoryBytes: 499 * MiB}, PressureWarn},
		{"free exactly critical boundary is warn", ResourceSnapshot{FreePhysicalMemoryBytes: 200 * MiB}, PressureWarn},
		{"free just under critical", ResourceSnapshot{FreePhysicalMemoryBytes: 199 * MiB}, PressureCritical},

		// Swap.
		{"swap warn", ResourceSnapshot{SwapUsedFraction: 0.60}, PressureWarn},
		{"swap critical", ResourceSnapshot{SwapUsedFraction: 0.80}, PressureCritical},
		{"swap exactly warn boundary stays none", ResourceSnapshot{SwapUsedFraction: 0.50}, PressureNone},
		{"swap exactly critical boundary is warn", ResourceSnapshot{SwapUsedFraction: 0.75}, PressureWarn},

		// Max of the two metrics wins.
		{"free warn + swap critical = critical", ResourceSnapshot{FreePhysicalMemoryBytes: 400 * MiB, SwapUsedFraction: 0.80}, PressureCritical},
	}

	var thr MemoryAlertThresholds // zero — defaults apply
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reasons := thr.classify(tt.snap)
			if got != tt.want {
				t.Errorf("classify(%+v) = %v, want %v", tt.snap, got, tt.want)
			}
			if tt.want == PressureNone && len(reasons) != 0 {
				t.Errorf("healthy snapshot produced reasons: %v", reasons)
			}
			if tt.want != PressureNone && len(reasons) == 0 {
				t.Error("breach produced no reasons")
			}
		})
	}
}

func TestMemoryAlertThresholds_CustomOverridesDefaults(t *testing.T) {
	thr := MemoryAlertThresholds{FreeWarnBytes: 1024 * MiB, SwapWarn: 0.30}
	// 800 MiB is healthy under defaults (>500) but trips the custom 1024 warn.
	if got, _ := thr.classify(ResourceSnapshot{FreePhysicalMemoryBytes: 800 * MiB}); got != PressureWarn {
		t.Errorf("custom FreeWarnBytes not honored: got %v", got)
	}
	// Critical floor still defaulted (200 MiB).
	if thr.withDefaults().FreeCriticalBytes != DefaultMemoryAlertThresholds.FreeCriticalBytes {
		t.Error("zero FreeCriticalBytes should default")
	}
}

// --- AGM notifier adapter ---

func TestAGMMemoryAlertNotifier_SendsMsg(t *testing.T) {
	var gotName string
	var gotArgs []string
	n := &AGMMemoryAlertNotifier{
		RunCommand: func(_ context.Context, name string, args ...string) ([]byte, error) {
			gotName = name
			gotArgs = args
			return []byte("ok"), nil
		},
	}
	err := n.Notify(context.Background(), RoleMetaOrchestrator, PressureCritical,
		ResourceSnapshot{FreePhysicalMemoryBytes: 150 * MiB}, "memory critical — free 150MiB")
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if gotName != "agm" {
		t.Errorf("binary = %q, want agm", gotName)
	}
	// Args: send msg meta-orchestrator --sender overseer --autonomous --prompt <...>
	if len(gotArgs) < 6 || gotArgs[0] != "send" || gotArgs[1] != "msg" || gotArgs[2] != "meta-orchestrator" {
		t.Errorf("unexpected args head: %v", gotArgs)
	}
	if gotArgs[3] != "--sender" || gotArgs[4] != string(RoleOverseer) {
		t.Errorf("sender not set to overseer: %v", gotArgs)
	}
	joined := strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "--autonomous") {
		t.Errorf("missing --autonomous: %v", gotArgs)
	}
	prompt := gotArgs[len(gotArgs)-1]
	if !strings.Contains(prompt, "MEMORY ALERT") || !strings.Contains(prompt, "memory critical") {
		t.Errorf("prompt missing alert summary: %q", prompt)
	}
}

func TestAGMMemoryAlertNotifier_PropagatesError(t *testing.T) {
	n := &AGMMemoryAlertNotifier{
		RunCommand: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("session not found"), errors.New("exit 1")
		},
	}
	err := n.Notify(context.Background(), RoleOrchestrator, PressureWarn, ResourceSnapshot{}, "x")
	if err == nil {
		t.Fatal("expected error from failing send")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("error should include command output: %v", err)
	}
}

func TestAGMMemoryAlertNotifier_EmptySessionIsError(t *testing.T) {
	n := &AGMMemoryAlertNotifier{SessionFor: func(Role) string { return "" }}
	if err := n.Notify(context.Background(), RoleMetaOrchestrator, PressureWarn, ResourceSnapshot{}, "x"); err == nil {
		t.Error("empty session name should error")
	}
}

// --- Overseer.Tick integration ---

func TestOverseer_Tick_MemoryAlert_DisabledByDefault(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{FreePhysicalMemoryBytes: 50 * MiB, SwapUsedFraction: 0.99})
	trail, buf := newBufferTrail()
	o, _ := NewOverseer(trail, probe, EscalationThreshold{})
	// No WithMemoryAlert call.
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(recordsOfKind(t, buf, "supervisor.over.memory_alert")) != 0 {
		t.Error("memory alert must be off until WithMemoryAlert is called")
	}
}

func TestOverseer_Tick_MemoryAlert_WarnRoutesToMetaOnly(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{FreePhysicalMemoryBytes: 400 * MiB}) // free warn only
	trail, buf := newBufferTrail()
	o, _ := NewOverseer(trail, probe, EscalationThreshold{})
	notifier := NewInMemoryMemoryAlertNotifier()
	o.WithMemoryAlert(notifier, MemoryAlertThresholds{})

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	calls := notifier.Calls()
	if len(calls) != 1 {
		t.Fatalf("WARN must notify exactly Meta-O, got %d calls", len(calls))
	}
	if calls[0].To != RoleMetaOrchestrator || calls[0].Level != PressureWarn {
		t.Errorf("WARN routed wrong: %+v", calls[0])
	}

	recs := recordsOfKind(t, buf, "supervisor.over.memory_alert")
	if len(recs) != 1 {
		t.Fatalf("want one memory_alert record, got %d", len(recs))
	}
	p := recs[0]["payload"].(map[string]any)
	if p["level"] != "warn" {
		t.Errorf("level = %v, want warn", p["level"])
	}
	targets := p["targets"].([]any)
	if len(targets) != 1 || targets[0] != string(RoleMetaOrchestrator) {
		t.Errorf("targets = %v, want [meta-orchestrator]", targets)
	}
}

func TestOverseer_Tick_MemoryAlert_CriticalRoutesToBoth(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{SwapUsedFraction: 0.90}) // swap critical
	trail, buf := newBufferTrail()
	o, _ := NewOverseer(trail, probe, EscalationThreshold{})
	notifier := NewInMemoryMemoryAlertNotifier()
	o.WithMemoryAlert(notifier, MemoryAlertThresholds{})

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	calls := notifier.Calls()
	if len(calls) != 2 {
		t.Fatalf("CRITICAL must notify Meta-O and Orchestrator, got %d calls", len(calls))
	}
	gotRoles := map[Role]bool{}
	for _, c := range calls {
		gotRoles[c.To] = true
		if c.Level != PressureCritical {
			t.Errorf("level = %v, want critical", c.Level)
		}
	}
	if !gotRoles[RoleMetaOrchestrator] || !gotRoles[RoleOrchestrator] {
		t.Errorf("CRITICAL did not reach both supervisors: %v", gotRoles)
	}

	recs := recordsOfKind(t, buf, "supervisor.over.memory_alert")
	if len(recs) != 1 {
		t.Fatalf("want one memory_alert record, got %d", len(recs))
	}
	p := recs[0]["payload"].(map[string]any)
	notified := p["notified"].([]any)
	if len(notified) != 2 {
		t.Errorf("notified = %v, want both supervisors", notified)
	}
}

func TestOverseer_Tick_MemoryAlert_HealthyStaysQuiet(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{FreePhysicalMemoryBytes: 8192 * MiB, SwapUsedFraction: 0.10})
	trail, buf := newBufferTrail()
	o, _ := NewOverseer(trail, probe, EscalationThreshold{})
	notifier := NewInMemoryMemoryAlertNotifier()
	o.WithMemoryAlert(notifier, MemoryAlertThresholds{})

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(notifier.Calls()) != 0 {
		t.Error("healthy snapshot must not notify")
	}
	if len(recordsOfKind(t, buf, "supervisor.over.memory_alert")) != 0 {
		t.Error("healthy snapshot must not record an alert")
	}
}

func TestOverseer_Tick_MemoryAlert_NotifierErrorDoesNotFailTick(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{SwapUsedFraction: 0.90}) // critical → two sends
	trail, buf := newBufferTrail()
	o, _ := NewOverseer(trail, probe, EscalationThreshold{})
	notifier := NewInMemoryMemoryAlertNotifier()
	notifier.SetError(errors.New("send failed"))
	o.WithMemoryAlert(notifier, MemoryAlertThresholds{})

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick must not propagate notifier errors: %v", err)
	}
	recs := recordsOfKind(t, buf, "supervisor.over.memory_alert")
	if len(recs) != 1 {
		t.Fatalf("want one memory_alert record, got %d", len(recs))
	}
	p := recs[0]["payload"].(map[string]any)
	if p["errors"] == nil {
		t.Error("notifier failures should be recorded in the trail")
	}
	if notified, ok := p["notified"].([]any); ok && len(notified) != 0 {
		t.Errorf("notified should be empty when all sends fail: %v", notified)
	}
}
