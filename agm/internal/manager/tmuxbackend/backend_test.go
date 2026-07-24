package tmuxbackend

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manager"
)

// TestBackendIdentity verifies the pure-function properties of TmuxBackend
// that do not require an actual tmux installation.
func TestBackendName(t *testing.T) {
	b := New()
	if b.Name() != "tmux" {
		t.Errorf("Name = %q, want tmux", b.Name())
	}
}

func TestBackendCapabilities(t *testing.T) {
	b := New()
	caps := b.Capabilities()
	if !caps.SupportsAttach {
		t.Error("SupportsAttach should be true for tmux backend")
	}
	if caps.SupportsStructuredIO {
		t.Error("SupportsStructuredIO should be false (tmux uses terminal scraping)")
	}
	if !caps.SupportsInterrupt {
		t.Error("SupportsInterrupt should be true for tmux backend")
	}
}

// TestBackendImplementsInterface is a compile-time check that *TmuxBackend
// satisfies the manager.Backend and manager.AttachableBackend interfaces.
func TestBackendImplementsInterface(t *testing.T) {
	var _ manager.Backend = (*TmuxBackend)(nil)
	var _ manager.AttachableBackend = (*TmuxBackend)(nil)
}

func TestTerminateSessionPropagatesTmuxFailure(t *testing.T) {
	wantErr := errors.New("kill denied")
	b := &TmuxBackend{killSession: func(name string) error {
		if name != "exact-target" {
			t.Fatalf("kill target = %q, want exact-target", name)
		}
		return wantErr
	}}

	err := b.TerminateSession(context.Background(), manager.SessionID("exact-target"))
	if !errors.Is(err, wantErr) {
		t.Fatalf("TerminateSession() error = %v, want %v", err, wantErr)
	}
}

func TestTerminateSessionReturnsSuccessAfterTmuxKill(t *testing.T) {
	called := false
	b := &TmuxBackend{killSession: func(name string) error {
		called = true
		return nil
	}}

	if err := b.TerminateSession(context.Background(), manager.SessionID("target")); err != nil {
		t.Fatalf("TerminateSession() error = %v", err)
	}
	if !called {
		t.Fatal("TerminateSession() did not invoke the tmux mutation")
	}
}

func TestBackendStateAndDeliveryPreserveCurrentCodexWelcomeGhostStyle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real tmux state integration in short mode")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not available")
	}
	dir, err := os.MkdirTemp("", "agm-backend-state") //nolint:usetesting // macOS Unix socket paths must stay short
	if err != nil {
		t.Fatalf("create short socket directory: %v", err)
	}
	socketPath := filepath.Join(dir, "agm.sock")
	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", socketPath, "kill-server").Run()
		_ = os.RemoveAll(dir)
	})

	const sessionName = "backend-state-codex-ghost"
	script := "printf '\\033[2m│ >_ \\033[0;1mOpenAI Codex\\033[0;2m (v0.145.0) │\\033[0m\\n" +
		"\\033[2m│ model: \\033[0mgpt-5.6 high\\033[2m \\033[0m/model to change │\\n" +
		"To get started, describe a task or try /review\\n\\n" +
		"\\033[1m›\\033[0m \\033[2mRun /review on my current changes\\033[0m\\n\\n" +
		"gpt-5.6 high · ~/src/project\\n'; sleep 30"
	if output, err := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", sessionName, "sh", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("create styled Codex fixture: %v\n%s", err, output)
	}

	backend := New()
	deadline := time.Now().Add(3 * time.Second)
	for {
		got, err := backend.CheckDelivery(t.Context(), manager.SessionID(sessionName))
		if err == nil && got == manager.CanReceiveYes {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("CheckDelivery() = %v, %v; want %v before deadline", got, err, manager.CanReceiveYes)
		}
		time.Sleep(50 * time.Millisecond)
	}

	got, err := backend.GetState(t.Context(), manager.SessionID(sessionName))
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if got.State != manager.StateIdle || got.Confidence != 0.95 {
		t.Fatalf("GetState() = %#v, want high-confidence IDLE", got)
	}
}
