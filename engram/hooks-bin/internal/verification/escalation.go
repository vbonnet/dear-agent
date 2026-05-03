package verification

import (
	"fmt"
	"io"
	"strings"
)

const (
	// EscalationThreshold is the number of tool uses before escalation.
	// After this many tool uses without addressing the verification,
	// a stronger reminder is output.
	EscalationThreshold = 5

	// MaxPendingAgeMinutes is the maximum age in minutes of a pending verification
	// before it's automatically pruned (prevents stale state accumulation).
	MaxPendingAgeMinutes = 30
)

// EscalationResult describes what escalation action was taken.
type EscalationResult struct {
	Escalated    bool
	Message      string
	Verification PendingVerification
}

// CheckEscalations reviews all pending verifications and returns
// any that have exceeded the escalation threshold.
func CheckEscalations(state *State) []EscalationResult {
	var results []EscalationResult

	for _, v := range state.Pending {
		if v.ToolUsesSince >= EscalationThreshold {
			result := EscalationResult{
				Escalated:    true,
				Verification: v,
				Message:      formatEscalation(v),
			}
			results = append(results, result)
		}
	}

	return results
}

// WriteEscalations outputs escalation messages to stderr.
// Returns the number of escalations written.
func WriteEscalations(stderr io.Writer, results []EscalationResult) int {
	count := 0
	for _, r := range results {
		if r.Escalated {
			fmt.Fprint(stderr, r.Message)
			count++
		}
	}
	return count
}

func formatEscalation(v PendingVerification) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("-", 50))
	sb.WriteString("\n")
	sb.WriteString("ESCALATION: Unaddressed Verification Finding\n")
	sb.WriteString(strings.Repeat("-", 50))
	sb.WriteString("\n")

	switch v.Type {
	case "bead_close":
		fmt.Fprintf(&sb, "Type: Bead close (swarm: %s)\n", v.SwarmLabel)
		fmt.Fprintf(&sb, "Bead: %s\n", v.BeadID)
		fmt.Fprintf(&sb, "Ignored for: %d tool uses\n", v.ToolUsesSince)
		sb.WriteString("\n")
		sb.WriteString("Action Required:\n")
		fmt.Fprintf(&sb, "  1. Check remaining beads: bd list -l %s\n", v.SwarmLabel)
		sb.WriteString("  2. Pick next task and continue work\n")
		sb.WriteString("  3. Or explicitly defer: bd defer <bead-id>\n")

	case "notification_send":
		fmt.Fprintf(&sb, "Type: Notification sent to %s\n", v.Recipient)
		fmt.Fprintf(&sb, "Ignored for: %d tool uses\n", v.ToolUsesSince)
		sb.WriteString("\n")
		sb.WriteString("Action Required:\n")
		fmt.Fprintf(&sb, "  1. Check for response from %s\n", v.Recipient)
		sb.WriteString("  2. Re-send if no response received\n")
		sb.WriteString("  3. Or continue with other work while waiting\n")

	default:
		fmt.Fprintf(&sb, "Type: %s\n", v.Type)
		fmt.Fprintf(&sb, "Message: %s\n", v.Message)
		fmt.Fprintf(&sb, "Ignored for: %d tool uses\n", v.ToolUsesSince)
	}

	sb.WriteString(strings.Repeat("-", 50))
	sb.WriteString("\n")

	return sb.String()
}
