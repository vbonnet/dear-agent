package tmux

import (
	"testing"
)

// TestContainsAnyHarnessPromptPattern_Gemini verifies that Gemini prompt patterns
// are correctly detected by the unified harness prompt detector.
func TestContainsAnyHarnessPromptPattern_Gemini(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Gemini input prompt",
			content:  " >   Type your message or @path/to/file",
			expected: true,
		},
		{
			name:     "Gemini box drawing top",
			content:  " ╭─────────────────────────────────────────╮",
			expected: true,
		},
		{
			name:     "Claude prompt",
			content:  "❯",
			expected: true,
		},
		{
			name:     "Empty content",
			content:  "",
			expected: false,
		},
		{
			name:     "Generic shell prompt",
			content:  "user@host:~$",
			expected: false,
		},
		{
			name:     "Gemini banner without prompt",
			content:  "███ GEMINI CLI v0.37.0",
			expected: false,
		},
		{
			name:     "Gemini status bar",
			content:  " workspace (/directory)                       sandbox                         gemini-2.5-flash-lite",
			expected: false,
		},
		{
			name:     "Gemini trust dialog content",
			content:  " Do you trust the files in this folder?",
			expected: false, // trust dialog is not a prompt pattern
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsAnyHarnessPromptPattern(tt.content)
			if result != tt.expected {
				t.Errorf("containsAnyHarnessPromptPattern(%q) = %v, expected %v",
					tt.content, result, tt.expected)
			}
		})
	}
}

// TestContainsGeminiPromptPattern_RealOutput tests against real Gemini CLI output lines
// captured during end-to-end testing (2026-04-12).
func TestContainsGeminiPromptPattern_RealOutput(t *testing.T) {
	// These are actual lines from Gemini CLI v0.37.0 captured via tmux capture-pane.
	tests := []struct {
		name     string
		content  string
		isPrompt bool
	}{
		{
			name:     "Real prompt line",
			content:  " >   Type your message or @path/to/file",
			isPrompt: true,
		},
		{
			name:     "Banner line",
			content:  " ▝▜▄     Gemini CLI v0.37.0",
			isPrompt: false,
		},
		{
			name:     "Auth line",
			content:  "   ▝▜▄",
			isPrompt: false,
		},
		{
			name:     "Status bar line",
			content:  " Shift+Tab to accept edits                                                                   1 skill",
			isPrompt: false,
		},
		{
			name:     "Top bar characters",
			content:  "▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀",
			isPrompt: false,
		},
		{
			name:     "Bottom bar characters",
			content:  "▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄",
			isPrompt: false,
		},
		{
			name:     "Agent error",
			content:  "✕ Agent loading error: Failed to load agent from /home/user/.gemini/agents/ecphory-explain.md:",
			isPrompt: false,
		},
		{
			name:     "Trust option",
			content:  " ● 1. Trust folder (tmp)",
			isPrompt: false,
		},
		{
			name:     "Shortcuts hint",
			content:  "                                                                                     ? for shortcuts",
			isPrompt: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsGeminiPromptPattern(tt.content)
			if result != tt.isPrompt {
				t.Errorf("containsGeminiPromptPattern(%q) = %v, expected %v",
					tt.content, result, tt.isPrompt)
			}
		})
	}
}

// TestContainsTrustPromptPattern_GeminiTrust tests trust prompt detection for Gemini CLI.
func TestContainsTrustPromptPattern_GeminiTrust(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Gemini trust dialog",
			content:  " │ Do you trust the files in this folder?                                                          │",
			expected: true,
		},
		{
			name:     "Claude trust dialog",
			content:  "Do you trust the files in this folder?",
			expected: true,
		},
		{
			name:     "No trust prompt",
			content:  " >   Type your message or @path/to/file",
			expected: false,
		},
		{
			name:     "Empty",
			content:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsTrustPromptPattern(tt.content)
			if result != tt.expected {
				t.Errorf("containsTrustPromptPattern(%q) = %v, expected %v",
					tt.content, result, tt.expected)
			}
		})
	}
}
