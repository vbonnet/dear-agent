package safety

import (
	"testing"
)

func TestDetectHumanTyping(t *testing.T) {
	tests := []struct {
		name          string
		paneContent   string
		wantViolation bool
	}{
		{
			name:          "empty prompt - no typing",
			paneContent:   "some output\n❯ \n",
			wantViolation: false,
		},
		{
			name:          "human typing text",
			paneContent:   "some output\n❯ please fix the bug",
			wantViolation: true,
		},
		{
			name:          "AGM sender header - not human",
			paneContent:   "some output\n❯ [From: astrocyte | ID: 123] check status",
			wantViolation: false,
		},
		{
			name:          "AGM sender header lowercase",
			paneContent:   "some output\n❯ [from: monitor | ID: 456] hello",
			wantViolation: false,
		},
		{
			name:          "no prompt visible",
			paneContent:   "some output\nthinking...\n",
			wantViolation: false,
		},
		{
			name:          "prompt with only whitespace after",
			paneContent:   "output line\n❯    \n",
			wantViolation: false,
		},
		{
			name:          "multiple prompt lines - checks last",
			paneContent:   "❯ old command\noutput\n❯ new typing here",
			wantViolation: true,
		},
		{
			name:          "empty content",
			paneContent:   "",
			wantViolation: false,
		},
		{
			name:          "long human input truncated in evidence",
			paneContent:   "❯ this is a very long input that should be truncated because it exceeds fifty characters in total length",
			wantViolation: true,
		},
		{
			name:          "permission prompt numbered option - not human typing",
			paneContent:   "some output\n❯ 1. Yes, allow all",
			wantViolation: false,
		},
		{
			name:          "permission prompt second option - not human typing",
			paneContent:   "some output\n❯ 2. No, deny",
			wantViolation: false,
		},
		{
			name:          "permission prompt y/N - not human typing",
			paneContent:   "some output\n❯ y/N",
			wantViolation: false,
		},
		{
			name:          "permission prompt yes - not human typing",
			paneContent:   "some output\n❯ yes",
			wantViolation: false,
		},
		{
			name:          "permission prompt allow - not human typing",
			paneContent:   "some output\n❯ Allow",
			wantViolation: false,
		},
		{
			name:          "permission prompt deny - not human typing",
			paneContent:   "some output\n❯ deny this action",
			wantViolation: false,
		},
		{
			name:          "permission prompt always allow - not human typing",
			paneContent:   "some output\n❯ always allow this tool",
			wantViolation: false,
		},
		{
			name:          "permission prompt allow once - not human typing",
			paneContent:   "some output\n❯ allow once",
			wantViolation: false,
		},
		{
			name:          "permission prompt don't allow - not human typing",
			paneContent:   "some output\n❯ don't allow",
			wantViolation: false,
		},
		{
			name:          "permission prompt (Y)es style - not human typing",
			paneContent:   "some output\n❯ (Y)es, proceed",
			wantViolation: false,
		},
		{
			name:          "permission prompt (N)o style - not human typing",
			paneContent:   "some output\n❯ (N)o, cancel",
			wantViolation: false,
		},
		{
			name:          "navigation hint use arrows - not human typing",
			paneContent:   "some output\n❯ Use arrows to select",
			wantViolation: false,
		},
		// Allowlist inversion (ce-py3x): unrecognized non-word pane chrome
		// defaults to NOT typing instead of firing. Each of these previously
		// tripped the denylist as "a human is typing".
		{
			name:          "separator chrome after prompt - not human typing",
			paneContent:   "some output\n❯ ────────────────────",
			wantViolation: false,
		},
		{
			name:          "braille spinner glyph leaked after prompt - not human typing",
			paneContent:   "some output\n❯ ⣾",
			wantViolation: false,
		},
		{
			name:          "box-drawing border after prompt - not human typing",
			paneContent:   "some output\n❯ ╭──────────╮",
			wantViolation: false,
		},
		{
			name:          "arrow/symbol-only UI chrome after prompt - not human typing",
			paneContent:   "some output\n❯ ⏵⏵ »",
			wantViolation: false,
		},
		{
			name:          "punctuation-only after prompt - not human typing",
			paneContent:   "some output\n❯ ...",
			wantViolation: false,
		},
		// The positive human-input signature still fires, including non-ASCII
		// input (Unicode-aware letters/digits), so the guard stays useful.
		{
			name:          "digits-only human input - typing",
			paneContent:   "some output\n❯ 42",
			wantViolation: true,
		},
		{
			name:          "non-ASCII human input - typing",
			paneContent:   "some output\n❯ 修复这个错误",
			wantViolation: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := detectHumanTyping(tt.paneContent)
			if tt.wantViolation && v == nil {
				t.Error("expected violation but got nil")
			}
			if !tt.wantViolation && v != nil {
				t.Errorf("expected no violation but got: %s", v.Message)
			}
			if v != nil && v.Guard != ViolationHumanTyping {
				t.Errorf("expected guard %s, got %s", ViolationHumanTyping, v.Guard)
			}
		})
	}
}

func TestIsGhostTextAfterPrompt(t *testing.T) {
	tests := []struct {
		name      string
		ansiInput string
		want      bool
	}{
		{
			name:      "ghost text with dim attribute on prompt line",
			ansiInput: "output\n❯ \x1b[2mstart the loop\x1b[0m\n",
			want:      true,
		},
		{
			name:      "normal text on prompt line - no dim",
			ansiInput: "output\n❯ please fix the bug\n",
			want:      false,
		},
		{
			name:      "empty prompt - no dim attribute",
			ansiInput: "output\n❯ \n",
			want:      false,
		},
		{
			name:      "no prompt line",
			ansiInput: "output\nthinking...\n",
			want:      false,
		},
		{
			name:      "dim attribute on non-prompt line does not count",
			ansiInput: "\x1b[2msome dim output\x1b[0m\n❯ real typing here\n",
			want:      false,
		},
		{
			name:      "dim attribute before the prompt marker does not count",
			ansiInput: "\x1b[2mpre-prompt dim prefix\x1b[0m ❯ real typing here\n",
			want:      false,
		},
		{
			name:      "ghost text overseer pattern from live capture",
			ansiInput: "some output\n❯ \x1b[2mstart the loop\x1b[0m\n─────────────────────────────────\n",
			want:      true,
		},
		{
			name:      "ghost text with 256-color grey (vroom overseer, ce-5miu)",
			ansiInput: "output\n❯ \x1b[38;5;241mstart the loop\x1b[0m\n",
			want:      true,
		},
		{
			name:      "ghost text with dim combined with 256-color grey",
			ansiInput: "output\n❯ \x1b[2;38;5;241mstart the loop\x1b[0m\n",
			want:      true,
		},
		{
			name:      "ghost text with bright-black (grey) foreground",
			ansiInput: "output\n❯ \x1b[90mstart the loop\x1b[0m\n",
			want:      true,
		},
		{
			name:      "ghost text with dim truecolor grey",
			ansiInput: "output\n❯ \x1b[38;2;120;120;120mstart the loop\x1b[0m\n",
			want:      true,
		},
		{
			name:      "256-color grey before the prompt marker does not count",
			ansiInput: "\x1b[38;5;241mpre-prompt grey\x1b[0m ❯ real typing here\n",
			want:      false,
		},
		{
			name:      "non-grey 256-color (red) after prompt is real input",
			ansiInput: "output\n❯ \x1b[38;5;196mplease fix the bug\x1b[0m\n",
			want:      false,
		},
		{
			name:      "bright white truecolor after prompt is real input",
			ansiInput: "output\n❯ \x1b[38;2;255;255;255mplease fix the bug\x1b[0m\n",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isGhostTextAfterPrompt(tt.ansiInput)
			if got != tt.want {
				t.Errorf("isGhostTextAfterPrompt(%q) = %v, want %v", tt.ansiInput, got, tt.want)
			}
		})
	}
}

func TestDetectSessionUninitialized(t *testing.T) {
	tests := []struct {
		name          string
		paneContent   string
		claudeRunning bool
		wantViolation bool
		wantEvidence  string
	}{
		{
			name:          "normal session with prompt",
			paneContent:   "some output\n❯ \n",
			claudeRunning: true,
			wantViolation: false,
		},
		{
			name:          "claude not running",
			paneContent:   "$ \n",
			claudeRunning: false,
			wantViolation: true,
			wantEvidence:  "no claude process",
		},
		{
			name:          "trust question without selected row remains uninitialized",
			paneContent:   "Do you trust the files in this folder?\n1. Yes\n2. No\n",
			claudeRunning: true,
			wantViolation: true,
			wantEvidence:  "no prompt character",
		},
		{
			name:          "live selected trust prompt is uninitialized",
			paneContent:   "Do you trust the files in this folder?\n❯ 1. Yes, proceed\n  2. No, exit\nEnter to confirm",
			claudeRunning: true,
			wantViolation: true,
			wantEvidence:  "trust prompt visible",
		},
		{
			name:          "live trust prompt with negative selected is uninitialized",
			paneContent:   "Quick safety check: Is this a project you created or one you trust?\n  1. Yes, I trust this folder\n❯ 2. No, exit\nEnter to confirm",
			claudeRunning: true,
			wantViolation: true,
			wantEvidence:  "trust prompt visible",
		},
		{
			name:          "historical trust selector above current composer is initialized",
			paneContent:   "❯ 1. Yes, I trust this folder\n  2. No, exit\n\nresponse complete\n❯ \n────────",
			claudeRunning: true,
			wantViolation: false,
		},
		{
			name:          "historical trust question and selector above current composer are initialized",
			paneContent:   "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder\n  2. No, exit\nEnter to confirm\n\nresponse complete\n❯ \n────────",
			claudeRunning: true,
			wantViolation: false,
		},
		{
			name:          "welcome screen without prompt",
			paneContent:   "Welcome to Claude Code\nVersion 3.0.0\n",
			claudeRunning: true,
			wantViolation: true,
			wantEvidence:  "welcome screen visible",
		},
		{
			name:          "welcome screen with prompt (initialized)",
			paneContent:   "Welcome to Claude Code\n❯ \n",
			claudeRunning: true,
			wantViolation: false,
		},
		{
			name:          "no prompt at all (bash shell)",
			paneContent:   "user@host:~$ \n",
			claudeRunning: true,
			wantViolation: true,
			wantEvidence:  "no prompt character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := detectSessionUninitialized(tt.paneContent, tt.claudeRunning)
			if tt.wantViolation && v == nil {
				t.Error("expected violation but got nil")
			}
			if !tt.wantViolation && v != nil {
				t.Errorf("expected no violation but got: %s", v.Message)
			}
			if v != nil && tt.wantEvidence != "" && v.Evidence != tt.wantEvidence {
				t.Errorf("expected evidence %q, got %q", tt.wantEvidence, v.Evidence)
			}
		})
	}
}

func TestDetectCodexSessionUninitialized(t *testing.T) {
	tests := []struct {
		name          string
		paneContent   string
		wantViolation bool
		wantEvidence  string
	}{
		{
			name:          "codex composer visible",
			paneContent:   "│ >_ OpenAI Codex (v0.142.0) │\n│ model: gpt-5.4 /model to change │",
			wantViolation: false,
		},
		{
			name:          "codex trust prompt",
			paneContent:   "Do you trust the contents of this directory?",
			wantViolation: true,
			wantEvidence:  "codex trust prompt visible",
		},
		{
			name:          "codex model prompt",
			paneContent:   "Choose how you'd like Codex to proceed\n1. Try new model\n2. Use existing model",
			wantViolation: true,
			wantEvidence:  "codex model prompt visible",
		},
		{
			name:          "shell prompt only",
			paneContent:   "vbonnet@mac merged %",
			wantViolation: true,
			wantEvidence:  "no codex composer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := detectCodexSessionUninitialized(tt.paneContent)
			if tt.wantViolation && v == nil {
				t.Error("expected violation but got nil")
			}
			if !tt.wantViolation && v != nil {
				t.Errorf("expected no violation but got: %s", v.Message)
			}
			if v != nil && tt.wantEvidence != "" && v.Evidence != tt.wantEvidence {
				t.Errorf("expected evidence %q, got %q", tt.wantEvidence, v.Evidence)
			}
		})
	}
}

func TestDetectAgySessionUninitialized(t *testing.T) {
	tests := []struct {
		name          string
		paneContent   string
		wantViolation bool
		wantEvidence  string
	}{
		{
			name:          "agy prompt visible",
			paneContent:   "AGM_NEW_OK\n>\n? for shortcuts",
			wantViolation: false,
		},
		{
			name:          "agy trust prompt",
			paneContent:   "Do you trust the contents of this project?\n> Yes, I trust this folder",
			wantViolation: true,
			wantEvidence:  "agy trust prompt visible",
		},
		{
			name:          "shell prompt only",
			paneContent:   "vbonnet@mac agy-e2e-workdir %",
			wantViolation: true,
			wantEvidence:  "no agy prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := detectAgySessionUninitialized(tt.paneContent)
			if tt.wantViolation && v == nil {
				t.Error("expected violation but got nil")
			}
			if !tt.wantViolation && v != nil {
				t.Errorf("expected no violation but got: %s", v.Message)
			}
			if v != nil && tt.wantEvidence != "" && v.Evidence != tt.wantEvidence {
				t.Errorf("expected evidence %q, got %q", tt.wantEvidence, v.Evidence)
			}
		})
	}
}

func TestDetectPiSessionUninitialized(t *testing.T) {
	tests := []struct {
		name          string
		paneContent   string
		wantViolation bool
	}{
		{name: "managed ready", paneContent: "AGM plan/ready"},
		{name: "managed working", paneContent: "AGM auto/working"},
		{name: "permission overlay", paneContent: "AGM default/permission", wantViolation: true},
		{name: "stale ready before permission overlay", paneContent: "AGM default/ready\nAGM default/permission", wantViolation: true},
		{name: "unmanaged Pi chrome", paneContent: "pi v0.81.0\n~/work • session", wantViolation: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := detectPiSessionUninitialized(tt.paneContent)
			if tt.wantViolation && v == nil {
				t.Fatal("expected Pi uninitialized violation")
			}
			if !tt.wantViolation && v != nil {
				t.Fatalf("unexpected Pi uninitialized violation: %s", v.Message)
			}
		})
	}
}

func TestDetectOpenCodeSessionUninitialized(t *testing.T) {
	tests := []struct {
		name          string
		paneContent   string
		wantViolation bool
	}{
		{name: "composer", paneContent: "session ready\n> "},
		{name: "product chrome", paneContent: "OpenCode\nworkspace"},
		{name: "active work", paneContent: "Running..."},
		{name: "shell only", paneContent: "vbonnet@mac work %", wantViolation: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := detectOpenCodeSessionUninitialized(tt.paneContent)
			if tt.wantViolation && v == nil {
				t.Fatal("expected OpenCode uninitialized violation")
			}
			if !tt.wantViolation && v != nil {
				t.Fatalf("unexpected OpenCode uninitialized violation: %s", v.Message)
			}
		})
	}
}

func TestNormalizeHarnessForSafety(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"codex", "codex-cli"},
		{"codex-cli", "codex-cli"},
		{"agy", "agy"},
		{"antigravity", "agy"},
		{"opencode", "opencode-cli"},
		{"opencode-cli", "opencode-cli"},
		{"pi", "pi-cli"},
		{"pi-cli", "pi-cli"},
		{"claude", "claude-code"},
		{"claude-code", "claude-code"},
		{"", "claude-code"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeHarnessForSafety(tt.input); got != tt.want {
				t.Errorf("normalizeHarnessForSafety(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDetectClaudeMidResponse(t *testing.T) {
	tests := []struct {
		name          string
		paneContent   string
		wantViolation bool
	}{
		{
			name:          "prompt visible - ready",
			paneContent:   "some output\n❯ ",
			wantViolation: false,
		},
		{
			name:          "spinner active - thinking",
			paneContent:   "some output\n✶ Thinking...\n",
			wantViolation: true,
		},
		{
			name:          "spinner active - mustering",
			paneContent:   "some output\n✻ Mustering resources\n",
			wantViolation: true,
		},
		{
			name:          "spinner active - evaporating",
			paneContent:   "some output\n✶ Evaporating slowly\n",
			wantViolation: true,
		},
		{
			name:          "braille spinner",
			paneContent:   "some output\n⣾ Processing\n",
			wantViolation: true,
		},
		{
			name:          "generic spinner",
			paneContent:   "some output\n· Loading...\n",
			wantViolation: true,
		},
		{
			name:          "spinner but prompt also visible at bottom",
			paneContent:   "✶ Thinking...\nresult text\n❯ ",
			wantViolation: false,
		},
		{
			name:          "no spinner no prompt",
			paneContent:   "just some text output\nmore output\n",
			wantViolation: false,
		},
		{
			name:          "empty content",
			paneContent:   "",
			wantViolation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := detectClaudeMidResponse(tt.paneContent)
			if tt.wantViolation && v == nil {
				t.Error("expected violation but got nil")
			}
			if !tt.wantViolation && v != nil {
				t.Errorf("expected no violation but got: %s (evidence: %s)", v.Message, v.Evidence)
			}
		})
	}
}

func TestCheckResultError(t *testing.T) {
	t.Run("safe result returns empty", func(t *testing.T) {
		r := &CheckResult{Safe: true}
		if r.Error() != "" {
			t.Errorf("expected empty error, got %q", r.Error())
		}
	})

	t.Run("violation result returns formatted error", func(t *testing.T) {
		r := &CheckResult{
			Safe: false,
			Violations: []Violation{
				{
					Guard:      ViolationHumanTyping,
					Message:    "test message",
					Suggestion: "test suggestion",
				},
			},
		}
		err := r.Error()
		if err == "" {
			t.Error("expected non-empty error")
		}
		if !containsStr(err, "human_typing") {
			t.Errorf("error should contain guard name, got: %s", err)
		}
		if !containsStr(err, "test message") {
			t.Errorf("error should contain message, got: %s", err)
		}
	})
}

func TestCheckResultHasViolation(t *testing.T) {
	r := &CheckResult{
		Safe: false,
		Violations: []Violation{
			{Guard: ViolationHumanTyping},
		},
	}

	if !r.HasViolation(ViolationHumanTyping) {
		t.Error("expected HasViolation to return true for human_typing")
	}
	if r.HasViolation(ViolationSessionUninitialized) {
		t.Error("expected HasViolation to return false for session_uninitialized")
	}
}

// TestHumanTypingIsAdvisoryNotBlocking documents the core contract of this
// change: a human_typing detection is an advisory that keeps Safe=true. It is
// surfaced via Advisories/HasAdvisory, never Violations, so callers proceed
// (and stash) instead of blocking.
func TestHumanTypingIsAdvisoryNotBlocking(t *testing.T) {
	r := &CheckResult{
		Safe: true,
		Advisories: []Violation{
			{Guard: ViolationHumanTyping, Message: "Unsent text detected in prompt: \"merge PR 527\""},
		},
	}

	if !r.Safe {
		t.Error("human_typing must not make a result unsafe")
	}
	if !r.HasAdvisory(ViolationHumanTyping) {
		t.Error("expected HasAdvisory to report the human_typing advisory")
	}
	if r.HasViolation(ViolationHumanTyping) {
		t.Error("human_typing must not appear as a blocking violation")
	}
	if r.Error() != "" {
		t.Errorf("advisory-only result must produce no blocking error, got %q", r.Error())
	}
}

func TestAutonomousModeBehavior(t *testing.T) {
	t.Run("autonomous mode sets AutonomousMode in options", func(t *testing.T) {
		opts := GuardOptions{AutonomousMode: true}
		if !opts.AutonomousMode {
			t.Error("expected AutonomousMode to be true")
		}
	})

	t.Run("autonomous mode is independent of SkipHumanTyping", func(t *testing.T) {
		opts := GuardOptions{AutonomousMode: true, SkipHumanTyping: false}
		if opts.SkipHumanTyping {
			t.Error("SkipHumanTyping should remain false")
		}
		if !opts.AutonomousMode {
			t.Error("AutonomousMode should be true")
		}
	})
}

func TestCooldownCache(t *testing.T) {
	t.Run("fresh cache has no cooldown active", func(t *testing.T) {
		ResetCooldownCache()
		if isHumanTypingCooldownActive("test-session") {
			t.Error("expected no cooldown for fresh cache")
		}
	})

	t.Run("cooldown active after recording", func(t *testing.T) {
		ResetCooldownCache()
		recordHumanTypingCooldown("test-session")
		if !isHumanTypingCooldownActive("test-session") {
			t.Error("expected cooldown to be active immediately after recording")
		}
	})

	t.Run("cooldown is per-session", func(t *testing.T) {
		ResetCooldownCache()
		recordHumanTypingCooldown("session-a")
		if isHumanTypingCooldownActive("session-b") {
			t.Error("session-b should not have cooldown from session-a")
		}
		if !isHumanTypingCooldownActive("session-a") {
			t.Error("session-a should still have active cooldown")
		}
	})

	t.Run("ResetCooldownCache clears all entries", func(t *testing.T) {
		ResetCooldownCache()
		recordHumanTypingCooldown("session-x")
		ResetCooldownCache()
		if isHumanTypingCooldownActive("session-x") {
			t.Error("expected cooldown cleared after reset")
		}
	})
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && indexStr(s, substr) != -1
}

func indexStr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
