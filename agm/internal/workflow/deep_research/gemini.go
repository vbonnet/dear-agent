package deep_research

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/workflow"
)

// GeminiDeepResearch implements the deep-research workflow for Gemini agent.
//
// This workflow uses the gemini-deep-research CLI tool to perform research on URLs.
// It extracts content, analyzes topics, and generates comprehensive research reports.
type GeminiDeepResearch struct {
	cliPath string
}

// NewGeminiDeepResearch creates a new Gemini deep-research workflow.
func NewGeminiDeepResearch() *GeminiDeepResearch {
	// Default CLI path (can be overridden via environment variable)
	cliPath := os.Getenv("GEMINI_DR_PATH")
	if cliPath == "" {
		// Try common locations
		homeDir, err := os.UserHomeDir()
		if err == nil {
			// Try ~/src/ai-tools/tools/gemini-deep-research/gemini-deep-research
			cliPath = filepath.Join(homeDir, "src/ai-tools/tools/gemini-deep-research/gemini-deep-research")
			if _, err := os.Stat(cliPath); err != nil {
				// Fallback to PATH
				cliPath = "gemini-deep-research"
			}
		}
	}

	return &GeminiDeepResearch{
		cliPath: cliPath,
	}
}

// Name returns the workflow identifier.
func (w *GeminiDeepResearch) Name() string {
	return "deep-research"
}

// Description returns a human-readable description.
func (w *GeminiDeepResearch) Description() string {
	return "Research URLs using Gemini Deep Research API and synthesize insights"
}

// SupportedHarnesses returns the list of harnesses that support this workflow.
func (w *GeminiDeepResearch) SupportedHarnesses() []string {
	return []string{"gemini-cli"}
}

// Execute runs the deep-research workflow.
// Supports both single and multi-URL research with parallel execution.
func (w *GeminiDeepResearch) Execute(ctx workflow.WorkflowContext) (workflow.WorkflowResult, error) {
	startTime := time.Now()

	// Extract URLs from prompt
	urls := extractURLs(ctx.Prompt)
	if len(urls) == 0 {
		return workflow.WorkflowResult{
			Success: false,
			Summary: "No URLs found in prompt",
		}, fmt.Errorf("no URLs detected in prompt")
	}

	// Single URL: direct execution
	if len(urls) == 1 {
		return w.executeSingleURL(ctx, urls[0], startTime)
	}

	// Multi-URL: parallel orchestration
	return w.executeMultiURL(ctx, urls, startTime)
}

// executeSingleURL handles research for a single URL.
func (w *GeminiDeepResearch) executeSingleURL(ctx workflow.WorkflowContext, url string, startTime time.Time) (workflow.WorkflowResult, error) {
	// Execute gemini-deep-research CLI
	reportPath, err := w.runDeepResearch(string(ctx.SessionID), url)
	if err != nil {
		return workflow.WorkflowResult{
			Success:       false,
			Summary:       fmt.Sprintf("Deep research failed: %v", err),
			ExecutionTime: time.Since(startTime),
		}, err
	}

	// Create artifact
	artifact := workflow.Artifact{
		Type: "research-report",
		Path: reportPath,
	}

	// Get file size
	if stat, err := os.Stat(reportPath); err == nil {
		artifact.Size = stat.Size()
	}

	return workflow.WorkflowResult{
		Success:       true,
		Artifacts:     []workflow.Artifact{artifact},
		Summary:       fmt.Sprintf("Research completed for %s", url),
		LogPath:       reportPath,
		ExecutionTime: time.Since(startTime),
	}, nil
}

// executeMultiURL handles parallel research for multiple URLs.
func (w *GeminiDeepResearch) executeMultiURL(ctx workflow.WorkflowContext, urls []string, startTime time.Time) (workflow.WorkflowResult, error) {
	// Create crash-resilient logger
	logger, err := NewResearchLogger(string(ctx.SessionID), urls, ctx.WorkingDirectory)
	if err != nil {
		return workflow.WorkflowResult{
			Success: false,
			Summary: fmt.Sprintf("Failed to create logger: %v", err),
		}, err
	}

	// Check for already-completed URLs (resume logic)
	completedURLs := logger.GetCompletedURLs()
	pendingURLs := logger.GetPendingURLs()

	if len(completedURLs) > 0 {
		fmt.Printf("⏭️  Resuming session - skipping %d already-completed URLs\n", len(completedURLs))
		for _, url := range completedURLs {
			fmt.Printf("   ✓ %s (already complete)\n", url)
		}
	}

	if len(pendingURLs) == 0 {
		fmt.Printf("✓ All URLs already researched - loading results\n")
		// All URLs already complete, just load artifacts and apply proposals
		artifacts := logger.GetArtifacts()
		applicationResult, err := w.applyResearch(ctx, artifacts, logger)
		if err != nil {
			fmt.Printf("Warning: Failed to apply research: %v\n", err)
		}

		// Add log file as artifact
		artifacts = append(artifacts, workflow.Artifact{
			Type: "research-log",
			Path: logger.GetLogPath(),
		})

		return workflow.WorkflowResult{
			Success:       len(artifacts) > 0,
			Artifacts:     artifacts,
			Summary:       fmt.Sprintf("All %d URLs already researched", len(urls)),
			ExecutionTime: time.Since(startTime),
			Metadata: map[string]interface{}{
				"urls_total":          len(urls),
				"urls_successful":     len(completedURLs),
				"urls_failed":         0,
				"resumed":             true,
				"proposals_generated": applicationResult != nil,
				"application_summary": getApplicationSummary(applicationResult),
			},
		}, nil
	}

	type researchResult struct {
		url        string
		reportPath string
		err        error
	}

	results := make(chan researchResult, len(pendingURLs))

	// Start parallel research workflows for pending URLs only
	for i, url := range pendingURLs {
		go func(index int, url string) {
			fmt.Printf("[%d/%d] Starting research: %s\n", index+1, len(urls), url)
			logger.MarkStarted(url)

			reportPath, err := w.runDeepResearch(string(ctx.SessionID), url)
			results <- researchResult{
				url:        url,
				reportPath: reportPath,
				err:        err,
			}
		}(i, url)
	}

	// Collect results
	successCount := len(completedURLs) // Count already-completed URLs
	var errors []string

	for i := 0; i < len(pendingURLs); i++ {
		result := <-results

		if result.err != nil {
			// Track error but continue with other results
			logger.MarkFailed(result.url, result.err)
			errors = append(errors, fmt.Sprintf("%s: %v", result.url, result.err))
			fmt.Printf("✗ Research failed: %s (%v)\n", result.url, result.err)
			continue
		}

		// Mark as completed in log
		logger.MarkCompleted(result.url, result.reportPath)
		successCount++
		fmt.Printf("✓ Research completed: %s\n", result.url)
	}

	// Build summary
	summary := fmt.Sprintf("Researched %d/%d URLs successfully", successCount, len(urls))
	if len(errors) > 0 {
		summary += fmt.Sprintf(" (%d failed)", len(errors))
	}
	if len(completedURLs) > 0 {
		summary += fmt.Sprintf(" (resumed from %d existing)", len(completedURLs))
	}

	// Determine overall success
	success := successCount > 0 // At least one URL succeeded

	// Get all artifacts (including previously completed ones)
	artifacts := logger.GetArtifacts()

	// Apply research insights if successful
	applicationResult, err := w.applyResearch(ctx, artifacts, logger)
	if err != nil {
		fmt.Printf("Warning: Failed to apply research: %v\n", err)
	}

	// Add log file as artifact
	artifacts = append(artifacts, workflow.Artifact{
		Type: "research-log",
		Path: logger.GetLogPath(),
	})

	metadata := map[string]interface{}{
		"urls_total":          len(urls),
		"urls_successful":     successCount,
		"urls_failed":         len(errors),
		"errors":              errors,
		"resumed":             len(completedURLs) > 0,
		"proposals_generated": applicationResult != nil,
		"application_summary": getApplicationSummary(applicationResult),
	}

	return workflow.WorkflowResult{
		Success:       success,
		Artifacts:     artifacts,
		Summary:       summary,
		ExecutionTime: time.Since(startTime),
		Metadata:      metadata,
	}, nil
}

// applyResearch applies research insights and updates the log.
func (w *GeminiDeepResearch) applyResearch(ctx workflow.WorkflowContext, artifacts []workflow.Artifact, logger *ResearchLogger) (*ApplicationResult, error) {
	if len(artifacts) == 0 {
		return nil, fmt.Errorf("no artifacts to apply")
	}

	applicator, err := NewResearchApplicator([]string{"engram", "ai-tools"})
	if err != nil {
		return nil, fmt.Errorf("create applicator: %w", err)
	}

	result, err := applicator.Apply(context.Background(), artifacts)
	if err != nil {
		return nil, fmt.Errorf("apply research: %w", err)
	}

	// Write proposals to separate file
	proposalsPath := ctx.OutputPath
	if proposalsPath == "" {
		proposalsPath = "research-proposals.md"
	}
	if err := WriteProposalsToMarkdown(result, proposalsPath); err != nil {
		return &result, fmt.Errorf("write proposals: %w", err)
	}
	fmt.Printf("✓ Proposals written to: %s\n", proposalsPath)

	// Add proposals to log file
	if err := logger.AddProposals(result); err != nil {
		return &result, fmt.Errorf("add proposals to log: %w", err)
	}
	fmt.Printf("✓ Proposals added to log: %s\n", logger.GetLogPath())

	return &result, nil
}

// getApplicationSummary safely extracts summary from application result.
func getApplicationSummary(result *ApplicationResult) string {
	if result == nil {
		return ""
	}
	return result.Summary
}

// runDeepResearch executes the gemini-deep-research CLI and returns the report path.
func (w *GeminiDeepResearch) runDeepResearch(sessionID, url string) (string, error) {
	// Create context with timeout (60 minute max for deep research)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	// Build command
	cmd := exec.CommandContext(ctx, w.cliPath, url)
	cmd.Env = os.Environ()

	// Capture stdout (contains cache path)
	output, err := cmd.Output()
	if err != nil {
		// Check for specific error types
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("deep research timed out after 60 minutes")
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("deep research failed: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("execute gemini-deep-research: %w", err)
	}

	// Parse stdout to extract report path
	reportPath := w.parseReportPath(string(output))
	if reportPath == "" {
		return "", fmt.Errorf("could not extract report path from CLI output")
	}

	// Expand tilde in path if present
	if strings.HasPrefix(reportPath, "~/") {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			reportPath = filepath.Join(homeDir, reportPath[2:])
		}
	}

	// Handle literal tilde directory bug (P2 bug from testing)
	// gemini-dr may create ./~/src/research/ instead of expanding ~
	if strings.HasPrefix(reportPath, "~/") && !filepath.IsAbs(reportPath) {
		// Try as literal path first
		if _, err := os.Stat(reportPath); os.IsNotExist(err) {
			// Try with ./ prefix (literal tilde directory)
			literalPath := "./" + reportPath
			if _, err := os.Stat(literalPath); err == nil {
				reportPath = literalPath
			}
		}
	}

	// Verify report file exists
	if _, err := os.Stat(reportPath); err != nil {
		return "", fmt.Errorf("report file not found at %s: %w", reportPath, err)
	}

	return reportPath, nil
}

// parseReportPath extracts the report path from CLI stdout.
// Expected format: "Research already exists at: ~/path/to/report.md"
// or "Deep Research completed. Report saved to: ~/path/to/report.md"
func (w *GeminiDeepResearch) parseReportPath(output string) string {
	// Common patterns in gemini-dr output
	patterns := []string{
		`Research already exists at: (.+)`,
		`Report saved to: (.+)`,
		`~/[^\s]+/report\.md`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(output)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
		// Try direct match (for simple path patterns)
		if match := re.FindString(output); match != "" {
			return strings.TrimSpace(match)
		}
	}

	// Fallback: look for any line containing report.md
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "report.md") {
			// Extract path-like string
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.Contains(part, "report.md") {
					return strings.TrimSpace(part)
				}
			}
		}
	}

	return ""
}

// extractURLs extracts HTTP/HTTPS URLs from a text prompt.
// Pattern: https?://[^\s<>"]+
func extractURLs(text string) []string {
	pattern := regexp.MustCompile(`https?://[^\s<>"]+`)
	matches := pattern.FindAllString(text, -1)

	// Clean up trailing punctuation
	var cleaned []string
	for _, url := range matches {
		url = strings.TrimRight(url, ".,;:!?")
		cleaned = append(cleaned, url)
	}

	return cleaned
}
