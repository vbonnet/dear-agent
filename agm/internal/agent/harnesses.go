package agent

import "slices"

// activeHarnesses is the canonical parity set. Claude Code is the reference
// implementation; Codex, AGY, OpenCode, and Pi must match its core outcomes.
var activeHarnesses = []string{"claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli"}

// deprecatedHarnesses remain accepted for backward compatibility but are not
// part of active parity promises or new user-facing defaults.
var deprecatedHarnesses = []string{"gemini-cli"}

// ActiveHarnesses returns the canonical active parity harnesses.
func ActiveHarnesses() []string {
	return append([]string(nil), activeHarnesses...)
}

// DeprecatedHarnesses returns supported legacy harnesses.
func DeprecatedHarnesses() []string {
	return append([]string(nil), deprecatedHarnesses...)
}

// KnownHarnesses returns active harnesses followed by deprecated harnesses.
func KnownHarnesses() []string {
	out := ActiveHarnesses()
	out = append(out, deprecatedHarnesses...)
	return out
}

// IsDeprecatedHarness reports whether a normalized harness is legacy-only.
func IsDeprecatedHarness(name string) bool {
	name = NormalizeHarnessName(name)
	return slices.Contains(deprecatedHarnesses, name)
}
