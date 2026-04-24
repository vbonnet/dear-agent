package tmux

import (
	"testing"
)

func TestContainsGeminiPromptPattern(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Gemini prompt text",
			content:  ">   Type your message or @path/to/file",
			expected: true,
		},
		{
			name:     "Gemini box top",
			content:  "╭──────────────────────────────────────────────────────────────────────────────╮",
			expected: true,
		},
		{
			name:     "Gemini box bottom",
			content:  "╰──────────────────────────────────────────────────────────────────────────────╯",
			expected: true,
		},
		{
			name:     "Partial prompt text",
			content:  "Type your message",
			expected: false, // Must match full pattern ">   Type your message"
		},
		{
			name:     "Path reference pattern",
			content:  "or @path/to/file",
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
			name:     "Random text",
			content:  "Hello world",
			expected: false,
		},
		{
			name:     "Gemini banner",
			content:  "███ GEMINI",
			expected: false, // Banner is not a prompt pattern
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsGeminiPromptPattern(tt.content)
			if result != tt.expected {
				t.Errorf("containsGeminiPromptPattern(%q) = %v, expected %v",
					tt.content, result, tt.expected)
			}
		})
	}
}

func TestGeminiPromptPatterns(t *testing.T) {
	// Verify we have the expected patterns defined
	expectedPatterns := []string{
		">   Type your message",
		"@path/to/file",
		"╭─",
		"╰─",
	}

	if len(GeminiPromptPatterns) != len(expectedPatterns) {
		t.Errorf("GeminiPromptPatterns has %d patterns, expected %d",
			len(GeminiPromptPatterns), len(expectedPatterns))
	}

	for i, expected := range expectedPatterns {
		if i >= len(GeminiPromptPatterns) {
			break
		}
		if GeminiPromptPatterns[i] != expected {
			t.Errorf("GeminiPromptPatterns[%d] = %q, expected %q",
				i, GeminiPromptPatterns[i], expected)
		}
	}
}
