package main

import (
	"strings"
	"testing"
)

// In the test harness stdin is not a terminal, so the break-glass TTY guard
// fires: this verifies the subcommand is wired into run() and refuses
// non-interactive callers (the structural agent exclusion).
func TestRun_BreakGlassRequiresTTY(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "vbonnet/dear-agent")
	err := run([]string{"break-glass", "42"})
	if err == nil {
		t.Fatal("expected break-glass to refuse without a TTY, got nil")
	}
	if !strings.Contains(err.Error(), "TTY") {
		t.Errorf("error should mention the TTY requirement, got: %v", err)
	}
}

func TestRun_BreakGlassMissingPR(t *testing.T) {
	err := runBreakGlass([]string{})
	if err == nil {
		t.Fatal("expected error when no PR given")
	}
	if !strings.Contains(err.Error(), "PR") {
		t.Errorf("error should mention the missing PR, got: %v", err)
	}
}

func TestBreakGlassUsageMentionsTTY(t *testing.T) {
	if !strings.Contains(breakGlassUsage, "TTY") {
		t.Errorf("usage should document the TTY requirement")
	}
}
