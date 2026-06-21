package main

import "slices"

// Progressive-disclosure verbosity policy.
//
// Agent-mode (ModeAgent) is terse by default: identifier fields (UUIDs/internal
// IDs) are stripped, sandbox/project paths are basenamed, and success
// suggestions / next-step hints are dropped — all to save tokens for machine
// consumers (see ce-s2tn.1/.2/.3/.4).
//
// The --detailed flag is the inverse knob: an agent that occasionally needs full
// identifiers or paths can opt back into the richer output without dropping to
// human-mode. Human-mode (ModeHuman) always emits the full, verbose output and
// is unaffected by --detailed.
//
// These helpers are the single decision points the output path consults so the
// policy lives in one place instead of being re-derived at each call site.

// includeID reports whether identifier fields (UUIDs, internal IDs) should be
// included in the output. Human-mode always includes them. Agent-mode strips
// them by default, restoring them only when the caller explicitly requests the
// "id" field via --fields or opts into --detailed.
func includeID(fields []string, detailed bool, mode OutputMode) bool {
	if mode == ModeHuman {
		return true
	}
	return detailed || slices.Contains(fields, "id")
}

// useFullPath reports whether sandbox/project paths should be printed in full
// rather than basenamed. Human-mode always uses full paths; agent-mode
// basenames them by default and restores them under --detailed.
func useFullPath(detailed bool, mode OutputMode) bool {
	return mode == ModeHuman || detailed
}

// includeHints reports whether success suggestions / next-step hints should be
// emitted. Human-mode always includes them; agent-mode drops them by default
// and restores them under --detailed.
func includeHints(detailed bool, mode OutputMode) bool {
	return mode == ModeHuman || detailed
}
