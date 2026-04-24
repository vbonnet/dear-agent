package tmux

import (
	"testing"
)

// TestContainsAnyHarnessPromptPattern_OpenCode verifies that OpenCode prompt patterns
// are correctly detected by the unified harness prompt detector.
func TestContainsAnyHarnessPromptPattern_OpenCode(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "OpenCode input prompt",
			content:  "> Type your message",
			expected: true,
		},
		{
			name:     "OpenCode alternative prompt",
			content:  ">> Type here",
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
			name:     "OpenCode banner without prompt",
			content:  "OpenCode CLI v1.0.0",
			expected: false,
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

// TestContainsOpenCodePromptPattern_Direct tests the OpenCode prompt detector directly
func TestContainsOpenCodePromptPattern_Direct(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		isPrompt bool
	}{
		{
			name:     "OpenCode input prompt with text",
			content:  "> Type your message",
			isPrompt: true,
		},
		{
			name:     "OpenCode alternative prompt with text",
			content:  ">> Type here",
			isPrompt: true,
		},
		{
			name:     "OpenCode prompt in line",
			content:  "session > ready",
			isPrompt: true,
		},
		{
			name:     "OpenCode with surrounding content",
			content:  "  user > waiting",
			isPrompt: true,
		},
		{
			name:     "Empty content",
			content:  "",
			isPrompt: false,
		},
		{
			name:     "Just whitespace",
			content:  "   ",
			isPrompt: false,
		},
		{
			name:     "Banner line",
			content:  "OpenCode CLI v1.0.0",
			isPrompt: false,
		},
		{
			name:     "Status line",
			content:  "Waiting for input...",
			isPrompt: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsOpenCodePromptPattern(tt.content)
			if result != tt.isPrompt {
				t.Errorf("containsOpenCodePromptPattern(%q) = %v, expected %v",
					tt.content, result, tt.isPrompt)
			}
		})
	}
}
