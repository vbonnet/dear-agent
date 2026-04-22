package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/context"
)

var (
	checkModel      string
	checkTokens     string
	checkPhaseState string
	checkJSON       bool
)

var contextCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check context usage and get compaction recommendation",
	Long: `Check context usage for a specific model and token count.

Calculates the current zone and provides smart compaction recommendations
based on phase state (start, middle, end).

FLAGS
  --model MODEL         Model ID (e.g., claude-sonnet-4.5, gemini-2.0-flash)
  --tokens USED/TOTAL   Token usage (e.g., 140000/200000)
  --phase-state STATE   Phase state: start, middle, end (default: middle)
  --json                Output as JSON

EXAMPLES
  # Check if compaction needed at 70% usage
  $ engram context check --model claude-sonnet-4.5 --tokens 140000/200000

  # Check at phase start (more aggressive threshold)
  $ engram context check --model claude-sonnet-4.5 --tokens 140000/200000 --phase-state start

  # Check at phase end (can push +5%)
  $ engram context check --model claude-sonnet-4.5 --tokens 170000/200000 --phase-state end

  # JSON output
  $ engram context check --model gemini-2.0-flash --tokens 600000/1000000 --json

SMART COMPACTION LOGIC
  Phase Start:  Compact at WARNING (70%) - new phase adds context
  Phase Middle: Compact at DANGER (80%) - standard threshold
  Phase End:    Can push +5% (85%) - finish current work first

OUTPUT
  Zone: ⚠️  warning (70.0%)
  Should compact: true (phase start)
  Reason: Phase start at 70.0% - will exceed danger zone mid-phase. Compact now.
  Urgency: high
  Estimated reduction: 15-25%`,
	RunE: runContextCheck,
}

func init() {
	contextCmd.AddCommand(contextCheckCmd)

	contextCheckCmd.Flags().StringVar(&checkModel, "model", "", "Model ID (required)")
	contextCheckCmd.Flags().StringVar(&checkTokens, "tokens", "", "Token usage USED/TOTAL (required)")
	contextCheckCmd.Flags().StringVar(&checkPhaseState, "phase-state", "middle", "Phase state: start, middle, end")
	contextCheckCmd.Flags().BoolVar(&checkJSON, "json", false, "Output as JSON")

	contextCheckCmd.MarkFlagRequired("model")
	contextCheckCmd.MarkFlagRequired("tokens")
}

func runContextCheck(cmd *cobra.Command, args []string) error {
	// Parse tokens
	used, total, err := parseTokens(checkTokens)
	if err != nil {
		return fmt.Errorf("invalid --tokens format: %w (expected USED/TOTAL)", err)
	}

	// Parse phase state
	phaseState, err := parsePhaseState(checkPhaseState)
	if err != nil {
		return fmt.Errorf("invalid --phase-state: %w", err)
	}

	// Load registry
	registry, err := context.NewRegistry("")
	if err != nil {
		return fmt.Errorf("failed to load model registry: %w", err)
	}

	// Create usage object
	percentage := float64(used) / float64(total) * 100.0
	usage := &context.Usage{
		TotalTokens:    total,
		UsedTokens:     used,
		PercentageUsed: percentage,
		ModelID:        checkModel,
		Source:         "manual",
	}

	// Calculate zone and compaction
	calculator := context.NewCalculator(registry)
	result, err := calculator.Check(usage, phaseState)
	if err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Get compaction recommendation
	recommendation, err := calculator.GetCompactionRecommendation(usage, phaseState)
	if err != nil {
		return fmt.Errorf("failed to get recommendation: %w", err)
	}

	// Output
	if checkJSON {
		return outputCheckJSON(result, recommendation, phaseState)
	}

	return outputCheckText(result, recommendation, phaseState)
}

func parseTokens(tokensStr string) (int, int, error) {
	parts := strings.Split(tokensStr, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected format USED/TOTAL")
	}

	used, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid used tokens: %w", err)
	}

	total, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid total tokens: %w", err)
	}

	if used < 0 || total <= 0 || used > total {
		return 0, 0, fmt.Errorf("invalid token values (used=%d, total=%d)", used, total)
	}

	return used, total, nil
}

func parsePhaseState(stateStr string) (context.PhaseState, error) {
	switch strings.ToLower(stateStr) {
	case "start":
		return context.PhaseStart, nil
	case "middle":
		return context.PhaseMiddle, nil
	case "end":
		return context.PhaseEnd, nil
	default:
		return "", fmt.Errorf("unknown phase state: %s (expected: start, middle, end)", stateStr)
	}
}

func outputCheckText(result *context.CheckResult, rec *context.CompactionRecommendation, phaseState context.PhaseState) error {
	fmt.Printf("Zone: %s (%s)\n",
		context.FormatZone(result.Zone),
		context.FormatPercentage(result.Percentage))

	fmt.Printf("Should compact: %v", result.ShouldCompact)
	if result.ShouldCompact {
		fmt.Printf(" (phase %s)\n", phaseState)
	} else {
		fmt.Println()
	}

	if rec.Recommended {
		fmt.Printf("Reason: %s\n", rec.Reason)
		fmt.Printf("Urgency: %s\n", rec.Urgency)
		fmt.Printf("Estimated reduction: %s\n", rec.EstimatedReduction)
	} else {
		fmt.Printf("Status: %s\n", rec.Reason)
	}

	return nil
}

func outputCheckJSON(result *context.CheckResult, rec *context.CompactionRecommendation, phaseState context.PhaseState) error {
	output := map[string]interface{}{
		"zone":           result.Zone,
		"percentage":     result.Percentage,
		"should_compact": result.ShouldCompact,
		"phase_state":    phaseState,
		"model_id":       result.ModelID,
		"recommendation": map[string]interface{}{
			"recommended":         rec.Recommended,
			"reason":              rec.Reason,
			"urgency":             rec.Urgency,
			"estimated_reduction": rec.EstimatedReduction,
		},
		"thresholds": map[string]interface{}{
			"sweet_spot_percentage": result.Thresholds.SweetSpotPercentage,
			"warning_percentage":    result.Thresholds.WarningPercentage,
			"danger_percentage":     result.Thresholds.DangerPercentage,
			"critical_percentage":   result.Thresholds.CriticalPercentage,
		},
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
