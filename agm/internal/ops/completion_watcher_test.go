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
	// The choreography below counts plain ticks; the codex-specific extended
	// debounce has its own dedicated test.
	watcher.ReadinessBlindConfirmMultiplier = 1
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

// strictMockTmux wraps MockTmux with a strict existence checker whose answers
// (and failures) are controlled independently of the plain HasSession.
type strictMockTmux struct {
	*session.MockTmux
	strictExists bool
	strictErr    error
}

func (s *strictMockTmux) HasSessionStrict(_ context.Context, _ string) (bool, error) {
	return s.strictExists, s.strictErr
}

func TestCompletionWatcher_TmuxBackendFailureIsNotAnExit(t *testing.T) {
	m := testManifest("worker-1", "", time.Time{})
	storage := &recordingStorage{mockStorage: mockStorage{sessions: []*manifest.Manifest{m}}}
	inner := session.NewMockTmux()
	inner.Sessions["worker-1"] = true
	inner.PaneContents = map[string]string{"worker-1": "booting…\n"}
	tmux := &strictMockTmux{MockTmux: inner, strictExists: true}
	watcher := NewCompletionWatcher(&OpContext{Storage: storage, Tmux: tmux})

	scanOnce(t, watcher) // baseline observes the session alive

	// The tmux backend starts failing: plain HasSession would collapse this
	// into "absent", but the strict checker reports the failure, so no exit
	// completion may fire and no OFFLINE state may be persisted.
	tmux.strictErr = context.DeadlineExceeded
	for range 3 {
		if events := scanOnce(t, watcher); len(events) != 0 {
			t.Fatalf("backend failure reported as exit: %v", events)
		}
	}
	for _, u := range storage.updated {
		if u.State == manifest.StateOffline {
			t.Fatalf("backend failure persisted OFFLINE: %+v", u)
		}
	}

	// Backend recovers and the session is truly gone: now the exit fires.
	tmux.strictErr = nil
	tmux.strictExists = false
	if events := scanOnce(t, watcher); len(events) != 1 || events[0].TransitionType != "exited" {
		t.Fatalf("confirmed absence did not report an exit: %v", events)
	}
}

func TestCompletionWatcher_PersistedWorkingStateRecoversMissedCompletion(t *testing.T) {
	// The watcher restarts after a worker already finished: the durable
	// session state still says WORKING (persisted by this pipeline before the
	// restart) and the pane is stable and input-ready. The completion must be
	// emitted rather than swallowed by the baseline.
	m := testManifest("worker-1", manifest.StateWorking, time.Now().Add(-time.Hour))
	m.Harness = "claude-code"
	storage := &recordingStorage{mockStorage: mockStorage{sessions: []*manifest.Manifest{m}}}
	tmux := session.NewMockTmux()
	tmux.Sessions["worker-1"] = true
	tmux.PaneContents = map[string]string{"worker-1": "RESULT: finished before restart\n"}
	tmux.InputReadiness = session.InputReadiness{Ready: true, State: "YES"}
	watcher := NewCompletionWatcher(&OpContext{Storage: storage, Tmux: tmux})
	watcher.IdleConfirmTicks = 2

	// Baseline arms activity from the persisted WORKING state and counts as
	// the first stable, input-ready observation; the second confirms.
	scanOnce(t, watcher)
	events := scanOnce(t, watcher)
	if len(events) != 1 || events[0].TransitionType != "idle" {
		t.Fatalf("missed pre-restart completion not recovered: %v", events)
	}
	if events[0].Output == "" {
		t.Fatal("recovered completion carried no output")
	}
}

func TestCompletionWatcher_ReadinessBlindHarnessGetsExtendedDebounce(t *testing.T) {
	watcher, _, tmux := watcherFixture(t)
	// Restore the default multiplier: codex-cli keeps its composer
	// input-ready while generating, so completion needs a longer stability
	// window than readiness-accurate harnesses.
	watcher.ReadinessBlindConfirmMultiplier = 3

	scanOnce(t, watcher) // baseline
	tmux.PaneContents["worker-1"] = "working…\n"
	scanOnce(t, watcher) // activity
	// 2 × 3 = 6 stable ticks required; the first five must stay silent.
	for i := range 5 {
		if events := scanOnce(t, watcher); len(events) != 0 {
			t.Fatalf("stable tick %d fired early for a readiness-blind harness: %v", i+1, events)
		}
	}
	if events := scanOnce(t, watcher); len(events) != 1 {
		t.Fatalf("extended debounce never completed: %v", events)
	}
}

func TestCompletionWatcher_WorkingSessionGoneAtRestartReportsExit(t *testing.T) {
	// The watcher restarts after a non-persistent worker finished and its
	// tmux ended: the durable WORKING state is the evidence that this is a
	// missed completion, not pre-watcher history.
	m := testManifest("worker-1", manifest.StateWorking, time.Now().Add(-time.Hour))
	storage := &recordingStorage{mockStorage: mockStorage{sessions: []*manifest.Manifest{m}}}
	tmux := session.NewMockTmux() // tmux target does not exist
	watcher := NewCompletionWatcher(&OpContext{Storage: storage, Tmux: tmux})

	events := scanOnce(t, watcher)
	if len(events) != 1 || events[0].TransitionType != "exited" {
		t.Fatalf("WORKING session gone at restart not reported: %v", events)
	}
	// And only once.
	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("restart exit re-emitted: %v", events)
	}
}

func TestCompletionWatcher_DryRunNeverWritesSessionRecords(t *testing.T) {
	watcher, storage, tmux := watcherFixture(t)
	watcher.DryRun = true

	scanOnce(t, watcher) // baseline
	tmux.PaneContents["worker-1"] = "working…\n"
	scanOnce(t, watcher) // activity
	scanOnce(t, watcher) // stable 1
	if events := scanOnce(t, watcher); len(events) != 1 {
		t.Fatalf("dry-run suppressed the event itself: %v", events)
	}
	if len(storage.updated) != 0 {
		t.Fatalf("dry-run wrote %d session record(s)", len(storage.updated))
	}
}
