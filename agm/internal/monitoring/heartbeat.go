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

// Loop heartbeat states, written to the State field. These let a peer
// supervisor diagnose *why* a loop is stuck across the process boundary,
// not merely that its heartbeat is stale (retro
// 2026-06-17-vroom-overnight-supervisor-cascade, gap 4).
const (
	// StateRunning means the last tick completed normally.
	StateRunning = "running"
	// StateStuck means the last tick errored or exceeded its budget.
	StateStuck = "stuck"
	// StateRecovering means the loop is mid-recovery (e.g. just restarted, or
	// actively unblocking a peer) and not yet back to steady state.
	StateRecovering = "recovering"
)

// LoopHeartbeat represents a heartbeat from a monitoring loop.
// Written to ~/.agm/heartbeats/loop-{session}.json on each loop cycle.
//
// The trailing diagnostic fields (LastTickError, LastTickDurationMs, State)
// are written when a loop has that context to share. They cross the process
// boundary so a peer supervisor can tell a stuck loop from a slow one and act
// accordingly (wake vs. wait vs. escalate). All are omitempty for backward
// compatibility with older readers and pre-diagnostic heartbeat files.
type LoopHeartbeat struct {
	Timestamp    time.Time `json:"timestamp"`
	Session      string    `json:"session"`
	IntervalSecs int       `json:"interval_secs"`
	CycleNumber  int       `json:"cycle_number"`
	OK           bool      `json:"ok"`

	// LastTickError is the error string from the most recent tick, empty when
	// the tick succeeded.
	LastTickError string `json:"last_tick_error,omitempty"`
	// LastTickDurationMs is how long the most recent tick took, in
	// milliseconds. Zero when unknown.
	LastTickDurationMs int64 `json:"last_tick_duration_ms,omitempty"`
	// State is one of StateRunning / StateStuck / StateRecovering. Empty when
	// the writer does not track loop state.
	State string `json:"state,omitempty"`
}

// Diagnostics carries the optional diagnostic state a loop can attach to its
// heartbeat. A nil *Diagnostics means "no diagnostic context" and leaves the
// corresponding JSON fields empty.
type Diagnostics struct {
	LastTickError      string
	LastTickDurationMs int64
	State              string
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

// Write writes a loop heartbeat for the given session, with no diagnostic
// state. Equivalent to WriteDiagnostic(..., nil).
func (w *HeartbeatWriter) Write(session string, intervalSecs int, cycleNumber int, ok bool) error {
	return w.WriteDiagnostic(session, intervalSecs, cycleNumber, ok, nil)
}

// WriteDiagnostic writes a loop heartbeat for the given session, attaching the
// optional diagnostic state (last tick error, tick duration, loop state) so
// peer supervisors can diagnose a stall across the process boundary. A nil
// diag writes only the base fields.
func (w *HeartbeatWriter) WriteDiagnostic(session string, intervalSecs int, cycleNumber int, ok bool, diag *Diagnostics) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	hb := LoopHeartbeat{
		Timestamp:    time.Now(),
		Session:      session,
		IntervalSecs: intervalSecs,
		CycleNumber:  cycleNumber,
		OK:           ok,
	}
	if diag != nil {
		hb.LastTickError = diag.LastTickError
		hb.LastTickDurationMs = diag.LastTickDurationMs
		hb.State = diag.State
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
