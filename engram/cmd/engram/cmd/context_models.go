package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/context"
)

var (
	modelsListAll bool
	modelsModel   string
	modelsJSON    bool
)

var contextModelsCmd = &cobra.Command{
	Use:   "models",
	Short: "List supported models and their thresholds",
	Long: `Display supported LLM models and their context window configurations.

Shows max context size, sweet spot thresholds, and benchmark data sources
for each model family (Claude, Gemini, GPT).

FLAGS
  --list          List all models (default)
  --model ID      Show details for specific model
  --json          Output as JSON

EXAMPLES
  # List all supported models
  $ engram context models --list

  # Show thresholds for Claude Sonnet 4.5
  $ engram context models --model claude-sonnet-4.5

  # Show Gemini 2.0 Flash configuration
  $ engram context models --model gemini-2.0-flash

  # JSON output
  $ engram context models --list --json

OUTPUT (list)
  ┌─────────────────────┬────────────┬─────────────┬─────────┬─────────┬────────────┐
  │ Model               │ Provider   │ Max Context │ Sweet   │ Warning │ Confidence │
  ├─────────────────────┼────────────┼─────────────┼─────────┼─────────┼────────────┤
  │ claude-sonnet-4.5   │ anthropic  │ 200K        │ 60%     │ 70%     │ HIGH       │
  │ gemini-2.0-flash    │ google     │ 1M          │ 50%     │ 60%     │ MEDIUM     │
  │ gpt-4o              │ openai     │ 128K        │ 50%     │ 60%     │ MEDIUM     │
  └─────────────────────┴────────────┴─────────────┴─────────┴─────────┴────────────┘

OUTPUT (specific model)
  Model: claude-sonnet-4.5
  Provider: anthropic
  Max Context: 200,000 tokens

  Thresholds:
    Sweet spot: 60.0% (120K tokens)
    Warning: 70.0% (140K tokens)
    Danger: 80.0% (160K tokens)
    Critical: 90.0% (180K tokens)

  Benchmark Sources:
    - RULER: 85% accuracy up to 60% fill (~120K tokens)
    - Lost in the Middle: Degradation begins at 80% fill (~160K tokens)
    - Production Swarm Data: 60/70/80/90% thresholds validated

  Confidence: HIGH
  Notes: Production validated from engram swarm data. High confidence.`,
	RunE: runContextModels,
}

func init() {
	contextCmd.AddCommand(contextModelsCmd)

	contextModelsCmd.Flags().BoolVar(&modelsListAll, "list", false, "List all models")
	contextModelsCmd.Flags().StringVar(&modelsModel, "model", "", "Show details for specific model")
	contextModelsCmd.Flags().BoolVar(&modelsJSON, "json", false, "Output as JSON")
}

func runContextModels(cmd *cobra.Command, args []string) error {
	// Load registry
	registry, err := context.NewRegistry("")
	if err != nil {
		return fmt.Errorf("failed to load model registry: %w", err)
	}

	// Determine mode
	if modelsModel != "" {
		// Show specific model
		return showModelDetails(registry, modelsModel)
	}

	// List all models (default)
	return listAllModels(registry)
}

func listAllModels(registry *context.Registry) error {
	modelIDs := registry.ListModels()

	// Sort alphabetically
	sort.Strings(modelIDs)

	// Filter out aliases and "default"
	var uniqueModels []string
	seen := make(map[string]bool)

	for _, id := range modelIDs {
		// Skip default
		if id == "default" {
			continue
		}

		// Normalize to avoid duplicates (claude-sonnet-4.5 vs claude-sonnet-4-5)
		normalized := strings.ReplaceAll(id, "-", "")
		if seen[normalized] {
			continue
		}
		seen[normalized] = true

		uniqueModels = append(uniqueModels, id)
	}

	if modelsJSON {
		return outputModelsJSON(registry, uniqueModels)
	}

	return outputModelsTable(registry, uniqueModels)
}

func outputModelsTable(registry *context.Registry, modelIDs []string) error {
	// Header
	fmt.Println("\n╔═════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                          Supported Models                                   ║")
	fmt.Println("╚═════════════════════════════════════════════════════════════════════════════╝")
	fmt.Printf("\n%-25s %-12s %-12s %-8s %-8s %-12s\n",
		"Model", "Provider", "Max Context", "Sweet", "Warning", "Confidence")
	fmt.Println(strings.Repeat("─", 85))

	for _, id := range modelIDs {
		model := registry.GetModel(id)
		if model == nil {
			continue
		}

		fmt.Printf("%-25s %-12s %-12s %-8s %-8s %-12s\n",
			model.ModelID,
			model.Provider,
			formatContextSize(model.MaxContextTokens),
			fmt.Sprintf("%.0f%%", model.SweetSpotThreshold*100),
			fmt.Sprintf("%.0f%%", model.WarningThreshold*100),
			model.Confidence,
		)
	}

	fmt.Println()
	return nil
}

func outputModelsJSON(registry *context.Registry, modelIDs []string) error {
	var models []map[string]interface{}

	for _, id := range modelIDs {
		model := registry.GetModel(id)
		if model == nil {
			continue
		}

		thresholds, _ := registry.GetThresholds(id)

		models = append(models, map[string]interface{}{
			"model_id":             model.ModelID,
			"provider":             model.Provider,
			"max_context_tokens":   model.MaxContextTokens,
			"sweet_spot_threshold": model.SweetSpotThreshold,
			"warning_threshold":    model.WarningThreshold,
			"danger_threshold":     model.DangerThreshold,
			"critical_threshold":   model.CriticalThreshold,
			"confidence":           model.Confidence,
			"thresholds": map[string]interface{}{
				"sweet_spot_tokens": thresholds.SweetSpotTokens,
				"warning_tokens":    thresholds.WarningTokens,
				"danger_tokens":     thresholds.DangerTokens,
				"critical_tokens":   thresholds.CriticalTokens,
			},
		})
	}

	output := map[string]interface{}{
		"models": models,
		"count":  len(models),
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func showModelDetails(registry *context.Registry, modelID string) error {
	model := registry.GetModel(modelID)
	if model == nil {
		return fmt.Errorf("model not found: %s", modelID)
	}

	thresholds, err := registry.GetThresholds(modelID)
	if err != nil {
		return fmt.Errorf("failed to get thresholds: %w", err)
	}

	if modelsJSON {
		return outputModelDetailsJSON(model, thresholds)
	}

	return outputModelDetailsText(model, thresholds)
}

func outputModelDetailsText(model *context.ModelConfig, thresholds *context.Thresholds) error {
	fmt.Printf("Model: %s\n", model.ModelID)
	fmt.Printf("Provider: %s\n", model.Provider)
	fmt.Printf("Max Context: %s tokens\n\n", formatContextSizeLong(model.MaxContextTokens))

	fmt.Println("Thresholds:")
	fmt.Printf("  Sweet spot: %.1f%% (%s tokens)\n",
		model.SweetSpotThreshold*100,
		formatContextSize(thresholds.SweetSpotTokens))
	fmt.Printf("  Warning: %.1f%% (%s tokens)\n",
		model.WarningThreshold*100,
		formatContextSize(thresholds.WarningTokens))
	fmt.Printf("  Danger: %.1f%% (%s tokens)\n",
		model.DangerThreshold*100,
		formatContextSize(thresholds.DangerTokens))
	fmt.Printf("  Critical: %.1f%% (%s tokens)\n\n",
		model.CriticalThreshold*100,
		formatContextSize(thresholds.CriticalTokens))

	if len(model.BenchmarkSources) > 0 {
		fmt.Println("Benchmark Sources:")
		for _, source := range model.BenchmarkSources {
			fmt.Printf("  - %s\n", source)
		}
		fmt.Println()
	}

	fmt.Printf("Confidence: %s\n", model.Confidence)

	if model.Notes != "" {
		fmt.Printf("Notes: %s\n", model.Notes)
	}

	return nil
}

func outputModelDetailsJSON(model *context.ModelConfig, thresholds *context.Thresholds) error {
	output := map[string]interface{}{
		"model_id":             model.ModelID,
		"provider":             model.Provider,
		"max_context_tokens":   model.MaxContextTokens,
		"sweet_spot_threshold": model.SweetSpotThreshold,
		"warning_threshold":    model.WarningThreshold,
		"danger_threshold":     model.DangerThreshold,
		"critical_threshold":   model.CriticalThreshold,
		"benchmark_sources":    model.BenchmarkSources,
		"notes":                model.Notes,
		"confidence":           model.Confidence,
		"last_updated":         model.LastUpdated,
		"thresholds": map[string]interface{}{
			"sweet_spot_tokens": thresholds.SweetSpotTokens,
			"warning_tokens":    thresholds.WarningTokens,
			"danger_tokens":     thresholds.DangerTokens,
			"critical_tokens":   thresholds.CriticalTokens,
		},
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func formatContextSize(tokens int) string {
	if tokens >= 1000000 {
		return fmt.Sprintf("%.0fM", float64(tokens)/1000000.0)
	} else if tokens >= 1000 {
		return fmt.Sprintf("%dK", tokens/1000)
	}
	return fmt.Sprintf("%d", tokens)
}

func formatContextSizeLong(tokens int) string {
	if tokens >= 1000000 {
		millions := float64(tokens) / 1000000.0
		if millions == float64(int(millions)) {
			return fmt.Sprintf("%d,000,000", int(millions))
		}
		return fmt.Sprintf("%.1fM", millions)
	} else if tokens >= 1000 {
		return fmt.Sprintf("%s", formatWithCommas(tokens))
	}
	return fmt.Sprintf("%d", tokens)
}

func formatWithCommas(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result []rune
	for i, digit := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, digit)
	}

	return string(result)
}
