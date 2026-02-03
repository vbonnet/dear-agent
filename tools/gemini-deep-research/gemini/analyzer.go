package gemini

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	// DefaultPromptTemplate is the default prompt for topic analysis
	DefaultPromptTemplate = `Analyze the following content and identify the main research topics.
Return ONLY valid JSON in this format:
{"topics": ["topic1", "topic2", "topic3"]}

%sCONTENT:
%s`
)

// AnalyzeTopics analyzes content using Gemini CLI and returns identified topics
//
// Parameters:
//   - content: The text content to analyze
//   - customPrompt: Optional custom prompt to focus analysis (empty string for default)
//
// Returns:
//   - []string: Array of identified topics (1-10 topics typically)
//   - error: Error if Gemini CLI fails, JSON is invalid, or no topics found
//
// Error types:
//   - ErrGeminiNotFound: gemini CLI not installed
//   - ErrInvalidJSON: Gemini returned invalid JSON
//   - ErrNoTopics: Gemini returned empty topics array
//   - ErrAPIError: Gemini API error occurred
//
// Example:
//
//	topics, err := AnalyzeTopics(content, "")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Identified topics: %v\n", topics)
func AnalyzeTopics(content, customPrompt string) ([]string, error) {
	// Check if gemini CLI is installed
	if !isGeminiInstalled() {
		return nil, NewGeminiNotFoundError()
	}

	// Build prompt
	prompt := buildPrompt(content, customPrompt)

	// Execute Gemini CLI
	output, err := runGeminiCLI(prompt)
	if err != nil {
		return nil, fmt.Errorf("run gemini CLI: %w", err)
	}

	// Parse JSON response
	topics, err := ParseTopicsJSON(output)
	if err != nil {
		return nil, fmt.Errorf("parse topics: %w", err)
	}

	return topics, nil
}

// isGeminiInstalled checks if the gemini CLI is available in PATH
func isGeminiInstalled() bool {
	_, err := exec.LookPath("gemini")
	return err == nil
}

// buildPrompt constructs the full prompt for Gemini CLI
func buildPrompt(content, customPrompt string) string {
	// Add custom prompt section if provided
	customSection := ""
	if customPrompt != "" {
		customSection = customPrompt + "\n\n"
	}

	return fmt.Sprintf(DefaultPromptTemplate, customSection, content)
}

// runGeminiCLI executes the gemini CLI with the given prompt
func runGeminiCLI(prompt string) (string, error) {
	// Execute: gemini "<prompt>"
	cmd := exec.Command("gemini", prompt)

	// Capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if it's an API error (non-zero exit code)
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", NewAPIError(fmt.Sprintf("exit code %d: %s", exitErr.ExitCode(), string(output)))
		}
		return "", fmt.Errorf("execute gemini command: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}
