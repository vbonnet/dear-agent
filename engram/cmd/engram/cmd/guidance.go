package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
	"github.com/vbonnet/dear-agent/engram/internal/guidance"
	"github.com/vbonnet/dear-agent/engram/internal/platform"
)

// guidanceCmd is the parent command for guidance operations
var guidanceCmd = &cobra.Command{
	Use:   "guidance",
	Short: "Search and manage guidance files",
	Long: `Search and manage guidance files (*.ai.md) in your engram library.

Guidance files provide patterns, workflows, and references for AI agents.

SUBCOMMANDS
  search     - Search guidance files by keyword

EXAMPLES
  # Search for encryption guidance
  $ engram guidance search "encryption"

  # Search with domain filter
  $ engram guidance search "error handling" --domain go

  # Get JSON output
  $ engram guidance search "testing" --format json`,
}

func init() {
	rootCmd.AddCommand(guidanceCmd)
}

// guidanceSearchConfig holds configuration for the search command
type guidanceSearchConfig struct {
	EngramPath string
	Domain     string
	Type       string
	Tag        string
	Limit      int
	Format     string // "table", "json", "paths"
}

var guidanceSearchCfg guidanceSearchConfig

// guidanceSearchCmd implements the 'guidance search' subcommand
var guidanceSearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search guidance files by keyword",
	Long: `Search guidance files by matching query against frontmatter metadata
(title, description, tags, domain).

Returns matching files with relevance scores. Use filters to narrow results.

EXAMPLES
  # Basic search
  $ engram guidance search "encryption"

  # Filter by domain
  $ engram guidance search "error handling" --domain go

  # Filter by type
  $ engram guidance search "testing" --type pattern

  # Limit results
  $ engram guidance search "patterns" --limit 5

  # JSON output (for scripting)
  $ engram guidance search "security" --format json

  # Paths only (for piping)
  $ engram guidance search "hipaa" --format paths`,
	Args: cobra.ExactArgs(1),
	RunE: runGuidanceSearch,
}

func init() {
	guidanceCmd.AddCommand(guidanceSearchCmd)

	// Flags
	guidanceSearchCmd.Flags().StringVarP(&guidanceSearchCfg.EngramPath, "path", "p", "", "Path to engrams directory (default: auto-detect)")
	guidanceSearchCmd.Flags().StringVar(&guidanceSearchCfg.Domain, "domain", "", "Filter by domain (e.g., go, python, hipaa)")
	guidanceSearchCmd.Flags().StringVarP(&guidanceSearchCfg.Type, "type", "t", "", "Filter by type (pattern, workflow, reference)")
	guidanceSearchCmd.Flags().StringVar(&guidanceSearchCfg.Tag, "tag", "", "Filter by tag")
	guidanceSearchCmd.Flags().IntVarP(&guidanceSearchCfg.Limit, "limit", "n", 10, "Maximum number of results (1-100)")
	guidanceSearchCmd.Flags().StringVar(&guidanceSearchCfg.Format, "format", "table", "Output format: table, json, paths")
}

func runGuidanceSearch(cmd *cobra.Command, args []string) error {
	query := args[0]

	// Validate limit
	if guidanceSearchCfg.Limit < 1 || guidanceSearchCfg.Limit > 100 {
		return &cli.EngramError{
			Symbol:  "✗",
			Message: "Invalid limit",
			Cause:   fmt.Errorf("limit must be between 1 and 100, got %d", guidanceSearchCfg.Limit),
			Suggestions: []string{
				"Use --limit with a value between 1 and 100",
			},
		}
	}

	// Validate format
	validFormats := map[string]bool{"table": true, "json": true, "paths": true}
	if !validFormats[guidanceSearchCfg.Format] {
		return &cli.EngramError{
			Symbol:  "✗",
			Message: "Invalid format",
			Cause:   fmt.Errorf("format must be one of: table, json, paths (got %s)", guidanceSearchCfg.Format),
			Suggestions: []string{
				"Use --format with one of: table, json, paths",
			},
		}
	}

	// Resolve engram path
	engramPath, err := resolveEngramPathForGuidance(guidanceSearchCfg.EngramPath)
	if err != nil {
		return &cli.EngramError{
			Symbol:  "✗",
			Message: "Failed to resolve engram path",
			Cause:   err,
			Suggestions: []string{
				"Ensure ENGRAM_HOME is set or you're in an engram directory",
				"Use --path to specify a custom engrams directory",
			},
		}
	}

	// Create search service
	searchSvc := guidance.NewSearchService(engramPath)

	// Build search options
	opts := guidance.SearchOptions{
		Query:  query,
		Domain: guidanceSearchCfg.Domain,
		Type:   guidanceSearchCfg.Type,
		Tag:    guidanceSearchCfg.Tag,
		Limit:  guidanceSearchCfg.Limit,
	}

	// Execute search
	results, err := searchSvc.Search(opts)
	if err != nil {
		return &cli.EngramError{
			Symbol:  "✗",
			Message: "Search failed",
			Cause:   err,
			Suggestions: []string{
				"Check that your engram path contains .ai.md files",
				"Try a different query or remove filters",
			},
		}
	}

	// Format and print output
	switch guidanceSearchCfg.Format {
	case "table":
		printTableFormat(results)
	case "json":
		printJSONFormat(results)
	case "paths":
		printPathsFormat(results)
	}

	return nil
}

// resolveEngramPathForGuidance resolves the engram path for guidance search
func resolveEngramPathForGuidance(customPath string) (string, error) {
	// If custom path provided, use it
	if customPath != "" {
		absPath, err := filepath.Abs(customPath)
		if err != nil {
			return "", fmt.Errorf("invalid path: %w", err)
		}
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			return "", fmt.Errorf("path does not exist: %s", absPath)
		}
		return absPath, nil
	}

	// Otherwise, use platform to resolve
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	plat, err := platform.New(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to initialize platform: %w", err)
	}
	defer plat.Close()

	// Get engram path from config
	engramPath := plat.Config().Platform.EngramPath

	// Verify path exists
	if _, err := os.Stat(engramPath); os.IsNotExist(err) {
		return "", fmt.Errorf("engram directory not found: %s", engramPath)
	}

	return engramPath, nil
}

// Output formatters

func printTableFormat(results []guidance.SearchResult) {
	if len(results) == 0 {
		fmt.Println("No guidance files found matching your query.")
		fmt.Println()
		fmt.Println("Suggestions:")
		fmt.Println("  - Try a broader query")
		fmt.Println("  - Check your spelling")
		fmt.Println("  - Remove filters (--domain, --type, --tag)")
		return
	}

	// Print header
	fmt.Printf("%-45s %-35s %-45s %s\n", "PATH", "TITLE", "DESCRIPTION", "SCORE")
	fmt.Println(strings.Repeat("-", 135))

	// Print results
	for _, result := range results {
		// Truncate fields if too long
		path := truncate(result.Path, 45)
		title := truncate(result.Title, 35)
		desc := truncate(result.Description, 45)

		fmt.Printf("%-45s %-35s %-45s %d\n", path, title, desc, result.Score)
	}

	fmt.Printf("\nFound %d result(s)\n", len(results))
}

func printJSONFormat(results []guidance.SearchResult) {
	output, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(output))
}

func printPathsFormat(results []guidance.SearchResult) {
	for _, result := range results {
		fmt.Println(result.Path)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
