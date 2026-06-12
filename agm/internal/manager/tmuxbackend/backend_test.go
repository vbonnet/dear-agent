package tmuxbackend

import (
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
