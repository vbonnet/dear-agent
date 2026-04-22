package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
	"github.com/vbonnet/dear-agent/engram/ecphory"
	"github.com/vbonnet/dear-agent/engram/ecphory/ranking"
	"github.com/vbonnet/dear-agent/pkg/engram"
	llmconfig "github.com/vbonnet/dear-agent/pkg/llm/config"
)

var explainCmd = &cobra.Command{
	Use:   "explain [query]",
	Short: "Explain ecphory retrieval process with tier-by-tier details",
	Long: `Show detailed tier-by-tier retrieval decisions for observability.

Displays:
  - Provider auto-detection (which provider was selected)
  - Tier 1: Frontmatter filter metrics (candidates matched)
  - Tier 2: Semantic ranking (provider, model, API cost, relevance scores)
  - Tier 3: Token budget allocation (engrams loaded, tokens used)
  - Total cost breakdown

This command is useful for:
  - Understanding why specific engrams were/weren't retrieved
  - Debugging retrieval quality issues
  - Monitoring API costs per query
  - Validating provider auto-detection

EXAMPLES
  # Explain retrieval for a query
  $ engram explain "How do I handle errors in Go?"

  # Show which provider would be used
  $ engram explain --query "Testing patterns"

  # Compare providers
  $ ANTHROPIC_API_KEY=... engram explain "query"  # Uses Anthropic
  $ unset ANTHROPIC_API_KEY && engram explain "query"  # Falls back`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEcphoryExplain,
}

type ecphoryExplainConfig struct {
	EngramPath  string
	Tag         string
	Type        string
	TokenBudget int
	Query       string
	NoAPI       bool
	Provider    string
}

var explainCfg ecphoryExplainConfig

func init() {
	// Add explain command directly to root
	rootCmd.AddCommand(explainCmd)

	// Flags for explain command
	explainCmd.Flags().StringVarP(&explainCfg.EngramPath, "path", "p", "engrams", "Path to engrams directory")
	explainCmd.Flags().StringVar(&explainCfg.Tag, "tag", "", "Filter by tag before ranking")
	explainCmd.Flags().StringVarP(&explainCfg.Type, "type", "t", "", "Filter by type (pattern, strategy, howto, principle)")
	explainCmd.Flags().IntVarP(&explainCfg.TokenBudget, "budget", "b", 10000, "Token budget for retrieval")
	explainCmd.Flags().StringVarP(&explainCfg.Query, "query", "q", "", "Search query (alternative to positional argument)")
	explainCmd.Flags().BoolVar(&explainCfg.NoAPI, "no-api", false, "Skip API ranking (local-only)")
	explainCmd.Flags().StringVar(&explainCfg.Provider, "provider", "", "Override provider (anthropic, gemini, openrouter)")
}

func runEcphoryExplain(cmd *cobra.Command, args []string) error {
	// 1. Get query from args or flags
	query, err := getExplainQuery(args)
	if err != nil {
		return err
	}

	// 2. Detect workspace and update engram path
	if explainCfg.EngramPath == "engrams" {
		basePath, err := getEngramBasePath()
		if err == nil {
			explainCfg.EngramPath = basePath
		}
	}

	// 3. Print header
	fmt.Println("Ecphory Retrieval Explanation")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Query: %s\n", query)
	fmt.Printf("Engram Path: %s\n", explainCfg.EngramPath)
	if explainCfg.Tag != "" {
		fmt.Printf("Tag Filter: %s\n", explainCfg.Tag)
	}
	if explainCfg.Type != "" {
		fmt.Printf("Type Filter: %s\n", explainCfg.Type)
	}
	fmt.Printf("Token Budget: %d\n", explainCfg.TokenBudget)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	// 4. Provider auto-detection
	if err := explainProviderDetection(); err != nil {
		return err
	}

	// 5. Tier 1: Fast filter
	tier1Results, err := explainTier1Filter()
	if err != nil {
		return err
	}

	if len(tier1Results) == 0 {
		fmt.Println("No engrams matched filters. Stopping.")
		return nil
	}

	// 6. Tier 2: Semantic ranking (or skip if --no-api)
	var tier2Results []ranking.RankedResult
	var provider ranking.Provider

	if !explainCfg.NoAPI {
		var err error
		tier2Results, provider, err = explainTier2Ranking(query, tier1Results)
		if err != nil {
			fmt.Printf("⚠️  Tier 2 ranking failed: %v\n", err)
			fmt.Println("Falling back to unranked results.")
			// For fallback, create unranked results from tier1
			tier2Results = make([]ranking.RankedResult, len(tier1Results))
			for i, path := range tier1Results {
				tier2Results[i] = ranking.RankedResult{
					Candidate: ranking.Candidate{Name: path},
					Score:     0.0,
					Reasoning: "Unranked (fallback)",
				}
			}
		}
	} else {
		// Skip API ranking, convert tier1 to unranked tier2
		tier2Results = make([]ranking.RankedResult, len(tier1Results))
		for i, path := range tier1Results {
			tier2Results[i] = ranking.RankedResult{
				Candidate: ranking.Candidate{Name: path},
				Score:     0.0,
				Reasoning: "No ranking (local-only mode)",
			}
		}
	}

	// 7. Tier 3: Token budget allocation
	if err := explainTier3Budget(tier2Results, provider); err != nil {
		return err
	}

	return nil
}

func getExplainQuery(args []string) (string, error) {
	// Check for ambiguous input
	if explainCfg.Query != "" && len(args) > 0 {
		return "", &cli.EngramError{
			Symbol:  "✗",
			Message: "Ambiguous query input",
			Cause:   fmt.Errorf("cannot specify both --query flag and positional argument"),
			Suggestions: []string{
				"Use either: engram explain --query \"search term\"",
				"Or: engram explain \"search term\"",
			},
		}
	}

	var query string
	if explainCfg.Query != "" {
		query = explainCfg.Query
	} else if len(args) > 0 {
		query = args[0]
	} else {
		return "", cli.InvalidInputError("query", "", "provide either --query flag or positional argument")
	}

	if err := cli.ValidateNonEmpty("query", query); err != nil {
		return "", err
	}

	return query, nil
}

func explainProviderDetection() error {
	fmt.Println("Provider Auto-Detection")
	fmt.Println(strings.Repeat("-", 60))

	if explainCfg.NoAPI {
		fmt.Println("Mode: Local-only (--no-api flag)")
		fmt.Println("Provider: local (Jaccard similarity)")
		fmt.Println()
		return nil
	}

	// Load LLM config for per-tool model preferences
	home, err := os.UserHomeDir()
	if err != nil {
		home = "~"
	}
	configPath := home + "/.engram/llm-config.yaml"
	llmCfg, err := llmconfig.LoadConfig(configPath)
	if err != nil {
		// Config not found, SelectModel will use hardcoded defaults
		llmCfg = nil
	}

	// Provider override from flag
	if explainCfg.Provider != "" {
		fmt.Printf("Provider Override: %s (from --provider flag)\n", explainCfg.Provider)
	}

	// Get model preference for ecphory tool
	providerFamily := explainCfg.Provider
	if providerFamily == "" {
		// Use default family from config or auto-detect
		providerFamily = "anthropic" // Default to anthropic for ecphory
	}

	// Select model based on tool + provider
	selectedModel := llmconfig.SelectModel(llmCfg, "ecphory", providerFamily)
	if selectedModel != "" {
		fmt.Printf("Model (from config): %s\n", selectedModel)
	}

	// Load ranking config and create factory
	rankingConfig := ranking.DefaultConfig()

	// Override model from LLM config if specified
	if selectedModel != "" {
		switch providerFamily {
		case "anthropic", "claude":
			rankingConfig.Ecphory.Providers.Anthropic.Model = selectedModel
		case "gemini", "google":
			rankingConfig.Ecphory.Providers.VertexAI.Model = selectedModel
		}
	}

	// Create factory
	factory, err := ranking.NewFactory(rankingConfig)
	if err != nil {
		return fmt.Errorf("failed to create provider factory: %w", err)
	}

	// Get provider (override or auto-detect)
	var provider ranking.Provider
	if explainCfg.Provider != "" {
		// Try to get specified provider
		provider, err = factory.GetProvider(explainCfg.Provider)
		if err != nil {
			fmt.Printf("⚠️  Provider '%s' not available, falling back to auto-detect\n", explainCfg.Provider)
			provider, err = factory.AutoDetect()
			if err != nil {
				return fmt.Errorf("failed to auto-detect provider: %w", err)
			}
		}
	} else {
		// Auto-detect
		provider, err = factory.AutoDetect()
		if err != nil {
			return fmt.Errorf("failed to auto-detect provider: %w", err)
		}
	}

	fmt.Printf("Selected Provider: %s\n", provider.Name())
	fmt.Printf("Model: %s\n", provider.Model())

	// Show capabilities
	caps := provider.Capabilities()
	fmt.Printf("Capabilities:\n")
	fmt.Printf("  - Caching: %v\n", caps.SupportsCaching)
	fmt.Printf("  - Structured Output: %v\n", caps.SupportsStructuredOutput)
	fmt.Printf("  - Max Tokens: %d\n", caps.MaxTokensPerRequest)
	fmt.Printf("  - Concurrency: %d\n", caps.MaxConcurrentRequests)

	// Show all available providers
	fmt.Printf("\nAvailable Providers: %v\n", factory.ListProviders())

	// Show environment-based precedence
	fmt.Println("\nPrecedence Order:")
	fmt.Println("  1. Anthropic (ANTHROPIC_API_KEY set)")
	fmt.Println("  2. Vertex Claude (GOOGLE_CLOUD_PROJECT + us-east5)")
	fmt.Println("  3. Vertex Gemini (GOOGLE_CLOUD_PROJECT + USE_VERTEX_GEMINI=true)")
	fmt.Println("  4. Local (always available)")

	fmt.Println()
	return nil
}

func explainTier1Filter() ([]string, error) {
	fmt.Println("Tier 1: Fast Frontmatter Filter")
	fmt.Println(strings.Repeat("-", 60))

	startTime := time.Now()

	// Build index using ecphory.Index
	idx := ecphory.NewIndex()
	if err := idx.Build(explainCfg.EngramPath); err != nil {
		return nil, fmt.Errorf("failed to build index: %w", err)
	}

	// Apply filters
	var candidates []string
	if len(explainCfg.Tag) > 0 {
		candidates = idx.FilterByTags([]string{explainCfg.Tag})
		fmt.Printf("Filtered by tag '%s': %d candidates\n", explainCfg.Tag, len(candidates))
	} else {
		candidates = idx.All()
		fmt.Printf("No tag filter: %d total candidates\n", len(candidates))
	}

	// Type filter
	if explainCfg.Type != "" {
		typeCandidates := idx.FilterByType(explainCfg.Type)
		// Intersect with existing candidates
		candidateSet := make(map[string]bool)
		for _, c := range candidates {
			candidateSet[c] = true
		}
		filtered := []string{}
		for _, c := range typeCandidates {
			if candidateSet[c] {
				filtered = append(filtered, c)
			}
		}
		candidates = filtered
		fmt.Printf("Filtered by type '%s': %d candidates\n", explainCfg.Type, len(candidates))
	}

	duration := time.Since(startTime)
	fmt.Printf("Duration: %v\n", duration)
	fmt.Printf("Result: %d candidates passed filter\n", len(candidates))
	fmt.Println()

	return candidates, nil
}

func explainTier2Ranking(query string, candidates []string) ([]ranking.RankedResult, ranking.Provider, error) {
	fmt.Println("Tier 2: Semantic Ranking")
	fmt.Println(strings.Repeat("-", 60))

	startTime := time.Now()

	// Load config and create factory
	config := ranking.DefaultConfig()
	factory, err := ranking.NewFactory(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create provider factory: %w", err)
	}

	// Auto-detect provider
	provider, err := factory.AutoDetect()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to auto-detect provider: %w", err)
	}

	fmt.Printf("Provider: %s\n", provider.Name())
	fmt.Printf("Model: %s\n", provider.Model())

	// Convert paths to candidates (need to load T0/T1 content)
	parser := engram.NewParser()
	rankCandidates := make([]ranking.Candidate, 0, len(candidates))

	for _, path := range candidates {
		eg, err := parser.Parse(path)
		if err != nil {
			fmt.Printf("⚠️  Failed to parse %s: %v\n", path, err)
			continue
		}

		// Use first 200 chars as description (T0 equivalent)
		desc := eg.Content
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}

		rankCandidates = append(rankCandidates, ranking.Candidate{
			Name:        eg.Frontmatter.Title,
			Description: desc,
			Frontmatter: nil, // Not needed for ranking
			Tags:        eg.Frontmatter.Tags,
		})
	}

	fmt.Printf("Candidates to rank: %d\n", len(rankCandidates))

	// Rank candidates
	ctx := context.Background()
	results, err := provider.Rank(ctx, query, rankCandidates)
	if err != nil {
		return nil, provider, fmt.Errorf("ranking failed: %w", err)
	}

	duration := time.Since(startTime)
	fmt.Printf("Duration: %v\n", duration)

	// Show top 10 scores
	fmt.Println("\nTop Relevance Scores:")
	for i, r := range results {
		if i >= 10 {
			break
		}
		fmt.Printf("  %2d. %s (%.2f)\n", i+1, r.Candidate.Name, r.Score)
		if r.Reasoning != "" && len(r.Reasoning) > 0 {
			reasoning := strings.Split(r.Reasoning, "\n")[0] // First line only
			if len(reasoning) > 80 {
				reasoning = reasoning[:77] + "..."
			}
			fmt.Printf("      %s\n", reasoning)
		}
	}

	// Cost estimation (if provider supports it)
	// For now, just show placeholder
	fmt.Println("\nCost Estimate:")
	fmt.Println("  Input tokens: ~N/A (not tracked yet)")
	fmt.Println("  Output tokens: ~N/A (not tracked yet)")
	fmt.Println("  Total cost: $N/A (integration pending)")
	fmt.Println("  (Cost tracking implementation: Task S8.9)")

	fmt.Println()
	return results, provider, nil
}

func explainTier3Budget(results []ranking.RankedResult, provider ranking.Provider) error {
	fmt.Println("Tier 3: Token Budget Allocation")
	fmt.Println(strings.Repeat("-", 60))

	fmt.Printf("Token Budget: %d\n", explainCfg.TokenBudget)

	// Track tokens and loaded count
	tokensUsed := 0
	loaded := 0

	for i, r := range results {
		// Estimate tokens (char/4 heuristic)
		// In reality, would parse the engram and get actual content
		estimatedTokens := len(r.Candidate.Description) / 4

		if tokensUsed+estimatedTokens > explainCfg.TokenBudget {
			fmt.Printf("\nBudget exhausted at rank %d\n", i+1)
			fmt.Printf("  Loaded: %d engrams\n", loaded)
			fmt.Printf("  Tokens used: %d / %d\n", tokensUsed, explainCfg.TokenBudget)
			fmt.Printf("  Remaining candidates: %d (not loaded)\n", len(results)-i)
			break
		}

		tokensUsed += estimatedTokens
		loaded++

		// Show first 5 loaded engrams
		if loaded <= 5 {
			fmt.Printf("  %2d. %s (~%d tokens)\n", loaded, r.Candidate.Name, estimatedTokens)
		}
	}

	if loaded == len(results) {
		fmt.Printf("\nAll %d candidates fit within budget\n", loaded)
		fmt.Printf("  Tokens used: %d / %d (%.1f%%)\n",
			tokensUsed, explainCfg.TokenBudget,
			float64(tokensUsed)/float64(explainCfg.TokenBudget)*100)
	}

	fmt.Println()

	// Summary
	fmt.Println("Summary")
	fmt.Println(strings.Repeat("-", 60))
	if provider != nil {
		fmt.Printf("Provider: %s (%s)\n", provider.Name(), provider.Model())
	} else {
		fmt.Printf("Provider: local (no API)\n")
	}
	fmt.Printf("Tier 1 (Filter): %d candidates\n", len(results))
	fmt.Printf("Tier 2 (Rank): %d ranked\n", len(results))
	fmt.Printf("Tier 3 (Budget): %d loaded (%d tokens)\n", loaded, tokensUsed)
	fmt.Println()

	return nil
}
