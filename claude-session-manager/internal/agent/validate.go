package agent

import (
	"fmt"
	"os"
	"strings"
)

// Known agent names
var knownAgents = []string{"claude", "gemini", "gpt"}

// Agent-to-environment-variable mapping
var agentEnvVars = map[string]string{
	"claude": "ANTHROPIC_API_KEY",
	"gemini": "GEMINI_API_KEY",
	"gpt":    "OPENAI_API_KEY",
}

// Agent help URLs
var agentHelpURLs = map[string]string{
	"claude": "https://console.anthropic.com/",
	"gemini": "https://ai.google.dev/",
	"gpt":    "https://platform.openai.com/api-keys",
}

// ValidateAgentName checks if the agent name is valid (claude, gemini, gpt)
func ValidateAgentName(name string) error {
	for _, known := range knownAgents {
		if name == known {
			return nil
		}
	}

	// Suggest closest match for typos
	suggestion := suggestAgent(name)
	if suggestion != "" {
		return fmt.Errorf("invalid agent '%s'. Must be one of: %v\n\nDid you mean '%s'?",
			name, knownAgents, suggestion)
	}

	return fmt.Errorf("invalid agent '%s'. Must be one of: %v", name, knownAgents)
}

// ValidateAgentAvailability checks if the agent's API key environment variable is set
func ValidateAgentAvailability(name string) error {
	envVar, ok := agentEnvVars[name]
	if !ok {
		return fmt.Errorf("unknown agent: %s", name)
	}

	// Skip API key check for Claude if Vertex AI is configured
	if name == "claude" && isVertexAIConfigured() {
		return nil
	}

	if os.Getenv(envVar) == "" {
		return &AgentUnavailableError{
			Agent:  name,
			EnvVar: envVar,
		}
	}

	return nil
}

// isVertexAIConfigured checks if Vertex AI environment is configured
func isVertexAIConfigured() bool {
	return os.Getenv("CLOUD_ML_REGION") != ""
}

// AgentUnavailableError represents an error when an agent's API key is not set
type AgentUnavailableError struct {
	Agent  string
	EnvVar string
}

// Error returns a formatted error message with helpful instructions
func (e *AgentUnavailableError) Error() string {
	helpURL, ok := agentHelpURLs[e.Agent]
	if !ok {
		helpURL = "https://www.anthropic.com/"
	}

	return fmt.Sprintf("Agent '%s' unavailable. %s environment variable not set.\n\n"+
		"To use %s, set your API key:\n  export %s=your-api-key\n\n"+
		"Get API key: %s",
		e.Agent, e.EnvVar, e.Agent, e.EnvVar, helpURL)
}

// suggestAgent suggests the closest matching agent name for typos
func suggestAgent(name string) string {
	// Simple prefix matching for common typos
	for _, known := range knownAgents {
		if strings.HasPrefix(known, name) {
			return known
		}
		// Also check if the input is a prefix match (e.g., "gem" -> "gemini")
		if strings.HasPrefix(known, strings.ToLower(name)) {
			return known
		}
	}

	// Check for common typos (edit distance 1)
	// For "claud" -> "claude", "gemi" -> "gemini"
	for _, known := range knownAgents {
		if levenshteinDistance(strings.ToLower(name), known) == 1 {
			return known
		}
	}

	return ""
}

// levenshteinDistance calculates the Levenshtein distance between two strings
func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); i++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(a)][len(b)]
}

func min(a, b, c int) int {
	if a < b && a < c {
		return a
	}
	if b < c {
		return b
	}
	return c
}
