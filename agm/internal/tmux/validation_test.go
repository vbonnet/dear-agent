package tmux

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidateSessionName tests session name validation for BUG-001 Phase 2
func TestValidateSessionName(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		wantHasIssues   bool
		wantSuggested   string
		wantWarningType string // "dots", "colons", "spaces", "multiple", "none"
	}{
		{
			name:            "safe name with dashes",
			input:           "my-session",
			wantHasIssues:   false,
			wantSuggested:   "",
			wantWarningType: "none",
		},
		{
			name:            "safe name with underscores",
			input:           "my_session_123",
			wantHasIssues:   false,
			wantSuggested:   "",
			wantWarningType: "none",
		},
		{
			name:            "safe alphanumeric name",
			input:           "session123",
			wantHasIssues:   false,
			wantSuggested:   "",
			wantWarningType: "none",
		},
		{
			name:            "name with dots",
			input:           "gemini-task-1.4",
			wantHasIssues:   true,
			wantSuggested:   "gemini-task-1-4",
			wantWarningType: "dots",
		},
		{
			name:            "name with colons",
			input:           "project:staging",
			wantHasIssues:   true,
			wantSuggested:   "project-staging",
			wantWarningType: "colons",
		},
		{
			name:            "name with spaces",
			input:           "my session",
			wantHasIssues:   true,
			wantSuggested:   "my-session",
			wantWarningType: "spaces",
		},
		{
			name:            "name with multiple problematic characters",
			input:           "project.v1:staging test",
			wantHasIssues:   true,
			wantSuggested:   "project-v1-staging-test",
			wantWarningType: "multiple",
		},
		{
			name:            "multiple dots",
			input:           "foo.bar.baz",
			wantHasIssues:   true,
			wantSuggested:   "foo-bar-baz",
			wantWarningType: "dots",
		},
		{
			name:            "edge case - only dots",
			input:           "...",
			wantHasIssues:   true,
			wantSuggested:   "---",
			wantWarningType: "dots",
		},
		{
			name:            "real incident case - gemini-task-1.4",
			input:           "gemini-task-1.4",
			wantHasIssues:   true,
			wantSuggested:   "gemini-task-1-4",
			wantWarningType: "dots",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings, suggestedName, hasIssues := ValidateSessionName(tt.input)

			// Check hasIssues flag
			assert.Equal(t, tt.wantHasIssues, hasIssues,
				"ValidateSessionName(%q).hasIssues = %v, want %v",
				tt.input, hasIssues, tt.wantHasIssues)

			// Check suggested name
			assert.Equal(t, tt.wantSuggested, suggestedName,
				"ValidateSessionName(%q).suggestedName = %q, want %q",
				tt.input, suggestedName, tt.wantSuggested)

			// Verify warnings content based on type
			if tt.wantHasIssues {
				assert.NotEmpty(t, warnings, "Expected warnings for %q", tt.input)

				// Verify warnings contain expected content
				warningText := strings.Join(warnings, " ")
				assert.Contains(t, warningText, tt.input,
					"Warnings should mention the problematic name")
				assert.Contains(t, warningText, suggestedName,
					"Warnings should include suggested name")

				// Check for specific warning types
				switch tt.wantWarningType {
				case "dots":
					assert.Contains(t, warningText, "dots (.)",
						"Expected warning about dots")
				case "colons":
					assert.Contains(t, warningText, "colons (:)",
						"Expected warning about colons")
				case "spaces":
					assert.Contains(t, warningText, "spaces",
						"Expected warning about spaces")
				case "multiple":
					// Should have multiple warnings
					assert.GreaterOrEqual(t, len(warnings), 3,
						"Expected multiple warnings for mixed problematic characters")
				}

				// Verify safe/unsafe character documentation is included
				assert.Contains(t, warningText, "Safe characters",
					"Warnings should document safe characters")
				assert.Contains(t, warningText, "Unsafe characters",
					"Warnings should document unsafe characters")
			} else {
				assert.Empty(t, warnings, "Expected no warnings for safe name %q", tt.input)
			}
		})
	}
}

// TestValidateSessionNameWarningContent tests that warnings include helpful information
func TestValidateSessionNameWarningContent(t *testing.T) {
	warnings, suggestedName, hasIssues := ValidateSessionName("project.v1.2:test")

	assert.True(t, hasIssues, "Should detect issues")
	assert.Equal(t, "project-v1-2-test", suggestedName)
	assert.NotEmpty(t, warnings, "Should have warnings")

	warningText := strings.Join(warnings, "\n")

	// Verify educational content
	t.Run("includes explanation", func(t *testing.T) {
		assert.Contains(t, warningText, "tmux will normalize",
			"Should explain what tmux does")
		assert.Contains(t, warningText, "converts",
			"Should explain the conversion behavior")
	})

	t.Run("includes safe characters documentation", func(t *testing.T) {
		assert.Contains(t, warningText, "alphanumeric",
			"Should document alphanumeric as safe")
		assert.Contains(t, warningText, "dash (-)",
			"Should document dash as safe")
		assert.Contains(t, warningText, "underscore (_)",
			"Should document underscore as safe")
	})

	t.Run("includes unsafe characters documentation", func(t *testing.T) {
		assert.Contains(t, warningText, "dots (.)",
			"Should document dots as unsafe")
		assert.Contains(t, warningText, "colons (:)",
			"Should document colons as unsafe")
		assert.Contains(t, warningText, "spaces",
			"Should document spaces as unsafe")
	})

	t.Run("includes background/rationale", func(t *testing.T) {
		assert.Contains(t, warningText, "Background",
			"Should include background on why this matters")
		assert.Contains(t, warningText, "lookup failures",
			"Should mention the impact (lookup failures)")
	})

	t.Run("includes suggested action", func(t *testing.T) {
		assert.Contains(t, warningText, "Suggested name",
			"Should provide suggested name")
		assert.Contains(t, warningText, suggestedName,
			"Should include the actual suggested name")
	})
}

// TestValidateSessionNameEdgeCases tests edge cases
func TestValidateSessionNameEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantHasIssues bool
	}{
		{
			name:          "empty string",
			input:         "",
			wantHasIssues: false,
		},
		{
			name:          "single dash",
			input:         "-",
			wantHasIssues: false,
		},
		{
			name:          "single underscore",
			input:         "_",
			wantHasIssues: false,
		},
		{
			name:          "mixed case alphanumeric",
			input:         "MySession123",
			wantHasIssues: false,
		},
		{
			name:          "leading dot",
			input:         ".hidden",
			wantHasIssues: true,
		},
		{
			name:          "trailing dot",
			input:         "session.",
			wantHasIssues: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, hasIssues := ValidateSessionName(tt.input)
			assert.Equal(t, tt.wantHasIssues, hasIssues)
		})
	}
}
