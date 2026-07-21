package tmuxbackend

import (
	"context"
	"errors"
	"testing"

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
