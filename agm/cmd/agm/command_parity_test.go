package main

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

func TestActiveHarnessesHaveTmuxCommandCoverage(t *testing.T) {
	for _, harness := range agent.ActiveHarnesses() {
		t.Run(harness, func(t *testing.T) {
			if !activeHarnessHasTmuxLauncher(harness) {
				t.Errorf("active harness %q has no session new tmux launcher", harness)
			}
			if !activeHarnessHasTmuxResumeCommand(harness) {
				t.Errorf("active harness %q has no session resume tmux command", harness)
			}
			if isAPIBasedAgent(harness) {
				t.Errorf("active harness %q should use tmux send delivery", harness)
			}
			if _, err := resolveSetModelInstruction(harness, setModelParityModel(t, harness)); err != nil {
				t.Errorf("active harness %q has no send set-model route: %v", harness, err)
			}
		})
	}
}

func TestSessionNewHelpIsHarnessNeutral(t *testing.T) {
	for _, text := range []string{newCmd.Short, newCmd.Long} {
		for _, forbidden := range []string{
			"new Claude session",
			"tmux/Claude session",
			"Start Claude CLI",
			"creates tmux + claude",
			"starts claude",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("session new help contains Claude-only wording %q in:\n%s", forbidden, text)
			}
		}
	}
}

func TestDeprecatedGeminiIsNotActiveCommandParity(t *testing.T) {
	if agent.IsDeprecatedHarness("gemini-cli") && activeHarnessHasTmuxLauncher("gemini-cli") {
		t.Fatal("deprecated gemini-cli should not be counted as active launcher parity")
	}
	if agent.IsDeprecatedHarness("gemini-cli") && activeHarnessHasTmuxResumeCommand("gemini-cli") {
		t.Fatal("deprecated gemini-cli should not be counted as active resume parity")
	}
}

func setModelParityModel(t *testing.T, harness string) string {
	t.Helper()
	switch harness {
	case "claude-code":
		return "sonnet"
	case "codex-cli":
		return "5.4-mini"
	case "agy":
		return "3.5-flash"
	case "opencode-cli":
		return "glm-5.2"
	case "pi-cli":
		return "gpt-fast"
	default:
		t.Fatalf("missing parity model for harness %q", harness)
		return ""
	}
}
