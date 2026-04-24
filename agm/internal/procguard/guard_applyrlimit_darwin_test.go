//go:build darwin

package procguard

import (
	"strings"
	"testing"
)

func TestApplyNprocLimit(t *testing.T) {
	// prlimit is Linux-only; on Darwin the stub returns a "not supported" error.
	err := ApplyNprocLimit(1, 128)
	if err == nil {
		t.Fatal("expected error: prlimit not supported on macOS")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected 'not supported' in error, got: %v", err)
	}
}

func TestApplyNprocLimit_InvalidPID(t *testing.T) {
	err := ApplyNprocLimit(999999, 128)
	if err == nil {
		t.Fatal("expected error for invalid PID on macOS (stub always errors)")
	}
}

func TestApplyNprocLimit_CurrentProcess(t *testing.T) {
	t.Skip("prlimit not supported on macOS")
}
