package procreaper

import (
	"context"
	"errors"
	"testing"
	"time"
)

// --- fakes -----------------------------------------------------------------

type fakeSupervisor struct {
	roots map[int]string
	err   error
}

func (f fakeSupervisor) SupervisedPIDs(context.Context) (map[int]string, error) {
	return f.roots, f.err
}

type fakeLister struct {
	procs []ProcessInfo
	err   error
}

func (f fakeLister) List(context.Context) ([]ProcessInfo, error) { return f.procs, f.err }

// signalEvent records one call to the signaler for ordering assertions.
type signalEvent struct {
	kind string // "term" | "kill"
	pid  int
}

// fakeSignaler records every Term/Kill in order and answers Alive from a
// per-PID script: aliveAfterTerm[pid] is whether the process is still alive
// after SIGTERM. PIDs absent from the map are assumed alive (so the default is
// the harder path: escalation to SIGKILL).
type fakeSignaler struct {
	events         []signalEvent
	aliveAfterTerm map[int]bool
	termErr        map[int]error
	killErr        map[int]error
	termed         map[int]bool
}

func newFakeSignaler() *fakeSignaler {
	return &fakeSignaler{
		aliveAfterTerm: map[int]bool{},
		termErr:        map[int]error{},
		killErr:        map[int]error{},
		termed:         map[int]bool{},
	}
}

func (f *fakeSignaler) Term(pid int) error {
	f.events = append(f.events, signalEvent{"term", pid})
	f.termed[pid] = true
	return f.termErr[pid]
}

func (f *fakeSignaler) Kill(pid int) error {
	f.events = append(f.events, signalEvent{"kill", pid})
	return f.killErr[pid]
}

func (f *fakeSignaler) Alive(pid int) bool {
	if !f.termed[pid] {
		return true // not yet signalled
	}
	alive, ok := f.aliveAfterTerm[pid]
	if !ok {
		return true // default: survives SIGTERM, forcing SIGKILL
	}
	return alive
}

// newTestReaper builds a Reaper with a no-op clock so the grace window does not
// actually sleep.
func newTestReaper(t *testing.T, sup SessionSupervisor, lister ProcessLister, sig Signaler) *Reaper {
	t.Helper()
	r, err := New(sup, lister, sig, 999999, withClock(func(time.Duration) {}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

func contains(haystack []ProcessInfo, pid int) bool {
	for _, p := range haystack {
		if p.PID == pid {
			return true
		}
	}
	return false
}

// --- required DoD test: never kill a supervised session --------------------

// TestReaper_NeverKillsSupervisedSession asserts the core invariant: a process
// that belongs to a supervised session is protected even when it is the single
// largest memory hog, and is never signalled.
func TestReaper_NeverKillsSupervisedSession(t *testing.T) {
	// pane PID 100 is the live session root; 101 is its child (gopls).
	// 200 is an unsupervised hog that should be reaped.
	sup := fakeSupervisor{roots: map[int]string{100: "worker-ce-abc"}}
	lister := fakeLister{procs: []ProcessInfo{
		{PID: 100, PPID: 1, RSSKB: 4_000_000, Command: "claude"},  // supervised root
		{PID: 101, PPID: 100, RSSKB: 5_000_000, Command: "gopls"}, // supervised child — biggest hog
		{PID: 200, PPID: 1, RSSKB: 2_000_000, Command: "gopls"},   // unsupervised orphan hog
	}}
	sig := newFakeSignaler()
	r := newTestReaper(t, sup, lister, sig)

	res, err := r.Reap(context.Background(), ReapRequest{MaxRSSMB: 1024})
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}

	// The supervised root and its child must be protected, never signalled.
	for _, pid := range []int{100, 101} {
		if !contains(res.Protected, pid) {
			t.Errorf("pid %d should be protected", pid)
		}
		if contains(res.Reapable, pid) || contains(res.Terminated, pid) {
			t.Errorf("pid %d must never be reapable/terminated", pid)
		}
		for _, ev := range sig.events {
			if ev.pid == pid {
				t.Errorf("supervised pid %d received a %s signal", pid, ev.kind)
			}
		}
	}

	// The unsupervised hog must be reaped.
	if !contains(res.Terminated, 200) {
		t.Errorf("unsupervised pid 200 should be terminated; terminated=%v", res.Terminated)
	}
}

// TestReaper_FailsClosedWhenSupervisedSetUnknown asserts that if the session DB
// cannot be queried, the reaper kills nothing and returns an error.
func TestReaper_FailsClosedWhenSupervisedSetUnknown(t *testing.T) {
	sup := fakeSupervisor{err: errors.New("dolt unreachable")}
	lister := fakeLister{procs: []ProcessInfo{
		{PID: 200, PPID: 1, RSSKB: 9_000_000, Command: "gopls"},
	}}
	sig := newFakeSignaler()
	r := newTestReaper(t, sup, lister, sig)

	_, err := r.Reap(context.Background(), ReapRequest{MaxRSSMB: 1024})
	if err == nil {
		t.Fatal("expected an error when supervised set is unknown")
	}
	if len(sig.events) != 0 {
		t.Errorf("no signals must be sent when failing closed; got %v", sig.events)
	}
}

// --- required DoD test: SIGTERM before SIGKILL -----------------------------

// TestReaper_SIGTERMBeforeSIGKILL asserts graduated escalation: a process that
// survives SIGTERM receives SIGKILL, and the SIGTERM strictly precedes it.
func TestReaper_SIGTERMBeforeSIGKILL(t *testing.T) {
	sup := fakeSupervisor{roots: map[int]string{}}
	lister := fakeLister{procs: []ProcessInfo{
		{PID: 200, PPID: 1, RSSKB: 3_000_000, Command: "gopls"},
	}}
	sig := newFakeSignaler()
	sig.aliveAfterTerm[200] = true // stubborn: survives SIGTERM
	r := newTestReaper(t, sup, lister, sig)

	res, err := r.Reap(context.Background(), ReapRequest{MaxRSSMB: 1024, GracePeriod: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}

	if len(sig.events) != 2 {
		t.Fatalf("expected exactly [term, kill], got %v", sig.events)
	}
	if sig.events[0].kind != "term" || sig.events[0].pid != 200 {
		t.Errorf("first signal must be SIGTERM to pid 200, got %+v", sig.events[0])
	}
	if sig.events[1].kind != "kill" || sig.events[1].pid != 200 {
		t.Errorf("second signal must be SIGKILL to pid 200, got %+v", sig.events[1])
	}
	if !contains(res.Terminated, 200) {
		t.Errorf("pid 200 should be terminated")
	}
}

// TestReaper_SIGTERMSufficient_NoSIGKILL asserts the graceful path: a process
// that exits on SIGTERM is NOT escalated to SIGKILL.
func TestReaper_SIGTERMSufficient_NoSIGKILL(t *testing.T) {
	sup := fakeSupervisor{roots: map[int]string{}}
	lister := fakeLister{procs: []ProcessInfo{
		{PID: 200, PPID: 1, RSSKB: 3_000_000, Command: "gopls"},
	}}
	sig := newFakeSignaler()
	sig.aliveAfterTerm[200] = false // exits promptly on SIGTERM
	r := newTestReaper(t, sup, lister, sig)

	if _, err := r.Reap(context.Background(), ReapRequest{MaxRSSMB: 1024}); err != nil {
		t.Fatalf("Reap: %v", err)
	}

	if len(sig.events) != 1 || sig.events[0].kind != "term" {
		t.Fatalf("expected exactly one SIGTERM and no SIGKILL, got %v", sig.events)
	}
}

// --- supporting behaviour ---------------------------------------------------

// TestReaper_DryRunSignalsNothing asserts dry-run computes the kill set but
// sends no signals.
func TestReaper_DryRunSignalsNothing(t *testing.T) {
	sup := fakeSupervisor{roots: map[int]string{}}
	lister := fakeLister{procs: []ProcessInfo{
		{PID: 200, PPID: 1, RSSKB: 3_000_000, Command: "gopls"},
	}}
	sig := newFakeSignaler()
	r := newTestReaper(t, sup, lister, sig)

	res, err := r.Reap(context.Background(), ReapRequest{MaxRSSMB: 1024, DryRun: true})
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	if len(sig.events) != 0 {
		t.Errorf("dry-run must send no signals, got %v", sig.events)
	}
	if !contains(res.Reapable, 200) {
		t.Errorf("dry-run should still report pid 200 as reapable")
	}
	if len(res.Terminated) != 0 {
		t.Errorf("dry-run must terminate nothing, got %v", res.Terminated)
	}
}

// TestReaper_ProtectsRolesByName asserts orchestrator/overseer/meta- processes
// are never reaped regardless of RSS.
func TestReaper_ProtectsRolesByName(t *testing.T) {
	sup := fakeSupervisor{roots: map[int]string{}}
	lister := fakeLister{procs: []ProcessInfo{
		{PID: 10, PPID: 1, RSSKB: 9_000_000, Command: "vroom-mesh", CmdLine: "vroom-mesh overseer"},
		{PID: 11, PPID: 1, RSSKB: 9_000_000, Command: "meta-orchestrator"},
		{PID: 200, PPID: 1, RSSKB: 3_000_000, Command: "gopls"},
	}}
	sig := newFakeSignaler()
	r := newTestReaper(t, sup, lister, sig)

	res, err := r.Reap(context.Background(), ReapRequest{MaxRSSMB: 1024})
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	for _, pid := range []int{10, 11} {
		if !contains(res.Protected, pid) {
			t.Errorf("role pid %d should be protected", pid)
		}
		if contains(res.Terminated, pid) {
			t.Errorf("role pid %d must not be terminated", pid)
		}
	}
	if !contains(res.Terminated, 200) {
		t.Errorf("non-role hog pid 200 should be terminated")
	}
}

// TestReaper_RSSFilterExcludesSmallProcesses asserts processes under the RSS
// threshold are not even candidates.
func TestReaper_RSSFilterExcludesSmallProcesses(t *testing.T) {
	sup := fakeSupervisor{roots: map[int]string{}}
	lister := fakeLister{procs: []ProcessInfo{
		{PID: 200, PPID: 1, RSSKB: 500_000, Command: "gopls"}, // ~488 MiB, under 1 GiB
	}}
	sig := newFakeSignaler()
	r := newTestReaper(t, sup, lister, sig)

	res, err := r.Reap(context.Background(), ReapRequest{MaxRSSMB: 1024})
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	if len(res.Candidates) != 0 {
		t.Errorf("process under RSS threshold should not be a candidate, got %v", res.Candidates)
	}
	if len(sig.events) != 0 {
		t.Errorf("nothing should be signalled, got %v", sig.events)
	}
}

// TestReaper_MinIdleFilter asserts idle-time gating: an active (non-idle)
// candidate is skipped when MinIdle is set, while an idle one is reaped.
func TestReaper_MinIdleFilter(t *testing.T) {
	sup := fakeSupervisor{roots: map[int]string{}}
	lister := fakeLister{procs: []ProcessInfo{
		{PID: 200, PPID: 1, RSSKB: 3_000_000, Command: "gopls", IdleSecs: 5},    // busy
		{PID: 201, PPID: 1, RSSKB: 3_000_000, Command: "gopls", IdleSecs: 3600}, // idle 1h
		{PID: 202, PPID: 1, RSSKB: 3_000_000, Command: "gopls", IdleSecs: 0},    // unknown idle
	}}
	sig := newFakeSignaler()
	r := newTestReaper(t, sup, lister, sig)

	res, err := r.Reap(context.Background(), ReapRequest{MaxRSSMB: 1024, MinIdle: 10 * time.Minute})
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	if contains(res.Candidates, 200) {
		t.Errorf("busy pid 200 should be filtered out by MinIdle")
	}
	if contains(res.Candidates, 202) {
		t.Errorf("unknown-idle pid 202 should be filtered out by MinIdle")
	}
	if !contains(res.Terminated, 201) {
		t.Errorf("idle pid 201 should be terminated")
	}
}

// TestExpandSubtree_DeepTree asserts the subtree walk protects grandchildren
// and tolerates a self-parented row without looping.
func TestExpandSubtree_DeepTree(t *testing.T) {
	roots := map[int]string{100: "s1"}
	procs := []ProcessInfo{
		{PID: 100, PPID: 1},
		{PID: 101, PPID: 100},
		{PID: 102, PPID: 101}, // grandchild
		{PID: 1, PPID: 1},     // self-parented init row
		{PID: 300, PPID: 1},   // unrelated
	}
	got := expandSubtree(roots, procs)
	for _, pid := range []int{100, 101, 102} {
		if _, ok := got[pid]; !ok {
			t.Errorf("pid %d should be in protected subtree", pid)
		}
	}
	if _, ok := got[300]; ok {
		t.Errorf("unrelated pid 300 must not be protected")
	}
}

// TestReaper_KillErrorIsBestEffort asserts a failed kill is recorded but does
// not abort the rest of the pass.
func TestReaper_KillErrorIsBestEffort(t *testing.T) {
	sup := fakeSupervisor{roots: map[int]string{}}
	lister := fakeLister{procs: []ProcessInfo{
		{PID: 200, PPID: 1, RSSKB: 3_000_000, Command: "gopls"},
		{PID: 201, PPID: 1, RSSKB: 3_000_000, Command: "gopls"},
	}}
	sig := newFakeSignaler()
	sig.termErr[200] = errors.New("no such process")
	sig.aliveAfterTerm[201] = false
	r := newTestReaper(t, sup, lister, sig)

	res, err := r.Reap(context.Background(), ReapRequest{MaxRSSMB: 1024})
	if err != nil {
		t.Fatalf("Reap: %v", err)
	}
	if len(res.Errors) != 1 {
		t.Errorf("expected 1 recorded error, got %v", res.Errors)
	}
	if !contains(res.Terminated, 201) {
		t.Errorf("pid 201 should still be terminated despite pid 200 failing")
	}
}
