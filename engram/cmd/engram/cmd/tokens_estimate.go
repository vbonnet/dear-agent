package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	contextdetect "github.com/vbonnet/dear-agent/engram/internal/context"
	"github.com/vbonnet/dear-agent/engram/internal/retrieval"
	"github.com/vbonnet/dear-agent/internal/tokens"
	"github.com/vbonnet/dear-agent/pkg/cliframe"
)

var estimateJSON bool
var estimateQuery string
var estimateAuto bool
var estimateEngramPath string
var estimateLimit int

var tokensEstimateCmd = &cobra.Command{
	Use:   "estimate [files...]",
	Short: "Estimate token counts for engram files or retrieval results",
	Long: `Estimate token counts for engram files using multiple tokenization methods.

The estimate command can work in two modes:

1. File mode (default): Analyze specific files
   - Provide file paths or glob patterns as arguments
   - Useful for estimating tokens in existing documents

2. Retrieval mode: Estimate tokens for retrieval results
   - Use --query or --auto to search engrams
   - Estimates tokens WITHOUT making API calls
   - Preview cost before running 'engram retrieve'

The estimate command provides token counts using:
  - char/4 heuristic (always available)
  - tiktoken (OpenAI cl100k_base encoding, if available)
  - simple word tokenizer (if available)

Output modes:
  - Text (default): Human-readable output with formatted numbers
  - JSON (--json): Machine-parseable structured output`,
	Args: cobra.ArbitraryArgs,
	RunE: runEstimate,
	Example: `  # File mode: Estimate tokens for specific files
  engram tokens estimate file.ai.md
  engram tokens estimate file1.md file2.md file3.md
  engram tokens estimate "**/*.ai.md"

  # Retrieval mode: Estimate tokens for a query
  engram tokens estimate --query "authentication patterns"
  engram tokens estimate -q "error handling" --limit 5

  # Retrieval mode: Auto-detect context from git
  engram tokens estimate --auto

  # Output as JSON for programmatic use
  engram tokens estimate --query "testing" --json`,
}

func init() {
	tokensCmd.AddCommand(tokensEstimateCmd)
	tokensEstimateCmd.Flags().BoolVar(&estimateJSON, "json", false, "Output JSON format")
	tokensEstimateCmd.Flags().StringVarP(&estimateQuery, "query", "q", "", "Search query (estimate tokens for retrieval results)")
	tokensEstimateCmd.Flags().BoolVar(&estimateAuto, "auto", false, "Auto-detect context from git repo or directory")
	tokensEstimateCmd.Flags().StringVarP(&estimateEngramPath, "path", "p", "engrams", "Path to engrams directory")
	tokensEstimateCmd.Flags().IntVarP(&estimateLimit, "limit", "n", 10, "Maximum number of results (for retrieval mode)")
}

func runEstimate(cmd *cobra.Command, args []string) error {
	// Determine mode: retrieval mode (--query or --auto) or file mode (default)
	isRetrievalMode := estimateQuery != "" || estimateAuto

	if isRetrievalMode {
		return runEstimateRetrieval(cmd, args)
	}

	return runEstimateFiles(cmd, args)
}

// runEstimateFiles handles file mode (original behavior)
func runEstimateFiles(cmd *cobra.Command, args []string) error {
	// Validate that at least one file argument is provided
	if len(args) == 0 {
		return fmt.Errorf("requires at least 1 arg(s), only received 0")
	}

	// Expand glob patterns to file paths
	files, err := expandGlobs(args)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return fmt.Errorf("no files matched patterns")
	}

	// Calculate token estimates
	estimate, err := tokens.Calculate(files)
	if err != nil {
		return fmt.Errorf("failed to estimate tokens: %w", err)
	}

	// Output results using cliframe for JSON, custom text for human readability
	return outputTokenEstimate(cmd, estimate, files, "", estimateJSON)
}

// runEstimateRetrieval handles retrieval mode (--query or --auto)
func runEstimateRetrieval(cmd *cobra.Command, args []string) error {
	// Get query (from --query flag or auto-detect)
	var query string
	var err error

	if estimateQuery != "" && estimateAuto {
		return fmt.Errorf("cannot specify both --query and --auto flags")
	}

	switch {
	case estimateQuery != "":
		query = estimateQuery
	case estimateAuto:
		query, err = contextdetect.DetectContext()
		if err != nil {
			return fmt.Errorf("auto-detection failed: %w", err)
		}
	default:
		return fmt.Errorf("retrieval mode requires --query or --auto flag")
	}

	if query == "" {
		return fmt.Errorf("query cannot be empty")
	}

	// Create retrieval service
	svc := retrieval.NewService()
	defer svc.Close()

	// Build search options (NO API ranking - we only want candidates)
	opts := retrieval.SearchOptions{
		EngramPath: estimateEngramPath,
		Query:      query,
		Limit:      estimateLimit,
		UseAPI:     false, // Critical: no API calls for estimation
	}

	// Perform search to get candidate files
	ctx := context.Background()
	results, err := svc.Search(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to search engrams: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No engrams found matching query.")
		return nil
	}

	// Extract file paths from results
	files := make([]string, len(results))
	for i, result := range results {
		files[i] = result.Path
	}

	// Calculate token estimates
	estimate, err := tokens.Calculate(files)
	if err != nil {
		return fmt.Errorf("failed to estimate tokens: %w", err)
	}

	// Output results with query context using cliframe for JSON, custom text for human readability
	return outputTokenEstimate(cmd, estimate, files, query, estimateJSON)
}

// expandGlobs expands glob patterns to file paths.
// Deduplicates files that match multiple patterns.
func expandGlobs(patterns []string) ([]string, error) {
	var files []string
	seen := make(map[string]bool)

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
		}

		// If no matches and pattern has no glob chars, treat as literal file
		if len(matches) == 0 && !hasGlobChars(pattern) {
			matches = []string{pattern}
		}

		// Deduplicate
		for _, match := range matches {
			if !seen[match] {
				seen[match] = true
				files = append(files, match)
			}
		}
	}

	return files, nil
}

// hasGlobChars checks if a string contains glob metacharacters.
func hasGlobChars(s string) bool {
	return strings.ContainsAny(s, "*?[]")
}

// formatTokensText formats token estimate for human reading.
func formatTokensText(est *tokens.Estimate, files []string, query string) string {
	var buf strings.Builder

	// Header (with query context if provided)
	if query != "" {
		fmt.Fprintf(&buf, "Token estimate for query: %q\n\n", query)
	}

	fileCount := len(files)
	if fileCount == 1 {
		fmt.Fprintf(&buf, "Token estimate for 1 file:\n\n")
	} else {
		fmt.Fprintf(&buf, "Token estimate for %d files:\n\n", fileCount)
	}

	// Character count
	fmt.Fprintf(&buf, "Character count: %s chars\n\n", formatNumber(est.CharCount))

	// Token estimates
	fmt.Fprintf(&buf, "Token estimates:\n")
	fmt.Fprintf(&buf, "  char/4:    %s tokens\n", formatNumber(est.TokensChar4))

	// Optional tokenizers (sorted for consistent output)
	if len(est.Tokenizers) > 0 {
		// Get sorted keys
		names := make([]string, 0, len(est.Tokenizers))
		for name := range est.Tokenizers {
			names = append(names, name)
		}
		// Simple alphabetic sort (good enough for 2-3 tokenizers)
		for i := 0; i < len(names); i++ {
			for j := i + 1; j < len(names); j++ {
				if names[i] > names[j] {
					names[i], names[j] = names[j], names[i]
				}
			}
		}

		for _, name := range names {
			count := est.Tokenizers[name]
			fmt.Fprintf(&buf, "  %-10s %s tokens\n", name+":", formatNumber(count))
		}
	}

	// Cost estimation (use tiktoken if available, otherwise char/4)
	tokenCount := est.TokensChar4
	if tiktokenCount, ok := est.Tokenizers["tiktoken"]; ok {
		tokenCount = tiktokenCount
	}

	fmt.Fprintf(&buf, "\nEstimated cost:\n")
	fmt.Fprintf(&buf, "  Sonnet 4.5:  $%.4f (input)\n", estimateCost(tokenCount, 3.0))
	fmt.Fprintf(&buf, "  Haiku 3.5:   $%.4f (input)\n", estimateCost(tokenCount, 1.0))
	fmt.Fprintf(&buf, "  Opus 4.5:    $%.4f (input)\n", estimateCost(tokenCount, 15.0))

	return buf.String()
}

// estimateCost calculates cost in USD for given token count and price per MTok
func estimateCost(tokens int, pricePerMTok float64) float64 {
	return float64(tokens) / 1_000_000.0 * pricePerMTok
}

// formatNumber formats an integer with thousands separators.
// Example: 12450 -> "12,450"
func formatNumber(n int) string {
	if n < 0 {
		return "-" + formatNumber(-n)
	}

	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		// s comes from fmt.Sprintf("%d", n) — pure ASCII digits.
		result = append(result, byte(c)) //nolint:gosec // ASCII-only source
	}
	return string(result)
}

// TokensJSONOutput represents the JSON output format for token estimation.
type TokensJSONOutput struct {
	Query        string         `json:"query,omitempty"`
	Files        []string       `json:"files"`
	CharCount    int            `json:"char_count"`
	TokensChar4  int            `json:"tokens_char4"`
	Tokenizers   map[string]int `json:"tokenizers,omitempty"`
	CostEstimate *CostEstimate  `json:"cost_estimate,omitempty"`
}

// CostEstimate represents estimated costs for different models
type CostEstimate struct {
	Tokens int                `json:"tokens"`
	Models map[string]float64 `json:"models"`
}

// outputTokenEstimate outputs token estimates using cliframe for JSON, custom text for human readability
func outputTokenEstimate(cmd *cobra.Command, est *tokens.Estimate, files []string, query string, useJSON bool) error {
	// Use tiktoken if available, otherwise char/4
	tokenCount := est.TokensChar4
	if tiktokenCount, ok := est.Tokenizers["tiktoken"]; ok {
		tokenCount = tiktokenCount
	}

	if useJSON {
		// Use cliframe for JSON output
		output := TokensJSONOutput{
			Query:       query,
			Files:       files,
			CharCount:   est.CharCount,
			TokensChar4: est.TokensChar4,
			Tokenizers:  est.Tokenizers,
			CostEstimate: &CostEstimate{
				Tokens: tokenCount,
				Models: map[string]float64{
					"sonnet-4.5": estimateCost(tokenCount, 3.0),
					"haiku-3.5":  estimateCost(tokenCount, 1.0),
					"opus-4.5":   estimateCost(tokenCount, 15.0),
				},
			},
		}

		formatter, err := cliframe.NewFormatter(cliframe.FormatJSON, cliframe.WithPrettyPrint(true))
		if err != nil {
			return err
		}

		writer := cliframe.NewWriter(cmd.OutOrStdout(), cmd.ErrOrStderr())
		writer = writer.WithFormatter(formatter)

		return writer.Output(output)
	}

	// Custom text output (preserved for human readability)
	fmt.Fprint(cmd.OutOrStdout(), formatTokensText(est, files, query))
	return nil
}
