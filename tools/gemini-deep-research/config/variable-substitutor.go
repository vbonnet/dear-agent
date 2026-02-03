package config

import (
	"fmt"
	"regexp"
	"strings"
)

// Variables represents template variables available for substitution
type Variables struct {
	URL         string   // Source URL
	Topics      []string // Extracted topics
	ContentType string   // Content type (article, video, PDF, etc.)
}

// ResolvedConfig represents configuration with @file syntax resolved
type ResolvedConfig struct {
	ExtractPrompt  string
	AnalyzePrompt  string
	ResearchPrompt string
}

// FinalPrompts represents configuration with all variables substituted
type FinalPrompts struct {
	ExtractPrompt  string
	AnalyzePrompt  string
	ResearchPrompt string
}

// VariableSubstitutor handles template variable replacement in prompts
type VariableSubstitutor struct{}

// NewVariableSubstitutor creates a new VariableSubstitutor instance
func NewVariableSubstitutor() *VariableSubstitutor {
	return &VariableSubstitutor{}
}

// Substitute replaces template variables in all prompts with actual values
// Returns error if unknown variables are found
func (vs *VariableSubstitutor) Substitute(config ResolvedConfig, variables Variables) (FinalPrompts, error) {
	extractPrompt, err := vs.substituteVariables(config.ExtractPrompt, variables)
	if err != nil {
		return FinalPrompts{}, fmt.Errorf("extract prompt: %w", err)
	}

	analyzePrompt, err := vs.substituteVariables(config.AnalyzePrompt, variables)
	if err != nil {
		return FinalPrompts{}, fmt.Errorf("analyze prompt: %w", err)
	}

	researchPrompt, err := vs.substituteVariables(config.ResearchPrompt, variables)
	if err != nil {
		return FinalPrompts{}, fmt.Errorf("research prompt: %w", err)
	}

	return FinalPrompts{
		ExtractPrompt:  extractPrompt,
		AnalyzePrompt:  analyzePrompt,
		ResearchPrompt: researchPrompt,
	}, nil
}

// substituteVariables replaces variables in a single template string
func (vs *VariableSubstitutor) substituteVariables(template string, variables Variables) (string, error) {
	result := template

	// Substitute {url}
	result = strings.ReplaceAll(result, "{url}", variables.URL)

	// Substitute {topics} (comma-separated)
	topicsStr := strings.Join(variables.Topics, ", ")
	result = strings.ReplaceAll(result, "{topics}", topicsStr)

	// Substitute {content_type}
	result = strings.ReplaceAll(result, "{content_type}", variables.ContentType)

	// Fail-fast: Check for unsubstituted variables
	pattern := regexp.MustCompile(`\{(\w+)\}`)
	remaining := pattern.FindAllString(result, -1)

	if len(remaining) > 0 {
		// Extract variable names (remove { })
		unknownVars := make([]string, len(remaining))
		for i, v := range remaining {
			unknownVars[i] = strings.Trim(v, "{}")
		}

		availableVars := []string{"url", "topics", "content_type"}
		return "", fmt.Errorf("unknown variables: %s (available: %s)",
			strings.Join(remaining, ", "),
			strings.Join(availableVars, ", "))
	}

	return result, nil
}
