package main

import "sync"

// dryRunThreadPredictions carries what the thread resolver WOULD have done into
// the dry-run merge-gate simulation.
//
// In a real tick the resolver resolves advisory bot threads and safe-merge's
// unresolved-thread gate then passes. In --dry-run the resolver only reports
// what it would resolve and leaves the threads open, so that same gate still
// saw them and returned ErrNotReady. The dry-run therefore claimed no merge for
// exactly the PRs whose only obstacle was advisory threads the loop resolves
// itself, which is the flow the reader is running --dry-run to inspect.
//
// This never affects a real merge: it is consulted only when dryRun is set, and
// in that mode no merge is executed.
type dryRunThreadPredictions struct {
	mu   sync.Mutex
	seen map[int]bool
}

func newDryRunThreadPredictions() *dryRunThreadPredictions {
	return &dryRunThreadPredictions{seen: map[int]bool{}}
}

// note records one PR's partition of its UNRESOLVED threads: those this loop
// would resolve, those withheld on severity, and those that are not ours to
// touch (human threads, already resolved, unreadable).
//
// The gate only clears when every unresolved thread would be resolved. A
// withheld finding keeps it shut by design, and so does a human thread: neither
// disappears because this tick ran.
func (p *dryRunThreadPredictions) note(pr, resolvable, withheld, other int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seen[pr] = resolvable > 0 && withheld == 0 && other == 0
}

// wouldClearThreadGate reports whether the unresolved-thread gate would be
// clear for this PR once the predicted resolutions were applied. A PR the
// resolver never reached is never predicted-clear.
func (p *dryRunThreadPredictions) wouldClearThreadGate(pr int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.seen[pr]
}
