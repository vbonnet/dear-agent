package cmd

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
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
	// Detect analysis mode FIRST (before URL validation)
	// This allows natural language queries like "competitive analysis of X vs Y"
	mode := modes.DetectMode(url, flags.Mode)
	log.Printf("Mode detection: %s (explicit=%v)", mode.String(), flags.Mode != "")

	// Validate URL only if NOT in competitive mode (competitive mode accepts natural language queries)
	if !mode.IsCompetitive() {
		if err := ValidateURL(url); err != nil {
			fmt.Fprintf(cfg.Stderr, "Error: %v\n", err)
			fmt.Fprintf(cfg.Stderr, "\nExamples of valid URLs:\n")
			fmt.Fprintf(cfg.Stderr, "  https://www.youtube.com/watch?v=VIDEO_ID\n")
			fmt.Fprintf(cfg.Stderr, "  https://arxiv.org/abs/2601.20802\n")
			fmt.Fprintf(cfg.Stderr, "  https://huggingface.co/papers/2501.12345\n")
			fmt.Fprintf(cfg.Stderr, "  https://example.com/article\n")
			fmt.Fprintf(cfg.Stderr, "\nOr try competitive analysis:\n")
			fmt.Fprintf(cfg.Stderr, "  gemini-deep-research \"competitive analysis of Tool X vs Tool Y\"\n")
			return 1
		}
	}

	// Log configuration
	fmt.Fprintf(cfg.Stdout, "Configuration:\n")
	if mode.IsCompetitive() {
		fmt.Fprintf(cfg.Stdout, "  Query: %s\n", url)
	} else {
		fmt.Fprintf(cfg.Stdout, "  URL: %s\n", url)
	}
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

	// Stage 0: Discovery (competitive mode only, unless --no-discovery)
	var targetURL string
	if mode.IsCompetitive() && !flags.NoDiscovery {
		fmt.Fprintf(cfg.Stdout, "Stage 0: Discovering competitor URLs...\n")
		discoveredURLs, err := runDiscovery(ctx, url, flags, cfg)
		if err != nil {
			// Enhanced error handling - check if provided input is a valid URL
			log.Printf("Discovery error: %v", err)
			fmt.Fprintf(cfg.Stderr, "Warning: Discovery failed: %v\n", err)

			// Check if the provided input is a valid URL for fallback
			if err := ValidateURL(url); err != nil {
				// Not a valid URL - cannot proceed
				fmt.Fprintf(cfg.Stderr, "\nError: Discovery failed and the provided query is not a valid URL.\n\n")
				fmt.Fprintf(cfg.Stderr, "Options:\n")
				fmt.Fprintf(cfg.Stderr, "  1. Set up Google Custom Search API (see error message above)\n")
				fmt.Fprintf(cfg.Stderr, "  2. Provide a competitor URL with --no-discovery:\n")
				fmt.Fprintf(cfg.Stderr, "     gemini-deep-research --mode competitive --no-discovery <competitor-url>\n\n")
				return 1
			}

			// Valid URL provided - use as fallback
			fmt.Fprintf(cfg.Stderr, "Falling back to analyzing the provided URL directly...\n\n")
			targetURL = url
			fmt.Fprintf(cfg.Stdout, "  Analyzing (fallback): %s\n\n", targetURL)
		} else if len(discoveredURLs) == 0 {
			// No URLs discovered - check if provided input is a valid URL for fallback
			log.Printf("No URLs discovered")
			fmt.Fprintf(cfg.Stderr, "Warning: No competitor URLs discovered\n")

			// Check if the provided input is a valid URL for fallback
			if err := ValidateURL(url); err != nil {
				// Not a valid URL - cannot proceed
				fmt.Fprintf(cfg.Stderr, "\nError: No URLs discovered and the provided query is not a valid URL.\n\n")
				fmt.Fprintf(cfg.Stderr, "Options:\n")
				fmt.Fprintf(cfg.Stderr, "  1. Try a more specific competitor name in your query\n")
				fmt.Fprintf(cfg.Stderr, "  2. Provide a competitor URL with --no-discovery:\n")
				fmt.Fprintf(cfg.Stderr, "     gemini-deep-research --mode competitive --no-discovery <competitor-url>\n\n")
				return 1
			}

			// Valid URL provided - use as fallback
			fmt.Fprintf(cfg.Stderr, "Falling back to analyzing the provided URL directly...\n\n")
			targetURL = url
			fmt.Fprintf(cfg.Stdout, "  Analyzing (fallback): %s\n\n", targetURL)
		} else {
			// Discovery successful
			log.Printf("Discovery successful: %d URLs found", len(discoveredURLs))
			fmt.Fprintf(cfg.Stdout, "  Discovered %d URLs:\n", len(discoveredURLs))
			for i, u := range discoveredURLs {
				fmt.Fprintf(cfg.Stdout, "    %d. %s\n", i+1, u)
			}
			fmt.Fprintf(cfg.Stdout, "\n")

			// Use first discovered URL for analysis
			targetURL = discoveredURLs[0]
			log.Printf("Selected URL for analysis: %s", targetURL)
			fmt.Fprintf(cfg.Stdout, "  Analyzing: %s\n\n", targetURL)
		}
	} else {
		// General mode OR competitive with --no-discovery: use provided URL directly
		if mode.IsCompetitive() && flags.NoDiscovery {
			fmt.Fprintf(cfg.Stdout, "Stage 0: Skipping discovery (--no-discovery flag)\n\n")
		}
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
		log.Printf("Loading competitive analysis template for query: %s", url)
		analyzePrompt, err = loadCompetitivePrompt(url, targetURL)
		if err != nil {
			log.Printf("Template loading failed: %v", err)
			fmt.Fprintf(cfg.Stderr, "Error loading competitive template: %v\n", err)
			return 1
		}
		log.Printf("Competitive template loaded successfully (length: %d chars)", len(analyzePrompt))
		fmt.Fprintf(cfg.Stdout, "  Loaded competitive analysis template\n\n")
	} else {
		// Use general mode prompts (existing behavior)
		log.Printf("Loading general mode prompts")
		resolvedPrompts, err := LoadAndResolvePrompts(flags)
		if err != nil {
			log.Printf("Prompt loading failed: %v", err)
			fmt.Fprintf(cfg.Stderr, "Error loading prompts: %v\n", err)
			return 1
		}
		analyzePrompt = resolvedPrompts.AnalyzePrompt
		log.Printf("General prompts loaded successfully")
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
		log.Printf("Loading gap analysis template with %d topics", len(topics))
		researchPrompt, err = loadGapAnalysisPrompt(url, targetURL, topics)
		if err != nil {
			log.Printf("Gap analysis template loading failed: %v", err)
			fmt.Fprintf(cfg.Stderr, "Error loading gap analysis template: %v\n", err)
			return 3
		}
		log.Printf("Gap analysis template loaded successfully (length: %d chars)", len(researchPrompt))
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

	// Prepare output configuration
	outputConfig := &OutputConfig{
		Mode: mode.String(),
	}
	if mode.IsCompetitive() {
		outputConfig.Competitor = discovery.ExtractCompetitorName(url)
		outputConfig.SourceQuery = url
	}

	// Also write to legacy output directory for backwards compatibility
	outputPath, err := WriteOutput(cfg.OutputDir, targetURL, contentType, content, topics, report, outputConfig)
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
func runDiscovery(ctx context.Context, query string, flags *types.Flags, cfg *config.Config) ([]string, error) {
	// Load API configuration from environment with enhanced validation
	apiKey := os.Getenv("GOOGLE_CUSTOM_SEARCH_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GOOGLE_CUSTOM_SEARCH_API_KEY environment variable not set\n\n" +
			"Setup instructions:\n" +
			"1. Create a Google Custom Search API key: https://developers.google.com/custom-search/v1/overview\n" +
			"2. Export the environment variable:\n" +
			"   export GOOGLE_CUSTOM_SEARCH_API_KEY=\"your-api-key\"\n" +
			"3. Alternatively, use --no-discovery flag to skip URL discovery")
	}

	searchEngineID := os.Getenv("GOOGLE_SEARCH_ENGINE_ID")
	if searchEngineID == "" {
		return nil, fmt.Errorf("GOOGLE_SEARCH_ENGINE_ID environment variable not set\n\n" +
			"Setup instructions:\n" +
			"1. Create a Custom Search Engine: https://programmablesearchengine.google.com/\n" +
			"2. Get your Search Engine ID from the control panel\n" +
			"3. Export the environment variable:\n" +
			"   export GOOGLE_SEARCH_ENGINE_ID=\"your-search-engine-id\"\n" +
			"4. Alternatively, use --no-discovery flag to skip URL discovery")
	}

	// Determine max results from flags or use default
	maxResults := 5 // Default: 5 URLs
	if flags.DiscoveryLimit > 0 {
		maxResults = flags.DiscoveryLimit
	}

	// Create discovery config
	discoveryConfig := discovery.SearchConfig{
		APIKey:         apiKey,
		SearchEngineID: searchEngineID,
		MaxResults:     maxResults,
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
		return "", fmt.Errorf("failed to create template loader: %w\n\n"+
			"This is likely a bug in the template system. Please report this issue.", err)
	}

	// Extract competitor name from query
	competitorName := discovery.ExtractCompetitorName(query)
	if competitorName == "" {
		return "", fmt.Errorf("failed to extract competitor name from query: %q\n\n"+
			"Tip: Try using a more explicit query like 'GitHub Copilot vs Cursor' or 'analyze GitHub Copilot'", query)
	}

	// Create prompt data
	data := templates.PromptData{
		Competitor: competitorName,
		Target:     "Our Tool", // TODO: Make configurable
		URL:        targetURL,
		Topics:     []string{}, // Will be filled after analysis
	}

	// Validate data
	if err := templates.ValidateData(data); err != nil {
		return "", fmt.Errorf("invalid template data: %w\n\n"+
			"Template data: Competitor=%q, Target=%q, URL=%q", err, competitorName, "Our Tool", targetURL)
	}

	// Render analyze template
	prompt, err := loader.Render(templates.TemplateAnalyze, data)
	if err != nil {
		return "", fmt.Errorf("failed to render competitive analysis template: %w\n\n"+
			"This is likely a bug in the template system. Please report this issue.", err)
	}

	return prompt, nil
}

// loadGapAnalysisPrompt loads the gap analysis template with topics
func loadGapAnalysisPrompt(query string, targetURL string, topics []string) (string, error) {
	// Import templates package
	loader, err := templates.NewLoader()
	if err != nil {
		return "", fmt.Errorf("failed to create template loader: %w\n\n"+
			"This is likely a bug in the template system. Please report this issue.", err)
	}

	// Extract competitor name from query
	competitorName := discovery.ExtractCompetitorName(query)
	if competitorName == "" {
		return "", fmt.Errorf("failed to extract competitor name from query: %q\n\n"+
			"Tip: Try using a more explicit query like 'GitHub Copilot vs Cursor' or 'analyze GitHub Copilot'", query)
	}

	// Validate topics
	if len(topics) == 0 {
		return "", fmt.Errorf("no topics provided for gap analysis\n\n"+
			"Topics should be identified during the analysis stage. This is likely a pipeline error.")
	}

	// Create prompt data
	data := templates.PromptData{
		Competitor: competitorName,
		Target:     "Our Tool", // TODO: Make configurable
		URL:        targetURL,
		Topics:     topics,
	}

	// Validate data
	if err := templates.ValidateData(data); err != nil {
		return "", fmt.Errorf("invalid template data: %w\n\n"+
			"Template data: Competitor=%q, Target=%q, URL=%q, Topics=%d",
			err, competitorName, "Our Tool", targetURL, len(topics))
	}

	// Render gap-analysis template
	prompt, err := loader.Render(templates.TemplateGapAnalysis, data)
	if err != nil {
		return "", fmt.Errorf("failed to render gap analysis template: %w\n\n"+
			"This is likely a bug in the template system. Please report this issue.", err)
	}

	return prompt, nil
}
