package tmux

import (
	"fmt"
	"strings"
)

// ValidateSessionName checks if a session name contains problematic characters
// and returns warnings and suggestions for normalization.
//
// This addresses BUG-001 Phase 2: Warning users about unsafe characters
// in session names that tmux will automatically normalize.
//
// Returns:
//   - warnings: list of issues found with the session name
//   - suggestedName: normalized version of the name (empty if no issues)
//   - hasIssues: true if the name contains problematic characters
func ValidateSessionName(name string) (warnings []string, suggestedName string, hasIssues bool) {
	warnings = []string{}
	hasIssues = false

	// Check for problematic characters
	hasDots := strings.Contains(name, ".")
	hasColons := strings.Contains(name, ":")
	hasSpaces := strings.Contains(name, " ")

	// Build warnings for each problematic character type
	if hasDots {
		warnings = append(warnings, "⚠️  Session name contains dots (.) which tmux converts to dashes (-)")
		hasIssues = true
	}

	if hasColons {
		warnings = append(warnings, "⚠️  Session name contains colons (:) which tmux converts to dashes (-)")
		hasIssues = true
	}

	if hasSpaces {
		warnings = append(warnings, "⚠️  Session name contains spaces which tmux converts to dashes (-)")
		hasIssues = true
	}

	// If issues found, generate normalized suggestion
	if hasIssues {
		suggestedName = NormalizeTmuxSessionName(name)

		// Add general warning about deprecated characters
		warnings = append([]string{
			fmt.Sprintf("❌ Session name '%s' contains characters that tmux will normalize", name),
		}, warnings...)

		warnings = append(warnings,
			"",
			fmt.Sprintf("💡 Suggested name: '%s'", suggestedName),
			"",
			"Safe characters: alphanumeric (a-z, A-Z, 0-9), dash (-), underscore (_)",
			"Unsafe characters: dots (.), colons (:), spaces",
			"",
			"Background: tmux automatically converts unsafe characters to dashes,",
			"which can cause session lookup failures and message delivery issues.",
			"",
			"Use the suggested name to avoid these problems.",
		)
	}

	return warnings, suggestedName, hasIssues
}

// PrintValidationWarnings prints session name validation warnings to the user
func PrintValidationWarnings(name string, warnings []string, suggestedName string) {
	if len(warnings) == 0 {
		return
	}

	fmt.Println()
	for _, warning := range warnings {
		fmt.Println(warning)
	}
	fmt.Println()
}
