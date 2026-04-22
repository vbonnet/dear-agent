// Package agent provides telemetry logging for sub-agent launches.
//
// This package tracks agent execution patterns to enable learning from
// prompt effectiveness, success rates, and resource consumption.
package agent

import (
	"regexp"
	"strings"
)

// Features represents extracted prompt characteristics.
type Features struct {
	WordCount             int     `json:"word_count"`
	TokenCount            int     `json:"token_count"`
	SpecificityScore      float64 `json:"specificity_score"`
	HasExamples           bool    `json:"has_examples"`
	HasConstraints        bool    `json:"has_constraints"`
	ContextEmbeddingScore float64 `json:"context_embedding_score"`
}

// Patterns for feature extraction
var (
	// Concrete terms: file paths, camelCase, numbers
	filePathPattern  = regexp.MustCompile(`\w+\.(go|md|sql|py|js|ts|yaml|json|sh|txt)`)
	camelCasePattern = regexp.MustCompile(`[A-Z][a-z]+[A-Z]`)
	numberPattern    = regexp.MustCompile(`\d+`)

	// Examples: code blocks, structured data
	codeBlockPattern  = regexp.MustCompile("```")
	structuredPattern = regexp.MustCompile(`[{\[]`)

	// Constraints: limit words
	constraintWords = []string{"limit", "max", "maximum", "min", "minimum", "must", "should", "exactly"}

	// References: contextual words
	referenceWords = []string{"above", "previous", "earlier", "it", "that", "this", "these", "those"}
)

// ExtractFeatures extracts prompt characteristics for telemetry analysis.
//
// Features extracted:
//   - WordCount: Number of words (split on whitespace)
//   - TokenCount: Approximate token count (same as word count for V1)
//   - SpecificityScore: Ratio of concrete terms to total words (0.0-1.0)
//   - HasExamples: Presence of code blocks or structured data
//   - HasConstraints: Presence of numbers or limit keywords
//   - ContextEmbeddingScore: Self-containedness (1.0 = fully embedded, 0.0 = many references)
//
// Example:
//
//	prompt := "Create a function calculateTotal() that takes an array of numbers and returns the sum. Limit to 100 elements."
//	features := ExtractFeatures(prompt)
//	// features.WordCount = 20
//	// features.SpecificityScore ≈ 0.30 (calculateTotal, numbers, 100)
//	// features.HasConstraints = true (Limit, 100)
func ExtractFeatures(prompt string) Features {
	if prompt == "" {
		return Features{}
	}

	// Word count and token count
	words := strings.Fields(prompt)
	wordCount := len(words)
	tokenCount := wordCount // V1 approximation, V2 can use tiktoken

	// Specificity score: concrete terms / total words
	specificityScore := calculateSpecificity(prompt, wordCount)

	// Has examples: code blocks or structured data
	hasExamples := detectExamples(prompt)

	// Has constraints: numbers or limit keywords
	hasConstraints := detectConstraints(prompt)

	// Context embedding score: 1.0 - (references / sentences)
	contextEmbeddingScore := calculateContextScore(prompt)

	return Features{
		WordCount:             wordCount,
		TokenCount:            tokenCount,
		SpecificityScore:      clamp(specificityScore, 0.0, 1.0),
		HasExamples:           hasExamples,
		HasConstraints:        hasConstraints,
		ContextEmbeddingScore: clamp(contextEmbeddingScore, 0.0, 1.0),
	}
}

// calculateSpecificity computes ratio of concrete terms to total words.
//
// Concrete terms include:
//   - File paths (e.g., "features.go", "README.md")
//   - CamelCase identifiers (e.g., "ExtractFeatures", "LogAgentLaunch")
//   - Numbers (e.g., "100", "42")
//
// Higher score = more specific, concrete prompts
// Lower score = vague, abstract prompts
func calculateSpecificity(prompt string, wordCount int) float64 {
	if wordCount == 0 {
		return 0.0
	}

	concreteCount := 0

	// Count file paths
	concreteCount += len(filePathPattern.FindAllString(prompt, -1))

	// Count camelCase identifiers
	concreteCount += len(camelCasePattern.FindAllString(prompt, -1))

	// Count numbers
	concreteCount += len(numberPattern.FindAllString(prompt, -1))

	return float64(concreteCount) / float64(wordCount)
}

// detectExamples checks for presence of code blocks or structured data.
//
// Indicators:
//   - Code blocks: ``` (triple backticks)
//   - Structured data: { or [ (JSON/YAML-like)
//
// Returns true if any indicator found.
func detectExamples(prompt string) bool {
	// Check for code blocks
	if codeBlockPattern.MatchString(prompt) {
		return true
	}

	// Check for structured data (JSON/YAML)
	if structuredPattern.MatchString(prompt) {
		return true
	}

	return false
}

// detectConstraints checks for presence of numbers or limit keywords.
//
// Indicators:
//   - Numbers: \d+ (e.g., "100", "42")
//   - Limit keywords: "limit", "max", "minimum", "must", etc.
//
// Returns true if any indicator found.
func detectConstraints(prompt string) bool {
	promptLower := strings.ToLower(prompt)

	// Check for numbers
	if numberPattern.MatchString(prompt) {
		return true
	}

	// Check for constraint keywords
	for _, keyword := range constraintWords {
		if strings.Contains(promptLower, keyword) {
			return true
		}
	}

	return false
}

// calculateContextScore computes self-containedness of prompt.
//
// Algorithm: 1.0 - (reference_count / sentence_count)
//
// Reference words: "above", "previous", "earlier", "it", "that", "this", etc.
//
// High score (0.8-1.0) = self-contained, all context embedded
// Low score (0.0-0.3) = many references to external context
func calculateContextScore(prompt string) float64 {
	// Count sentences (approximate with periods)
	sentences := strings.Split(prompt, ".")
	sentenceCount := len(sentences)
	if sentenceCount == 0 {
		return 1.0 // Empty prompt is fully embedded (vacuously true)
	}

	promptLower := strings.ToLower(prompt)
	referenceCount := 0

	// Count reference words
	for _, refWord := range referenceWords {
		referenceCount += strings.Count(promptLower, " "+refWord+" ")
		referenceCount += strings.Count(promptLower, " "+refWord+".")
		referenceCount += strings.Count(promptLower, " "+refWord+",")
	}

	// Score: 1.0 - (references / sentences)
	score := 1.0 - (float64(referenceCount) / float64(sentenceCount))
	return score
}

// clamp restricts value to [min, max] range.
func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
