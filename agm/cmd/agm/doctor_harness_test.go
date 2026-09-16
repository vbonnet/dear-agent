package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestCheckHarnessHealth_DefaultIsNonGating verifies that the always-on
// claude-code default check (injected when no session uses it) is
// informational: it never fails the overall health check on its own,
// regardless of whether the claude binary is installed in the test
// environment.
func TestCheckHarnessHealth_DefaultIsNonGating(t *testing.T) {
	home := t.TempDir()
	if got := checkHarnessHealth(nil, home); !got {
		t.Errorf("checkHarnessHealth(nil) = false, want true (default check must not gate)")
	}

	// Empty-harness sessions are treated as claude-code, still the
	// (gating) default — but this case exercises the empty-string fallback,
	// not a missing-binary failure, so we only assert it does not panic.
	_ = checkHarnessHealth([]*manifest.Manifest{{Name: "s1"}, {Name: "s2"}}, home)
}

// TestCheckHarnessHealth_UnknownHarnessGates verifies that a live session
// pinned to an unrecognised harness fails the overall check. An unknown
// harness is unhealthy in every environment, so this is deterministic.
func TestCheckHarnessHealth_UnknownHarnessGates(t *testing.T) {
	manifests := []*manifest.Manifest{
		{Name: "weird", Harness: "definitely-not-a-real-harness"},
	}
	if got := checkHarnessHealth(manifests, t.TempDir()); got {
		t.Errorf("checkHarnessHealth with unknown in-use harness = true, want false")
	}
}

func TestCheckHarnessHealthUsesExplicitHome(t *testing.T) {
	retainedHome := t.TempDir()
	driftHome := t.TempDir()
	retainedCodexDir := filepath.Join(retainedHome, ".codex")
	if err := os.MkdirAll(retainedCodexDir, 0o700); err != nil {
		t.Fatalf("create retained Codex directory: %v", err)
	}
	binDir := t.TempDir()
	writeDoctorExecutable(t, filepath.Join(binDir, "codex"))
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", driftHome)
	t.Setenv("USERPROFILE", driftHome)
	t.Setenv("OPENAI_API_KEY", "")

	var healthy bool
	output := captureStdout(t, func() {
		healthy = checkHarnessHealth([]*manifest.Manifest{{Name: "c", Harness: "codex-cli"}}, retainedHome)
	})
	if !healthy {
		t.Fatalf("checkHarnessHealth() ignored explicit HOME; output:\n%s", output)
	}
	if !strings.Contains(output, retainedCodexDir) {
		t.Fatalf("health output %q does not contain retained config dir %q", output, retainedCodexDir)
	}
	if strings.Contains(output, driftHome) {
		t.Fatalf("health output %q contains drift HOME %q", output, driftHome)
	}
}
