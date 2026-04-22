package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/context"
)

var (
	compactFocus       string
	compactStrategy    string
	compactSessionType string
	compactModel       string
	compactTokens      string
	compactJSON        bool
)

var contextCompactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Compact context window with three-layer defense",
	Long: `Three-layer context compaction with per-session-type thresholds and anti-loop safety.

LAYERS
  Layer 1 (Prevention):  Reduce verbosity before compaction needed
  Layer 2 (Compaction):  Active context compaction
  Layer 3 (Rotation):    Session rotation when compaction insufficient

SESSION TYPES & THRESHOLDS (prevention/compaction/rotation)
  orchestrator:      55% / 65% / 80%
  worker:            70% / 80% / 90%
  meta-orchestrator: 50% / 60% / 75%

ANTI-LOOP SAFETY
  - 2hr cooldown window
  - Max 3 compactions per session
  - Post-compact baseline check

FLAGS
  --focus INSTRUCTION     Context preservation instructions (required)
  --strategy MODE         conservative or aggressive (default: conservative)
  --session-type TYPE     orchestrator, worker, meta-orchestrator (default: orchestrator)
  --model MODEL           Model ID (required)
  --tokens USED/TOTAL     Token usage (required)
  --json                  Output as JSON

EXAMPLES
  $ engram context compact --focus "preserve API design decisions" \
      --model claude-sonnet-4.5 --tokens 140000/200000

  $ engram context compact --focus "keep test patterns" \
      --strategy aggressive --session-type worker \
      --model claude-sonnet-4.5 --tokens 160000/200000 --json`,
	RunE: runContextCompact,
}

func init() {
	contextCmd.AddCommand(contextCompactCmd)

	contextCompactCmd.Flags().StringVar(&compactFocus, "focus", "", "Context preservation instructions (required)")
	contextCompactCmd.Flags().StringVar(&compactStrategy, "strategy", "conservative", "Compaction strategy: conservative, aggressive")
	contextCompactCmd.Flags().StringVar(&compactSessionType, "session-type", "orchestrator", "Session type: orchestrator, worker, meta-orchestrator")
	contextCompactCmd.Flags().StringVar(&compactModel, "model", "", "Model ID (required)")
	contextCompactCmd.Flags().StringVar(&compactTokens, "tokens", "", "Token usage USED/TOTAL (required)")
	contextCompactCmd.Flags().BoolVar(&compactJSON, "json", false, "Output as JSON")

	contextCompactCmd.MarkFlagRequired("focus")
	contextCompactCmd.MarkFlagRequired("model")
	contextCompactCmd.MarkFlagRequired("tokens")
}

func runContextCompact(cmd *cobra.Command, args []string) error {
	used, total, err := parseTokens(compactTokens)
	if err != nil {
		return fmt.Errorf("invalid --tokens format: %w (expected USED/TOTAL)", err)
	}

	sessionType, err := context.ParseSessionType(compactSessionType)
	if err != nil {
		return err
	}

	strategy, err := context.ParseStrategy(compactStrategy)
	if err != nil {
		return err
	}

	registry, err := context.NewRegistry("")
	if err != nil {
		return fmt.Errorf("failed to load model registry: %w", err)
	}

	percentage := float64(used) / float64(total) * 100.0
	usage := &context.Usage{
		TotalTokens:    total,
		UsedTokens:     used,
		PercentageUsed: percentage,
		ModelID:        compactModel,
		Source:         "manual",
	}

	engine := context.NewCompactEngine(registry)
	result := engine.Compact(&context.CompactConfig{
		Focus:       compactFocus,
		SessionType: sessionType,
		Strategy:    strategy,
		Usage:       usage,
	})

	if compactJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}

	return outputCompactText(result)
}

func outputCompactText(result *context.CompactResult) error {
	if !result.Success {
		fmt.Fprintf(os.Stderr, "Error: %s\n", result.Error)
		os.Exit(1)
	}

	fmt.Printf("Layer: %s\n", result.Layer)
	fmt.Printf("Reduction: %.0f%%\n", result.ReductionPct)
	fmt.Printf("New zone: %s\n", context.FormatZone(result.NewZone))

	if len(result.CompactedItems) > 0 {
		fmt.Println("\nActions:")
		for _, item := range result.CompactedItems {
			fmt.Printf("  [%s] %s → %s\n", item.Type, item.Description, item.Action)
		}
	}

	if result.Cooldown != nil {
		fmt.Printf("\nCompactions: %d/%d", result.Cooldown.CompactionCount, result.Cooldown.MaxCompactions)
		if result.Cooldown.Active {
			fmt.Printf(" (cooldown: %s)", result.Cooldown.CooldownRemaining)
		}
		fmt.Println()
	}

	return nil
}
