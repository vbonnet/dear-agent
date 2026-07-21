package tmux

import (
	"strings"
	"testing"
)

func TestClassifyHarnessInputRequiresCurrentHarnessComposer(t *testing.T) {
	tests := []struct {
		name    string
		harness string
		content string
		ready   bool
		state   string
	}{
		{
			name:    "structural Codex composer",
			harness: "codex-cli",
			content: "│ >_ OpenAI Codex (vtest) │\n│ model: gpt-5.5 /model to change │\n›",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "Codex model footer without input owner",
			harness: "codex-cli",
			content: "Working (12s)\ngpt-5.5 xhigh · /repo",
			state:   HarnessInputBusy,
		},
		{
			name:    "stale Codex header without input owner",
			harness: "codex-cli",
			content: "│ >_ OpenAI Codex (vtest) │\n│ model: gpt-5.5 /model to change │\nWorking (12s)",
			state:   HarnessInputBusy,
		},
		{
			name:    "Codex trust owns input",
			harness: "codex-cli",
			content: "Do you trust the contents of this directory?\n› 1. Yes, continue",
			state:   HarnessInputOnboarding,
		},
		{
			name:    "Claude permission wins over prompt glyph",
			harness: "claude-code",
			content: "Do you want to proceed?\n❯ 1. Yes\n❯",
			state:   HarnessInputPermission,
		},
		{
			name:    "wrong harness composer",
			harness: "codex-cli",
			content: "❯",
			state:   HarnessInputBusy,
		},
		{
			name:    "AGY survey wins over bare prompt",
			harness: "agy",
			content: ">\nHow's the CLI experience so far?\n[1] Good [2] Fine [3] Bad [0] Skip",
			state:   HarnessInputOverlay,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ready, state, err := ClassifyHarnessInput(tt.content, tt.harness)
			if err != nil {
				t.Fatalf("ClassifyHarnessInput() error = %v", err)
			}
			if ready != tt.ready || state != tt.state {
				t.Fatalf("ClassifyHarnessInput() = (%v, %q), want (%v, %q)", ready, state, tt.ready, tt.state)
			}
		})
	}
}

func TestClassifyHarnessInputRejectsStalePromptOutsideTail(t *testing.T) {
	content := "❯\n" + strings.Repeat("historical output\n", 13)
	ready, state, err := ClassifyHarnessInput(content, "claude-code")
	if err != nil {
		t.Fatalf("ClassifyHarnessInput() error = %v", err)
	}
	if ready || state != HarnessInputBusy {
		t.Fatalf("stale prompt = (%v, %q), want busy", ready, state)
	}
}

func TestExpectedHarnessMatcherRejectsWrongProcess(t *testing.T) {
	procs := []ProcEntry{
		{PID: 10, PPID: 1, Comm: "zsh"},
		{PID: 11, PPID: 10, Comm: "agy"},
	}
	codex := ClassifyPaneLiveness([]int{10}, procs, expectedHarnessMatcher("codex-cli"))
	if codex.HarnessAlive {
		t.Fatalf("AGY process classified as Codex: %#v", codex)
	}
	agy := ClassifyPaneLiveness([]int{10}, procs, expectedHarnessMatcher("agy"))
	if !agy.HarnessAlive {
		t.Fatalf("AGY process not detected: %#v", agy)
	}
}

func TestExpectedHarnessMatcherAcceptsNodeBackedCodex(t *testing.T) {
	procs := []ProcEntry{
		{PID: 10, PPID: 1, Comm: "zsh"},
		{PID: 11, PPID: 10, Comm: "/usr/local/bin/node"},
	}
	got := ClassifyPaneLiveness([]int{10}, procs, expectedHarnessMatcher("codex-cli"))
	if !got.HarnessAlive {
		t.Fatalf("node-backed Codex liveness = %#v, want alive", got)
	}
}
