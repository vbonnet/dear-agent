package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionHeartbeatWriter_Beat(t *testing.T) {
	tmpDir := t.TempDir()

	writer, err := NewSessionHeartbeatWriter(tmpDir)
	require.NoError(t, err)

	err = writer.Beat("test-session", true)
	require.NoError(t, err)

	// Verify file exists and contains correct data.
	path := filepath.Join(tmpDir, "test-session.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var hb SessionHeartbeat
	require.NoError(t, json.Unmarshal(data, &hb))

	assert.Equal(t, "test-session", hb.SessionName)
	assert.True(t, hb.ScanOK)
	assert.WithinDuration(t, time.Now(), hb.Timestamp, 5*time.Second)
}

func TestSessionHeartbeatWriter_Beat_MultipleSessionsAndOverwrite(t *testing.T) {
	tmpDir := t.TempDir()

	writer, err := NewSessionHeartbeatWriter(tmpDir)
	require.NoError(t, err)

	// Write heartbeats for two sessions.
	require.NoError(t, writer.Beat("session-a", true))
	require.NoError(t, writer.Beat("session-b", false))

	// Verify both files exist.
	_, err = os.Stat(filepath.Join(tmpDir, "session-a.json"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(tmpDir, "session-b.json"))
	assert.NoError(t, err)

	// Overwrite session-b.
	require.NoError(t, writer.Beat("session-b", true))

	data, err := os.ReadFile(filepath.Join(tmpDir, "session-b.json"))
	require.NoError(t, err)
	var hb SessionHeartbeat
	require.NoError(t, json.Unmarshal(data, &hb))
	assert.True(t, hb.ScanOK, "overwritten heartbeat should have scanOK=true")
}

func TestSessionHeartbeatMonitor_StalenessDetection(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	// Write heartbeats at various ages.
	writeSessionHeartbeat(t, tmpDir, "fresh", now.Add(-2*time.Minute))
	writeSessionHeartbeat(t, tmpDir, "warn-level", now.Add(-15*time.Minute))
	writeSessionHeartbeat(t, tmpDir, "alert-level", now.Add(-45*time.Minute))

	monitor := newTestMonitor(t, tmpDir, now)

	results := monitor.CheckAll()
	require.Len(t, results, 3)

	// Build a map for easy lookup.
	byName := make(map[string]SessionStalenessResult)
	for _, r := range results {
		byName[r.SessionName] = r
	}

	assert.Equal(t, "ok", byName["fresh"].Level)
	assert.Equal(t, "warn", byName["warn-level"].Level)
	assert.Equal(t, "alert", byName["alert-level"].Level)
}

func TestSessionHeartbeatMonitor_AlertGeneration(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	// Write a 45-minute-old heartbeat.
	writeSessionHeartbeat(t, tmpDir, "stale-session", now.Add(-45*time.Minute))

	var capturedArgs []string
	commandCallCount := 0

	monitor := newTestMonitor(t, tmpDir, now)
	monitor.runCommand = func(name string, args ...string) ([]byte, error) {
		commandCallCount++
		capturedArgs = append([]string{name}, args...)
		return []byte("sent"), nil
	}

	// First check: should trigger alert.
	results := monitor.CheckAll()
	require.Len(t, results, 1)
	assert.Equal(t, "alert", results[0].Level)
	assert.Equal(t, 1, commandCallCount)

	// Verify command args.
	assert.Equal(t, "agm", capturedArgs[0])
	assert.Equal(t, "send", capturedArgs[1])
	assert.Equal(t, "msg", capturedArgs[2])
	assert.Equal(t, "orchestrator", capturedArgs[3])
	assert.Contains(t, capturedArgs, "--sender")
	assert.Contains(t, capturedArgs, "astrocyte")
	assert.Contains(t, capturedArgs, "--priority")
	assert.Contains(t, capturedArgs, "urgent")

	// Second check (same time): should NOT re-alert (per-session 1h cooldown).
	_ = monitor.CheckAll()
	assert.Equal(t, 1, commandCallCount, "should not re-alert within per-session cooldown")

	// Third check after 61 minutes: should re-alert (past 1h cooldown).
	monitor.nowFunc = func() time.Time { return now.Add(61 * time.Minute) }
	_ = monitor.CheckAll()
	assert.Equal(t, 2, commandCallCount, "should re-alert after per-session cooldown expires")
}

func TestSessionHeartbeatMonitor_ExemptSessions(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	// Write stale heartbeats for exempt and non-exempt sessions.
	writeSessionHeartbeat(t, tmpDir, "orchestrator", now.Add(-45*time.Minute))
	writeSessionHeartbeat(t, tmpDir, "human-session", now.Add(-45*time.Minute))
	writeSessionHeartbeat(t, tmpDir, "worker-1", now.Add(-45*time.Minute))

	commandCallCount := 0
	monitor := newTestMonitor(t, tmpDir, now)
	monitor.SetExemptSessions([]string{"orchestrator", "human"})
	monitor.runCommand = func(name string, args ...string) ([]byte, error) {
		commandCallCount++
		return []byte("sent"), nil
	}

	results := monitor.CheckAll()
	require.Len(t, results, 3)

	// Only worker-1 should have triggered an alert.
	assert.Equal(t, 1, commandCallCount, "exempt sessions should not trigger alerts")
}

func TestSessionHeartbeatMonitor_TmuxFilter(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	// Write stale heartbeats — only "active-session" exists in tmux.
	writeSessionHeartbeat(t, tmpDir, "active-session", now.Add(-45*time.Minute))
	writeSessionHeartbeat(t, tmpDir, "deleted-session", now.Add(-45*time.Minute))

	commandCallCount := 0
	monitor := newTestMonitor(t, tmpDir, now)
	monitor.listTmuxSessions = func() ([]string, error) {
		return []string{"active-session", "other-session"}, nil
	}
	monitor.runCommand = func(name string, args ...string) ([]byte, error) {
		commandCallCount++
		return []byte("sent"), nil
	}

	results := monitor.CheckAll()
	require.Len(t, results, 2)

	// Only active-session should trigger an alert; deleted-session is not in tmux.
	assert.Equal(t, 1, commandCallCount, "should not alert for sessions not in tmux")
}

func TestSessionHeartbeatMonitor_PerCycleLimit(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	// Write 8 stale heartbeats — max 5 alerts per cycle.
	for i := 0; i < 8; i++ {
		writeSessionHeartbeat(t, tmpDir, fmt.Sprintf("session-%d", i), now.Add(-45*time.Minute))
	}

	commandCallCount := 0
	monitor := newTestMonitor(t, tmpDir, now)
	monitor.runCommand = func(name string, args ...string) ([]byte, error) {
		commandCallCount++
		return []byte("sent"), nil
	}

	monitor.CheckAll()
	assert.Equal(t, 5, commandCallCount, "should cap alerts at maxAlertsPerCycle=5")
}

func TestSessionHeartbeatMonitor_PerSessionHourlyCooldown(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	writeSessionHeartbeat(t, tmpDir, "worker", now.Add(-45*time.Minute))

	commandCallCount := 0
	monitor := newTestMonitor(t, tmpDir, now)
	monitor.runCommand = func(name string, args ...string) ([]byte, error) {
		commandCallCount++
		return []byte("sent"), nil
	}

	// First call: alert sent.
	monitor.CheckAll()
	assert.Equal(t, 1, commandCallCount)

	// 30 minutes later: still within 1h cooldown.
	monitor.nowFunc = func() time.Time { return now.Add(30 * time.Minute) }
	monitor.CheckAll()
	assert.Equal(t, 1, commandCallCount, "should not re-alert within 1h cooldown")

	// 59 minutes later: still within cooldown.
	monitor.nowFunc = func() time.Time { return now.Add(59 * time.Minute) }
	monitor.CheckAll()
	assert.Equal(t, 1, commandCallCount, "should not re-alert at 59 minutes")

	// 61 minutes later: cooldown expired.
	monitor.nowFunc = func() time.Time { return now.Add(61 * time.Minute) }
	monitor.CheckAll()
	assert.Equal(t, 2, commandCallCount, "should re-alert after 1h cooldown")
}

func TestSessionHeartbeatMonitor_CircuitBreakerTrigger(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	// Create 12 stale sessions to exceed the CB threshold of 10.
	for i := 0; i < 12; i++ {
		writeSessionHeartbeat(t, tmpDir, fmt.Sprintf("s-%d", i), now.Add(-45*time.Minute))
	}

	commandCallCount := 0
	monitor := newTestMonitor(t, tmpDir, now)
	// Raise per-cycle limit so it doesn't interfere.
	monitor.maxAlertsPerCycle = 20
	monitor.runCommand = func(name string, args ...string) ([]byte, error) {
		commandCallCount++
		return []byte("sent"), nil
	}

	// First cycle: sends alerts until CB triggers at >10.
	monitor.CheckAll()
	firstCycleCount := commandCallCount

	// Circuit breaker should have been tripped.
	assert.Equal(t, 1, monitor.circuitBreakerTrips)
	assert.False(t, monitor.circuitBreakerUntil.IsZero(), "CB cooldown should be set")

	// Second cycle immediately after: should be blocked by CB.
	monitor.CheckAll()
	assert.Equal(t, firstCycleCount, commandCallCount, "no alerts during CB cooldown")

	// After 30 min cooldown: alerts should resume.
	monitor.nowFunc = func() time.Time { return now.Add(31 * time.Minute) }
	// Reset per-session cooldowns for this test by clearing alertedSessions.
	monitor.alertedSessions = make(map[string]time.Time)
	monitor.CheckAll()
	assert.Greater(t, commandCallCount, firstCycleCount, "alerts should resume after CB cooldown")
}

func TestSessionHeartbeatMonitor_CircuitBreakerDisable(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	// Create 12 sessions to trigger CB each time.
	for i := 0; i < 12; i++ {
		writeSessionHeartbeat(t, tmpDir, fmt.Sprintf("s-%d", i), now.Add(-45*time.Minute))
	}

	monitor := newTestMonitor(t, tmpDir, now)
	monitor.maxAlertsPerCycle = 20
	monitor.runCommand = func(name string, args ...string) ([]byte, error) {
		return []byte("sent"), nil
	}

	// Trip 1.
	monitor.CheckAll()
	assert.Equal(t, 1, monitor.circuitBreakerTrips)
	assert.False(t, monitor.disabled)

	// Trip 2: advance past cooldown, reset per-session state.
	now = now.Add(31 * time.Minute)
	monitor.nowFunc = func() time.Time { return now }
	monitor.alertedSessions = make(map[string]time.Time)
	monitor.alertTimestamps = nil
	monitor.CheckAll()
	assert.Equal(t, 2, monitor.circuitBreakerTrips)
	assert.False(t, monitor.disabled)

	// Trip 3: should disable permanently.
	now = now.Add(31 * time.Minute)
	monitor.nowFunc = func() time.Time { return now }
	monitor.alertedSessions = make(map[string]time.Time)
	monitor.alertTimestamps = nil
	monitor.CheckAll()
	assert.Equal(t, 3, monitor.circuitBreakerTrips)
	assert.True(t, monitor.disabled, "monitor should be disabled after 3 CB trips")

	// Subsequent calls should return nil immediately.
	results := monitor.CheckAll()
	assert.Nil(t, results, "disabled monitor should return nil")
}

func TestSessionHeartbeatMonitor_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	persistDir := t.TempDir()
	persistPath := filepath.Join(persistDir, "alerts.json")
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	writeSessionHeartbeat(t, tmpDir, "worker", now.Add(-45*time.Minute))

	// First monitor instance: sends alert and persists.
	m1 := newTestMonitor(t, tmpDir, now)
	m1.persistPath = persistPath
	alertCount := 0
	m1.runCommand = func(name string, args ...string) ([]byte, error) {
		alertCount++
		return []byte("sent"), nil
	}
	m1.CheckAll()
	assert.Equal(t, 1, alertCount)

	// Verify persist file exists.
	_, err := os.Stat(persistPath)
	require.NoError(t, err, "persist file should exist")

	// Second monitor instance: loads state, should not re-alert (within cooldown).
	m2 := newTestMonitor(t, tmpDir, now)
	m2.persistPath = persistPath
	m2.loadAlertState()
	m2.runCommand = func(name string, args ...string) ([]byte, error) {
		alertCount++
		return []byte("sent"), nil
	}
	m2.CheckAll()
	assert.Equal(t, 1, alertCount, "second instance should respect persisted cooldown")
}

func TestSessionHeartbeatWriter_DefaultDir(t *testing.T) {
	// Test that default dir is constructed without error.
	writer, err := NewSessionHeartbeatWriter("")
	require.NoError(t, err)
	assert.NotNil(t, writer)
}

func TestSessionHeartbeatMonitor_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	monitor, err := NewSessionHeartbeatMonitor(tmpDir, nil)
	require.NoError(t, err)
	monitor.nowFunc = func() time.Time { return time.Now() }

	results := monitor.CheckAll()
	assert.Empty(t, results)
}

// newTestMonitor creates a SessionHeartbeatMonitor suitable for testing.
// Suppresses command execution, tmux listing, and persistence by default.
func newTestMonitor(t *testing.T, dir string, now time.Time) *SessionHeartbeatMonitor {
	t.Helper()
	monitor, err := NewSessionHeartbeatMonitor(dir, nil)
	require.NoError(t, err)
	monitor.nowFunc = func() time.Time { return now }
	monitor.persistPath = filepath.Join(t.TempDir(), "alerts.json")
	// Reset state loaded from real persist file to avoid test pollution.
	monitor.alertedSessions = make(map[string]time.Time)
	monitor.alertTimestamps = nil
	monitor.circuitBreakerTrips = 0
	monitor.circuitBreakerUntil = time.Time{}
	monitor.disabled = false
	monitor.runCommand = func(name string, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}
	// By default, return nil from tmux (skip tmux filter).
	monitor.listTmuxSessions = func() ([]string, error) {
		return nil, fmt.Errorf("no tmux in test")
	}
	return monitor
}

// writeSessionHeartbeat writes a session heartbeat file for testing.
func writeSessionHeartbeat(t *testing.T, dir, sessionName string, ts time.Time) {
	t.Helper()
	hb := SessionHeartbeat{
		Timestamp:   ts,
		SessionName: sessionName,
		ScanOK:      true,
	}
	data, err := json.Marshal(hb)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, sessionName+".json"), data, 0600))
}
