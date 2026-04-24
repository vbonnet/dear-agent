package daemon

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/monitoring"
)

func newTestLoopMonitor(t *testing.T) (*LoopMonitor, string) {
	t.Helper()
	dir := t.TempDir()

	m := &LoopMonitor{
		heartbeatDir: dir,
		persistPath:  filepath.Join(dir, "state.json"),
		agmBinary:    "agm",
		logger:       slog.Default(),
		state: LoopMonitorState{
			WakeAttempts: make(map[string]*WakeAttemptState),
		},
		maxAttempts: 3,
		cooldown:    2 * time.Minute,
		runCommand: func(name string, args ...string) ([]byte, error) {
			return []byte("ok"), nil
		},
		nowFunc: time.Now,
	}

	return m, dir
}

func writeTestHeartbeat(t *testing.T, dir, session string, age time.Duration, intervalSecs int) {
	t.Helper()
	hb := monitoring.LoopHeartbeat{
		Timestamp:    time.Now().Add(-age),
		Session:      session,
		IntervalSecs: intervalSecs,
		CycleNumber:  1,
		OK:           true,
	}
	data, _ := json.Marshal(hb)
	path := filepath.Join(dir, "loop-"+session+".json")
	os.WriteFile(path, data, 0600)
}

func TestLoopMonitor_CheckAll_FreshHeartbeat(t *testing.T) {
	m, dir := newTestLoopMonitor(t)

	// Write a fresh heartbeat (10s old, 300s interval)
	writeTestHeartbeat(t, dir, "fresh-session", 10*time.Second, 300)

	wakeCalled := false
	m.runCommand = func(name string, args ...string) ([]byte, error) {
		wakeCalled = true
		return []byte("ok"), nil
	}

	m.CheckAll()

	if wakeCalled {
		t.Error("wake should not be called for fresh heartbeat")
	}
}

func TestLoopMonitor_CheckAll_StaleHeartbeat(t *testing.T) {
	m, dir := newTestLoopMonitor(t)

	// Write a stale heartbeat (10min old, 300s interval → threshold 360s)
	writeTestHeartbeat(t, dir, "stale-session", 10*time.Minute, 300)

	wakeTarget := ""
	m.runCommand = func(name string, args ...string) ([]byte, error) {
		if len(args) >= 3 && args[1] == "wake-loop" {
			wakeTarget = args[2]
		}
		return []byte("ok"), nil
	}

	m.CheckAll()

	if wakeTarget != "stale-session" {
		t.Errorf("expected wake for 'stale-session', got %q", wakeTarget)
	}
}

func TestLoopMonitor_CheckAll_CircuitBreaker(t *testing.T) {
	m, dir := newTestLoopMonitor(t)
	m.cooldown = 0 // Disable cooldown for test

	writeTestHeartbeat(t, dir, "stuck-session", 10*time.Minute, 300)

	wakeCount := 0
	m.runCommand = func(name string, args ...string) ([]byte, error) {
		wakeCount++
		return []byte("ok"), nil
	}

	// Run CheckAll 4 times — should only wake 3 times (circuit breaker)
	for i := 0; i < 4; i++ {
		m.CheckAll()
	}

	if wakeCount != 3 {
		t.Errorf("expected 3 wake attempts (circuit breaker), got %d", wakeCount)
	}
}

func TestLoopMonitor_CheckAll_Recovery(t *testing.T) {
	m, dir := newTestLoopMonitor(t)
	m.cooldown = 0

	// Start with stale heartbeat
	writeTestHeartbeat(t, dir, "recovering-session", 10*time.Minute, 300)

	wakeCount := 0
	m.runCommand = func(name string, args ...string) ([]byte, error) {
		wakeCount++
		return []byte("ok"), nil
	}

	m.CheckAll() // Should wake
	if wakeCount != 1 {
		t.Fatalf("expected 1 wake, got %d", wakeCount)
	}

	// Session recovers — write fresh heartbeat
	writeTestHeartbeat(t, dir, "recovering-session", 10*time.Second, 300)

	m.CheckAll() // Should NOT wake, and should reset attempts
	if wakeCount != 1 {
		t.Errorf("expected no additional wakes after recovery, got %d", wakeCount)
	}

	// Verify attempts were reset
	if ws, ok := m.state.WakeAttempts["recovering-session"]; ok && ws.Attempts > 0 {
		t.Error("expected wake attempts to be reset after recovery")
	}
}

func TestLoopMonitor_CheckAll_Cooldown(t *testing.T) {
	m, dir := newTestLoopMonitor(t)
	m.cooldown = 10 * time.Minute // Long cooldown

	writeTestHeartbeat(t, dir, "cooldown-session", 10*time.Minute, 300)

	wakeCount := 0
	m.runCommand = func(name string, args ...string) ([]byte, error) {
		wakeCount++
		return []byte("ok"), nil
	}

	// First check should wake
	m.CheckAll()
	if wakeCount != 1 {
		t.Fatalf("expected 1 wake, got %d", wakeCount)
	}

	// Second check should skip (cooldown)
	m.CheckAll()
	if wakeCount != 1 {
		t.Errorf("expected no additional wakes during cooldown, got %d", wakeCount)
	}
}

func TestLoopMonitor_StatePersistence(t *testing.T) {
	m, dir := newTestLoopMonitor(t)
	m.cooldown = 0

	writeTestHeartbeat(t, dir, "persist-session", 10*time.Minute, 300)

	m.runCommand = func(name string, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}

	m.CheckAll()

	// Verify state was persisted
	data, err := os.ReadFile(m.persistPath)
	if err != nil {
		t.Fatalf("failed to read persisted state: %v", err)
	}

	var state LoopMonitorState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("failed to unmarshal state: %v", err)
	}

	ws, ok := state.WakeAttempts["persist-session"]
	if !ok {
		t.Fatal("expected wake attempt state for 'persist-session'")
	}
	if ws.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", ws.Attempts)
	}
}
