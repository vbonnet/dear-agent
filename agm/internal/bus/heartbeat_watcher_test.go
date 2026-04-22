package bus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestWatcher wires up a SupervisorHeartbeatWatcher whose Emitter is
// a real Emitter BUT swapped for a capture via a small seam: we override
// the emitter's dial path with a broker whose on-receive handler records
// the frame. Simpler: construct a real broker in-process, connect the
// emitter, and watch the broker's Registry for emitted frames.
func newTestWatcher(t *testing.T, dir string, staleAfter, interval time.Duration) (*SupervisorHeartbeatWatcher, *Server, context.CancelFunc) {
	t.Helper()
	s, _ := startServer(t)
	em := NewEmitter("hbw-test")
	em.SocketPath = s.SocketPath
	em.DialTimeout = 500 * time.Millisecond

	w := NewSupervisorHeartbeatWatcher(dir, em)
	w.StaleAfter = staleAfter
	w.Interval = interval
	w.RepeatInterval = 0 // no debounce in tests
	_, cancel := context.WithCancel(context.Background())
	return w, s, cancel
}

// writeHeartbeat creates dir/<id>/heartbeat.json with the given age.
// Returns the parsed record for reference.
func writeHeartbeat(t *testing.T, base, id string, ageOverZero time.Duration) {
	t.Helper()
	supDir := filepath.Join(base, id)
	if err := os.MkdirAll(supDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := map[string]any{
		"id":            id,
		"last_beat_utc": time.Now().UTC().Add(-ageOverZero),
		"pid":           1234,
		"primary_for":   "",
		"tertiary_for":  "",
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(supDir, "heartbeat.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// subscribeTo sets up a dialer session registered as "subscriber" so
// any frame the watcher emits with To="subscriber" arrives. We use a
// direct Emit call with a targeted recipient in a later test; for
// scanOnce we don't have a target, so we verify via LastErr + a
// separate integration check.

func TestHeartbeatWatcherDetectsStale(t *testing.T) {
	// Use a dir outside tempdir to keep socket path short; we still use
	// t.TempDir() for the heartbeat state since those are plain files.
	base := t.TempDir()
	writeHeartbeat(t, base, "s1", 10*time.Minute) // stale: 10m > 1m
	writeHeartbeat(t, base, "s2", 10*time.Second) // fresh: 10s < 1m

	w, _, cancel := newTestWatcher(t, base, 1*time.Minute, 50*time.Millisecond)
	defer cancel()

	// Verify behavior indirectly via the watcher's lastEmittedAt map,
	// which records which supervisors triggered an EmitEvent call. The
	// Emitter itself points at a real broker (from newTestWatcher);
	// whether the frame actually landed at a subscriber is covered by
	// emitter_test.go's TestEmitterSendsFrameThroughBroker.
	ctx, c2 := context.WithTimeout(context.Background(), 1*time.Second)
	defer c2()
	w.scanOnce(ctx)

	w.mu.Lock()
	defer w.mu.Unlock()
	if _, saw := w.lastEmittedAt["s1"]; !saw {
		t.Error("s1 (stale) should have been emitted")
	}
	if _, saw := w.lastEmittedAt["s2"]; saw {
		t.Error("s2 (fresh) should NOT have been emitted")
	}
}

func TestHeartbeatWatcherDetectsNever(t *testing.T) {
	base := t.TempDir()
	// Create a supervisor dir with NO heartbeat file.
	if err := os.MkdirAll(filepath.Join(base, "s-never"), 0o755); err != nil {
		t.Fatal(err)
	}
	w, _, cancel := newTestWatcher(t, base, 1*time.Minute, 50*time.Millisecond)
	defer cancel()

	ctx, c2 := context.WithTimeout(context.Background(), 1*time.Second)
	defer c2()
	w.scanOnce(ctx)

	w.mu.Lock()
	defer w.mu.Unlock()
	if _, saw := w.lastEmittedAt["s-never"]; !saw {
		t.Error("supervisor with no heartbeat file should be emitted as never-state")
	}
}

func TestHeartbeatWatcherRepeatIntervalDebounces(t *testing.T) {
	base := t.TempDir()
	writeHeartbeat(t, base, "s1", 10*time.Minute) // stale

	w, _, cancel := newTestWatcher(t, base, 1*time.Minute, 50*time.Millisecond)
	defer cancel()
	w.RepeatInterval = 500 * time.Millisecond

	ctx, c2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer c2()
	w.scanOnce(ctx)
	first := w.lastEmittedAt["s1"]
	if first.IsZero() {
		t.Fatal("s1 should have been emitted on first scan")
	}
	// Immediate second scan: still stale but within debounce window.
	w.scanOnce(ctx)
	if !w.lastEmittedAt["s1"].Equal(first) {
		t.Errorf("debounce should suppress repeat emission within RepeatInterval")
	}

	// Wait past debounce, scan again: should re-emit.
	time.Sleep(550 * time.Millisecond)
	w.scanOnce(ctx)
	if w.lastEmittedAt["s1"].Equal(first) {
		t.Error("after RepeatInterval, stale supervisor should re-emit")
	}
}

func TestHeartbeatWatcherMissingDirIsSilent(t *testing.T) {
	w, _, cancel := newTestWatcher(t, "/tmp/does-not-exist-heartbeat-dir", time.Minute, time.Second)
	defer cancel()
	ctx, c2 := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer c2()
	// Should not panic, should not emit.
	w.scanOnce(ctx)
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.lastEmittedAt) != 0 {
		t.Errorf("missing dir should not produce emissions; got %v", w.lastEmittedAt)
	}
}

func TestHeartbeatWatcherRunHonorsCancel(t *testing.T) {
	base := t.TempDir()
	writeHeartbeat(t, base, "s1", time.Hour) // guaranteed stale

	w, _, cancel := newTestWatcher(t, base, time.Minute, 50*time.Millisecond)
	_ = cancel
	ctx, cancelCtx := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelCtx()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned err: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Run did not exit on context cancel")
	}
}

func TestHeartbeatWatcherRunRequiresDirAndEmitter(t *testing.T) {
	w := &SupervisorHeartbeatWatcher{}
	err := w.Run(context.Background())
	if err == nil {
		t.Error("Run should error when Dir is missing")
	}
}
