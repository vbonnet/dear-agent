package sandbox

import (
	"context"
	"os"
	"sync"
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
	assertProcessMonitorStopped(t, m)
}

func TestProcessMonitorDoubleStart(t *testing.T) {
	m := NewProcessMonitor(os.Getpid(), ProcessLimits{PollInterval: 50 * time.Millisecond}, nil)
	ctx := context.Background()
	m.Start(ctx)
	m.Start(ctx) // should be no-op
	m.Stop()
	assertProcessMonitorStopped(t, m)
}

func TestProcessMonitorParentCancellationClearsRunningAndAllowsRestart(t *testing.T) {
	m := NewProcessMonitor(os.Getpid(), ProcessLimits{PollInterval: time.Hour}, nil)
	ctx, cancel := context.WithCancel(context.Background())

	m.Start(ctx)
	cancel()
	waitForProcessMonitorStopped(t, m)

	m.Start(context.Background())
	m.Stop()
	assertProcessMonitorStopped(t, m)
}

func TestProcessMonitorStartResetsProcessCountBaseline(t *testing.T) {
	m := NewProcessMonitor(os.Getpid(), ProcessLimits{PollInterval: time.Hour}, nil)
	m.mu.Lock()
	m.lastCount = 99
	m.mu.Unlock()

	m.Start(context.Background())
	m.mu.Lock()
	lastCount := m.lastCount
	m.mu.Unlock()
	m.Stop()

	if lastCount != 0 {
		t.Fatalf("lastCount after restart = %d, want fresh baseline", lastCount)
	}
}

func TestProcessMonitorAlertCallbackCanStopMonitor(t *testing.T) {
	callbackDone := make(chan struct{})
	var m *ProcessMonitor
	m = NewProcessMonitor(os.Getpid(), ProcessLimits{PollInterval: time.Hour}, func(AlertType, string) {
		m.Stop()
		close(callbackDone)
	})
	m.Start(context.Background())
	m.emitAlert(AlertProcessLimit, "test alert")

	select {
	case <-callbackDone:
	case <-time.After(time.Second):
		t.Fatal("alert callback deadlocked while stopping monitor")
	}
	waitForProcessMonitorStopped(t, m)
}

func TestProcessMonitorSerializesRepeatedAlertCallbacks(t *testing.T) {
	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	var calls atomic.Int32
	m := NewProcessMonitor(os.Getpid(), ProcessLimits{PollInterval: time.Hour}, func(AlertType, string) {
		if calls.Add(1) == 1 {
			close(callbackStarted)
		}
		<-releaseCallback
	})

	m.emitAlert(AlertProcessLimit, "first")
	select {
	case <-callbackStarted:
	case <-time.After(time.Second):
		t.Fatal("first alert callback did not start")
	}
	for range 100 {
		m.emitAlert(AlertProcessLimit, "duplicate")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("concurrent alert callbacks = %d, want 1", got)
	}

	close(releaseCallback)
	deadline := time.Now().Add(time.Second)
	for {
		m.mu.Lock()
		alerting := m.alerting
		m.mu.Unlock()
		if !alerting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("alert callback did not release its in-flight slot")
		}
		time.Sleep(time.Millisecond)
	}
	m.emitAlert(AlertProcessLimit, "next")
	deadline = time.Now().Add(time.Second)
	for calls.Load() != 2 {
		if time.Now().After(deadline) {
			t.Fatalf("alert callbacks = %d, want a later serialized callback", calls.Load())
		}
		time.Sleep(time.Millisecond)
	}
}

func TestProcessMonitorStaleAlertCannotStopRestartedRun(t *testing.T) {
	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	callbackDone := make(chan struct{})
	var m *ProcessMonitor
	m = NewProcessMonitor(os.Getpid(), ProcessLimits{PollInterval: time.Hour}, func(AlertType, string) {
		close(callbackStarted)
		<-releaseCallback
		m.Stop()
		close(callbackDone)
	})

	m.Start(context.Background())
	m.mu.Lock()
	oldDone := m.done
	m.mu.Unlock()
	m.emitAlert(AlertProcessLimit, "delayed")
	select {
	case <-callbackStarted:
	case <-time.After(time.Second):
		t.Fatal("alert callback did not start")
	}

	// Stop the loop from another goroutine while its callback remains active.
	m.Stop()
	m.Start(context.Background())
	m.mu.Lock()
	currentDone := m.done
	running := m.running
	m.mu.Unlock()
	if !running || currentDone != oldDone {
		t.Fatal("monitor restarted while an old alert callback was still active")
	}

	close(releaseCallback)
	select {
	case <-callbackDone:
	case <-time.After(time.Second):
		t.Fatal("delayed callback did not finish")
	}
	waitForProcessMonitorStopped(t, m)

	m.Start(context.Background())
	m.mu.Lock()
	restartedDone := m.done
	m.mu.Unlock()
	if restartedDone == oldDone {
		t.Fatal("monitor did not create a fresh lifecycle after callback drained")
	}
	m.Stop()
	assertProcessMonitorStopped(t, m)
}

func TestProcessMonitorConcurrentStartStop(t *testing.T) {
	m := NewProcessMonitor(os.Getpid(), ProcessLimits{PollInterval: time.Hour}, nil)

	for range 100 {
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(2)
			go func() {
				defer wg.Done()
				m.Start(ctx)
			}()
			go func() {
				defer wg.Done()
				m.Stop()
			}()
		}
		wg.Wait()
		cancel()
		m.Stop()
		waitForProcessMonitorStopped(t, m)
	}
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

func waitForProcessMonitorStopped(t *testing.T, m *ProcessMonitor) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for {
		if processMonitorStopped(m) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("process monitor did not stop")
		}
		time.Sleep(time.Millisecond)
	}
}

func assertProcessMonitorStopped(t *testing.T, m *ProcessMonitor) {
	t.Helper()

	if !processMonitorStopped(m) {
		t.Fatal("process monitor still has active lifecycle state")
	}
}

func processMonitorStopped(m *ProcessMonitor) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return !m.running && m.cancel == nil && m.done == nil
}
