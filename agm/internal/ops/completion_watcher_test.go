package ops

import (
	"context"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// recordingStorage records UpdateSession calls so tests can prove durable
// completion capture.
type recordingStorage struct {
	mockStorage
	updated []*manifest.Manifest
}

func (r *recordingStorage) UpdateSession(m *manifest.Manifest) error {
	copied := *m
	r.updated = append(r.updated, &copied)
	return nil
}

func watcherFixture(t *testing.T) (*CompletionWatcher, *recordingStorage, *session.MockTmux, *manifest.Manifest) {
	t.Helper()
	m := testManifest("worker-1", "", time.Time{})
	m.Harness = "codex-cli"
	storage := &recordingStorage{mockStorage: mockStorage{sessions: []*manifest.Manifest{m}}}
	tmux := session.NewMockTmux()
	tmux.Sessions["worker-1"] = true
	tmux.PaneContents = map[string]string{"worker-1": "task done\nRESULT: 42\n"}
	watcher := NewCompletionWatcher(&OpContext{Storage: storage, Tmux: tmux})
	watcher.IdleConfirmTicks = 2
	return watcher, storage, tmux, m
}

func scanOnce(t *testing.T, watcher *CompletionWatcher) []CompletionEvent {
	t.Helper()
	events, err := watcher.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return events
}

func TestCompletionWatcher_IdleAfterBusyEmitsOnce(t *testing.T) {
	watcher, storage, tmux, _ := watcherFixture(t)

	// Tick 1: busy.
	tmux.InputReadiness = session.InputReadiness{Ready: false, State: "NO"}
	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("busy tick emitted events: %v", events)
	}

	// Ticks 2-3: idle; second consecutive idle crosses the debounce.
	tmux.InputReadiness = session.InputReadiness{Ready: true, State: "YES"}
	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("first idle tick should debounce, got %v", events)
	}
	events := scanOnce(t, watcher)
	if len(events) != 1 {
		t.Fatalf("expected 1 completion, got %d", len(events))
	}
	event := events[0]
	if event.TransitionType != "idle" || event.SessionName != "worker-1" {
		t.Fatalf("unexpected event: %+v", event)
	}
	if event.Output == "" {
		t.Fatal("completion carried no output")
	}

	// Durable capture persisted.
	if len(storage.updated) == 0 {
		t.Fatal("completion was not persisted")
	}
	last := storage.updated[len(storage.updated)-1]
	if last.State != manifest.StateDone || last.StateSource != "completion-watcher" {
		t.Fatalf("persisted state = %q/%q", last.State, last.StateSource)
	}
	if last.FinalOutput == "" || last.FinalOutputAt.IsZero() {
		t.Fatalf("final output not persisted: %+v", last)
	}

	// Staying idle emits nothing further.
	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("steady idle re-emitted: %v", events)
	}
}

func TestCompletionWatcher_IdleFromStartIsSilent(t *testing.T) {
	watcher, _, tmux, _ := watcherFixture(t)
	tmux.InputReadiness = session.InputReadiness{Ready: true, State: "YES"}
	for range 4 {
		if events := scanOnce(t, watcher); len(events) != 0 {
			t.Fatalf("never-busy session emitted: %v", events)
		}
	}
}

func TestCompletionWatcher_RearmsAfterNewWork(t *testing.T) {
	watcher, _, tmux, _ := watcherFixture(t)

	tmux.InputReadiness = session.InputReadiness{Ready: false, State: "NO"}
	scanOnce(t, watcher)
	tmux.InputReadiness = session.InputReadiness{Ready: true, State: "YES"}
	scanOnce(t, watcher)
	if events := scanOnce(t, watcher); len(events) != 1 {
		t.Fatalf("expected first completion, got %v", events)
	}

	// New message arrives: busy again, then idle → a second completion.
	tmux.InputReadiness = session.InputReadiness{Ready: false, State: "NO"}
	scanOnce(t, watcher)
	tmux.InputReadiness = session.InputReadiness{Ready: true, State: "YES"}
	scanOnce(t, watcher)
	if events := scanOnce(t, watcher); len(events) != 1 {
		t.Fatalf("expected second completion, got %v", events)
	}
}

func TestCompletionWatcher_ExitEmitsWithLastTail(t *testing.T) {
	watcher, storage, tmux, _ := watcherFixture(t)

	tmux.InputReadiness = session.InputReadiness{Ready: false, State: "NO"}
	scanOnce(t, watcher) // observes the session alive and busy, caches the tail

	delete(tmux.Sessions, "worker-1")
	events := scanOnce(t, watcher)
	if len(events) != 1 || events[0].TransitionType != "exited" {
		t.Fatalf("expected exited completion, got %v", events)
	}
	if events[0].Output == "" {
		t.Fatal("exit event lost the cached tail")
	}
	last := storage.updated[len(storage.updated)-1]
	if last.State != manifest.StateOffline {
		t.Fatalf("persisted state = %q, want offline", last.State)
	}

	// Gone stays gone: no repeat.
	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("exit re-emitted: %v", events)
	}
}

func TestCompletionWatcher_NeverSeenSessionExitIsSilent(t *testing.T) {
	m := testManifest("pre-existing", "", time.Time{})
	storage := &recordingStorage{mockStorage: mockStorage{sessions: []*manifest.Manifest{m}}}
	tmux := session.NewMockTmux() // session's tmux does not exist
	watcher := NewCompletionWatcher(&OpContext{Storage: storage, Tmux: tmux})

	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("watcher replayed history for a session it never saw alive: %v", events)
	}
}
