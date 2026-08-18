package session

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

var _ StrictSessionExistenceChecker = (*RealTmux)(nil)

func TestNewRealTmux(t *testing.T) {
	rt := NewRealTmux()
	if rt == nil {
		t.Fatal("NewRealTmux() returned nil")
	}
}

func TestRealTmux_HasSession_NonExistent(t *testing.T) {
	rt := NewRealTmux()
	exists, err := rt.HasSession("agm-test-nonexistent-xyz-99999")
	if err != nil {
		// tmux may not be available in CI, just skip
		t.Skipf("tmux not available: %v", err)
	}
	if exists {
		t.Error("non-existent session should not exist")
	}
}

func TestRealTmux_ListSessions(t *testing.T) {
	rt := NewRealTmux()
	sessions, err := rt.ListSessions()
	if err != nil {
		t.Skipf("tmux not available: %v", err)
	}
	// sessions can be empty or have entries; just check no panic
	_ = sessions
}

func TestRealTmux_ListSessionsWithInfo(t *testing.T) {
	rt := NewRealTmux()
	sessions, err := rt.ListSessionsWithInfo()
	if err != nil {
		t.Skipf("tmux not available: %v", err)
	}
	_ = sessions
}

func TestRealTmux_ListClients(t *testing.T) {
	rt := NewRealTmux()
	// List clients for a nonexistent session - should not panic
	clients, err := rt.ListClients("agm-test-nonexistent-xyz-99999")
	if err != nil {
		// Expected - session doesn't exist
		return
	}
	_ = clients
}

func TestRealTmux_KillSessionIsIdempotentWhenTargetDisappears(t *testing.T) {
	rt := NewRealTmux()
	suffix := time.Now().UnixNano()
	keeper := fmt.Sprintf("agm-real-tmux-keeper-%d", suffix)
	target := fmt.Sprintf("agm-real-tmux-kill-%d", suffix)
	workdir := t.TempDir()
	if err := rt.CreateSession(keeper, workdir); err != nil {
		t.Skipf("tmux not available: %v", err)
	}
	t.Cleanup(func() { _ = rt.KillSession(keeper) })
	if err := rt.CreateSession(target, workdir); err != nil {
		t.Fatalf("CreateSession(%q): %v", target, err)
	}
	t.Cleanup(func() { _ = rt.KillSession(target) })

	if err := rt.KillSession(target); err != nil {
		t.Fatalf("first KillSession(%q): %v", target, err)
	}
	if err := rt.KillSession(target); err != nil {
		t.Fatalf("idempotent KillSession(%q): %v", target, err)
	}
}

func TestRealTmux_KillLastSessionVerifiesServerShutdownAsAbsence(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not available")
	}
	dir, err := os.MkdirTemp("", "agm-last-session") //nolint:usetesting // keep Unix socket path short on macOS
	if err != nil {
		t.Fatalf("create short socket directory: %v", err)
	}
	socketPath := filepath.Join(dir, "tmux.sock")
	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", socketPath, "kill-server").Run()
		_ = os.RemoveAll(dir)
	})

	rt := NewRealTmux()
	const target = "agm-real-tmux-last-session"
	if err := rt.CreateSession(target, t.TempDir()); err != nil {
		t.Fatalf("CreateSession(%q): %v", target, err)
	}
	if err := rt.KillSession(target); err != nil {
		t.Fatalf("KillSession(%q): %v", target, err)
	}
	exists, err := rt.HasSessionStrict(t.Context(), target)
	if err != nil || exists {
		t.Fatalf("last-session postcondition = (exists=%v err=%v), want verified absence", exists, err)
	}
}
