package main

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/diskledger"
)

func TestExitForSilentGCIsDown(t *testing.T) {
	h := diskledger.ReclaimHealth{Status: diskledger.StatusSilentGC}
	if got := exitFor(h, nil); got != exitDown {
		t.Errorf("exitFor(silent_gc) = %d, want %d", got, exitDown)
	}
}

func TestExitForPressureIsDegraded(t *testing.T) {
	h := diskledger.ReclaimHealth{Status: diskledger.StatusPressure}
	if got := exitFor(h, nil); got != exitDegraded {
		t.Errorf("exitFor(pressure) = %d, want %d", got, exitDegraded)
	}
}

func TestExitForLeaksIsDegradedEvenWhenRootsAreFine(t *testing.T) {
	h := diskledger.ReclaimHealth{Status: diskledger.StatusOK}
	leaks := []diskledger.Record{{RunID: "r", LeakedBytes: 1 << 20}}
	if got := exitFor(h, leaks); got != exitDegraded {
		t.Errorf("exitFor(ok + leaks) = %d, want %d", got, exitDegraded)
	}
}

func TestExitForCleanIsHealthy(t *testing.T) {
	h := diskledger.ReclaimHealth{Status: diskledger.StatusOK}
	if got := exitFor(h, nil); got != exitHealthy {
		t.Errorf("exitFor(ok) = %d, want %d", got, exitHealthy)
	}
}

// An absent gc.jsonl must read as "reclaimed nothing", not as "reclaim is
// fine". Assuming success from a missing log is how a dead collector stays
// invisible.
func TestReclaimedFromMissingGCLogIsZero(t *testing.T) {
	if got := reclaimedFromGCLog("/nonexistent/gc.jsonl", time.Hour); got != 0 {
		t.Errorf("reclaimedFromGCLog(missing) = %d, want 0", got)
	}
}
