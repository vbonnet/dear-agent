package deep_research

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/workflow"
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
			// Try ~/src/ws/oss/repos/ai-tools/main/tools/gemini-deep-research/gemini-deep-research
			cliPath = filepath.Join(homeDir, "src/ws/oss/repos/ai-tools/main/tools/gemini-deep-research/gemini-deep-research")
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

// SupportedAgents returns the list of agents that support this workflow.
func (w *GeminiDeepResearch) SupportedAgents() []string {
	return []string{"gemini"}
}

// Execute runs the deep-research workflow.
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

	// For now, support single URL research
	// Multi-URL orchestration will be added in Phase 3
	if len(urls) > 1 {
		return workflow.WorkflowResult{
			Success: false,
			Summary: fmt.Sprintf("Multiple URL research not yet supported (found %d URLs)", len(urls)),
		}, fmt.Errorf("multi-URL research not implemented (use Phase 3)")
	}

	url := urls[0]

	// Execute gemini-deep-research CLI
	reportPath, err := w.runDeepResearch(string(ctx.SessionID), url)
	if err != nil {
		return workflow.WorkflowResult{
			Success: false,
			Summary: fmt.Sprintf("Deep research failed: %v", err),
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
		Success: true,
		Artifacts: []workflow.Artifact{artifact},
		Summary: fmt.Sprintf("Research completed for %s", url),
		LogPath: reportPath,
		ExecutionTime: time.Since(startTime),
	}, nil
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
		if exitErr, ok := err.(*exec.ExitError); ok {
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
	// gemini-dr may create ./~/src/ws/oss/research/ instead of expanding ~
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
