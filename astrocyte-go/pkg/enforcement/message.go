package enforcement

import (
	"fmt"
	"strings"
)

// GenerateRejectionMessage creates a user-facing rejection message for a detected violation.
// The message format matches Python Astrocyte's rejection messages, including:
// - Pattern ID and reason for rejection
// - Alternative/correct approach
// - Examples demonstrating the correct pattern
//
// This message is intended to be sent to the agent via tmux or other messaging systems.
func GenerateRejectionMessage(pattern *Pattern, command string) string {
	var msg strings.Builder

	// Header with pattern ID
	msg.WriteString("🚫 Violation Detected: ")
	msg.WriteString(pattern.ID)
	msg.WriteString("\n\n")

	// Command that was rejected
	if command != "" {
		msg.WriteString("Command: ")
		msg.WriteString(command)
		msg.WriteString("\n\n")
	}

	// Reason for rejection
	msg.WriteString("Reason: ")
	msg.WriteString(pattern.Reason)
	msg.WriteString("\n\n")

	// Alternative/correct approach
	msg.WriteString("Correct approach: ")
	msg.WriteString(pattern.Alternative)
	msg.WriteString("\n\n")

	// Include tier1_example if available (shows BAD vs GOOD)
	if pattern.Tier1Example != "" {
		msg.WriteString("Example:\n")
		msg.WriteString(pattern.Tier1Example)
		msg.WriteString("\n\n")
	} else if len(pattern.Examples) > 0 {
		// Fallback to examples list if tier1_example not available
		msg.WriteString("Examples of violations:\n")
		for _, ex := range pattern.Examples {
			msg.WriteString("  - ")
			msg.WriteString(ex)
			msg.WriteString("\n")
		}
		msg.WriteString("\n")
	}

	// Footer
	msg.WriteString("Please revise your approach and try again.")

	return msg.String()
}

// GenerateShortRejectionMessage creates a brief rejection message suitable for
// notifications or log summaries. Format: "[pattern-id] Reason"
func GenerateShortRejectionMessage(pattern *Pattern) string {
	return fmt.Sprintf("[%s] %s", pattern.ID, pattern.Reason)
}

// GenerateRejectionMessageWithSeverity creates a rejection message that includes
// the severity level of the violation.
func GenerateRejectionMessageWithSeverity(pattern *Pattern, command string) string {
	var msg strings.Builder

	// Header with pattern ID and severity
	msg.WriteString("🚫 Violation Detected: ")
	msg.WriteString(pattern.ID)
	msg.WriteString(" [")
	msg.WriteString(strings.ToUpper(pattern.Severity))
	msg.WriteString("]\n\n")

	// Command that was rejected
	if command != "" {
		msg.WriteString("Command: ")
		msg.WriteString(command)
		msg.WriteString("\n\n")
	}

	// Reason for rejection
	msg.WriteString("Reason: ")
	msg.WriteString(pattern.Reason)
	msg.WriteString("\n\n")

	// Alternative/correct approach
	msg.WriteString("Correct approach: ")
	msg.WriteString(pattern.Alternative)
	msg.WriteString("\n\n")

	// Include tier1_example if available
	if pattern.Tier1Example != "" {
		msg.WriteString("Example:\n")
		msg.WriteString(pattern.Tier1Example)
		msg.WriteString("\n\n")
	}

	// Severity-specific footer
	switch pattern.Severity {
	case "critical":
		msg.WriteString("⚠️  CRITICAL: This violation may cause data loss or system instability.\n")
		msg.WriteString("Please revise your approach immediately.")
	case "high":
		msg.WriteString("⚠️  This is a high-severity violation.\n")
		msg.WriteString("Please revise your approach and try again.")
	default:
		msg.WriteString("Please revise your approach and try again.")
	}

	return msg.String()
}
