package supervisor

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDiskAlertThresholds_Classify(t *testing.T) {
	// measured wraps a free-bytes reading with a nonzero used fraction, the
	// marker that statfs succeeded (an unmeasured snapshot is all-zero).
	measured := func(free uint64) ResourceSnapshot {
		return ResourceSnapshot{DiskFreeBytes: free, DiskUsedFraction: 0.5}
	}

	tests := []struct {
		name string
		snap ResourceSnapshot
		want PressureLevel
	}{
		{"healthy", measured(500 * GiB), PressureNone},
		{"disk unmeasured stays none", ResourceSnapshot{}, PressureNone},

		// Free-space floor.
		{"free warn", measured(10 * GiB), PressureWarn},
		{"free critical", measured(2 * GiB), PressureCritical},
		{"free exactly warn boundary stays none", measured(20 * GiB), PressureNone},
		{"free just under warn", measured(20*GiB - 1), PressureWarn},
		{"free exactly critical boundary is warn", measured(5 * GiB), PressureWarn},
		{"free just under critical", measured(5*GiB - 1), PressureCritical},
		// The ce-6fel crash state: measured zero free must alarm, not read as
		// "unknown" — DiskUsedFraction > 0 proves the probe measured the disk.
		{"measured zero free is critical", ResourceSnapshot{DiskFreeBytes: 0, DiskUsedFraction: 1.0}, PressureCritical},

		// Inodes.
		{"inode warn", ResourceSnapshot{DiskFreeBytes: 500 * GiB, DiskUsedFraction: 0.5, InodeUsedFraction: 0.92}, PressureWarn},
		{"inode critical", ResourceSnapshot{DiskFreeBytes: 500 * GiB, DiskUsedFraction: 0.5, InodeUsedFraction: 0.97}, PressureCritical},
		{"inode exactly warn boundary stays none", ResourceSnapshot{DiskFreeBytes: 500 * GiB, DiskUsedFraction: 0.5, InodeUsedFraction: 0.90}, PressureNone},
		{"inode exactly critical boundary is warn", ResourceSnapshot{DiskFreeBytes: 500 * GiB, DiskUsedFraction: 0.5, InodeUsedFraction: 0.95}, PressureWarn},

		// Max of the two metrics wins.
		{"free warn + inode critical = critical", ResourceSnapshot{DiskFreeBytes: 10 * GiB, DiskUsedFraction: 0.9, InodeUsedFraction: 0.97}, PressureCritical},
	}

	var thr DiskAlertThresholds // zero — defaults apply
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reasons := thr.Classify(tt.snap)
			if got != tt.want {
				t.Errorf("Classify(%+v) = %v, want %v", tt.snap, got, tt.want)
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

func TestDiskAlertThresholds_CustomOverridesDefaults(t *testing.T) {
	thr := DiskAlertThresholds{FreeWarnBytes: 100 * GiB, InodeWarn: 0.50}
	// 50 GiB is healthy under defaults (>20) but trips the custom 100 warn.
	if got, _ := thr.Classify(ResourceSnapshot{DiskFreeBytes: 50 * GiB, DiskUsedFraction: 0.5}); got != PressureWarn {
		t.Errorf("custom FreeWarnBytes not honored: got %v", got)
	}
	// Critical floor still defaulted (5 GiB).
	if thr.withDefaults().FreeCriticalBytes != DefaultDiskAlertThresholds.FreeCriticalBytes {
		t.Error("zero FreeCriticalBytes should default")
	}
}

// --- AGM notifier adapter ---

func TestAGMDiskAlertNotifier_SendsMsg(t *testing.T) {
	var gotName string
	var gotArgs []string
	n := &AGMDiskAlertNotifier{
		RunCommand: func(_ context.Context, name string, args ...string) ([]byte, error) {
			gotName = name
			gotArgs = args
			return []byte("ok"), nil
		},
	}
	err := n.Notify(context.Background(), RoleMetaOrchestrator, PressureCritical,
		ResourceSnapshot{DiskFreeBytes: 2 * GiB, DiskUsedFraction: 0.99}, "disk critical — free 2.0GiB")
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
	if !strings.Contains(prompt, "DISK ALERT") || !strings.Contains(prompt, "disk critical") {
		t.Errorf("prompt missing alert summary: %q", prompt)
	}
}

func TestAGMDiskAlertNotifier_PropagatesError(t *testing.T) {
	n := &AGMDiskAlertNotifier{
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

func TestAGMDiskAlertNotifier_EmptySessionIsError(t *testing.T) {
	n := &AGMDiskAlertNotifier{SessionFor: func(Role) string { return "" }}
	if err := n.Notify(context.Background(), RoleMetaOrchestrator, PressureWarn, ResourceSnapshot{}, "x"); err == nil {
		t.Error("empty session name should error")
	}
}

// --- Overseer.Tick integration ---

func TestOverseer_Tick_DiskAlert_DisabledByDefault(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{DiskFreeBytes: 1 * GiB, DiskUsedFraction: 0.99, InodeUsedFraction: 0.99})
	trail, buf := newBufferTrail()
	o, _ := NewOverseer(trail, probe, EscalationThreshold{})
	// No WithDiskAlert call.
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(recordsOfKind(t, buf, "supervisor.over.disk_alert")) != 0 {
		t.Error("disk alert must be off until WithDiskAlert is called")
	}
}

func TestOverseer_Tick_DiskAlert_WarnRoutesToMetaOnly(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{DiskFreeBytes: 10 * GiB, DiskUsedFraction: 0.5}) // free warn only
	trail, buf := newBufferTrail()
	o, _ := NewOverseer(trail, probe, EscalationThreshold{})
	notifier := NewInMemoryDiskAlertNotifier()
	o.WithDiskAlert(notifier, DiskAlertThresholds{})

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

	recs := recordsOfKind(t, buf, "supervisor.over.disk_alert")
	if len(recs) != 1 {
		t.Fatalf("want one disk_alert record, got %d", len(recs))
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

func TestOverseer_Tick_DiskAlert_CriticalRoutesToBoth(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{DiskFreeBytes: 2 * GiB, DiskUsedFraction: 0.99}) // free critical
	trail, buf := newBufferTrail()
	o, _ := NewOverseer(trail, probe, EscalationThreshold{})
	notifier := NewInMemoryDiskAlertNotifier()
	o.WithDiskAlert(notifier, DiskAlertThresholds{})

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

	recs := recordsOfKind(t, buf, "supervisor.over.disk_alert")
	if len(recs) != 1 {
		t.Fatalf("want one disk_alert record, got %d", len(recs))
	}
	p := recs[0]["payload"].(map[string]any)
	notified := p["notified"].([]any)
	if len(notified) != 2 {
		t.Errorf("notified = %v, want both supervisors", notified)
	}
}

func TestOverseer_Tick_DiskAlert_HealthyStaysQuiet(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{DiskFreeBytes: 500 * GiB, DiskUsedFraction: 0.4, InodeUsedFraction: 0.10})
	trail, buf := newBufferTrail()
	o, _ := NewOverseer(trail, probe, EscalationThreshold{})
	notifier := NewInMemoryDiskAlertNotifier()
	o.WithDiskAlert(notifier, DiskAlertThresholds{})

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(notifier.Calls()) != 0 {
		t.Error("healthy snapshot must not notify")
	}
	if len(recordsOfKind(t, buf, "supervisor.over.disk_alert")) != 0 {
		t.Error("healthy snapshot must not record an alert")
	}
}

func TestOverseer_Tick_DiskAlert_NotifierErrorDoesNotFailTick(t *testing.T) {
	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{DiskFreeBytes: 2 * GiB, DiskUsedFraction: 0.99}) // critical → two sends
	trail, buf := newBufferTrail()
	o, _ := NewOverseer(trail, probe, EscalationThreshold{})
	notifier := NewInMemoryDiskAlertNotifier()
	notifier.SetError(errors.New("send failed"))
	o.WithDiskAlert(notifier, DiskAlertThresholds{})

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick must not propagate notifier errors: %v", err)
	}
	recs := recordsOfKind(t, buf, "supervisor.over.disk_alert")
	if len(recs) != 1 {
		t.Fatalf("want one disk_alert record, got %d", len(recs))
	}
	p := recs[0]["payload"].(map[string]any)
	if p["errors"] == nil {
		t.Error("notifier failures should be recorded in the trail")
	}
	if notified, ok := p["notified"].([]any); ok && len(notified) != 0 {
		t.Errorf("notified should be empty when all sends fail: %v", notified)
	}
}
