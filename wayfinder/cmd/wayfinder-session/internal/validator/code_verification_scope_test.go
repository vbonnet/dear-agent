package validator

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeBrokenGoProject creates a module whose build and tests cannot possibly
// succeed, so any phase that actually shells out to the toolchain will fail.
func writeBrokenGoProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "go.mod"), "module broken\n\ngo 1.24\n")
	mustWrite(t, filepath.Join(dir, "broken.go"), "package broken\n\nthis is not go\n")
	return dir
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestGate9SkipsToolchainOutsideBuild pins ADR-001's scope: the build and test
// commands verify BUILD completion. Before this, every phase shelled out to
// `go build ./...` and `go test ./...` across the whole repository, so
// completing a CHARTER phase in a monorepo ran the entire test suite.
func TestGate9SkipsToolchainOutsideBuild(t *testing.T) {
	t.Parallel()

	dir := writeBrokenGoProject(t)

	for _, phase := range []string{"CHARTER", "PROBLEM", "DESIGN", "SPEC", "PLAN", "SETUP", "RETRO"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()

			start := time.Now()
			if err := validateCodeDeliverables(phase, dir); err != nil {
				t.Fatalf("validateCodeDeliverables(%s) = %v, want nil: the toolchain must not run for a phase with no code deliverables", phase, err)
			}
			if elapsed := time.Since(start); elapsed > 5*time.Second {
				t.Errorf("validateCodeDeliverables(%s) took %s; it should not shell out at all", phase, elapsed)
			}
		})
	}
}

// TestGate9StillFailsClosedOnBuild proves the narrowed scope did not neuter the
// gate: BUILD still runs the real commands and still refuses a broken tree.
func TestGate9StillFailsClosedOnBuild(t *testing.T) {
	t.Parallel()

	dir := writeBrokenGoProject(t)

	err := validateCodeDeliverables("BUILD", dir)
	if err == nil {
		t.Fatal("validateCodeDeliverables(BUILD) on a non-compiling module = nil, want a Gate 9 failure")
	}
	if !contains(err.Error(), "Gate 9") {
		t.Errorf("error should name Gate 9; got %q", err.Error())
	}
}
