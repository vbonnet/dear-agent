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
	binPath := filepath.Join(t.TempDir(), "agm-mcp-server")
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
}

func TestMainSmoke_Help(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping smoke test in short mode")
	}
	binPath := filepath.Join(t.TempDir(), "agm-mcp-server")
	build := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	// flag package exits 2 on -help, so we just check it produces output
	helpCmd := exec.Command(binPath, "-help")
	out, _ := helpCmd.CombinedOutput()
	if len(out) == 0 {
		t.Fatal("-help produced no output")
	}
}
