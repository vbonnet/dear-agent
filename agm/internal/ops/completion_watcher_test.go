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

func watcherFixture(t *testing.T) (*CompletionWatcher, *recordingStorage, *session.MockTmux) {
	t.Helper()
	m := testManifest("worker-1", "", time.Time{})
	m.Harness = "codex-cli"
	storage := &recordingStorage{mockStorage: mockStorage{sessions: []*manifest.Manifest{m}}}
	tmux := session.NewMockTmux()
	tmux.Sessions["worker-1"] = true
	tmux.PaneContents = map[string]string{"worker-1": "booting…\n"}
	// codex-cli keeps the composer input-ready even while generating, so the
	// fixture mirrors that: readiness alone must never gate activity detection.
	tmux.InputReadiness = session.InputReadiness{Ready: true, State: "YES"}
	watcher := NewCompletionWatcher(&OpContext{Storage: storage, Tmux: tmux})
	watcher.IdleConfirmTicks = 2
	return watcher, storage, tmux
}

func scanOnce(t *testing.T, watcher *CompletionWatcher) []CompletionEvent {
	t.Helper()
	events, err := watcher.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return events
}

func TestCompletionWatcher_ActivityThenStableEmitsOnce(t *testing.T) {
	watcher, storage, tmux := watcherFixture(t)

	// Tick 1 establishes the baseline; no activity yet.
	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("baseline tick emitted events: %v", events)
	}

	// Tick 2: pane content changed — the session is working.
	tmux.PaneContents["worker-1"] = "working…\ncomputing primes\n"
	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("activity tick emitted events: %v", events)
	}

	// Tick 3: more output, still working.
	tmux.PaneContents["worker-1"] = "working…\ncomputing primes\nRESULT: 42\n"
	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("second activity tick emitted events: %v", events)
	}

	// Ticks 4-5: content stable and input-ready; second stable tick fires.
	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("first stable tick should debounce, got %v", events)
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

	// Staying stable emits nothing further.
	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("steady state re-emitted: %v", events)
	}
}

func TestCompletionWatcher_StaticFromStartIsSilent(t *testing.T) {
	watcher, _, _ := watcherFixture(t)
	for range 5 {
		if events := scanOnce(t, watcher); len(events) != 0 {
			t.Fatalf("static session emitted: %v", events)
		}
	}
}

func TestCompletionWatcher_NotInputReadyBlocksCompletion(t *testing.T) {
	watcher, _, tmux := watcherFixture(t)

	scanOnce(t, watcher) // baseline
	tmux.PaneContents["worker-1"] = "May I run rm -rf? [y/n]\n"
	scanOnce(t, watcher) // activity
	// Content now stable but the composer is a permission prompt — not ready.
	tmux.InputReadiness = session.InputReadiness{Ready: false, State: "PERMISSION"}
	for range 4 {
		if events := scanOnce(t, watcher); len(events) != 0 {
			t.Fatalf("permission-prompt session reported completed: %v", events)
		}
	}
	// Prompt answered: composer ready again, content stable → completion.
	tmux.InputReadiness = session.InputReadiness{Ready: true, State: "YES"}
	scanOnce(t, watcher)
	if events := scanOnce(t, watcher); len(events) != 1 {
		t.Fatalf("expected completion after readiness returned, got %v", events)
	}
}

func TestCompletionWatcher_RearmsAfterNewWork(t *testing.T) {
	watcher, _, tmux := watcherFixture(t)

	scanOnce(t, watcher) // baseline
	tmux.PaneContents["worker-1"] = "first task output\n"
	scanOnce(t, watcher) // activity
	scanOnce(t, watcher) // stable 1
	if events := scanOnce(t, watcher); len(events) != 1 {
		t.Fatalf("expected first completion, got %v", events)
	}

	// New message arrives: fresh output, then stable → a second completion.
	tmux.PaneContents["worker-1"] = "first task output\nsecond task output\n"
	scanOnce(t, watcher) // activity
	scanOnce(t, watcher) // stable 1
	if events := scanOnce(t, watcher); len(events) != 1 {
		t.Fatalf("expected second completion, got %v", events)
	}
}

func TestCompletionWatcher_ExitEmitsWithLastTail(t *testing.T) {
	watcher, storage, tmux := watcherFixture(t)

	scanOnce(t, watcher) // baseline observes the session alive, caches the tail

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
