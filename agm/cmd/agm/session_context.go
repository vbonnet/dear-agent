package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var sessionContextCmd = &cobra.Command{
	Use:   "context <identifier>",
	Short: "Show context window usage for a session",
	Long: `Show context window usage (token count, percentage) for a session.

Detects context usage from multiple sources in priority order:
  1. Manifest (updated by PostToolUse hook)
  2. Statusline file (written by statusline command)
  3. Conversation log parsing (fallback)

Examples:
  agm session context my-session           # Show context usage
  agm session context my-session -o json   # JSON output

See Also:
  • agm session get     - Full session metadata
  • agm session compact - Trigger context compaction`,
	Args: cobra.ExactArgs(1),
	RunE: runSessionContext,
}

func init() {
	sessionCmd.AddCommand(sessionContextCmd)
}

// SessionContextResult is the structured output for JSON mode.
type SessionContextResult struct {
	Operation string               `json:"operation"`
	SessionID string               `json:"session_id"`
	Name      string               `json:"name"`
	Context   *SessionContextUsage `json:"context,omitempty"`
	Error     string               `json:"error,omitempty"`
}

// SessionContextUsage holds context usage details for display.
type SessionContextUsage struct {
	TotalTokens    int     `json:"total_tokens"`
	UsedTokens     int     `json:"used_tokens"`
	PercentageUsed float64 `json:"percentage_used"`
	ModelID        string  `json:"model_id,omitempty"`
	Source         string  `json:"source"`
	LastUpdated    string  `json:"last_updated"`
	EstimatedCost  float64 `json:"estimated_cost,omitempty"`
}

func runSessionContext(_ *cobra.Command, args []string) error {
	identifier := args[0]

	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer cleanup()

	// Resolve session
	getResult, opErr := ops.GetSession(opCtx, &ops.GetSessionRequest{
		Identifier: identifier,
	})
	if opErr != nil {
		return handleError(opErr)
	}

	s := getResult.Session

	// Get the raw manifest for context detection
	m, err := opCtx.Storage.GetSession(s.ID)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	// Detect context usage via cascade
	usage, err := session.DetectContextFromManifestOrLog(m)

	result := &SessionContextResult{
		Operation: "session_context",
		SessionID: s.ID,
		Name:      s.Name,
	}

	if err != nil {
		result.Error = "Context usage unavailable. Session may not have been active recently."
		return printResult(result, func() {
			fmt.Printf("Context Usage for %s:\n", ui.Bold(s.Name))
			ui.PrintWarning("Context usage unavailable. Session may not have been active recently.")
			fmt.Printf("\nSuggestions:\n")
			fmt.Printf("  - Ensure the session is running: agm session get %s\n", identifier)
			fmt.Printf("  - Wait for Claude to respond (context is extracted from conversation)\n")
		})
	}

	result.Context = &SessionContextUsage{
		TotalTokens:    usage.TotalTokens,
		UsedTokens:     usage.UsedTokens,
		PercentageUsed: usage.PercentageUsed,
		ModelID:        usage.ModelID,
		Source:         usage.Source,
		LastUpdated:    usage.LastUpdated.Format(time.RFC3339),
		EstimatedCost:  usage.EstimatedCost,
	}

	return printResult(result, func() {
		fmt.Printf("Context Usage for %s:\n", ui.Bold(s.Name))
		fmt.Printf("  Tokens:     %s / %s\n", formatTokenCount(usage.UsedTokens), formatTokenCount(usage.TotalTokens))
		fmt.Printf("  Usage:      %.1f%%\n", usage.PercentageUsed)
		if usage.ModelID != "" {
			fmt.Printf("  Model:      %s\n", usage.ModelID)
		}
		fmt.Printf("  Source:     %s\n", usage.Source)
		fmt.Printf("  Updated:    %s\n", usage.LastUpdated.Format("2006-01-02 15:04:05"))
		if usage.EstimatedCost > 0 {
			fmt.Printf("  Est. Cost:  $%.2f\n", usage.EstimatedCost)
		}

		// Show a visual bar for percentage
		fmt.Printf("  Bar:        %s\n", contextBar(usage.PercentageUsed))
	})
}

// formatTokenCount formats a token count with commas for readability.
func formatTokenCount(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	return fmt.Sprintf("%d,%03d,%03d", n/1000000, (n/1000)%1000, n%1000)
}

// contextBar renders a simple ASCII progress bar for context usage.
func contextBar(percentage float64) string {
	const barWidth = 30
	filled := int(percentage / 100.0 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	bar := ""
	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	return fmt.Sprintf("[%s] %.1f%%", bar, percentage)
}
