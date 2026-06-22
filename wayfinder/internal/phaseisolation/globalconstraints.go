package phaseisolation

import (
	"strings"
)

// GlobalConstraint pairs a spec-wide invariant with a short rationale.
type GlobalConstraint struct {
	Rule string // The invariant that holds across every phase and task
	Why  string // Why it matters (kept terse for prompt economy)
}

// globalConstraints lists the invariants that apply across all phases and
// tasks of a wayfinder plan. They are hoisted into a single "Global
// Constraints" block at the top of every plan template so cross-task
// behavior stays consistent and the same rules don't have to be re-stated
// in every per-step prompt.
//
// Keep this list short and genuinely global: a constraint earns a place
// here only if violating it in any phase would break the plan. Phase- or
// task-specific guidance belongs in that phase's definition, not here.
//
// It is unexported so callers cannot mutate this shared slice; reach it
// through GetGlobalConstraints, which hands back a defensive copy.
var globalConstraints = []GlobalConstraint{
	{
		Rule: "Never write to the golden source checkout under `~/src/`; do all work in a dedicated worktree.",
		Why:  "The source checkouts are read-only references shared across sessions; mutating them corrupts every other worker's view.",
	},
	{
		Rule: "Always run git network commands with `GIT_TERMINAL_PROMPT=0` (and a timeout).",
		Why:  "An interactive credential prompt has no terminal to answer it and silently hangs the phase until it times out.",
	},
	{
		Rule: "Session names must contain no dots (`.`); separate segments with hyphens.",
		Why:  "Dots collide with the session-addressing scheme and route messages to the wrong session.",
	},
	{
		Rule: "Never bypass checks with `--no-verify` or rewrite shared history with `--force`.",
		Why:  "These skip the safety gates the plan depends on and can destroy work other phases already built on.",
	},
}

// GetGlobalConstraints returns a copy of the spec-wide invariants applied to
// every plan template. Returns nil if none are defined. The copy keeps the
// shared backing slice read-only for callers.
func GetGlobalConstraints() []GlobalConstraint {
	if len(globalConstraints) == 0 {
		return nil
	}
	out := make([]GlobalConstraint, len(globalConstraints))
	copy(out, globalConstraints)
	return out
}

// FormatGlobalConstraints renders the constraints as a markdown bullet list
// for injection into phase/plan templates. Each bullet states the rule in
// bold followed by its rationale. Returns "" when there are no constraints.
func FormatGlobalConstraints(constraints []GlobalConstraint) string {
	if len(constraints) == 0 {
		return ""
	}

	var b strings.Builder
	for i, c := range constraints {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("- **")
		b.WriteString(c.Rule)
		b.WriteString("** ")
		b.WriteString(c.Why)
	}
	return b.String()
}
