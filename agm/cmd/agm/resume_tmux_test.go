package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func requireCodexResumeTmuxIntegration(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping isolated tmux integration test in short mode")
	}
	if os.Getenv("CI_SKIP_TMUX") == "true" {
		t.Skip("skipping isolated tmux integration test because CI_SKIP_TMUX=true")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not available")
	}
}

// setupRegressionSocket keeps isolated tmux socket paths below the macOS Unix
// socket length limit.
func setupRegressionSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "agm") //nolint:usetesting // t.TempDir paths exceed the tmux socket limit on macOS
	if err != nil {
		t.Fatalf("setup isolated tmux directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove isolated tmux directory: %v", err)
		}
	})
	socketPath := filepath.Join(dir, "agm.sock")
	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", socketPath, "kill-server").Run()
	})
	return socketPath
}
