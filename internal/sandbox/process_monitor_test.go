package sandbox

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultProcessLimits(t *testing.T) {
	limits := DefaultProcessLimits()
	if limits.MaxProcesses != 500 {
		t.Errorf("expected MaxProcesses=500, got %d", limits.MaxProcesses)
	}
	if limits.MaxProcessSpawnRate != 50 {
		t.Errorf("expected MaxProcessSpawnRate=50, got %d", limits.MaxProcessSpawnRate)
	}
	if limits.PollInterval != 2*time.Second {
		t.Errorf("expected PollInterval=2s, got %s", limits.PollInterval)
	}
}

func TestNewProcessMonitorDefaults(t *testing.T) {
	m := NewProcessMonitor(1, ProcessLimits{}, nil)
	if m.limits.MaxProcesses != 500 {
		t.Errorf("expected default MaxProcesses=500, got %d", m.limits.MaxProcesses)
	}
	if m.limits.MaxProcessSpawnRate != 50 {
		t.Errorf("expected default MaxProcessSpawnRate=50, got %d", m.limits.MaxProcessSpawnRate)
	}
	if m.limits.PollInterval != 2*time.Second {
		t.Errorf("expected default PollInterval=2s, got %v", m.limits.PollInterval)
	}
}

func TestCountDescendantsSelf(t *testing.T) {
	pid := os.Getpid()
	count, err := countDescendants(pid)
	if err != nil {
		t.Skipf("cannot read /proc on this platform: %v", err)
	}
	// Current process should have >= 0 descendants
	if count < 0 {
		t.Errorf("expected non-negative descendant count, got %d", count)
	}
}

func TestCountDescendantsInvalidPID(t *testing.T) {
	_, err := countDescendants(999999999)
	if err == nil {
		t.Error("expected error for invalid PID")
	}
}

func TestProcessMonitorStartStop(t *testing.T) {
	var alertCount atomic.Int32
	m := NewProcessMonitor(os.Getpid(), ProcessLimits{
		MaxProcesses:        10000,
		MaxProcessSpawnRate: 10000,
		PollInterval:        50 * time.Millisecond,
	}, func(at AlertType, msg string) {
		alertCount.Add(1)
	})

	ctx := context.Background()
	m.Start(ctx)
	time.Sleep(150 * time.Millisecond)
	m.Stop()

	// Normal process should not trigger alerts
	if alertCount.Load() > 0 {
		t.Errorf("unexpected alerts for normal process: %d", alertCount.Load())
	}
}

func TestProcessMonitorDoubleStart(t *testing.T) {
	m := NewProcessMonitor(os.Getpid(), ProcessLimits{PollInterval: 50 * time.Millisecond}, nil)
	ctx := context.Background()
	m.Start(ctx)
	m.Start(ctx) // should be no-op
	m.Stop()
}

func TestAlertTypeString(t *testing.T) {
	tests := []struct {
		at   AlertType
		want string
	}{
		{AlertForkBomb, "fork_bomb"},
		{AlertProcessLimit, "process_limit"},
		{AlertType(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.at.String(); got != tt.want {
			t.Errorf("AlertType(%d).String() = %q, want %q", tt.at, got, tt.want)
		}
	}
}
