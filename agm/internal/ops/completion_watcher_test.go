package ops

import (
	"context"
	"errors"
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

// TestCompletionWatcher_RearmsFromPersistedTerminalState proves the restart
// case: a session whose durable record already says DONE (written before the
// watcher bounced) must be flipped back to WORKING as soon as its pane shows
// new activity, even though the fresh in-memory observation never reported
// that completion itself.
func TestCompletionWatcher_RearmsFromPersistedTerminalState(t *testing.T) {
	watcher, storage, tmux := watcherFixture(t)
	storage.sessions[0].State = manifest.StateDone

	// Baseline tick after the "restart": reportedIdle is false.
	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("baseline tick emitted events: %v", events)
	}

	// New work arrives: the record must stop claiming DONE.
	tmux.PaneContents["worker-1"] = "picked up a new task\n"
	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("activity tick emitted events: %v", events)
	}
	if len(storage.updated) != 1 {
		t.Fatalf("expected one durable state write, got %d", len(storage.updated))
	}
	if got := storage.updated[0].State; got != manifest.StateWorking {
		t.Fatalf("state = %q, want %q", got, manifest.StateWorking)
	}
}

// TestCompletionWatcher_CaptureFailureDoesNotCompleteSession proves a pane the
// watcher cannot read is never mistaken for a pane whose contents are stable.
// Readiness alone would otherwise advance the stability window and fire a
// completion carrying a stale tail, with no observation of stable content.
func TestCompletionWatcher_CaptureFailureDoesNotCompleteSession(t *testing.T) {
	watcher, storage, tmux := watcherFixture(t)

	scanOnce(t, watcher) // baseline
	tmux.PaneContents["worker-1"] = "working…\nhalf a result\n"
	scanOnce(t, watcher) // activity recorded

	// The pane goes unreadable while the session is still alive. Readiness
	// keeps answering YES (codex-cli's composer stays input-ready).
	tmux.CapturePaneError = errors.New("capture-pane failed")
	for i := range 6 {
		if events := scanOnce(t, watcher); len(events) != 0 {
			t.Fatalf("tick %d completed on an unreadable pane: %v", i, events)
		}
	}
	if len(storage.updated) != 0 {
		t.Fatalf("unreadable pane produced durable writes: %+v", storage.updated)
	}
}

// TestCompletionWatcher_SkipsPureAPISessions proves a paneless API session is
// never mistaken for one that exited. Its tmux target is absent by design, and
// a persisted WORKING state would otherwise take the restart exception in
// observeGone and emit a false "exited" completion plus an OFFLINE write.
func TestCompletionWatcher_SkipsPureAPISessions(t *testing.T) {
	m := testManifest("api-worker", manifest.StateWorking, time.Now())
	m.Harness = "openai"
	storage := &recordingStorage{mockStorage: mockStorage{sessions: []*manifest.Manifest{m}}}
	tmux := session.NewMockTmux() // no tmux session registered
	watcher := NewCompletionWatcher(&OpContext{Storage: storage, Tmux: tmux})

	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("pure API session emitted completions: %v", events)
	}
	if len(storage.updated) != 0 {
		t.Fatalf("pure API session got durable writes: %+v", storage.updated)
	}
}

// flakyStorage fails the first failWrites durable writes, then records the
// ones that land — the storage outage a completion has to survive.
type flakyStorage struct {
	mockStorage
	failWrites int
	updated    []*manifest.Manifest
}

func (f *flakyStorage) UpdateSession(m *manifest.Manifest) error {
	if f.failWrites > 0 {
		f.failWrites--
		return errors.New("storage unavailable")
	}
	copied := *m
	f.updated = append(f.updated, &copied)
	return nil
}

// flakyWatcherFixture mirrors watcherFixture with a caller-supplied storage so
// the durable-write retry path can be exercised.
func flakyWatcherFixture(t *testing.T, storage *flakyStorage) (*CompletionWatcher, *session.MockTmux) {
	t.Helper()
	tmux := session.NewMockTmux()
	tmux.Sessions["worker-1"] = true
	tmux.PaneContents = map[string]string{"worker-1": "booting…\n"}
	tmux.InputReadiness = session.InputReadiness{Ready: true, State: "YES"}
	watcher := NewCompletionWatcher(&OpContext{Storage: storage, Tmux: tmux})
	watcher.IdleConfirmTicks = 2
	watcher.ReadinessBlindConfirmMultiplier = 1
	return watcher, tmux
}

// driveToIdleCompletion runs the tick choreography that produces one idle
// completion: baseline, two ticks of pane activity, then stable input-ready
// ticks until the debounce fires.
func driveToIdleCompletion(t *testing.T, watcher *CompletionWatcher, tmux *session.MockTmux) CompletionEvent {
	t.Helper()
	scanOnce(t, watcher) // baseline
	tmux.PaneContents["worker-1"] = "working…\n"
	scanOnce(t, watcher)
	tmux.PaneContents["worker-1"] = "working…\nRESULT: 42\n"
	scanOnce(t, watcher)
	scanOnce(t, watcher) // first stable tick debounces
	events := scanOnce(t, watcher)
	if len(events) != 1 {
		t.Fatalf("expected 1 completion, got %d", len(events))
	}
	return events[0]
}

// TestCompletionWatcher_RetryPreservesTheOriginalCompletionTime pins the
// timestamps a retried write carries. When storage is down for several scans,
// the retry replays a capture taken at detection time; restamping it with the
// retry's clock tells every consumer that an old tail was captured at recovery
// time, so freshness and ordering decisions rank stale evidence first.
func TestCompletionWatcher_RetryPreservesTheOriginalCompletionTime(t *testing.T) {
	m := testManifest("worker-1", "", time.Time{})
	m.Harness = "codex-cli"
	storage := &flakyStorage{mockStorage: mockStorage{sessions: []*manifest.Manifest{m}}, failWrites: 1}
	watcher, tmux := flakyWatcherFixture(t, storage)

	event := driveToIdleCompletion(t, watcher, tmux)
	if len(storage.updated) != 0 {
		t.Fatalf("the failing write was recorded as landed: %+v", storage.updated)
	}

	// A later stable scan retries quietly — no second event for the operator.
	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("retry re-emitted the completion: %v", events)
	}
	if len(storage.updated) != 1 {
		t.Fatalf("retry did not land the durable write: %+v", storage.updated)
	}

	landed := storage.updated[0]
	if !landed.FinalOutputAt.Equal(event.DetectedAt) {
		t.Errorf("retry stamped FinalOutputAt %v, want the original capture time %v", landed.FinalOutputAt, event.DetectedAt)
	}
	if !landed.StateUpdatedAt.Equal(event.DetectedAt) {
		t.Errorf("retry stamped StateUpdatedAt %v, want the original detection time %v", landed.StateUpdatedAt, event.DetectedAt)
	}
	if landed.State != manifest.StateDone || landed.FinalOutput == "" {
		t.Errorf("retried write lost the completion: %+v", landed)
	}
}

// TestCompletionWatcher_RetryDoesNotOverwriteNewerState pins the other half of
// the retry contract. Between a failed write and its retry the session can
// accept new work and a state hook can persist WORKING; the pane tail has not
// changed yet, so the retry still looks due. Replaying DONE there makes an
// actively running task read as finished to every durable-state consumer.
func TestCompletionWatcher_RetryDoesNotOverwriteNewerState(t *testing.T) {
	m := testManifest("worker-1", "", time.Time{})
	m.Harness = "codex-cli"
	storage := &flakyStorage{mockStorage: mockStorage{sessions: []*manifest.Manifest{m}}, failWrites: 1}
	watcher, tmux := flakyWatcherFixture(t, storage)

	driveToIdleCompletion(t, watcher, tmux)
	if len(storage.updated) != 0 {
		t.Fatalf("the failing write was recorded as landed: %+v", storage.updated)
	}

	// A state hook records new work before the next scan.
	m.State = manifest.StateWorking
	m.StateUpdatedAt = time.Now()
	m.StateSource = "state-hook"

	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("retry re-emitted the completion: %v", events)
	}
	if len(storage.updated) != 0 {
		t.Fatalf("retry overwrote a newer state transition: %+v", storage.updated)
	}
	if m.State != manifest.StateWorking || m.StateSource != "state-hook" {
		t.Fatalf("record was clobbered by the superseded completion: state=%q source=%q", m.State, m.StateSource)
	}

	// The superseded completion is settled, not pending: it must not retry on
	// every later tick either.
	if events := scanOnce(t, watcher); len(events) != 0 {
		t.Fatalf("second retry emitted events: %v", events)
	}
	if len(storage.updated) != 0 {
		t.Fatalf("superseded completion kept retrying: %+v", storage.updated)
	}
}
