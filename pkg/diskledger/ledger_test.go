package diskledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testLedger(t *testing.T) (*Ledger, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "diskledger.jsonl")
	l, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return l, path
}

func readRecords(t *testing.T, path string) []Record {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	var out []Record
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("unmarshal %q: %v", line, err)
		}
		out = append(out, r)
	}
	return out
}

func TestOpenRecordsMeasuredBaseline(t *testing.T) {
	l, path := testLedger(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.bin"), 4096)

	if err := l.Open(Run{RunID: "run-1", Owner: "sess-a", Kind: KindSandbox, Path: dir}); err != nil {
		t.Fatalf("Open: %v", err)
	}

	recs := readRecords(t, path)
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	if recs[0].Op != OpOpen {
		t.Errorf("Op = %q, want %q", recs[0].Op, OpOpen)
	}
	if recs[0].RunID != "run-1" || recs[0].Owner != "sess-a" {
		t.Errorf("identity = %q/%q, want run-1/sess-a", recs[0].RunID, recs[0].Owner)
	}
	if recs[0].Usage.Files != 1 {
		t.Errorf("baseline Files = %d, want 1", recs[0].Usage.Files)
	}
}

// The whole point of the ledger: a run that deletes what it made leaks zero,
// and a run that leaves bytes behind is charged for exactly those bytes.
func TestCloseChargesOnlyUnreleasedBytes(t *testing.T) {
	l, path := testLedger(t)
	dir := t.TempDir()
	run := Run{RunID: "run-2", Owner: "sess-b", Kind: KindScratch, Path: dir}

	if err := l.Open(run); err != nil {
		t.Fatalf("Open: %v", err)
	}
	writeFile(t, filepath.Join(dir, "big.bin"), 1<<20)
	if err := l.Close("run-2"); err != nil {
		t.Fatalf("Close: %v", err)
	}

	recs := readRecords(t, path)
	last := recs[len(recs)-1]
	if last.Op != OpClose {
		t.Fatalf("Op = %q, want %q", last.Op, OpClose)
	}
	if last.LeakedBytes <= 0 {
		t.Errorf("LeakedBytes = %d, want > 0 (run left 1MiB behind)", last.LeakedBytes)
	}
}

func TestCloseAfterCleanupLeaksNothing(t *testing.T) {
	l, path := testLedger(t)
	parent := t.TempDir()
	dir := filepath.Join(parent, "work")
	writeFile(t, filepath.Join(dir, "big.bin"), 1<<20)

	if err := l.Open(Run{RunID: "run-3", Owner: "sess-c", Kind: KindSandbox, Path: dir}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if err := l.Close("run-3"); err != nil {
		t.Fatalf("Close: %v", err)
	}

	last := readRecords(t, path)[1]
	if last.LeakedBytes != 0 {
		t.Errorf("LeakedBytes = %d, want 0 after full cleanup", last.LeakedBytes)
	}
	if !last.Usage.Missing {
		t.Error("closed usage should record Missing for a removed path")
	}
}

func TestCloseUnknownRunIsAnError(t *testing.T) {
	l, _ := testLedger(t)
	if err := l.Close("never-opened"); err == nil {
		t.Error("Close on unknown run should error, got nil")
	}
}

// Crash safety: a run killed by SIGKILL never calls Close. Those entries are
// the leaks that matter most, and they are only findable by reconciling open
// entries against session liveness -- a defer/trap cannot help here.
func TestReconcileAttributesOrphanedRunsToTheirOwner(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "leaked.bin"), 1<<20)

	open := []Record{{
		Op: OpOpen, RunID: "run-dead", Owner: "sess-dead", Kind: KindSandbox,
		Path: dir, At: time.Now().Add(-2 * time.Hour),
	}}
	alive := func(owner string) bool { return owner != "sess-dead" }

	leaks := Reconcile(open, alive, time.Hour)
	if len(leaks) != 1 {
		t.Fatalf("got %d leaks, want 1", len(leaks))
	}
	if leaks[0].Owner != "sess-dead" {
		t.Errorf("Owner = %q, want sess-dead", leaks[0].Owner)
	}
	if leaks[0].LeakedBytes <= 0 {
		t.Errorf("LeakedBytes = %d, want > 0", leaks[0].LeakedBytes)
	}
}

func TestReconcileSpareLiveOwners(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "inflight.bin"), 1<<20)

	open := []Record{{
		Op: OpOpen, RunID: "run-live", Owner: "sess-live", Kind: KindSandbox,
		Path: dir, At: time.Now().Add(-2 * time.Hour),
	}}
	alive := func(string) bool { return true }

	if leaks := Reconcile(open, alive, time.Hour); len(leaks) != 0 {
		t.Errorf("got %d leaks, want 0 for a live owner", len(leaks))
	}
}

// A young entry belongs to a run that may simply still be working. Charging it
// as a leak is how a reaper starts deleting live work.
func TestReconcileRespectsGracePeriod(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "fresh.bin"), 1<<20)

	open := []Record{{
		Op: OpOpen, RunID: "run-young", Owner: "sess-dead", Kind: KindSandbox,
		Path: dir, At: time.Now().Add(-1 * time.Minute),
	}}
	alive := func(string) bool { return false }

	if leaks := Reconcile(open, alive, time.Hour); len(leaks) != 0 {
		t.Errorf("got %d leaks, want 0 inside the grace period", len(leaks))
	}
}

// An entry that was closed must never be reconciled again as an orphan.
func TestOpenEntriesExcludesClosedRuns(t *testing.T) {
	recs := []Record{
		{Op: OpOpen, RunID: "a", Owner: "s1", Path: "/x"},
		{Op: OpOpen, RunID: "b", Owner: "s2", Path: "/y"},
		{Op: OpClose, RunID: "a", Owner: "s1", Path: "/x"},
	}
	open := OpenEntries(recs)
	if len(open) != 1 || open[0].RunID != "b" {
		t.Errorf("OpenEntries = %+v, want only run b", open)
	}
}
