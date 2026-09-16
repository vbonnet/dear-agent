package main

import "testing"

// TestDryRunPredictionsGateSimulation pins the review finding that a --dry-run
// tick reported no would-be merge for exactly the PRs the new resolver flow
// exists to unblock.
//
// In dry-run the resolver only prints "would resolve" and leaves advisory
// threads open. safe-merge's real unresolved-thread gate then still sees them
// and returns ErrNotReady, so the tick reported merged=0 even though the
// equivalent real tick resolves those threads first and merges. The prediction
// has to be carried into the dry-run gate simulation, but only when nothing
// else would keep the gate shut.
func TestDryRunPredictionsGateSimulation(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		resolvable, withheld, other int
		wantSkip                    bool
	}{
		{"all advisory and resolvable", 3, 0, 0, true},
		{"a blocking finding remains", 3, 1, 0, false},
		{"a human thread remains", 3, 0, 1, false},
		{"nothing to resolve", 0, 0, 0, false},
		{"only a human thread", 0, 0, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newDryRunThreadPredictions()
			p.note(42, tc.resolvable, tc.withheld, tc.other)
			if got := p.wouldClearThreadGate(42); got != tc.wantSkip {
				t.Errorf("wouldClearThreadGate = %v, want %v", got, tc.wantSkip)
			}
		})
	}

	// A PR the resolver never saw must never be treated as predicted-clear.
	p := newDryRunThreadPredictions()
	if p.wouldClearThreadGate(99) {
		t.Error("an unseen PR must not be reported as clearing the thread gate")
	}
}

// TestUnresolvedNotOursCount pins the review finding that deriving the
// not-ours count by subtraction counted ALREADY-RESOLVED threads as blockers.
//
// threadResolvability maps a resolved thread to resolvabilityNotOurs, and the
// listing includes resolved threads, so len(threads)-len(resolvable)-withheld
// charged the predictor for threads that cannot block anything. A PR with one
// resolved thread and one advisory thread was therefore reported as no
// would-be merge, although a real tick resolves the advisory one and proceeds.
func TestUnresolvedNotOursCount(t *testing.T) {
	bot := []threadComment{{author: "chatgpt-codex-connector", body: "nit: advisory only"}}
	human := []threadComment{{author: "vbonnet", body: "please change this"}}
	threads := []reviewThread{
		{id: "resolved-advisory", isResolved: true, comments: bot},
		{id: "open-advisory", comments: bot},
	}
	if got := unresolvedNotOurs(threads); got != 0 {
		t.Errorf("unresolvedNotOurs = %d, want 0: a resolved thread cannot block a merge", got)
	}

	threads = append(threads, reviewThread{id: "open-human", comments: human})
	if got := unresolvedNotOurs(threads); got != 1 {
		t.Errorf("unresolvedNotOurs = %d, want 1: the open human thread still blocks", got)
	}
}

// TestPredictionsClearedBeforeEachPass pins the review finding that the
// prediction map is reused across ticks in `mergeloop run --dry-run`.
//
// If a PR was predicted clear on one tick and the next ResolveBotThreads call
// fails before note() updates the entry, a later read still saw the stale
// true and skipped the unresolved-thread check, so the run could report a
// would-be merge over review feedback that had appeared in between. Clearing
// before each pass makes the failure mode fail closed.
func TestPredictionsClearedBeforeEachPass(t *testing.T) {
	p := newDryRunThreadPredictions()
	p.note(7, 2, 0, 0)
	if !p.wouldClearThreadGate(7) {
		t.Fatalf("precondition: PR 7 should be predicted clear")
	}
	// Next tick begins; the resolver clears before it re-decides, then fails
	// before it can note a new verdict.
	p.clear(7)
	if p.wouldClearThreadGate(7) {
		t.Error("a stale prediction survived into the next pass: the gate must fail closed")
	}
}
