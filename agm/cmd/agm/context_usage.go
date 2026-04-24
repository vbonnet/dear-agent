package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

var (
	contextUsageSession string
)

var setContextUsageCmd = &cobra.Command{
	Use:   "set-context-usage <percentage>",
	Short: "Set context usage percentage for a session",
	Long: `Set the context usage percentage for an AGM session.

This command updates the session's manifest with context usage information,
which is then displayed in the tmux status line.

Examples:
  agm session set-context-usage 75
  agm session set-context-usage 82 --session my-session`,
	Args: cobra.ExactArgs(1),
	RunE: runSetContextUsage,
}

func init() {
	setContextUsageCmd.Flags().StringVarP(
		&contextUsageSession,
		"session",
		"s",
		"",
		"Session name (default: auto-detect from tmux)",
	)
	sessionCmd.AddCommand(setContextUsageCmd)
}

func runSetContextUsage(cmd *cobra.Command, args []string) error {
	// Parse percentage argument
	percentStr := args[0]
	percent, err := strconv.ParseFloat(percentStr, 64)
	if err != nil {
		return fmt.Errorf("invalid percentage '%s': must be a number", percentStr)
	}

	// Validate percentage range
	if percent < 0 || percent > 100 {
		return fmt.Errorf("invalid percentage %.2f: must be between 0 and 100", percent)
	}

	// Get Dolt storage adapter
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer adapter.Close()

	// Determine session name
	sessionName := contextUsageSession
	if sessionName == "" {
		// Auto-detect from tmux
		detectedName, err := autoDetectTmuxSession()
		if err != nil {
			return fmt.Errorf("failed to detect session: %w\nUse --session flag to specify explicitly", err)
		}
		sessionName = detectedName
	}

	// Find manifest
	m, err := findManifestBySession(sessionName)
	if err != nil {
		return err
	}

	// Update context usage
	// Assume Claude 4 context window: 200,000 tokens
	const defaultTotalTokens = 200000
	totalTokens := defaultTotalTokens

	// Calculate used tokens from percentage
	usedTokens := int(float64(totalTokens) * (percent / 100.0))

	// Create or update ContextUsage
	if m.ContextUsage == nil {
		m.ContextUsage = &manifest.ContextUsage{}
	}

	m.ContextUsage.TotalTokens = totalTokens
	m.ContextUsage.UsedTokens = usedTokens
	m.ContextUsage.PercentageUsed = percent
	m.ContextUsage.LastUpdated = time.Now()
	m.ContextUsage.Source = "manual"

	// Write updated manifest to Dolt
	if err := adapter.UpdateSession(m); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	// Display success
	fmt.Printf("✓ Updated context usage for session '%s'\n", m.Name)
	fmt.Printf("  Percentage: %.1f%%\n", percent)
	fmt.Printf("  Used: %s / %s tokens\n", formatTokens(usedTokens), formatTokens(totalTokens))
	fmt.Printf("  Updated: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	return nil
}

// formatTokens formats token count with thousand separators
func formatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d,%03d", n/1000, n%1000)
}
