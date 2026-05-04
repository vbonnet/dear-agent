package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
	contextdetect "github.com/vbonnet/dear-agent/engram/internal/context"
	"github.com/vbonnet/dear-agent/pkg/cliframe"
	"github.com/vbonnet/dear-agent/engram/retrieval"
)

var retrieveCmd = &cobra.Command{
	Use:   "retrieve [query]",
	Short: "Retrieve relevant engrams using AI-powered search",
	Long: `Retrieve engrams relevant to a query using ecphory (AI-powered retrieval).

Uses a 3-tier retrieval system:
1. Fast filter: Index-based filtering by tags/type
2. API ranking: Claude AI ranks candidates by relevance
3. Budget: Returns top results within token budget

QUERY SYNTAX

  You can provide the search query in two ways:

  1. Positional argument (traditional):
     $ engram retrieve "How do I handle errors in Go?"

  2. --query flag (alternative):
     $ engram retrieve --query "How do I handle errors in Go?"
     $ engram retrieve -q "Testing patterns"

EXAMPLES

  # Positional argument syntax
  engram retrieve "How do I handle errors in Go?"

  # Flag syntax
  engram retrieve --query "Python async patterns"
  engram retrieve -q "Git workflows"

  # With filters
  engram retrieve --query "Testing" --tag python --limit 5

  # Auto-detect context
  engram retrieve --auto --format json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRetrieve,
}

type retrieveConfig struct {
	EngramPath string
	Tag        string
	Type       string
	Limit      int
	NoAPI      bool   // Skip API ranking (fast filter only)
	Auto       bool   // Auto-detect context from git/directory
	Format     string // Output format: table or json
	Query      string // Query text (alternative to positional argument)
}

var retrieveCfg retrieveConfig

func init() {
	rootCmd.AddCommand(retrieveCmd)

	retrieveCmd.Flags().StringVarP(&retrieveCfg.EngramPath, "path", "p", "engrams", "Path to engrams directory")
	retrieveCmd.Flags().StringVar(&retrieveCfg.Tag, "tag", "", "Filter by tag before ranking")
	retrieveCmd.Flags().StringVarP(&retrieveCfg.Type, "type", "t", "", "Filter by type (pattern, strategy, howto, principle)")
	retrieveCmd.Flags().IntVarP(&retrieveCfg.Limit, "limit", "n", 10, "Maximum number of results")
	retrieveCmd.Flags().BoolVar(&retrieveCfg.NoAPI, "no-api", false, "Skip API ranking (fast index-based filter only)")
	retrieveCmd.Flags().BoolVar(&retrieveCfg.Auto, "auto", false, "Auto-detect context from git repo or directory")
	retrieveCmd.Flags().StringVar(&retrieveCfg.Format, "format", "table", "Output format: table or json")
	retrieveCmd.Flags().StringVarP(&retrieveCfg.Query, "query", "q", "", "Search query (alternative to positional argument)")
}

func runRetrieve(cmd *cobra.Command, args []string) error {
	// 1. Detect workspace and update engram path if not explicitly set
	if retrieveCfg.EngramPath == "engrams" {
		// Default value not overridden - try workspace detection
		basePath, err := getEngramBasePath()
		if err == nil {
			retrieveCfg.EngramPath = basePath
		}
	}

	// 2. Validate inputs
	if err := validateRetrieveInputs(); err != nil {
		return err
	}

	// 3. Get query from flags/args/auto
	query, err := getRetrieveQuery(args)
	if err != nil {
		return err
	}

	// 4. Build search options and execute
	opts := buildSearchOptions(query)
	svc := retrieval.NewService()

	ctx := context.Background()
	results, err := svc.Search(ctx, opts)
	if err != nil {
		return err
	}

	// 5. Output results
	return outputResults(cmd, query, results, !retrieveCfg.NoAPI)
}

// validateRetrieveInputs validates all command-line inputs
func validateRetrieveInputs() error {
	if err := cli.ValidateRangeInt("limit", retrieveCfg.Limit, 1, 100); err != nil {
		return err
	}

	if err := cli.ValidateOutputFormat(retrieveCfg.Format, cli.FormatTable, cli.FormatJSON); err != nil {
		return err
	}

	// Validate EngramPath is safe (prevent path traversal attacks)
	allowedPaths, err := cli.GetAllowedPaths()
	if err != nil {
		return err
	}
	return cli.ValidateSafePath("path", retrieveCfg.EngramPath, allowedPaths)
}

// getRetrieveQuery determines the query from flags, args, or auto-detection
func getRetrieveQuery(args []string) (string, error) {
	// Check for ambiguous input
	if retrieveCfg.Query != "" && len(args) > 0 {
		return "", &cli.EngramError{
			Symbol:  "✗",
			Message: "Ambiguous query input",
			Cause:   fmt.Errorf("cannot specify both --query flag and positional argument"),
			Suggestions: []string{
				"Use either: engram retrieve --query \"search term\"",
				"Or: engram retrieve \"search term\"",
				"But not both",
			},
		}
	}

	var query string
	var err error

	// Priority: --query flag > --auto > positional arg
	switch {
	case retrieveCfg.Query != "":
		query = retrieveCfg.Query
	case retrieveCfg.Auto:
		query, err = contextdetect.DetectContext()
		if err != nil {
			return "", &cli.EngramError{
				Symbol:  "✗",
				Message: "Auto-detection failed",
				Cause:   err,
				Suggestions: []string{
					"Ensure you're in a git repository or project directory",
					"Provide a query manually: engram retrieve \"your search query\"",
					"Or use --query flag: engram retrieve --query \"your search query\"",
				},
			}
		}
	case len(args) > 0:
		query = args[0]
	default:
		return "", cli.InvalidInputError("query", "", "provide either --query flag or positional argument")
	}

	if err := cli.ValidateNonEmpty("query", query); err != nil {
		return "", err
	}

	if err := cli.ValidateMaxLength("query", query, cli.MaxQueryLength); err != nil {
		return "", err
	}

	return query, nil
}

// buildSearchOptions constructs SearchOptions from config and query
func buildSearchOptions(query string) retrieval.SearchOptions {
	sessionID := uuid.New().String()
	transcript := query // V1: use query as transcript

	tags := []string{}
	if retrieveCfg.Tag != "" {
		tags = append(tags, retrieveCfg.Tag)
	}

	return retrieval.SearchOptions{
		EngramPath: retrieveCfg.EngramPath,
		Query:      query,
		SessionID:  sessionID,
		Transcript: transcript,
		Tags:       tags,
		Type:       retrieveCfg.Type,
		Limit:      retrieveCfg.Limit,
		UseAPI:     !retrieveCfg.NoAPI,
	}
}

// outputResults displays results using cliframe formatter
func outputResults(cmd *cobra.Command, query string, results []*retrieval.SearchResult, usedAPI bool) error {
	if len(results) == 0 {
		fmt.Println("No engrams found.")
		if retrieveCfg.Tag != "" || retrieveCfg.Type != "" {
			fmt.Println("Try removing filters to see all engrams.")
		}
		return nil
	}

	// Convert to output format
	output := JSONOutput{
		Query:      query,
		Candidates: make([]JSONCandidate, 0, len(results)),
		Metadata: JSONMetadata{
			Count:    len(results),
			UsedAPI:  usedAPI,
			Fallback: !usedAPI,
		},
	}

	for _, result := range results {
		candidate := JSONCandidate{
			Path:      result.Path,
			Title:     result.Engram.Frontmatter.Title,
			Tags:      result.Engram.Frontmatter.Tags,
			Type:      result.Engram.Frontmatter.Type,
			Relevance: result.Score,
		}

		// Try to get file modification time
		if info, err := os.Stat(result.Path); err == nil {
			candidate.Modified = info.ModTime()
		}

		output.Candidates = append(output.Candidates, candidate)
	}

	// Use cliframe for output
	var format cliframe.Format
	var data interface{}

	switch retrieveCfg.Format {
	case "json":
		format = cliframe.FormatJSON
		data = output // Full structure with metadata for JSON
	case "table":
		format = cliframe.FormatTable
		data = output.Candidates // Just the slice for table rendering
	default:
		format = cliframe.FormatTable
		data = output.Candidates
	}

	formatter, err := cliframe.NewFormatter(format, cliframe.WithPrettyPrint(true))
	if err != nil {
		return err
	}

	writer := cliframe.NewWriter(cmd.OutOrStdout(), cmd.ErrOrStderr())
	writer = writer.WithFormatter(formatter)

	return writer.Output(data)
}

// JSONCandidate represents an engram in JSON output
type JSONCandidate struct {
	Path      string    `json:"path"`
	Title     string    `json:"title"`
	Tags      []string  `json:"tags"`
	Type      string    `json:"type"`
	Modified  time.Time `json:"modified,omitempty"`
	Relevance float64   `json:"relevance,omitempty"`
}

// JSONOutput represents the complete JSON output structure
type JSONOutput struct {
	Query      string          `json:"query"`
	Candidates []JSONCandidate `json:"candidates"`
	Metadata   JSONMetadata    `json:"metadata"`
}

// JSONMetadata contains metadata about the search
type JSONMetadata struct {
	Count    int  `json:"count"`
	UsedAPI  bool `json:"used_api"`
	Fallback bool `json:"fallback"`
}
