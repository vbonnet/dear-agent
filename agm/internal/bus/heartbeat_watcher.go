package bus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SupervisorHeartbeatWatcher tails the supervisor heartbeat dir and emits
// "heartbeat_stale" events to the broker when any supervisor's last
// heartbeat is older than the stale-after threshold.
//
// Compact companion to the larger sentinel/daemon/loop_monitor.go —
// that file handles WORKER loop heartbeats with wake/restart semantics.
// This watcher deals specifically with SUPERVISOR heartbeats from
// `agm supervisor heartbeat` and reports them on the bus so peer
// supervisors (primary/tertiary) can react.
//
// The watcher is read-only: it doesn't try to restart or escalate.
// Reacting to stale supervisors is the peer mesh's job, which it
// coordinates via A2A messages on the bus.
type SupervisorHeartbeatWatcher struct {
	// Dir is the supervisor state dir. Typically ~/.agm/supervisors/.
	Dir string
	// Emitter is how we publish stale events. Required.
	Emitter *Emitter
	// Interval is how often we scan. Defaults to 30s. The scan itself
	// is cheap (a few file reads) — lower intervals are fine.
	Interval time.Duration
	// StaleAfter is the age threshold. Defaults to 5 min (matches
	// `agm supervisor status --stale-after` default).
	StaleAfter time.Duration
	// Logger is slog-based. Defaults to slog.Default().
	Logger *slog.Logger

	// RepeatInterval optionally debounces repeat emissions for the same
	// supervisor id. Zero means "emit every tick while stale", which is
	// noisy; 10 min is a good default.
	RepeatInterval time.Duration

	mu            sync.Mutex
	lastEmittedAt map[string]time.Time
}

// NewSupervisorHeartbeatWatcher constructs a watcher with sensible
// defaults. Dir and emitter must be set by the caller.
func NewSupervisorHeartbeatWatcher(dir string, em *Emitter) *SupervisorHeartbeatWatcher {
	return &SupervisorHeartbeatWatcher{
		Dir:            dir,
		Emitter:        em,
		Interval:       30 * time.Second,
		StaleAfter:     5 * time.Minute,
		RepeatInterval: 10 * time.Minute,
		Logger:         slog.Default(),
		lastEmittedAt:  make(map[string]time.Time),
	}
}

// Run blocks until ctx is cancelled. Each tick scans the supervisor
// state dir and emits a heartbeat_stale event for any supervisor whose
// last-beat age exceeds StaleAfter, debouncing per RepeatInterval.
func (w *SupervisorHeartbeatWatcher) Run(ctx context.Context) error {
	if w.Dir == "" {
		return errors.New("heartbeat_watcher: Dir is required")
	}
	if w.Emitter == nil {
		return errors.New("heartbeat_watcher: Emitter is required")
	}
	interval := w.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if w.Logger == nil {
		w.Logger = slog.Default()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run one scan immediately so a supervisor that's stale at startup
	// gets reported without waiting a full interval.
	w.scanOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			w.scanOnce(ctx)
		}
	}
}

// scanOnce does a single pass over w.Dir. Errors are logged but don't
// stop the watcher — transient filesystem issues shouldn't take out the
// liveness signal for the whole mesh.
func (w *SupervisorHeartbeatWatcher) scanOnce(ctx context.Context) {
	now := time.Now().UTC()
	entries, err := os.ReadDir(w.Dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No supervisors registered yet — quiet no-op.
			return
		}
		w.Logger.Warn("heartbeat_watcher: read dir failed", "dir", w.Dir, "err", err)
		return
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		id := ent.Name()
		rec, err := w.readRecord(id)
		if err != nil {
			w.Logger.Debug("heartbeat_watcher: read heartbeat failed", "supervisor", id, "err", err)
			continue
		}
		if rec == nil {
			// No heartbeat file yet — treat as NEVER, which is also
			// worth reporting (distinguished by meta.state="never").
			w.maybeEmit(ctx, now, id, "never", 0, nil)
			continue
		}
		age := now.Sub(rec.LastBeatUTC)
		if age < w.StaleAfter {
			continue
		}
		w.maybeEmit(ctx, now, id, "stale", age, rec)
	}
}

// maybeEmit honors the RepeatInterval debounce so a persistently-stale
// supervisor doesn't spam the bus every tick.
func (w *SupervisorHeartbeatWatcher) maybeEmit(
	ctx context.Context,
	now time.Time,
	id, state string,
	age time.Duration,
	rec *supervisorHeartbeatRecord,
) {
	w.mu.Lock()
	last := w.lastEmittedAt[id]
	if w.RepeatInterval > 0 && !last.IsZero() && now.Sub(last) < w.RepeatInterval {
		w.mu.Unlock()
		return
	}
	w.lastEmittedAt[id] = now
	w.mu.Unlock()

	meta := map[string]string{
		"supervisor": id,
		"state":      state,
		"age_secs":   fmt.Sprintf("%.0f", age.Seconds()),
	}
	if rec != nil {
		if rec.PrimaryFor != "" {
			meta["primary_for"] = rec.PrimaryFor
		}
		if rec.TertiaryFor != "" {
			meta["tertiary_for"] = rec.TertiaryFor
		}
	}
	text := fmt.Sprintf("supervisor %s heartbeat %s (age=%s)", id, state, age)
	if err := w.Emitter.EmitEvent(ctx, "heartbeat_"+state, text, meta); err != nil {
		w.Logger.Warn("heartbeat_watcher: emit failed", "supervisor", id, "err", err)
	}
	w.Logger.Info("heartbeat event emitted", "supervisor", id, "state", state, "age", age)
}

// supervisorHeartbeatRecord mirrors agm/cmd/agm/supervisor.go's
// heartbeatRecord, deliberately duplicated here so the bus package
// doesn't import from the agm CLI package (which would be a dep cycle).
// Keep the JSON fields in sync.
type supervisorHeartbeatRecord struct {
	ID          string    `json:"id"`
	PrimaryFor  string    `json:"primary_for,omitempty"`
	TertiaryFor string    `json:"tertiary_for,omitempty"`
	LastBeatUTC time.Time `json:"last_beat_utc"`
	PID         int       `json:"pid,omitempty"`
}

// readRecord returns (nil, nil) when no heartbeat file exists (never-
// heartbeated supervisor). Returns an error only for malformed JSON —
// a missing file is a valid state we want to surface as "never".
func (w *SupervisorHeartbeatWatcher) readRecord(id string) (*supervisorHeartbeatRecord, error) {
	path := filepath.Join(w.Dir, id, "heartbeat.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var rec supervisorHeartbeatRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &rec, nil
}
