package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMainSmoke_Build(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping smoke test in short mode")
	}
	binPath := filepath.Join(t.TempDir(), "agm-daemon")
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
}

// Note: agm-daemon has no --help flag (no cobra/flag help support).
// It starts a long-running daemon process, so we only verify it builds.
