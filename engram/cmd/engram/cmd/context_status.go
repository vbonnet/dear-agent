package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/context"
)

var (
	statusSessionID string
	statusCLI       string
	statusJSON      bool
)

var contextStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current context usage and zone",
	Long: `Display current context window usage with zone classification.

Auto-detects CLI type (Claude Code, Gemini CLI, OpenCode, Codex) and extracts
token usage from session data. Shows percentage, zone, and model information.

FLAGS
  --session ID    Session ID to check (auto-detects if omitted)
  --cli TYPE      CLI type: claude, gemini, opencode, codex (auto-detects if omitted)
  --json          Output as JSON

EXAMPLES
  # Show status for current session (auto-detect)
  $ engram context status

  # Show status for specific session
  $ engram context status --session abc-123-def

  # Show status for Gemini CLI
  $ engram context status --cli gemini

  # JSON output for programmatic use
  $ engram context status --json

OUTPUT
  Context: 72.0% (144K/200K)
  Zone: ⚠️  warning
  Model: claude-sonnet-4.5
  Source: claude-cli
  Thresholds:
    Sweet spot: 60.0% (120K tokens)
    Warning: 70.0% (140K tokens)
    Danger: 80.0% (160K tokens)
    Critical: 90.0% (180K tokens)`,
	RunE: runContextStatus,
}

func init() {
	contextCmd.AddCommand(contextStatusCmd)

	contextStatusCmd.Flags().StringVar(&statusSessionID, "session", "", "Session ID to check")
	contextStatusCmd.Flags().StringVar(&statusCLI, "cli", "", "CLI type (claude, gemini, opencode, codex)")
	contextStatusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output as JSON")
}

func runContextStatus(cmd *cobra.Command, args []string) error {
	// Load registry
	registry, err := context.NewRegistry("")
	if err != nil {
		return fmt.Errorf("failed to load model registry: %w", err)
	}

	// Create detector
	detector := context.NewDetector(registry)

	// Detect usage
	var usage *context.Usage
	if statusSessionID != "" && statusCLI != "" {
		// Explicit session and CLI
		cliType := parseCLIType(statusCLI)
		usage, err = detector.DetectFromSession(statusSessionID, cliType)
	} else if statusSessionID != "" {
		// Explicit session, auto-detect CLI
		cliType := detector.DetectCLI()
		usage, err = detector.DetectFromSession(statusSessionID, cliType)
	} else {
		// Auto-detect everything
		usage, err = detector.Detect()
	}

	if err != nil {
		return fmt.Errorf("failed to detect context usage: %w", err)
	}

	// Calculate zone and thresholds
	calculator := context.NewCalculator(registry)
	result, err := calculator.Check(usage, context.PhaseMiddle)
	if err != nil {
		return fmt.Errorf("failed to calculate context zone: %w", err)
	}

	// Output
	if statusJSON {
		return outputStatusJSON(usage, result)
	}

	return outputStatusText(usage, result)
}

func parseCLIType(cliStr string) context.CLI {
	switch cliStr {
	case "claude":
		return context.CLIClaude
	case "gemini":
		return context.CLIGemini
	case "opencode":
		return context.CLIOpenCode
	case "codex":
		return context.CLICodex
	default:
		return context.CLIUnknown
	}
}

func outputStatusText(usage *context.Usage, result *context.CheckResult) error {
	fmt.Printf("Context: %s (%s)\n",
		context.FormatPercentage(result.Percentage),
		context.FormatTokens(usage.UsedTokens, usage.TotalTokens))
	fmt.Printf("Zone: %s\n", context.FormatZone(result.Zone))
	fmt.Printf("Model: %s\n", usage.ModelID)
	fmt.Printf("Source: %s\n", usage.Source)

	fmt.Println("\nThresholds:")
	fmt.Printf("  Sweet spot: %s (%s tokens)\n",
		context.FormatPercentage(result.Thresholds.SweetSpotPercentage*100),
		formatTokenCount(result.Thresholds.SweetSpotTokens))
	fmt.Printf("  Warning: %s (%s tokens)\n",
		context.FormatPercentage(result.Thresholds.WarningPercentage*100),
		formatTokenCount(result.Thresholds.WarningTokens))
	fmt.Printf("  Danger: %s (%s tokens)\n",
		context.FormatPercentage(result.Thresholds.DangerPercentage*100),
		formatTokenCount(result.Thresholds.DangerTokens))
	fmt.Printf("  Critical: %s (%s tokens)\n",
		context.FormatPercentage(result.Thresholds.CriticalPercentage*100),
		formatTokenCount(result.Thresholds.CriticalTokens))

	// Show compaction recommendation if needed
	if result.ShouldCompact {
		fmt.Println("\n⚠️  Compaction recommended")
	}

	return nil
}

func outputStatusJSON(usage *context.Usage, result *context.CheckResult) error {
	output := map[string]interface{}{
		"percentage":     result.Percentage,
		"zone":           result.Zone,
		"model_id":       usage.ModelID,
		"source":         usage.Source,
		"used_tokens":    usage.UsedTokens,
		"total_tokens":   usage.TotalTokens,
		"should_compact": result.ShouldCompact,
		"thresholds": map[string]interface{}{
			"sweet_spot_percentage": result.Thresholds.SweetSpotPercentage,
			"sweet_spot_tokens":     result.Thresholds.SweetSpotTokens,
			"warning_percentage":    result.Thresholds.WarningPercentage,
			"warning_tokens":        result.Thresholds.WarningTokens,
			"danger_percentage":     result.Thresholds.DangerPercentage,
			"danger_tokens":         result.Thresholds.DangerTokens,
			"critical_percentage":   result.Thresholds.CriticalPercentage,
			"critical_tokens":       result.Thresholds.CriticalTokens,
		},
		"timestamp": result.Timestamp,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func formatTokenCount(tokens int) string {
	if tokens >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(tokens)/1000000.0)
	} else if tokens >= 1000 {
		return fmt.Sprintf("%dK", tokens/1000)
	}
	return fmt.Sprintf("%d", tokens)
}
