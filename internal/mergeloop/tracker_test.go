package mergeloop

import (
	"path/filepath"
	"testing"
	"time"
)

func TestTrackerPersistAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)

	tr, err := LoadTracker("owner/repo", path)
	if err != nil {
		t.Fatalf("LoadTracker: %v", err)
	}
	tr.RecordAgentSpawn(42, "Build & Test", "mergeloop/pr-42", now)
	tr.RecordAction(42, StateCIFailing, now)
	tr.RecordAction(43, StateBehind, now)
	if err := tr.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := LoadTracker("owner/repo", path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := reloaded.Attempts(42); got != 1 {
		t.Errorf("attempts after reload = %d, want 1", got)
	}
	if got := reloaded.Get(43, now).LastRebaseAt; !got.Equal(now) {
		t.Errorf("last_rebase_at after reload = %v, want %v", got, now)
	}
}

func TestTrackerResetsOnNewFailureSignature(t *testing.T) {
	tr, _ := LoadTracker("owner/repo", filepath.Join(t.TempDir(), "s.json"))
	now := time.Now()
	tr.RecordAgentSpawn(1, "sigA", "s", now)
	tr.RecordAgentSpawn(1, "sigA", "s", now)
	if got := tr.Attempts(1); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	// A different failure is a fresh problem — counter resets to 1.
	tr.RecordAgentSpawn(1, "sigB", "s", now)
	if got := tr.Attempts(1); got != 1 {
		t.Errorf("attempts after new sig = %d, want 1", got)
	}
}

func TestTrackerMultiRepoIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	now := time.Now()
	a, _ := LoadTracker("owner/a", path)
	a.RecordAgentSpawn(1, "x", "s", now)
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	b, _ := LoadTracker("owner/b", path)
	b.RecordAgentSpawn(1, "y", "s", now)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	// Reloading repo a must still see its record (b's Save must not clobber it).
	a2, _ := LoadTracker("owner/a", path)
	if got := a2.Attempts(1); got != 1 {
		t.Errorf("repo a attempts = %d, want 1 (b clobbered it?)", got)
	}
}

func TestForgetDropsRecord(t *testing.T) {
	tr, _ := LoadTracker("owner/repo", filepath.Join(t.TempDir(), "s.json"))
	now := time.Now()
	tr.RecordAgentSpawn(5, "x", "s", now)
	tr.Forget(5)
	if got := tr.Attempts(5); got != 0 {
		t.Errorf("attempts after forget = %d, want 0", got)
	}
}

func TestTrackerResetAttemptsIfPassing(t *testing.T) {
	tr, _ := LoadTracker("owner/repo", filepath.Join(t.TempDir(), "s.json"))
	now := time.Now()
	tr.RecordAgentSpawn(1, "sigA", "s", now)
	tr.RecordAgentSpawn(1, "sigA", "s", now)
	if got := tr.Attempts(1); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	tr.ResetAttemptsIfPassing(1)
	if got := tr.Attempts(1); got != 0 {
		t.Errorf("attempts after ResetAttemptsIfPassing = %d, want 0", got)
	}
	// Subsequent failure with same signature sigA starts at attempt 1 because failure sig was cleared
	tr.ResetAttemptsIfSigChanged(1, "sigA")
	if got := tr.Attempts(1); got != 0 {
		t.Errorf("attempts = %d, want 0", got)
	}
}
