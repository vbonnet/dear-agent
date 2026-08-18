package main

import (
	"fmt"
	"io"
)

// The -max-dispatch cap bounds how many beads one run may place, for a
// scheduled/unattended loop that wants a blast radius of its own rather than
// whatever spawn backpressure happens to allow.
//
// Two properties make it a cap rather than a suggestion:
//
// It counts SUCCESSFUL dispatches, not list positions. Truncating the candidate
// list up front would let a bead that can never spawn — a deterministic
// per-bead spawn failure, the ce-b1zw class — consume the whole budget: with
// -max-dispatch=1 a poisoned P0 would be selected, skipped, and dispatch zero,
// then repeat identically on every scheduled run, starving the queue forever.
// Counting successes means a skipped bead costs nothing and the run walks on.
//
// It refuses a negative value instead of reading it as unlimited. A safety cap
// that goes inert when misconfigured is a cap in name only; only the documented
// 0 disables it.

// validateMaxDispatch rejects a negative cap. 0 is the documented "unlimited"
// default; a negative value (-1 reads as unlimited in plenty of other tools) is
// a misconfiguration and must fail loudly rather than silently uncap the run.
func validateMaxDispatch(maxDispatch int) error {
	if maxDispatch < 0 {
		return fmt.Errorf("-max-dispatch must be >= 0 (0 = unlimited), got %d", maxDispatch)
	}
	return nil
}

// dispatchCapReached reports whether the run has already placed its full
// -max-dispatch budget, and if so reports the eligible beads it is deferring so
// a scheduled run's telemetry shows the work left behind rather than silently
// stopping short. A non-positive maxDispatch is unlimited and never reached.
func dispatchCapReached(maxDispatch, dispatched, remaining int, errOut io.Writer) bool {
	if maxDispatch <= 0 || dispatched < maxDispatch {
		return false
	}
	fmt.Fprintf(errOut,
		"vroom-dispatch-direct: -max-dispatch=%d reached, deferring %d remaining eligible bead(s) to a later run\n",
		maxDispatch, remaining)
	return true
}
