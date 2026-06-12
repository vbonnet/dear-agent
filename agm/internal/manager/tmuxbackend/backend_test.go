package tmuxbackend

import "testing"

func TestNew_NotNil(t *testing.T) {
	b := New()
	if b == nil {
		t.Fatal("New() returned nil")
	}
}

func TestName_IsTmux(t *testing.T) {
	b := New()
	if got := b.Name(); got != "tmux" {
		t.Errorf("Name() = %q, want %q", got, "tmux")
	}
}

func TestCapabilities_SupportsAttach(t *testing.T) {
	b := New()
	caps := b.Capabilities()
	if !caps.SupportsAttach {
		t.Error("SupportsAttach should be true for tmux backend")
	}
}

func TestCapabilities_SupportsInterrupt(t *testing.T) {
	b := New()
	caps := b.Capabilities()
	if !caps.SupportsInterrupt {
		t.Error("SupportsInterrupt should be true for tmux backend")
	}
}
