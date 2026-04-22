package tmux

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidationIntegration_DotsInSessionName tests end-to-end validation
// for BUG-001 scenario: session names with dots
func TestValidationIntegration_DotsInSessionName(t *testing.T) {
	// This is the actual BUG-001 incident case
	sessionName := "gemini-task-1.4"

	// Step 1: Validate the session name
	warnings, suggestedName, hasIssues := ValidateSessionName(sessionName)

	// Verify validation detected the issue
	assert.True(t, hasIssues, "Should detect dots in session name")
	assert.Equal(t, "gemini-task-1-4", suggestedName, "Should suggest normalized name")
	assert.NotEmpty(t, warnings, "Should provide warnings")

	// Step 2: Verify normalization matches suggested name
	normalizedName := NormalizeTmuxSessionName(sessionName)
	assert.Equal(t, suggestedName, normalizedName,
		"Suggested name should match normalization result")

	// Step 3: Verify suggested name passes validation
	_, _, suggestedHasIssues := ValidateSessionName(suggestedName)
	assert.False(t, suggestedHasIssues,
		"Suggested name should be safe (no issues)")
}

// TestValidationIntegration_ColonsInSessionName tests colon handling
func TestValidationIntegration_ColonsInSessionName(t *testing.T) {
	sessionName := "project:staging"

	_, suggestedName, hasIssues := ValidateSessionName(sessionName)

	assert.True(t, hasIssues)
	assert.Equal(t, "project-staging", suggestedName)

	normalizedName := NormalizeTmuxSessionName(sessionName)
	assert.Equal(t, suggestedName, normalizedName)

	// Suggested name should be safe
	_, _, suggestedHasIssues := ValidateSessionName(suggestedName)
	assert.False(t, suggestedHasIssues)
}

// TestValidationIntegration_SpacesInSessionName tests space handling
func TestValidationIntegration_SpacesInSessionName(t *testing.T) {
	sessionName := "my session"

	_, suggestedName, hasIssues := ValidateSessionName(sessionName)

	assert.True(t, hasIssues)
	assert.Equal(t, "my-session", suggestedName)

	normalizedName := NormalizeTmuxSessionName(sessionName)
	assert.Equal(t, suggestedName, normalizedName)

	// Suggested name should be safe
	_, _, suggestedHasIssues := ValidateSessionName(suggestedName)
	assert.False(t, suggestedHasIssues)
}

// TestValidationIntegration_MultipleProblematicCharacters tests complex case
func TestValidationIntegration_MultipleProblematicCharacters(t *testing.T) {
	sessionName := "project.v1:staging test"

	warnings, suggestedName, hasIssues := ValidateSessionName(sessionName)

	assert.True(t, hasIssues)
	assert.Equal(t, "project-v1-staging-test", suggestedName)

	// Verify warnings mention all problematic character types
	warningText := ""
	for _, w := range warnings {
		warningText += w + " "
	}
	assert.Contains(t, warningText, "dots")
	assert.Contains(t, warningText, "colons")
	assert.Contains(t, warningText, "spaces")

	normalizedName := NormalizeTmuxSessionName(sessionName)
	assert.Equal(t, suggestedName, normalizedName)

	// Suggested name should be safe
	_, _, suggestedHasIssues := ValidateSessionName(suggestedName)
	assert.False(t, suggestedHasIssues)
}

// TestValidationIntegration_SafeNamePassesThrough tests that safe names work
func TestValidationIntegration_SafeNamePassesThrough(t *testing.T) {
	testCases := []string{
		"my-session",
		"my_session",
		"session123",
		"MySession",
		"complex-session_name-123",
	}

	for _, sessionName := range testCases {
		t.Run(sessionName, func(t *testing.T) {
			warnings, suggestedName, hasIssues := ValidateSessionName(sessionName)

			assert.False(t, hasIssues, "Safe name should pass validation")
			assert.Empty(t, warnings, "Safe name should have no warnings")
			assert.Empty(t, suggestedName, "Safe name needs no suggestion")

			// Normalization should be identity function for safe names
			normalizedName := NormalizeTmuxSessionName(sessionName)
			assert.Equal(t, sessionName, normalizedName,
				"Safe names should not be changed by normalization")
		})
	}
}

// TestValidationIntegration_WarningQuality tests warning message quality
func TestValidationIntegration_WarningQuality(t *testing.T) {
	sessionName := "test.session"

	warnings, suggestedName, hasIssues := ValidateSessionName(sessionName)
	require.True(t, hasIssues)

	warningText := ""
	for _, w := range warnings {
		warningText += w + "\n"
	}

	// Verify warning includes all required information
	tests := []struct {
		name     string
		contains string
		why      string
	}{
		{
			name:     "mentions problematic name",
			contains: sessionName,
			why:      "User needs to know which name is problematic",
		},
		{
			name:     "mentions suggested name",
			contains: suggestedName,
			why:      "User needs to know what to use instead",
		},
		{
			name:     "explains safe characters",
			contains: "alphanumeric",
			why:      "User needs to know what's allowed",
		},
		{
			name:     "explains unsafe characters",
			contains: "dots",
			why:      "User needs to know what's not allowed",
		},
		{
			name:     "provides background",
			contains: "Background",
			why:      "User needs to understand why this matters",
		},
		{
			name:     "mentions consequences",
			contains: "lookup failures",
			why:      "User needs to know the impact",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, warningText, tt.contains, tt.why)
		})
	}
}

// TestValidationIntegration_IdempotentNormalization tests normalization is idempotent
func TestValidationIntegration_IdempotentNormalization(t *testing.T) {
	// Normalizing a normalized name should be a no-op
	testCases := []string{
		"gemini-task-1.4",
		"project:staging",
		"my session",
		"complex.name:with spaces",
	}

	for _, original := range testCases {
		t.Run(original, func(t *testing.T) {
			// First normalization
			normalized1 := NormalizeTmuxSessionName(original)

			// Second normalization (should be idempotent)
			normalized2 := NormalizeTmuxSessionName(normalized1)

			assert.Equal(t, normalized1, normalized2,
				"Normalization should be idempotent: normalizing twice gives same result")

			// Normalized name should pass validation
			_, _, hasIssues := ValidateSessionName(normalized1)
			assert.False(t, hasIssues,
				"Normalized name should always pass validation")
		})
	}
}
