package tmuxbackend

import (
	"testing"
)

func TestNew_NotNil(t *testing.T) {
	t.Parallel()
	b := New()
	if b == nil {
		t.Fatal("New() returned nil")
	}
}

func TestName(t *testing.T) {
	t.Parallel()
	b := New()
	if got := b.Name(); got != "tmux" {
		t.Errorf("Name() = %q, want \"tmux\"", got)
	}
}

func TestCapabilities(t *testing.T) {
	t.Parallel()
	b := New()
	caps := b.Capabilities()
	if !caps.SupportsAttach {
		t.Error("Capabilities.SupportsAttach = false, want true")
	}
	if !caps.SupportsInterrupt {
		t.Error("Capabilities.SupportsInterrupt = false, want true")
	}
	if caps.MaxConcurrentSessions != 0 {
		t.Errorf("Capabilities.MaxConcurrentSessions = %d, want 0 (unlimited)", caps.MaxConcurrentSessions)
	}
}
