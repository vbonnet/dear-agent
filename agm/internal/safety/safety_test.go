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
			name:          "trust prompt visible",
			paneContent:   "Do you trust the files in this folder?\n1. Yes\n2. No\n",
			claudeRunning: true,
			wantViolation: true,
			wantEvidence:  "trust prompt visible",
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
