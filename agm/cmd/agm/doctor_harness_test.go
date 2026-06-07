package main

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestCheckHarnessHealth_DefaultIsNonGating verifies that the always-on
// claude-code default check (injected when no session uses it) is
// informational: it never fails the overall health check on its own,
// regardless of whether the claude binary is installed in the test
// environment.
func TestCheckHarnessHealth_DefaultIsNonGating(t *testing.T) {
	if got := checkHarnessHealth(nil); !got {
		t.Errorf("checkHarnessHealth(nil) = false, want true (default check must not gate)")
	}

	// Empty-harness sessions are treated as claude-code, still the
	// (gating) default — but this case exercises the empty-string fallback,
	// not a missing-binary failure, so we only assert it does not panic.
	_ = checkHarnessHealth([]*manifest.Manifest{{Name: "s1"}, {Name: "s2"}})
}

// TestCheckHarnessHealth_UnknownHarnessGates verifies that a live session
// pinned to an unrecognised harness fails the overall check. An unknown
// harness is unhealthy in every environment, so this is deterministic.
func TestCheckHarnessHealth_UnknownHarnessGates(t *testing.T) {
	manifests := []*manifest.Manifest{
		{Name: "weird", Harness: "definitely-not-a-real-harness"},
	}
	if got := checkHarnessHealth(manifests); got {
		t.Errorf("checkHarnessHealth with unknown in-use harness = true, want false")
	}
}
