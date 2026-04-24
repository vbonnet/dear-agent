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
	binPath := filepath.Join(t.TempDir(), "wayfinder")
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
}

func TestMainSmoke_Help(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping smoke test in short mode")
	}
	binPath := filepath.Join(t.TempDir(), "wayfinder")
	build := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	helpCmd := exec.Command(binPath, "--help")
	out, err := helpCmd.CombinedOutput()
	if err != nil {
		if len(out) == 0 {
			t.Fatalf("--help produced no output: %v", err)
		}
	}
	if len(out) == 0 {
		t.Fatal("--help produced no output")
	}
}
