package cmd

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/config"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/detector"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/extractors"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/gemini"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/internal/cache"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/internal/discovery"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/internal/modes"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/internal/templates"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/research"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/types"
)

// Run executes the main command logic
// Returns exit code (0 for success, >0 for errors)
func Run(url string, flags *types.Flags, cfg *config.Config) int {
	// Validate URL
	if err := ValidateURL(url); err != nil {
		fmt.Fprintf(cfg.Stderr, "Error: %v\n", err)
		fmt.Fprintf(cfg.Stderr, "\nExamples of valid URLs:\n")
		fmt.Fprintf(cfg.Stderr, "  https://www.youtube.com/watch?v=VIDEO_ID\n")
		fmt.Fprintf(cfg.Stderr, "  https://arxiv.org/abs/2601.20802\n")
		fmt.Fprintf(cfg.Stderr, "  https://huggingface.co/papers/2501.12345\n")
		fmt.Fprintf(cfg.Stderr, "  https://example.com/article\n")
		return 1
	}

	// Detect analysis mode
	mode := modes.DetectMode(url, flags.Mode)

	// Log configuration
	fmt.Fprintf(cfg.Stdout, "Configuration:\n")
	fmt.Fprintf(cfg.Stdout, "  URL: %s\n", url)
	fmt.Fprintf(cfg.Stdout, "  Mode: %s", mode.String())
	if flags.Mode == "" {
		fmt.Fprintf(cfg.Stdout, " (auto-detected)")
	} else {
		fmt.Fprintf(cfg.Stdout, " (explicit)")
	}
	fmt.Fprintf(cfg.Stdout, "\n")
	fmt.Fprintf(cfg.Stdout, "  Output Directory: %s\n", cfg.OutputDir)
	fmt.Fprintf(cfg.Stdout, "  Timeout: %d minutes\n", cfg.Timeout)
	if cfg.ProjectID != "" {
		fmt.Fprintf(cfg.Stdout, "  GCP Project: %s\n", cfg.ProjectID)
	}
	if flags.ExtractPrompt != "" {
		fmt.Fprintf(cfg.Stdout, "  Custom Extract Prompt: %s\n", truncate(flags.ExtractPrompt, 50))
	}
	if flags.AnalyzePrompt != "" {
		fmt.Fprintf(cfg.Stdout, "  Custom Analyze Prompt: %s\n", truncate(flags.AnalyzePrompt, 50))
	}
	if flags.ResearchPrompt != "" {
		fmt.Fprintf(cfg.Stdout, "  Custom Research Prompt: %s\n", truncate(flags.ResearchPrompt, 50))
	}
	// Also show legacy --input if used
	if flags.Input != "" {
		fmt.Fprintf(cfg.Stdout, "  Custom Prompt (legacy): %s\n", truncate(flags.Input, 50))
	}
	if flags.Type != "" {
		fmt.Fprintf(cfg.Stdout, "  Content Type Override: %s\n", flags.Type)
	}
	fmt.Fprintf(cfg.Stdout, "\n")

	// Execute pipeline with detected mode
	return executePipeline(url, flags, cfg, mode)
}

// truncate truncates a string to maxLen characters with ellipsis
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// executePipeline executes the full E2E pipeline
func executePipeline(url string, flags *types.Flags, cfg *config.Config, mode modes.Mode) int {
	ctx := context.Background()

	// Stage 0: Discovery (competitive mode only)
	var targetURL string
	if mode.IsCompetitive() {
		fmt.Fprintf(cfg.Stdout, "Stage 0: Discovering competitor URLs...\n")
		discoveredURLs, err := runDiscovery(ctx, url, cfg)
		if err != nil {
			fmt.Fprintf(cfg.Stderr, "Error during discovery: %v\n", err)
			fmt.Fprintf(cfg.Stderr, "Tip: Use --no-discovery flag to skip URL discovery\n")
			return 1
		}
		if len(discoveredURLs) == 0 {
			fmt.Fprintf(cfg.Stderr, "No competitor URLs discovered\n")
			return 1
		}
		fmt.Fprintf(cfg.Stdout, "  Discovered %d URLs:\n", len(discoveredURLs))
		for i, u := range discoveredURLs {
			fmt.Fprintf(cfg.Stdout, "    %d. %s\n", i+1, u)
		}
		fmt.Fprintf(cfg.Stdout, "\n")

		// Use first discovered URL for analysis
		targetURL = discoveredURLs[0]
		fmt.Fprintf(cfg.Stdout, "  Analyzing: %s\n\n", targetURL)
	} else {
		// General mode: use provided URL directly
		targetURL = url
	}

	// Step 1: Detect content type
	fmt.Fprintf(cfg.Stdout, "Step 1: Detecting content type...\n")
	contentType, err := detector.DetectContentType(targetURL, flags.Type)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "Error detecting content type: %v\n", err)
		return 1
	}
	fmt.Fprintf(cfg.Stdout, "  Detected: %s\n\n", contentType.String())

	// Step 1.5: Check cache (unless --force)
	if !flags.Force {
		fmt.Fprintf(cfg.Stdout, "Checking cache for existing research...\n")
		cachePath, exists, err := cache.Check(targetURL, cfg.CacheDir, contentType.String())
		if err != nil {
			// Log warning but continue (graceful degradation)
			fmt.Fprintf(cfg.Stderr, "Warning: Cache check failed: %v\n", err)
		} else if exists {
			fmt.Fprintf(cfg.Stdout, "Research already exists at: %s/report.md\n", cachePath)
			fmt.Fprintf(cfg.Stdout, "Run with --force to refresh the research.\n")
			return 0
		}
		fmt.Fprintf(cfg.Stdout, "  No cached research found, proceeding with extraction...\n\n")
	} else {
		fmt.Fprintf(cfg.Stdout, "Force mode enabled, skipping cache check...\n\n")
	}

	// Step 2: Extract content
	fmt.Fprintf(cfg.Stdout, "Step 2: Extracting content...\n")
	factory, err := extractors.NewExtractorFactory()
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "Error creating extractor factory: %v\n", err)
		return 2
	}

	extractor, err := factory.GetExtractor(contentType)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "Error getting extractor: %v\n", err)
		return 2
	}

	content, err := extractor.Extract(ctx, targetURL)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "Error extracting content: %v\n", err)
		return 2
	}
	fmt.Fprintf(cfg.Stdout, "  Extracted %d characters\n\n", len(content.Raw))

	// Step 2.5: Load and resolve prompts (ConfigParser → FileResolver or Templates)
	fmt.Fprintf(cfg.Stdout, "Step 2.5: Loading prompt configuration...\n")

	var analyzePrompt string
	if mode.IsCompetitive() {
		// Use competitive templates
		analyzePrompt, err = loadCompetitivePrompt(url, targetURL)
		if err != nil {
			fmt.Fprintf(cfg.Stderr, "Error loading competitive template: %v\n", err)
			return 1
		}
		fmt.Fprintf(cfg.Stdout, "  Loaded competitive analysis template\n\n")
	} else {
		// Use general mode prompts (existing behavior)
		resolvedPrompts, err := LoadAndResolvePrompts(flags)
		if err != nil {
			fmt.Fprintf(cfg.Stderr, "Error loading prompts: %v\n", err)
			return 1
		}
		analyzePrompt = resolvedPrompts.AnalyzePrompt
		fmt.Fprintf(cfg.Stdout, "  Prompts loaded and @file syntax resolved\n\n")
	}

	// Step 3: Analyze topics with Gemini (using custom analyze prompt)
	fmt.Fprintf(cfg.Stdout, "Step 3: Analyzing topics with Gemini...\n")
	topics, err := gemini.AnalyzeTopics(content.Raw, analyzePrompt)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "Error analyzing topics: %v\n", err)
		return 3
	}
	fmt.Fprintf(cfg.Stdout, "  Identified %d topics:\n", len(topics))
	for i, topic := range topics {
		fmt.Fprintf(cfg.Stdout, "    %d. %s\n", i+1, topic)
	}
	fmt.Fprintf(cfg.Stdout, "\n")

	// Step 3.5: Generate research prompt (mode-specific)
	var researchPrompt string
	if mode.IsCompetitive() {
		// Use competitive gap-analysis template with topics
		researchPrompt, err = loadGapAnalysisPrompt(url, targetURL, topics)
		if err != nil {
			fmt.Fprintf(cfg.Stderr, "Error loading gap analysis template: %v\n", err)
			return 3
		}
	} else {
		// General mode: use variable substitution (existing behavior)
		resolvedPrompts, err := LoadAndResolvePrompts(flags)
		if err != nil {
			fmt.Fprintf(cfg.Stderr, "Error loading prompts: %v\n", err)
			return 1
		}
		finalVariables := config.Variables{
			URL:         targetURL,
			Topics:      topics,
			ContentType: contentType.String(),
		}
		finalPrompts, err := SubstituteVariables(resolvedPrompts, finalVariables)
		if err != nil {
			fmt.Fprintf(cfg.Stderr, "Error substituting final variables: %v\n", err)
			return 3
		}
		researchPrompt = finalPrompts.ResearchPrompt
	}

	// Step 4: Run Deep Research
	fmt.Fprintf(cfg.Stdout, "Step 4: Running Deep Research...\n")
	fmt.Fprintf(cfg.Stdout, "Research prompt: %s\n", researchPrompt)
	client, err := research.NewClient(cfg.ProjectID)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "Error creating research client: %v\n", err)
		return 4
	}

	pollConfig := &research.PollConfig{
		IntervalSeconds: cfg.PollInterval,
		TimeoutMinutes:  cfg.Timeout,
	}

	report, err := client.RunResearch(ctx, topics, pollConfig)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "Error running research: %v\n", err)
		return 4
	}
	fmt.Fprintf(cfg.Stdout, "\n")

	// Step 5: Write output
	fmt.Fprintf(cfg.Stdout, "Step 5: Writing output files...\n")

	// Write to cache
	cachePath, err := writeToCache(targetURL, contentType.String(), content.Raw, topics, report, cfg, flags)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "Warning: Failed to write to cache: %v\n", err)
		// Continue with legacy output
	} else {
		fmt.Fprintf(cfg.Stdout, "  Cached research at: %s/report.md\n", cachePath)
	}

	// Also write to legacy output directory for backwards compatibility
	outputPath, err := WriteOutput(cfg.OutputDir, targetURL, contentType, content, topics, report)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "Error writing output: %v\n", err)
		return 5
	}
	fmt.Fprintf(cfg.Stdout, "  Output directory: %s\n\n", outputPath)

	// Success
	fmt.Fprintf(cfg.Stdout, "Pipeline completed successfully!\n")
	return 0
}

// writeToCache writes research results to the cache directory
func writeToCache(url, contentType, content string, topics []string, report string, cfg *config.Config, flags *types.Flags) (string, error) {
	// Calculate content hash
	contentHash := calculateContentHash(content)

	// Create Research struct
	research := &cache.Research{
		URL:         url,
		ContentHash: contentHash,
		Topics:      topics,
		Content:     report,
		ContentType: contentType,
	}

	// Write to cache
	return cache.Write(research, cfg.CacheDir, flags.Force)
}

// calculateContentHash computes SHA-256 hash of content
func calculateContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("sha256:%x", hash)
}

// runDiscovery executes Stage 0: competitor URL discovery
func runDiscovery(ctx context.Context, query string, cfg *config.Config) ([]string, error) {
	// Load API configuration from environment
	apiKey := os.Getenv("GOOGLE_CUSTOM_SEARCH_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GOOGLE_CUSTOM_SEARCH_API_KEY environment variable not set")
	}

	searchEngineID := os.Getenv("GOOGLE_SEARCH_ENGINE_ID")
	if searchEngineID == "" {
		return nil, fmt.Errorf("GOOGLE_SEARCH_ENGINE_ID environment variable not set")
	}

	// Create discovery config
	discoveryConfig := discovery.SearchConfig{
		APIKey:         apiKey,
		SearchEngineID: searchEngineID,
		MaxResults:     5, // Default: 5 URLs
	}

	// Discover competitor URLs
	urls, err := discovery.DiscoverCompetitorURLs(ctx, query, discoveryConfig)
	if err != nil {
		return nil, fmt.Errorf("discovery failed: %w", err)
	}

	return urls, nil
}

// loadCompetitivePrompt loads the competitive analysis template
func loadCompetitivePrompt(query string, targetURL string) (string, error) {
	// Import templates package
	loader, err := templates.NewLoader()
	if err != nil {
		return "", fmt.Errorf("failed to create template loader: %w", err)
	}

	// Extract competitor name from query
	competitorName := discovery.ExtractCompetitorName(query)

	// Create prompt data
	data := templates.PromptData{
		Competitor: competitorName,
		Target:     "Our Tool", // TODO: Make configurable
		URL:        targetURL,
		Topics:     []string{}, // Will be filled after analysis
	}

	// Validate data
	if err := templates.ValidateData(data); err != nil {
		return "", fmt.Errorf("invalid template data: %w", err)
	}

	// Render analyze template
	prompt, err := loader.Render(templates.TemplateAnalyze, data)
	if err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}

	return prompt, nil
}

// loadGapAnalysisPrompt loads the gap analysis template with topics
func loadGapAnalysisPrompt(query string, targetURL string, topics []string) (string, error) {
	// Import templates package
	loader, err := templates.NewLoader()
	if err != nil {
		return "", fmt.Errorf("failed to create template loader: %w", err)
	}

	// Extract competitor name from query
	competitorName := discovery.ExtractCompetitorName(query)

	// Create prompt data
	data := templates.PromptData{
		Competitor: competitorName,
		Target:     "Our Tool", // TODO: Make configurable
		URL:        targetURL,
		Topics:     topics,
	}

	// Validate data
	if err := templates.ValidateData(data); err != nil {
		return "", fmt.Errorf("invalid template data: %w", err)
	}

	// Render gap-analysis template
	prompt, err := loader.Render(templates.TemplateGapAnalysis, data)
	if err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}

	return prompt, nil
}
