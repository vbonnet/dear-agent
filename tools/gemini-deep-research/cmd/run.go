package cmd

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/config"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/detector"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/extractors"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/gemini"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/internal/cache"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/internal/modes"
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

	// Execute pipeline
	return executePipeline(url, flags, cfg)
}

// truncate truncates a string to maxLen characters with ellipsis
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// executePipeline executes the full E2E pipeline
func executePipeline(url string, flags *types.Flags, cfg *config.Config) int {
	ctx := context.Background()

	// Step 1: Detect content type
	fmt.Fprintf(cfg.Stdout, "Step 1: Detecting content type...\n")
	contentType, err := detector.DetectContentType(url, flags.Type)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "Error detecting content type: %v\n", err)
		return 1
	}
	fmt.Fprintf(cfg.Stdout, "  Detected: %s\n\n", contentType.String())

	// Step 1.5: Check cache (unless --force)
	if !flags.Force {
		fmt.Fprintf(cfg.Stdout, "Checking cache for existing research...\n")
		cachePath, exists, err := cache.Check(url, cfg.CacheDir, contentType.String())
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

	content, err := extractor.Extract(ctx, url)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "Error extracting content: %v\n", err)
		return 2
	}
	fmt.Fprintf(cfg.Stdout, "  Extracted %d characters\n\n", len(content.Raw))

	// Step 2.5: Load and resolve prompts (ConfigParser → FileResolver)
	fmt.Fprintf(cfg.Stdout, "Step 2.5: Loading prompt configuration...\n")
	resolvedPrompts, err := LoadAndResolvePrompts(flags)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "Error loading prompts: %v\n", err)
		return 1
	}
	fmt.Fprintf(cfg.Stdout, "  Prompts loaded and @file syntax resolved\n\n")

	// Step 2.6: Variable substitution for extract/analyze prompts (url, content_type available)
	initialVariables := config.Variables{
		URL:         url,
		Topics:      []string{}, // Not available yet
		ContentType: contentType.String(),
	}
	partialPrompts, err := SubstituteVariables(resolvedPrompts, initialVariables)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "Error substituting variables: %v\n", err)
		return 1
	}

	// Step 3: Analyze topics with Gemini (using custom analyze prompt)
	fmt.Fprintf(cfg.Stdout, "Step 3: Analyzing topics with Gemini...\n")
	topics, err := gemini.AnalyzeTopics(content.Raw, partialPrompts.AnalyzePrompt)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "Error analyzing topics: %v\n", err)
		return 3
	}
	fmt.Fprintf(cfg.Stdout, "  Identified %d topics:\n", len(topics))
	for i, topic := range topics {
		fmt.Fprintf(cfg.Stdout, "    %d. %s\n", i+1, topic)
	}
	fmt.Fprintf(cfg.Stdout, "\n")

	// Step 3.5: Re-substitute variables with topics for research prompt
	finalVariables := config.Variables{
		URL:         url,
		Topics:      topics,
		ContentType: contentType.String(),
	}
	finalPrompts, err := SubstituteVariables(resolvedPrompts, finalVariables)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "Error substituting final variables: %v\n", err)
		return 3
	}

	// Use finalPrompts.ResearchPrompt for the research stage (with topics substituted)
	// Note: finalPrompts has {topics} variable replaced with actual topics list

	// Step 4: Run Deep Research
	fmt.Fprintf(cfg.Stdout, "Step 4: Running Deep Research...\n")
	fmt.Fprintf(cfg.Stdout, "Research prompt: %s\n", finalPrompts.ResearchPrompt)
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
	cachePath, err := writeToCache(url, contentType.String(), content.Raw, topics, report, cfg, flags)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "Warning: Failed to write to cache: %v\n", err)
		// Continue with legacy output
	} else {
		fmt.Fprintf(cfg.Stdout, "  Cached research at: %s/report.md\n", cachePath)
	}

	// Also write to legacy output directory for backwards compatibility
	outputPath, err := WriteOutput(cfg.OutputDir, url, contentType, content, topics, report)
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
