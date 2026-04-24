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

func TestNewWatchdog_Defaults(t *testing.T) {
	w := NewWatchdog(WatchdogConfig{})

	assert.Equal(t, 5*time.Minute, w.config.StaleThreshold)
	assert.Equal(t, 30*time.Second, w.config.CheckInterval)
	assert.Equal(t, 3, w.config.MaxRestartAttempts)
	assert.NotEmpty(t, w.config.HeartbeatPath)
	assert.NotEmpty(t, w.config.IncidentsFile)
	assert.Equal(t, []string{"systemctl", "--user", "restart", "astrocyte"}, w.config.RestartCommand)
	assert.NotNil(t, w.runCommand)
	assert.NotNil(t, w.nowFunc)
}

func TestNewWatchdog_CustomConfig(t *testing.T) {
	cfg := WatchdogConfig{
		HeartbeatPath:      "/tmp/test-heartbeat.json",
		StaleThreshold:     10 * time.Minute,
		CheckInterval:      1 * time.Minute,
		MaxRestartAttempts: 5,
		RestartCommand:     []string{"restart-daemon"},
		IncidentsFile:      "/tmp/test-incidents.jsonl",
	}
	w := NewWatchdog(cfg)

	assert.Equal(t, "/tmp/test-heartbeat.json", w.config.HeartbeatPath)
	assert.Equal(t, 10*time.Minute, w.config.StaleThreshold)
	assert.Equal(t, 1*time.Minute, w.config.CheckInterval)
	assert.Equal(t, 5, w.config.MaxRestartAttempts)
	assert.Equal(t, []string{"restart-daemon"}, w.config.RestartCommand)
}

func TestWatchdog_Check_HealthyHeartbeat(t *testing.T) {
	tmpDir := t.TempDir()
	hbPath := filepath.Join(tmpDir, "heartbeat.json")
	incidentsPath := filepath.Join(tmpDir, "incidents.jsonl")

	// Write a fresh heartbeat.
	writeHeartbeat(t, hbPath, time.Now(), 3, true)

	restartCalled := false
	w := NewWatchdog(WatchdogConfig{
		HeartbeatPath:  hbPath,
		StaleThreshold: 5 * time.Minute,
		IncidentsFile:  incidentsPath,
	})
	w.runCommand = func(name string, args ...string) error {
		restartCalled = true
		return nil
	}

	w.check()

	assert.False(t, restartCalled, "restart should not be called for healthy heartbeat")
	assert.Equal(t, 0, w.restartAttempts)
}

func TestWatchdog_Check_StaleHeartbeat_RestartSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	hbPath := filepath.Join(tmpDir, "heartbeat.json")
	incidentsPath := filepath.Join(tmpDir, "incidents.jsonl")

	// Write a stale heartbeat (10 minutes old).
	writeHeartbeat(t, hbPath, time.Now().Add(-10*time.Minute), 2, true)

	restartCalled := false
	w := NewWatchdog(WatchdogConfig{
		HeartbeatPath:  hbPath,
		StaleThreshold: 5 * time.Minute,
		IncidentsFile:  incidentsPath,
	})
	w.runCommand = func(name string, args ...string) error {
		restartCalled = true
		return nil
	}

	w.check()

	assert.True(t, restartCalled, "restart should be called for stale heartbeat")
	assert.Equal(t, 1, w.restartAttempts)
}

func TestWatchdog_Check_StaleHeartbeat_RestartFailure(t *testing.T) {
	tmpDir := t.TempDir()
	hbPath := filepath.Join(tmpDir, "heartbeat.json")
	incidentsPath := filepath.Join(tmpDir, "incidents.jsonl")

	// Write a stale heartbeat.
	writeHeartbeat(t, hbPath, time.Now().Add(-10*time.Minute), 1, false)

	w := NewWatchdog(WatchdogConfig{
		HeartbeatPath:  hbPath,
		StaleThreshold: 5 * time.Minute,
		IncidentsFile:  incidentsPath,
	})
	w.runCommand = func(_ string, _ ...string) error {
		return fmt.Errorf("systemctl not found")
	}
	w.nowFunc = func() time.Time {
		return time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	}

	w.check()

	assert.Equal(t, 1, w.restartAttempts)

	// Verify incident was written.
	incidents := readIncidents(t, incidentsPath)
	require.Len(t, incidents, 1)
	assert.Equal(t, "daemon_unhealthy", incidents[0].Symptom)
	assert.Equal(t, "restart_failed", incidents[0].Reason)
	assert.Contains(t, incidents[0].Error, "systemctl not found")
	assert.Equal(t, 1, incidents[0].RestartAttempts)
}

func TestWatchdog_Check_MaxRestartsExhausted(t *testing.T) {
	tmpDir := t.TempDir()
	hbPath := filepath.Join(tmpDir, "heartbeat.json")
	incidentsPath := filepath.Join(tmpDir, "incidents.jsonl")

	// Write a stale heartbeat.
	writeHeartbeat(t, hbPath, time.Now().Add(-10*time.Minute), 0, false)

	restartCount := 0
	w := NewWatchdog(WatchdogConfig{
		HeartbeatPath:      hbPath,
		StaleThreshold:     5 * time.Minute,
		IncidentsFile:      incidentsPath,
		MaxRestartAttempts: 2,
	})
	w.runCommand = func(_ string, _ ...string) error {
		restartCount++
		return fmt.Errorf("restart failed")
	}
	w.nowFunc = func() time.Time {
		return time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	}

	// First two checks trigger restart attempts.
	w.check()
	assert.Equal(t, 1, restartCount)
	w.check()
	assert.Equal(t, 2, restartCount)

	// Third check: max exhausted, no more restart attempts.
	w.check()
	assert.Equal(t, 2, restartCount, "should not attempt restart after max exhausted")

	// Verify max_restarts_exhausted incident was logged.
	incidents := readIncidents(t, incidentsPath)
	// Should have: 2 restart_failed + 1 max_restarts_exhausted
	require.Len(t, incidents, 3)
	assert.Equal(t, "restart_failed", incidents[0].Reason)
	assert.Equal(t, "restart_failed", incidents[1].Reason)
	assert.Equal(t, "max_restarts_exhausted", incidents[2].Reason)
}

func TestWatchdog_Check_RecoveryResetsCounter(t *testing.T) {
	tmpDir := t.TempDir()
	hbPath := filepath.Join(tmpDir, "heartbeat.json")
	incidentsPath := filepath.Join(tmpDir, "incidents.jsonl")

	// Start with a stale heartbeat.
	writeHeartbeat(t, hbPath, time.Now().Add(-10*time.Minute), 0, false)

	w := NewWatchdog(WatchdogConfig{
		HeartbeatPath:  hbPath,
		StaleThreshold: 5 * time.Minute,
		IncidentsFile:  incidentsPath,
	})
	w.runCommand = func(_ string, _ ...string) error { return nil }

	// Trigger a restart attempt.
	w.check()
	assert.Equal(t, 1, w.restartAttempts)

	// Now write a fresh heartbeat (daemon recovered).
	writeHeartbeat(t, hbPath, time.Now(), 5, true)

	w.check()
	assert.Equal(t, 0, w.restartAttempts, "restart counter should reset on recovery")
}

func TestWatchdog_Check_NoHeartbeatFile(t *testing.T) {
	tmpDir := t.TempDir()
	hbPath := filepath.Join(tmpDir, "nonexistent.json")
	incidentsPath := filepath.Join(tmpDir, "incidents.jsonl")

	restartCalled := false
	w := NewWatchdog(WatchdogConfig{
		HeartbeatPath:  hbPath,
		StaleThreshold: 5 * time.Minute,
		IncidentsFile:  incidentsPath,
	})
	w.runCommand = func(_ string, _ ...string) error {
		restartCalled = true
		return nil
	}

	w.check()

	assert.True(t, restartCalled, "should attempt restart when heartbeat file missing")
	assert.Equal(t, 1, w.restartAttempts)
}

func TestWatchdog_StartStop(t *testing.T) {
	tmpDir := t.TempDir()
	hbPath := filepath.Join(tmpDir, "heartbeat.json")

	// Write a fresh heartbeat so check passes.
	writeHeartbeat(t, hbPath, time.Now(), 1, true)

	w := NewWatchdog(WatchdogConfig{
		HeartbeatPath: hbPath,
		CheckInterval: 10 * time.Millisecond,
	})

	errChan := make(chan error, 1)
	go func() {
		errChan <- w.Start()
	}()

	// Let it run briefly.
	time.Sleep(50 * time.Millisecond)
	w.Stop()

	err := <-errChan
	assert.NoError(t, err)
}

func TestWatchdog_Start_AlreadyRunning(t *testing.T) {
	tmpDir := t.TempDir()
	hbPath := filepath.Join(tmpDir, "heartbeat.json")
	writeHeartbeat(t, hbPath, time.Now(), 1, true)

	w := NewWatchdog(WatchdogConfig{
		HeartbeatPath: hbPath,
		CheckInterval: 10 * time.Millisecond,
	})

	go func() {
		_ = w.Start()
	}()
	time.Sleep(20 * time.Millisecond)

	err := w.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	w.Stop()
}

func TestWatchdog_LogIncident_Format(t *testing.T) {
	tmpDir := t.TempDir()
	incidentsPath := filepath.Join(tmpDir, "incidents.jsonl")

	fixedTime := time.Date(2026, 3, 29, 14, 30, 0, 0, time.UTC)
	hb := &Heartbeat{
		Timestamp:    fixedTime.Add(-10 * time.Minute),
		SessionCount: 3,
		ScanOK:       true,
		PID:          12345,
	}

	w := NewWatchdog(WatchdogConfig{
		IncidentsFile:      incidentsPath,
		MaxRestartAttempts: 3,
	})
	w.restartAttempts = 2
	w.nowFunc = func() time.Time { return fixedTime }

	w.logIncident(hb, fmt.Errorf("heartbeat stale"), "restart_failed")

	incidents := readIncidents(t, incidentsPath)
	require.Len(t, incidents, 1)

	inc := incidents[0]
	assert.Equal(t, "watchdog-20260329-143000", inc.ID)
	assert.Equal(t, "2026-03-29T14:30:00Z", inc.Timestamp)
	assert.Equal(t, "daemon_unhealthy", inc.Symptom)
	assert.Equal(t, "restart_failed", inc.Reason)
	assert.Equal(t, 2, inc.RestartAttempts)
	assert.Equal(t, 3, inc.MaxAttempts)
	assert.Equal(t, 12345, inc.DaemonPID)
	assert.NotNil(t, inc.LastHeartbeat)
	assert.Equal(t, "heartbeat stale", inc.Error)
}

func TestWatchdog_LogIncident_NilHeartbeat(t *testing.T) {
	tmpDir := t.TempDir()
	incidentsPath := filepath.Join(tmpDir, "incidents.jsonl")

	w := NewWatchdog(WatchdogConfig{
		IncidentsFile: incidentsPath,
	})
	w.nowFunc = func() time.Time {
		return time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	}

	w.logIncident(nil, fmt.Errorf("no heartbeat file found"), "restart_failed")

	incidents := readIncidents(t, incidentsPath)
	require.Len(t, incidents, 1)
	assert.Equal(t, 0, incidents[0].DaemonPID)
	assert.Nil(t, incidents[0].LastHeartbeat)
}

func TestNotifySystemdWatchdog_NoSocket(t *testing.T) {
	// Unset NOTIFY_SOCKET if set.
	orig := os.Getenv("NOTIFY_SOCKET")
	t.Setenv("NOTIFY_SOCKET", "")
	defer func() {
		if orig != "" {
			os.Setenv("NOTIFY_SOCKET", orig)
		}
	}()

	err := NotifySystemdWatchdog()
	assert.NoError(t, err, "should return nil when NOTIFY_SOCKET is not set")
}

func TestNotifySystemdReady_NoSocket(t *testing.T) {
	orig := os.Getenv("NOTIFY_SOCKET")
	t.Setenv("NOTIFY_SOCKET", "")
	defer func() {
		if orig != "" {
			os.Setenv("NOTIFY_SOCKET", orig)
		}
	}()

	err := NotifySystemdReady()
	assert.NoError(t, err, "should return nil when NOTIFY_SOCKET is not set")
}

// writeHeartbeat writes a heartbeat JSON file for testing.
func writeHeartbeat(t *testing.T, path string, ts time.Time, sessionCount int, scanOK bool) {
	t.Helper()
	hb := Heartbeat{
		Timestamp:    ts,
		SessionCount: sessionCount,
		ScanOK:       scanOK,
		PID:          os.Getpid(), // Use current PID so /proc check passes.
	}
	data, err := json.Marshal(hb)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0750))
	require.NoError(t, os.WriteFile(path, data, 0600))
}

// readIncidents reads WatchdogIncident entries from a JSONL file.
func readIncidents(t *testing.T, path string) []WatchdogIncident {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var incidents []WatchdogIncident
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var inc WatchdogIncident
		require.NoError(t, json.Unmarshal(line, &inc))
		incidents = append(incidents, inc)
	}
	return incidents
}

// splitLines splits byte data into lines.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
