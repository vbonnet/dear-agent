// Package monitoring provides loop heartbeat monitoring infrastructure.
package monitoring

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LoopHeartbeat represents a heartbeat from a monitoring loop.
// Written to ~/.agm/heartbeats/loop-{session}.json on each loop cycle.
type LoopHeartbeat struct {
	Timestamp    time.Time `json:"timestamp"`
	Session      string    `json:"session"`
	IntervalSecs int       `json:"interval_secs"`
	CycleNumber  int       `json:"cycle_number"`
	OK           bool      `json:"ok"`
}

// HeartbeatWriter writes loop heartbeat files atomically.
type HeartbeatWriter struct {
	mu  sync.Mutex
	dir string
}

// NewHeartbeatWriter creates a writer for loop heartbeat files.
// If dir is empty, defaults to ~/.agm/heartbeats/.
func NewHeartbeatWriter(dir string) (*HeartbeatWriter, error) {
	if dir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		dir = filepath.Join(homeDir, ".agm", "heartbeats")
	}

	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create heartbeats directory: %w", err)
	}

	return &HeartbeatWriter{dir: dir}, nil
}

// Write writes a loop heartbeat for the given session.
func (w *HeartbeatWriter) Write(session string, intervalSecs int, cycleNumber int, ok bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	hb := LoopHeartbeat{
		Timestamp:    time.Now(),
		Session:      session,
		IntervalSecs: intervalSecs,
		CycleNumber:  cycleNumber,
		OK:           ok,
	}

	data, err := json.Marshal(hb)
	if err != nil {
		return fmt.Errorf("failed to marshal loop heartbeat: %w", err)
	}

	path := filepath.Join(w.dir, "loop-"+session+".json")
	return os.WriteFile(path, data, 0600)
}

// ReadHeartbeat reads a loop heartbeat file for the given session.
func ReadHeartbeat(dir, session string) (*LoopHeartbeat, error) {
	if dir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		dir = filepath.Join(homeDir, ".agm", "heartbeats")
	}

	path := filepath.Join(dir, "loop-"+session+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var hb LoopHeartbeat
	if err := json.Unmarshal(data, &hb); err != nil {
		return nil, fmt.Errorf("failed to parse loop heartbeat: %w", err)
	}

	return &hb, nil
}

// ListHeartbeats returns all loop heartbeat files in the directory.
func ListHeartbeats(dir string) ([]*LoopHeartbeat, error) {
	if dir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		dir = filepath.Join(homeDir, ".agm", "heartbeats")
	}

	entries, err := filepath.Glob(filepath.Join(dir, "loop-*.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob loop heartbeat files: %w", err)
	}

	var heartbeats []*LoopHeartbeat
	for _, path := range entries {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var hb LoopHeartbeat
		if err := json.Unmarshal(data, &hb); err != nil {
			continue
		}
		heartbeats = append(heartbeats, &hb)
	}

	return heartbeats, nil
}

// CheckStaleness returns the staleness status of a loop heartbeat.
// Returns "ok", "warn", or "stale" based on the heartbeat age vs interval.
func CheckStaleness(hb *LoopHeartbeat) string {
	if hb == nil {
		return "stale"
	}
	age := time.Since(hb.Timestamp)
	threshold := time.Duration(hb.IntervalSecs)*time.Second + 60*time.Second
	if age > threshold {
		return "stale"
	}
	// Warn at 80% of threshold
	if age > time.Duration(float64(threshold)*0.8) {
		return "warn"
	}
	return "ok"
}

// CheckStalenessWithMaxAge returns whether the heartbeat is stale based on
// an explicit max-age duration rather than the interval-based threshold.
// Returns "ok", "warn", or "stale".
func CheckStalenessWithMaxAge(hb *LoopHeartbeat, maxAge time.Duration) string {
	if hb == nil {
		return "stale"
	}
	age := time.Since(hb.Timestamp)
	if age > maxAge {
		return "stale"
	}
	if age > time.Duration(float64(maxAge)*0.8) {
		return "warn"
	}
	return "ok"
}

// RemoveHeartbeat deletes a loop heartbeat file.
func RemoveHeartbeat(dir, session string) error {
	if dir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		dir = filepath.Join(homeDir, ".agm", "heartbeats")
	}

	path := filepath.Join(dir, "loop-"+session+".json")
	return os.Remove(path)
}
