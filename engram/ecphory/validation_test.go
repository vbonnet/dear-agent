package ecphory

import (
	"testing"
)

// TestValidateRankingResults tests the validation logic for ranking results
func TestValidateRankingResults_Valid(t *testing.T) {
	candidates := []string{"foo.ai.md", "bar.ai.md"}
	results := []RankingResult{
		{Path: "foo.ai.md", Relevance: 0.9, Reasoning: "Good match"},
		{Path: "bar.ai.md", Relevance: 0.5, Reasoning: "Partial match"},
	}

	err := validateRankingResults(results, candidates)
	if err != nil {
		t.Errorf("Valid results should pass validation, got: %v", err)
	}
}

// TestValidateRankingResults_EmptyResults tests validation with empty results
func TestValidateRankingResults_EmptyResults(t *testing.T) {
	candidates := []string{"foo.ai.md"}
	results := []RankingResult{}

	err := validateRankingResults(results, candidates)
	if err == nil {
		t.Error("Empty results should fail validation")
	}
}

// TestValidateRankingResults_EmptyPath tests validation with empty path
func TestValidateRankingResults_EmptyPath(t *testing.T) {
	candidates := []string{"foo.ai.md"}
	results := []RankingResult{
		{Path: "", Relevance: 0.9, Reasoning: "Test"},
	}

	err := validateRankingResults(results, candidates)
	if err == nil {
		t.Error("Empty path should fail validation")
	}
}

// TestValidateRankingResults_InvalidRelevance tests validation with out-of-range relevance
func TestValidateRankingResults_InvalidRelevance(t *testing.T) {
	candidates := []string{"foo.ai.md"}

	testCases := []struct {
		name      string
		relevance float64
	}{
		{"negative relevance", -0.1},
		{"relevance too high", 1.5},
		{"relevance way too high", 100.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			results := []RankingResult{
				{Path: "foo.ai.md", Relevance: tc.relevance, Reasoning: "Test"},
			}

			err := validateRankingResults(results, candidates)
			if err == nil {
				t.Errorf("Relevance %.2f should fail validation", tc.relevance)
			}
		})
	}
}

// TestValidateRankingResults_PathNotInCandidates tests validation when path not in candidates
func TestValidateRankingResults_PathNotInCandidates(t *testing.T) {
	candidates := []string{"foo.ai.md", "bar.ai.md"}
	results := []RankingResult{
		{Path: "baz.ai.md", Relevance: 0.9, Reasoning: "Test"},
	}

	err := validateRankingResults(results, candidates)
	if err == nil {
		t.Error("Path not in candidates should fail validation")
	}
}

// TestValidateRankingResults_BoundaryValues tests validation at boundary values
func TestValidateRankingResults_BoundaryValues(t *testing.T) {
	candidates := []string{"foo.ai.md"}

	testCases := []struct {
		name      string
		relevance float64
		shouldErr bool
	}{
		{"relevance 0.0", 0.0, false},
		{"relevance 1.0", 1.0, false},
		{"relevance 0.5", 0.5, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			results := []RankingResult{
				{Path: "foo.ai.md", Relevance: tc.relevance, Reasoning: "Test"},
			}

			err := validateRankingResults(results, candidates)
			if tc.shouldErr && err == nil {
				t.Errorf("Expected error for relevance %.2f", tc.relevance)
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("Unexpected error for relevance %.2f: %v", tc.relevance, err)
			}
		})
	}
}
