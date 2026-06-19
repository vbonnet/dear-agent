package supervisor

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

// fixedClock returns a clock func pinned to t.
func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestNewWorkerReaper_RejectsNilDeps(t *testing.T) {
	trail, _ := newBufferTrail()
	lister := NewInMemoryWorkerLister()
	archiver := NewInMemoryWorkerArchiver()
	if _, err := NewWorkerReaper(nil, lister, archiver); err == nil {
		t.Error("nil trail accepted")
	}
	if _, err := NewWorkerReaper(trail, nil, archiver); err == nil {
		t.Error("nil lister accepted")
	}
	if _, err := NewWorkerReaper(trail, lister, nil); err == nil {
		t.Error("nil archiver accepted")
	}
}

func TestWorkerReaper_ReapsStoppedPastThreshold(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	lister := NewInMemoryWorkerLister(
		WorkerSession{SessionID: "worker-old", BeadID: "ce-1", Liveness: WorkerStopped, LastActivity: now.Add(-30 * time.Minute)},
		WorkerSession{SessionID: "worker-fresh", BeadID: "ce-2", Liveness: WorkerStopped, LastActivity: now.Add(-2 * time.Minute)},
	)
	archiver := NewInMemoryWorkerArchiver()
	trail, buf := newBufferTrail()
	r, _ := NewWorkerReaper(trail, lister, archiver)
	r.withClock(fixedClock(now)) // default 15m threshold

	res, err := r.Reap(context.Background())
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	if res.Scanned != 2 || res.Reaped != 1 || res.Failed != 0 {
		t.Fatalf("result = %+v, want scanned=2 reaped=1 failed=0", res)
	}
	calls := archiver.Calls()
	if len(calls) != 1 || calls[0].SessionID != "worker-old" || calls[0].Async {
		t.Fatalf("calls = %+v, want one plain archive of worker-old", calls)
	}
	assertReapReason(t, buf, "worker-old", "stopped-stale", "plain")
}

func TestWorkerReaper_ReapsStoppedWithZeroActivity(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	lister := NewInMemoryWorkerLister(
		WorkerSession{SessionID: "worker-undated", BeadID: "ce-1", Liveness: WorkerStopped}, // zero LastActivity
	)
	archiver := NewInMemoryWorkerArchiver()
	trail, _ := newBufferTrail()
	r, _ := NewWorkerReaper(trail, lister, archiver)
	r.withClock(fixedClock(now))

	res, err := r.Reap(context.Background())
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	if res.Reaped != 1 {
		t.Fatalf("reaped = %d, want 1 (zero LastActivity treated as stale)", res.Reaped)
	}
}

func TestWorkerReaper_ReapsGhostStatesImmediately(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	lister := NewInMemoryWorkerLister(
		WorkerSession{SessionID: "worker-unknown", BeadID: "ce-1", Liveness: WorkerUnknown, LastActivity: now},
		WorkerSession{SessionID: "worker-orphan", BeadID: "ce-2", Liveness: WorkerOrphaned, LastActivity: now},
	)
	archiver := NewInMemoryWorkerArchiver()
	trail, buf := newBufferTrail()
	r, _ := NewWorkerReaper(trail, lister, archiver)
	r.withClock(fixedClock(now))

	res, err := r.Reap(context.Background())
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	if res.Reaped != 2 {
		t.Fatalf("reaped = %d, want 2 (both ghosts)", res.Reaped)
	}
	for _, c := range archiver.Calls() {
		if c.Async {
			t.Errorf("ghost %s reaped async; want plain (no pane to drain)", c.SessionID)
		}
	}
	assertReapReason(t, buf, "worker-unknown", "ghost-unknown-state", "plain")
	assertReapReason(t, buf, "worker-orphan", "ghost-no-tmux-pane", "plain")
}

func TestWorkerReaper_LeavesActiveOpenBeadWorker(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	lister := NewInMemoryWorkerLister(
		WorkerSession{SessionID: "worker-busy", BeadID: "ce-1", Liveness: WorkerActive, LastActivity: now},
	)
	archiver := NewInMemoryWorkerArchiver()
	beads := NewInMemoryBeadStatusChecker() // ce-1 open by default
	trail, _ := newBufferTrail()
	r, _ := NewWorkerReaper(trail, lister, archiver)
	r.WithBeadStatusChecker(beads).withClock(fixedClock(now))

	res, err := r.Reap(context.Background())
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	if res.Reaped != 0 || len(archiver.Calls()) != 0 {
		t.Fatalf("active worker on open bead was reaped: res=%+v calls=%+v", res, archiver.Calls())
	}
}

func TestWorkerReaper_ReapsClosedBeadActiveAsync(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	lister := NewInMemoryWorkerLister(
		WorkerSession{SessionID: "worker-done", BeadID: "ce-closed", Liveness: WorkerActive, LastActivity: now},
	)
	archiver := NewInMemoryWorkerArchiver()
	beads := NewInMemoryBeadStatusChecker()
	beads.SetClosed("ce-closed", true)
	trail, buf := newBufferTrail()
	r, _ := NewWorkerReaper(trail, lister, archiver)
	r.WithBeadStatusChecker(beads).withClock(fixedClock(now))

	res, err := r.Reap(context.Background())
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	if res.Reaped != 1 {
		t.Fatalf("reaped = %d, want 1", res.Reaped)
	}
	calls := archiver.Calls()
	if len(calls) != 1 || !calls[0].Async {
		t.Fatalf("calls = %+v, want one async archive (active session must drain)", calls)
	}
	assertReapReason(t, buf, "worker-done", "bead-closed", "async")
}

func TestWorkerReaper_ReapsClosedBeadStoppedPlain(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	// Stopped & recent (would NOT be reaped by the stopped rule) but bead is
	// closed → reaped immediately, plain.
	lister := NewInMemoryWorkerLister(
		WorkerSession{SessionID: "worker-done", BeadID: "ce-closed", Liveness: WorkerStopped, LastActivity: now.Add(-1 * time.Minute)},
	)
	archiver := NewInMemoryWorkerArchiver()
	beads := NewInMemoryBeadStatusChecker()
	beads.SetClosed("ce-closed", true)
	trail, _ := newBufferTrail()
	r, _ := NewWorkerReaper(trail, lister, archiver)
	r.WithBeadStatusChecker(beads).withClock(fixedClock(now))

	res, err := r.Reap(context.Background())
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	calls := archiver.Calls()
	if res.Reaped != 1 || len(calls) != 1 || calls[0].Async {
		t.Fatalf("res=%+v calls=%+v, want one plain archive", res, calls)
	}
}

func TestWorkerReaper_NoBeadCheckerSkipsClosedBeadRule(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	lister := NewInMemoryWorkerLister(
		WorkerSession{SessionID: "worker-active", BeadID: "ce-1", Liveness: WorkerActive, LastActivity: now},
	)
	archiver := NewInMemoryWorkerArchiver()
	trail, _ := newBufferTrail()
	r, _ := NewWorkerReaper(trail, lister, archiver) // no bead checker
	r.withClock(fixedClock(now))

	res, err := r.Reap(context.Background())
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	if res.Reaped != 0 {
		t.Fatalf("reaped = %d, want 0 (active worker, no bead checker)", res.Reaped)
	}
}

func TestWorkerReaper_BeadCheckerErrorFallsThrough(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	// Active worker whose bead check errors: must NOT be reaped (we can't
	// prove the bead is closed, and an active session is not otherwise a zombie).
	lister := NewInMemoryWorkerLister(
		WorkerSession{SessionID: "worker-active", BeadID: "ce-1", Liveness: WorkerActive, LastActivity: now},
	)
	archiver := NewInMemoryWorkerArchiver()
	beads := NewInMemoryBeadStatusChecker()
	beads.SetError(errors.New("bd unavailable"))
	trail, _ := newBufferTrail()
	r, _ := NewWorkerReaper(trail, lister, archiver)
	r.WithBeadStatusChecker(beads).withClock(fixedClock(now))

	res, err := r.Reap(context.Background())
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	if res.Reaped != 0 {
		t.Fatalf("reaped = %d, want 0 (bead status unknown, active worker)", res.Reaped)
	}
}

func TestWorkerReaper_ArchiveFailureRecordedAndCounted(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	lister := NewInMemoryWorkerLister(
		WorkerSession{SessionID: "worker-a", BeadID: "ce-1", Liveness: WorkerUnknown, LastActivity: now},
		WorkerSession{SessionID: "worker-b", BeadID: "ce-2", Liveness: WorkerUnknown, LastActivity: now},
	)
	archiver := NewInMemoryWorkerArchiver()
	archiver.FailSession("worker-a", errors.New("archive boom"))
	trail, buf := newBufferTrail()
	r, _ := NewWorkerReaper(trail, lister, archiver)
	r.withClock(fixedClock(now))

	res, err := r.Reap(context.Background())
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	if res.Reaped != 1 || res.Failed != 1 {
		t.Fatalf("res = %+v, want reaped=1 failed=1", res)
	}
	// The advise record is still written before the failed archive.
	sawAdvise, sawFail := false, false
	for _, rec := range parseTrail(t, buf) {
		p, _ := rec["payload"].(map[string]any)
		if rec["kind"] == "supervisor.orch.reap" && p["session_id"] == "worker-a" {
			sawAdvise = true
		}
		if rec["kind"] == "supervisor.orch.reap_failed" && p["session_id"] == "worker-a" {
			sawFail = true
		}
	}
	if !sawAdvise {
		t.Error("missing supervisor.orch.reap advise record for failed session")
	}
	if !sawFail {
		t.Error("missing supervisor.orch.reap_failed record")
	}
}

func TestWorkerReaper_ListerErrorRecorded(t *testing.T) {
	lister := NewInMemoryWorkerLister()
	lister.SetError(errors.New("agm down"))
	archiver := NewInMemoryWorkerArchiver()
	trail, buf := newBufferTrail()
	r, _ := NewWorkerReaper(trail, lister, archiver)

	if _, err := r.Reap(context.Background()); err == nil {
		t.Fatal("Reap returned nil when lister errored")
	}
	saw := false
	for _, rec := range parseTrail(t, buf) {
		if rec["kind"] == "supervisor.orch.reap_error" {
			saw = true
		}
	}
	if !saw {
		t.Error("missing supervisor.orch.reap_error record")
	}
}

func TestWorkerReaper_CustomThreshold(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	lister := NewInMemoryWorkerLister(
		WorkerSession{SessionID: "worker-x", BeadID: "ce-1", Liveness: WorkerStopped, LastActivity: now.Add(-3 * time.Minute)},
	)
	archiver := NewInMemoryWorkerArchiver()
	trail, _ := newBufferTrail()
	r, _ := NewWorkerReaper(trail, lister, archiver)
	r.WithStoppedThreshold(2 * time.Minute).withClock(fixedClock(now))

	res, err := r.Reap(context.Background())
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	if res.Reaped != 1 {
		t.Fatalf("reaped = %d, want 1 (3m > 2m threshold)", res.Reaped)
	}
}

// TestOrchestrator_Tick_RunsReaper verifies the reaper is invoked from the
// Orchestrator's Tick and that dispatch still proceeds.
func TestOrchestrator_Tick_RunsReaper(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	q := NewInMemoryQueue()
	must(t, q.Enqueue(Task{ID: "t1", Title: "x", Worker: "coder"}))

	lister := NewInMemoryWorkerLister(
		WorkerSession{SessionID: "worker-zombie", BeadID: "ce-9", Liveness: WorkerUnknown, LastActivity: now},
	)
	archiver := NewInMemoryWorkerArchiver()
	trail, buf := newBufferTrail()
	r, _ := NewWorkerReaper(trail, lister, archiver)
	r.withClock(fixedClock(now))

	o, err := NewOrchestrator(trail, q)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	o.WithWorkerReaper(r)

	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if calls := archiver.Calls(); len(calls) != 1 || calls[0].SessionID != "worker-zombie" {
		t.Fatalf("reaper not invoked from Tick: calls=%+v", calls)
	}
	if got := q.Dispatched(); len(got) != 1 || got[0] != "t1" {
		t.Fatalf("dispatch did not proceed after reap: %v", got)
	}
	// Both a reap record and a dispatch record exist.
	var sawReap, sawDispatch bool
	for _, rec := range parseTrail(t, buf) {
		switch rec["kind"] {
		case "supervisor.orch.reap":
			sawReap = true
		case "supervisor.orch.dispatched":
			sawDispatch = true
		}
	}
	if !sawReap || !sawDispatch {
		t.Errorf("trail incomplete: sawReap=%v sawDispatch=%v", sawReap, sawDispatch)
	}
}

func TestOrchestrator_Tick_NoReaperStillWorks(t *testing.T) {
	q := NewInMemoryQueue()
	must(t, q.Enqueue(Task{ID: "t1", Title: "x", Worker: "coder"}))
	trail, _ := newBufferTrail()
	o, err := NewOrchestrator(trail, q)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	if err := o.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if got := q.Dispatched(); len(got) != 1 {
		t.Fatalf("dispatch failed with no reaper wired: %v", got)
	}
}

// assertReapReason scans the trail for a supervisor.orch.reap record matching
// the given session and asserts its reason and mode.
func assertReapReason(t *testing.T, buf *bytes.Buffer, sessionID, wantReason, wantMode string) {
	t.Helper()
	for _, rec := range parseTrail(t, buf) {
		p, _ := rec["payload"].(map[string]any)
		if rec["kind"] == "supervisor.orch.reap" && p["session_id"] == sessionID {
			if p["reason"] != wantReason {
				t.Errorf("%s reason = %v, want %q", sessionID, p["reason"], wantReason)
			}
			if p["mode"] != wantMode {
				t.Errorf("%s mode = %v, want %q", sessionID, p["mode"], wantMode)
			}
			return
		}
	}
	t.Errorf("no supervisor.orch.reap record for %s", sessionID)
}
