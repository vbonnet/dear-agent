package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

func setupRealReadinessTmux(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping isolated tmux readiness integration in short mode")
	}
	if os.Getenv("CI_SKIP_TMUX") == "true" {
		t.Skip("skipping isolated tmux readiness integration because CI_SKIP_TMUX=true")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not available")
	}
	dir, err := os.MkdirTemp("", "agm-ready") //nolint:usetesting // macOS Unix socket paths must stay short
	if err != nil {
		t.Fatalf("create short socket directory: %v", err)
	}
	socketPath := filepath.Join(dir, "agm.sock")
	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", socketPath, "kill-server").Run()
		_ = os.RemoveAll(dir)
	})
	return socketPath
}

func TestRealTmuxReadinessDetectsFakeCodexComposer(t *testing.T) {
	setupRealReadinessTmux(t)
	sessionName := "real-ready-codex"
	if err := tmux.NewSession(sessionName, t.TempDir()); err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	t.Cleanup(func() { tmux.KillSession(sessionName) })

	realTmux := NewRealTmux()
	before, err := realTmux.CheckInputReadiness(sessionName, "codex-cli")
	if err != nil {
		t.Fatalf("CheckInputReadiness(shell) error = %v", err)
	}
	if before.Ready {
		t.Fatalf("bare shell classified ready: %#v", before)
	}

	if err := tmux.SendCommand(sessionName, "printf 'OpenAI Codex\\n/model to change\\n'; sleep 10"); err != nil {
		t.Fatalf("SendCommand(fake Codex) error = %v", err)
	}
	if err := realTmux.WaitForHarnessReady(sessionName, "codex-cli", 3*time.Second); err != nil {
		t.Fatalf("WaitForHarnessReady(fake Codex) error = %v", err)
	}
	after, err := realTmux.CheckInputReadiness(sessionName, "codex-cli")
	if err != nil {
		t.Fatalf("CheckInputReadiness(composer) error = %v", err)
	}
	if !after.Ready || after.State != "YES" {
		t.Fatalf("fake Codex composer readiness = %#v, want ready YES", after)
	}
}

func TestRealTmuxInputReadinessReportsMissingSession(t *testing.T) {
	setupRealReadinessTmux(t)

	readiness, err := NewRealTmux().CheckInputReadiness("missing-readiness-session", "codex-cli")
	if err != nil {
		t.Fatalf("CheckInputReadiness() error = %v", err)
	}
	if readiness.Ready || readiness.State != "NOT_FOUND" {
		t.Fatalf("missing session readiness = %#v, want NOT_FOUND", readiness)
	}
}

func TestRealTmuxWaitForHarnessReadyRejectsUnknownHarness(t *testing.T) {
	err := NewRealTmux().WaitForHarnessReady("unused", "unknown", time.Second)
	if err == nil {
		t.Fatal("WaitForHarnessReady() accepted an unknown harness")
	}
}
