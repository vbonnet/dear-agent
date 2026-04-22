package ranking

import (
	"context"
	"sort"
	"strings"
)

// LocalProvider implements local tag-based ranking using Jaccard similarity
type LocalProvider struct{}

// NewLocal creates a new local fallback provider
func NewLocal() *LocalProvider {
	return &LocalProvider{}
}

// Name returns provider name
func (p *LocalProvider) Name() string {
	return "local"
}

// Model returns model identifier
func (p *LocalProvider) Model() string {
	return "local-jaccard-v1"
}

// Rank performs local ranking using tag Jaccard similarity + description keyword overlap.
// The final score blends tag matching (weight 0.7) with description keyword matching
// (weight 0.3) to produce more meaningful relevance scores even without an AI provider.
func (p *LocalProvider) Rank(ctx context.Context, query string, candidates []Candidate) ([]RankedResult, error) {
	// Extract query keywords (simple whitespace split + lowercasing)
	queryKeywords := extractKeywords(query)

	// Rank each candidate
	results := make([]RankedResult, len(candidates))
	for i, candidate := range candidates {
		tagScore, matched := jaccardSimilarity(queryKeywords, candidate.Tags)

		// Also score against description keywords
		descKeywords := extractKeywords(candidate.Description)
		descScore, descMatched := jaccardSimilarity(queryKeywords, descKeywords)

		// Blend: tags (0.7) + description (0.3)
		score := tagScore*0.7 + descScore*0.3

		reasoning := formatReasoningWithDesc(matched, descMatched)

		results[i] = RankedResult{
			Candidate: candidate,
			Score:     score,
			Reasoning: reasoning,
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results, nil
}

// Capabilities returns local provider capabilities
func (p *LocalProvider) Capabilities() Capabilities {
	return Capabilities{
		SupportsCaching:          false,
		SupportsStructuredOutput: false,
		MaxConcurrentRequests:    0, // No limit for local
		MaxTokensPerRequest:      0, // No limit for local
	}
}

// extractKeywords extracts keywords from query string
func extractKeywords(query string) []string {
	// Lowercase and split by whitespace
	query = strings.ToLower(query)
	words := strings.Fields(query)

	// Remove duplicates and empty strings
	seen := make(map[string]bool)
	keywords := make([]string, 0, len(words))
	for _, word := range words {
		if word != "" && !seen[word] {
			keywords = append(keywords, word)
			seen[word] = true
		}
	}

	return keywords
}

// jaccardSimilarity calculates Jaccard similarity between two sets
// Returns similarity score [0.0, 1.0] and matched tags
func jaccardSimilarity(setA, setB []string) (float64, []string) {
	if len(setA) == 0 && len(setB) == 0 {
		return 0.0, nil
	}

	// Normalize tags to lowercase for case-insensitive matching
	normalizedA := make([]string, len(setA))
	for i, s := range setA {
		normalizedA[i] = strings.ToLower(s)
	}

	normalizedB := make([]string, len(setB))
	for i, s := range setB {
		normalizedB[i] = strings.ToLower(s)
	}

	// Calculate intersection
	intersection := make(map[string]bool)
	setAMap := make(map[string]bool)
	for _, item := range normalizedA {
		setAMap[item] = true
	}

	matched := make([]string, 0)
	for _, item := range normalizedB {
		if setAMap[item] {
			intersection[item] = true
			matched = append(matched, item)
		}
	}

	// Calculate union
	union := make(map[string]bool)
	for _, item := range normalizedA {
		union[item] = true
	}
	for _, item := range normalizedB {
		union[item] = true
	}

	if len(union) == 0 {
		return 0.0, nil
	}

	// Jaccard similarity = |intersection| / |union|
	similarity := float64(len(intersection)) / float64(len(union))
	return similarity, matched
}

// formatReasoning creates reasoning string for ranking (tag-only, kept for backward compat)
func formatReasoning(matched []string) string {
	if len(matched) == 0 {
		return "No matching tags"
	}

	return "Matched tags: " + strings.Join(matched, ", ")
}

// formatReasoningWithDesc creates reasoning string that includes both tag and description matches.
func formatReasoningWithDesc(tagMatched, descMatched []string) string {
	parts := make([]string, 0, 2)
	if len(tagMatched) > 0 {
		parts = append(parts, "Matched tags: "+strings.Join(tagMatched, ", "))
	}
	if len(descMatched) > 0 {
		parts = append(parts, "Matched description keywords: "+strings.Join(descMatched, ", "))
	}
	if len(parts) == 0 {
		return "No matches"
	}
	return strings.Join(parts, "; ")
}
