// Package daemon provides background daemon monitoring.
package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Heartbeat represents the daemon's last-known-alive state.
// Written to ~/.agm/astrocyte/heartbeat.json on every scan cycle.
type Heartbeat struct {
	Timestamp    time.Time `json:"timestamp"`
	SessionCount int       `json:"session_count"`
	ScanOK       bool      `json:"scan_ok"`
	PID          int       `json:"pid"`
}

// HeartbeatWriter manages writing heartbeat files.
type HeartbeatWriter struct {
	mu       sync.Mutex
	filePath string
	pid      int
}

// NewHeartbeatWriter creates a heartbeat writer at the given path.
// If path is empty, defaults to ~/.agm/astrocyte/heartbeat.json.
func NewHeartbeatWriter(filePath string) (*HeartbeatWriter, error) {
	if filePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		filePath = filepath.Join(homeDir, ".agm/logs/astrocyte/heartbeat.json")
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create heartbeat directory: %w", err)
	}

	return &HeartbeatWriter{
		filePath: filePath,
		pid:      os.Getpid(),
	}, nil
}

// Beat writes a heartbeat with the current timestamp and session count.
func (w *HeartbeatWriter) Beat(sessionCount int, scanOK bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	hb := Heartbeat{
		Timestamp:    time.Now(),
		SessionCount: sessionCount,
		ScanOK:       scanOK,
		PID:          w.pid,
	}

	data, err := json.Marshal(hb)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat: %w", err)
	}

	return os.WriteFile(w.filePath, data, 0600)
}

// CheckHealth reads the heartbeat file and returns an error if the daemon
// appears unhealthy (heartbeat older than maxAge).
func CheckHealth(heartbeatPath string, maxAge time.Duration) (*Heartbeat, error) {
	data, err := os.ReadFile(heartbeatPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no heartbeat file found: daemon may not be running")
		}
		return nil, fmt.Errorf("failed to read heartbeat: %w", err)
	}

	var hb Heartbeat
	if err := json.Unmarshal(data, &hb); err != nil {
		return nil, fmt.Errorf("corrupt heartbeat file: %w", err)
	}

	age := time.Since(hb.Timestamp)
	if age > maxAge {
		return &hb, fmt.Errorf("heartbeat stale: last beat %s ago (threshold %s)", age.Round(time.Second), maxAge)
	}

	// Check if the PID is still alive
	if hb.PID > 0 {
		proc, err := os.FindProcess(hb.PID)
		if err != nil {
			return &hb, fmt.Errorf("daemon process %d not found", hb.PID)
		}
		// On Unix, FindProcess always succeeds. Check /proc to verify.
		if _, err := os.Stat(fmt.Sprintf("/proc/%d", hb.PID)); err != nil {
			_ = proc // suppress unused
			return &hb, fmt.Errorf("daemon process %d is not running", hb.PID)
		}
	}

	return &hb, nil
}

// DefaultHeartbeatPath returns the default heartbeat file location.
func DefaultHeartbeatPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".agm/logs/astrocyte/heartbeat.json")
}
