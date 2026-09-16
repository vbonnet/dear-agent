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
