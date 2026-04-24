package enforcement

import (
	"fmt"
	"strings"
)

// GenerateRejectionMessage creates a user-facing rejection message for a
// detected violation, including pattern ID, reason, alternative, and examples.
func GenerateRejectionMessage(pattern *Pattern, command string) string {
	var msg strings.Builder

	msg.WriteString("Pattern detected: ")
	msg.WriteString(pattern.ID)
	msg.WriteString("\n\n")

	if command != "" {
		msg.WriteString("Command: ")
		msg.WriteString(command)
		msg.WriteString("\n\n")
	}

	msg.WriteString("Reason: ")
	msg.WriteString(pattern.Reason)
	msg.WriteString("\n\n")

	msg.WriteString("Alternative: ")
	msg.WriteString(pattern.Alternative)
	msg.WriteString("\n\n")

	if pattern.Tier1Example != "" {
		msg.WriteString("Example:\n")
		msg.WriteString(pattern.Tier1Example)
		msg.WriteString("\n\n")
	} else if len(pattern.Examples) > 0 {
		msg.WriteString("Examples:\n")
		for _, ex := range pattern.Examples {
			msg.WriteString("  - ")
			msg.WriteString(ex)
			msg.WriteString("\n")
		}
		msg.WriteString("\n")
	}

	msg.WriteString("Please use the alternative approach above.")

	return msg.String()
}

// GenerateShortRejectionMessage creates a brief rejection message:
// "[pattern-id] Reason"
func GenerateShortRejectionMessage(pattern *Pattern) string {
	return fmt.Sprintf("[%s] %s", pattern.ID, pattern.Reason)
}

// GenerateRejectionMessageWithSeverity creates a rejection message that
// includes the severity level and severity-specific footer.
func GenerateRejectionMessageWithSeverity(pattern *Pattern, command string) string {
	var msg strings.Builder

	msg.WriteString("Pattern detected: ")
	msg.WriteString(pattern.ID)
	msg.WriteString(" [")
	msg.WriteString(strings.ToUpper(pattern.Severity))
	msg.WriteString("]\n\n")

	if command != "" {
		msg.WriteString("Command: ")
		msg.WriteString(command)
		msg.WriteString("\n\n")
	}

	msg.WriteString("Reason: ")
	msg.WriteString(pattern.Reason)
	msg.WriteString("\n\n")

	msg.WriteString("Alternative: ")
	msg.WriteString(pattern.Alternative)
	msg.WriteString("\n\n")

	if pattern.Tier1Example != "" {
		msg.WriteString("Example:\n")
		msg.WriteString(pattern.Tier1Example)
		msg.WriteString("\n\n")
	}

	switch pattern.Severity {
	case "critical":
		msg.WriteString("Note: This pattern may affect reliability. Please use the alternative above.")
	case "high":
		msg.WriteString("This pattern is flagged for review. Please use the alternative above.")
	default:
		msg.WriteString("Please use the alternative approach above.")
	}

	return msg.String()
}

// FormatHookDenial formats a pattern match result for PreToolUse hook output.
// Returns the pattern name and remediation suitable for hook stderr/JSON.
func FormatHookDenial(pattern *Pattern) (name string, remediation string) {
	name = pattern.PatternName
	if name == "" {
		name = pattern.ID
	}
	remediation = pattern.Remediation
	if remediation == "" {
		remediation = pattern.Alternative
	}
	return name, remediation
}
