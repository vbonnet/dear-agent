package tmux

import (
	"testing"
)

func TestContainsPromptPattern(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Claude cursor pattern",
			content:  "▌",
			expected: true,
		},
		{
			name:     "Claude cursor with text",
			content:  "some text ▌",
			expected: true,
		},
		{
			name:     "Common prompt",
			content:  "> ",
			expected: true,
		},
		{
			name:     "Shell prompt",
			content:  "$ ",
			expected: true,
		},
		{
			name:     "Root prompt",
			content:  "# ",
			expected: true,
		},
		{
			name:     "Prompt with path prefix",
			content:  "user@host:~/dir $ ",
			expected: true,
		},
		{
			name:     "Ends with >",
			content:  "user@host>",
			expected: true,
		},
		{
			name:     "Ends with $",
			content:  "bash-5.1$",
			expected: true,
		},
		{
			name:     "Ends with #",
			content:  "root@host#",
			expected: true,
		},
		{
			name:     "Empty string",
			content:  "",
			expected: false,
		},
		{
			name:     "Whitespace only",
			content:  "   ",
			expected: false,
		},
		{
			name:     "Regular text",
			content:  "hello world",
			expected: false,
		},
		{
			name:     "Hash in middle of text",
			content:  "test #tag here",
			expected: false,
		},
		{
			name:     "Dollar in middle of text",
			content:  "costs $100 today",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsPromptPattern(tt.content)
			if result != tt.expected {
				t.Errorf("containsPromptPattern(%q) = %v, expected %v",
					tt.content, result, tt.expected)
			}
		})
	}
}

func TestClaudePromptPatterns(t *testing.T) {
	// Verify that all expected patterns are defined
	expectedPatterns := map[string]bool{
		"▌":  false,
		"> ": false,
		"$ ": false,
		"# ": false,
	}

	if len(ClaudePromptPatterns) != len(expectedPatterns) {
		t.Errorf("Expected %d patterns, got %d", len(expectedPatterns), len(ClaudePromptPatterns))
	}

	for _, pattern := range ClaudePromptPatterns {
		if _, exists := expectedPatterns[pattern]; !exists {
			t.Errorf("Unexpected pattern: %q", pattern)
		}
		expectedPatterns[pattern] = true
	}

	for pattern, found := range expectedPatterns {
		if !found {
			t.Errorf("Missing expected pattern: %q", pattern)
		}
	}
}
