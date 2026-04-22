package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- CheckWorkerHealth tests ---

func TestCheckWorkerHealth_Healthy(t *testing.T) {
	tmpDir := t.TempDir()
	hbPath := filepath.Join(tmpDir, "heartbeat.json")

	// Write a fresh heartbeat.
	writeHeartbeat(t, hbPath, time.Now(), 3, true)

	var alerts []Alert
	d := NewSilentFailureDetector(SilentFailureConfig{
		HeartbeatPath:   hbPath,
		MaxHeartbeatAge: 5 * time.Minute,
		AlertFunc: func(a Alert) {
			alerts = append(alerts, a)
		},
	})

	err := d.CheckWorkerHealth()
	assert.NoError(t, err)
	assert.Empty(t, alerts, "no alerts should fire for healthy heartbeat")
}

func TestCheckWorkerHealth_StaleHeartbeat(t *testing.T) {
	tmpDir := t.TempDir()
	hbPath := filepath.Join(tmpDir, "heartbeat.json")
	incidentsPath := filepath.Join(tmpDir, "incidents.jsonl")

	// Write a stale heartbeat (10 minutes old).
	writeHeartbeat(t, hbPath, time.Now().Add(-10*time.Minute), 2, true)

	var alerts []Alert
	d := NewSilentFailureDetector(SilentFailureConfig{
		HeartbeatPath:   hbPath,
		MaxHeartbeatAge: 5 * time.Minute,
		IncidentsFile:   incidentsPath,
		AlertFunc: func(a Alert) {
			alerts = append(alerts, a)
		},
	})

	err := d.CheckWorkerHealth()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stale")
	require.Len(t, alerts, 1)
	assert.Equal(t, "worker_health", alerts[0].Type)
	assert.Equal(t, "critical", alerts[0].Severity)
}

func TestCheckWorkerHealth_NoHeartbeatFile(t *testing.T) {
	tmpDir := t.TempDir()
	hbPath := filepath.Join(tmpDir, "nonexistent.json")
	incidentsPath := filepath.Join(tmpDir, "incidents.jsonl")

	var alerts []Alert
	d := NewSilentFailureDetector(SilentFailureConfig{
		HeartbeatPath:   hbPath,
		MaxHeartbeatAge: 5 * time.Minute,
		IncidentsFile:   incidentsPath,
		AlertFunc: func(a Alert) {
			alerts = append(alerts, a)
		},
	})

	err := d.CheckWorkerHealth()
	assert.Error(t, err)
	require.Len(t, alerts, 1)
	assert.Equal(t, "worker_health", alerts[0].Type)
	assert.Contains(t, alerts[0].Message, "unhealthy")
}

func TestCheckWorkerHealth_LogsIncident(t *testing.T) {
	tmpDir := t.TempDir()
	hbPath := filepath.Join(tmpDir, "heartbeat.json")
	incidentsPath := filepath.Join(tmpDir, "incidents.jsonl")

	writeHeartbeat(t, hbPath, time.Now().Add(-10*time.Minute), 1, false)

	d := NewSilentFailureDetector(SilentFailureConfig{
		HeartbeatPath:   hbPath,
		MaxHeartbeatAge: 5 * time.Minute,
		IncidentsFile:   incidentsPath,
	})
	d.nowFunc = func() time.Time {
		return time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	}

	_ = d.CheckWorkerHealth()

	// Verify incident was written.
	data, err := os.ReadFile(incidentsPath)
	require.NoError(t, err)

	var alert Alert
	require.NoError(t, json.Unmarshal(splitLines(data)[0], &alert))
	assert.Equal(t, "worker_health", alert.Type)
	assert.Equal(t, "worker-health-20260401-120000", alert.ID)
}

// --- CheckForkBomb tests ---

func TestCheckForkBomb_NoBomb(t *testing.T) {
	var alerts []Alert
	d := NewSilentFailureDetector(SilentFailureConfig{
		MaxChildProcesses: 50,
		AlertFunc: func(a Alert) {
			alerts = append(alerts, a)
		},
	})
	d.listChildPIDs = func(pid int) ([]int, error) {
		return []int{100, 101, 102}, nil
	}

	detected, err := d.CheckForkBomb(1)
	assert.NoError(t, err)
	assert.False(t, detected)
	assert.Empty(t, alerts)
}

func TestCheckForkBomb_Detected(t *testing.T) {
	tmpDir := t.TempDir()
	incidentsPath := filepath.Join(tmpDir, "incidents.jsonl")

	var alerts []Alert
	var killedPIDs []int
	d := NewSilentFailureDetector(SilentFailureConfig{
		MaxChildProcesses: 3,
		IncidentsFile:     incidentsPath,
		AlertFunc: func(a Alert) {
			alerts = append(alerts, a)
		},
	})
	d.listChildPIDs = func(pid int) ([]int, error) {
		return []int{100, 101, 102, 103, 104}, nil
	}
	d.killProcess = func(pid int) error {
		killedPIDs = append(killedPIDs, pid)
		return nil
	}
	d.nowFunc = func() time.Time {
		return time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	}

	detected, err := d.CheckForkBomb(1)
	assert.NoError(t, err)
	assert.True(t, detected)
	require.Len(t, alerts, 1)
	assert.Equal(t, "fork_bomb", alerts[0].Type)
	assert.Equal(t, "critical", alerts[0].Severity)
	assert.Contains(t, alerts[0].Message, "5 children")
	assert.Equal(t, "5", alerts[0].Details["killed_count"])

	// Verify killed in reverse order (newest first).
	assert.Equal(t, []int{104, 103, 102, 101, 100}, killedPIDs)
}

func TestCheckForkBomb_KillFailure(t *testing.T) {
	tmpDir := t.TempDir()
	incidentsPath := filepath.Join(tmpDir, "incidents.jsonl")

	var alerts []Alert
	d := NewSilentFailureDetector(SilentFailureConfig{
		MaxChildProcesses: 2,
		IncidentsFile:     incidentsPath,
		AlertFunc: func(a Alert) {
			alerts = append(alerts, a)
		},
	})
	d.listChildPIDs = func(pid int) ([]int, error) {
		return []int{100, 101, 102}, nil
	}
	d.killProcess = func(pid int) error {
		if pid == 101 {
			return fmt.Errorf("permission denied")
		}
		return nil
	}

	detected, err := d.CheckForkBomb(1)
	assert.NoError(t, err)
	assert.True(t, detected)
	require.Len(t, alerts, 1)
	// Only 2 of 3 killed successfully.
	assert.Equal(t, "2", alerts[0].Details["killed_count"])
}

func TestCheckForkBomb_ListError(t *testing.T) {
	d := NewSilentFailureDetector(SilentFailureConfig{})
	d.listChildPIDs = func(pid int) ([]int, error) {
		return nil, fmt.Errorf("proc read failed")
	}

	detected, err := d.CheckForkBomb(1)
	assert.Error(t, err)
	assert.False(t, detected)
	assert.Contains(t, err.Error(), "proc read failed")
}

func TestCheckForkBomb_ExactlyAtThreshold(t *testing.T) {
	d := NewSilentFailureDetector(SilentFailureConfig{
		MaxChildProcesses: 5,
	})
	d.listChildPIDs = func(pid int) ([]int, error) {
		return []int{100, 101, 102, 103, 104}, nil // Exactly 5.
	}

	detected, err := d.CheckForkBomb(1)
	assert.NoError(t, err)
	assert.False(t, detected, "should not trigger at exactly the threshold")
}

// --- CheckStaleSessions tests ---

func TestCheckStaleSessions_NoStale(t *testing.T) {
	d := NewSilentFailureDetector(SilentFailureConfig{
		StaleScanThreshold: 2,
	})

	// First scan: establishes baseline.
	stale := d.CheckStaleSessions(map[string]int{"s1": 100, "s2": 200})
	assert.Empty(t, stale)

	// Second scan: content changed.
	stale = d.CheckStaleSessions(map[string]int{"s1": 150, "s2": 250})
	assert.Empty(t, stale)
}

func TestCheckStaleSessions_DetectedAfterThreshold(t *testing.T) {
	tmpDir := t.TempDir()
	incidentsPath := filepath.Join(tmpDir, "incidents.jsonl")

	var alerts []Alert
	d := NewSilentFailureDetector(SilentFailureConfig{
		StaleScanThreshold: 2,
		IncidentsFile:      incidentsPath,
		AlertFunc: func(a Alert) {
			alerts = append(alerts, a)
		},
	})

	sessions := map[string]int{"worker-1": 100}

	// Scan 1: baseline.
	stale := d.CheckStaleSessions(sessions)
	assert.Empty(t, stale)

	// Scan 2: same content — stale count = 1, not yet at threshold.
	stale = d.CheckStaleSessions(sessions)
	assert.Empty(t, stale)

	// Scan 3: same content — stale count = 2, triggers.
	stale = d.CheckStaleSessions(sessions)
	require.Len(t, stale, 1)
	assert.Equal(t, "worker-1", stale[0])
	require.Len(t, alerts, 1)
	assert.Equal(t, "stale_session", alerts[0].Type)
	assert.Equal(t, "warning", alerts[0].Severity)
}

func TestCheckStaleSessions_ResetsOnProgress(t *testing.T) {
	d := NewSilentFailureDetector(SilentFailureConfig{
		StaleScanThreshold: 2,
	})

	// Scan 1: baseline.
	d.CheckStaleSessions(map[string]int{"s1": 100})

	// Scan 2: unchanged — stale count = 1.
	d.CheckStaleSessions(map[string]int{"s1": 100})

	// Scan 3: content changed — resets counter.
	d.CheckStaleSessions(map[string]int{"s1": 200})

	// Scan 4: unchanged again — stale count = 1 (not 2, because reset).
	stale := d.CheckStaleSessions(map[string]int{"s1": 200})
	assert.Empty(t, stale, "should not be stale after progress reset")
}

func TestCheckStaleSessions_CleansUpRemovedSessions(t *testing.T) {
	d := NewSilentFailureDetector(SilentFailureConfig{
		StaleScanThreshold: 2,
	})

	// Track two sessions.
	d.CheckStaleSessions(map[string]int{"s1": 100, "s2": 200})

	// Remove s2 from next scan.
	d.CheckStaleSessions(map[string]int{"s1": 100})

	// Verify s2 state is cleaned up.
	d.mu.Lock()
	_, s2Exists := d.sessionLastLengths["s2"]
	d.mu.Unlock()
	assert.False(t, s2Exists, "removed session should be cleaned up")
}

func TestCheckStaleSessions_MultipleStaleSessions(t *testing.T) {
	var alerts []Alert
	d := NewSilentFailureDetector(SilentFailureConfig{
		StaleScanThreshold: 2,
		AlertFunc: func(a Alert) {
			alerts = append(alerts, a)
		},
	})

	sessions := map[string]int{"s1": 100, "s2": 200, "s3": 300}

	// Three scans with no changes.
	d.CheckStaleSessions(sessions)
	d.CheckStaleSessions(sessions)
	stale := d.CheckStaleSessions(sessions)

	assert.Len(t, stale, 3)
	assert.Len(t, alerts, 3)
}

func TestCheckStaleSessions_ResetsAfterAlert(t *testing.T) {
	var alertCount int
	d := NewSilentFailureDetector(SilentFailureConfig{
		StaleScanThreshold: 2,
		AlertFunc: func(a Alert) {
			alertCount++
		},
	})

	sessions := map[string]int{"s1": 100}

	// Trigger first alert.
	d.CheckStaleSessions(sessions)
	d.CheckStaleSessions(sessions)
	d.CheckStaleSessions(sessions)
	assert.Equal(t, 1, alertCount)

	// Counter was reset after alert, so need 2 more unchanged scans.
	d.CheckStaleSessions(sessions)
	stale := d.CheckStaleSessions(sessions)
	assert.Len(t, stale, 1)
	assert.Equal(t, 2, alertCount)
}

// --- ClearSession tests ---

func TestClearSession(t *testing.T) {
	d := NewSilentFailureDetector(SilentFailureConfig{
		StaleScanThreshold: 2,
	})

	d.CheckStaleSessions(map[string]int{"s1": 100})
	d.CheckStaleSessions(map[string]int{"s1": 100}) // stale count = 1

	d.ClearSession("s1")

	// After clearing, this is a fresh baseline — no alert.
	d.CheckStaleSessions(map[string]int{"s1": 100})
	d.mu.Lock()
	count := d.sessionScanCounts["s1"]
	d.mu.Unlock()
	assert.Equal(t, 0, count, "scan count should be 0 after clear and fresh baseline")
}

// --- Config defaults tests ---

func TestNewSilentFailureDetector_Defaults(t *testing.T) {
	d := NewSilentFailureDetector(SilentFailureConfig{})

	assert.Equal(t, 5*time.Minute, d.config.MaxHeartbeatAge)
	assert.Equal(t, 50, d.config.MaxChildProcesses)
	assert.Equal(t, 30*time.Second, d.config.ForkBombCheckInterval)
	assert.Equal(t, 2, d.config.StaleScanThreshold)
	assert.NotEmpty(t, d.config.HeartbeatPath)
	assert.NotEmpty(t, d.config.IncidentsFile)
	assert.NotNil(t, d.listChildPIDs)
	assert.NotNil(t, d.killProcess)
	assert.NotNil(t, d.nowFunc)
}

func TestNewSilentFailureDetector_CustomConfig(t *testing.T) {
	d := NewSilentFailureDetector(SilentFailureConfig{
		HeartbeatPath:         "/tmp/hb.json",
		MaxHeartbeatAge:       10 * time.Minute,
		MaxChildProcesses:     100,
		ForkBombCheckInterval: 15 * time.Second,
		StaleScanThreshold:    3,
		IncidentsFile:         "/tmp/incidents.jsonl",
	})

	assert.Equal(t, "/tmp/hb.json", d.config.HeartbeatPath)
	assert.Equal(t, 10*time.Minute, d.config.MaxHeartbeatAge)
	assert.Equal(t, 100, d.config.MaxChildProcesses)
	assert.Equal(t, 15*time.Second, d.config.ForkBombCheckInterval)
	assert.Equal(t, 3, d.config.StaleScanThreshold)
}

// --- Start/Stop tests ---

func TestSilentFailureDetector_StartStop(t *testing.T) {
	tmpDir := t.TempDir()
	hbPath := filepath.Join(tmpDir, "heartbeat.json")
	writeHeartbeat(t, hbPath, time.Now(), 1, true)

	d := NewSilentFailureDetector(SilentFailureConfig{
		HeartbeatPath:         hbPath,
		ForkBombCheckInterval: 10 * time.Millisecond,
	})
	d.listChildPIDs = func(pid int) ([]int, error) { return nil, nil }

	errChan := make(chan error, 1)
	go func() {
		errChan <- d.Start()
	}()

	time.Sleep(50 * time.Millisecond)
	d.Stop()

	err := <-errChan
	assert.NoError(t, err)
}

func TestSilentFailureDetector_StartAlreadyRunning(t *testing.T) {
	tmpDir := t.TempDir()
	hbPath := filepath.Join(tmpDir, "heartbeat.json")
	writeHeartbeat(t, hbPath, time.Now(), 1, true)

	d := NewSilentFailureDetector(SilentFailureConfig{
		HeartbeatPath:         hbPath,
		ForkBombCheckInterval: 10 * time.Millisecond,
	})
	d.listChildPIDs = func(pid int) ([]int, error) { return nil, nil }

	go func() { _ = d.Start() }()
	time.Sleep(20 * time.Millisecond)

	err := d.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	d.Stop()
}

// --- Concurrency safety test ---

func TestCheckStaleSessions_ConcurrentAccess(t *testing.T) {
	d := NewSilentFailureDetector(SilentFailureConfig{
		StaleScanThreshold: 2,
	})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sessions := map[string]int{
				fmt.Sprintf("s-%d", i): 100 + i,
			}
			d.CheckStaleSessions(sessions)
		}(i)
	}

	wg.Wait()
	// No panic = pass.
}
