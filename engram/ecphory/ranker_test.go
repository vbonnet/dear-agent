package ecphory

import (
	"context"
	"testing"
)

func TestRankingPromptGeneration(t *testing.T) {
	// Create a ranker (we won't call the API)
	r := &Ranker{
		provider:    nil, // Not testing API calls here
		rateLimiter: nil,
	}

	// Test rankingPrompt generation
	query := "How do I handle errors in Go?"
	candidates := []string{
		"/tmp/test/.engram/core/engrams/patterns/go/error-handling.ai.md",
		"/tmp/test/.engram/core/engrams/patterns/go/error-wrapping.ai.md",
		"/tmp/test/.engram/core/engrams/patterns/python/error-handling.ai.md",
	}

	rankingPrompt, err := r.buildRankingPrompt(query, candidates)
	if err != nil {
		t.Fatalf("buildRankingPrompt() error = %v", err)
	}

	// Verify rankingPrompt contains key elements
	if rankingPrompt == "" {
		t.Fatal("Prompt should not be empty")
	}

	// Check that query is included
	if !contains(rankingPrompt, query) {
		t.Errorf("Prompt should contain query: %s", query)
	}

	// Check that all candidates are included
	for _, candidate := range candidates {
		if !contains(rankingPrompt, candidate) {
			t.Errorf("Prompt should contain candidate: %s", candidate)
		}
	}

	// Check for JSON structure request
	if !contains(rankingPrompt, "JSON") {
		t.Error("Prompt should request JSON output")
	}

	if !contains(rankingPrompt, "path") || !contains(rankingPrompt, "relevance") || !contains(rankingPrompt, "reasoning") {
		t.Error("Prompt should specify required JSON fields (path, relevance, reasoning)")
	}
}

func TestNewRanker_NoAPIKey(t *testing.T) {
	// Save current API key
	originalKey := ""
	if key, exists := lookupEnv("ANTHROPIC_API_KEY"); exists {
		originalKey = key
		t.Setenv("ANTHROPIC_API_KEY", "")
	}
	defer func() {
		if originalKey != "" {
			t.Setenv("ANTHROPIC_API_KEY", originalKey)
		}
	}()

	// Test that NewRanker returns error when API key is missing
	_, err := NewRanker()
	if err == nil {
		t.Error("NewRanker should return error when ANTHROPIC_API_KEY is not set")
	}

	expectedErr := "neither GOOGLE_CLOUD_PROJECT nor ANTHROPIC_API_KEY environment variable set"
	if err.Error() != expectedErr {
		t.Errorf("Expected error: %s, got: %s", expectedErr, err.Error())
	}
}

// P0-1 TEST: Test API key validation
func TestNewRanker_InvalidAPIKeyFormat(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
		want   string
	}{
		{
			name:   "empty key",
			apiKey: "",
			want:   "neither GOOGLE_CLOUD_PROJECT nor ANTHROPIC_API_KEY environment variable set",
		},
		{
			name:   "invalid prefix - missing sk-ant-",
			apiKey: "invalid-key-12345",
			want:   "invalid API key format (expected sk-ant- prefix)",
		},
		{
			name:   "invalid prefix - old format sk-",
			apiKey: "sk-12345",
			want:   "invalid API key format (expected sk-ant- prefix)",
		},
		{
			name:   "invalid prefix - random string",
			apiKey: "randomstring",
			want:   "invalid API key format (expected sk-ant- prefix)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ANTHROPIC_API_KEY", tt.apiKey)

			_, err := NewRanker()
			if err == nil {
				t.Errorf("NewRanker() should return error for invalid API key: %s", tt.apiKey)
				return
			}

			if err.Error() != tt.want {
				t.Errorf("NewRanker() error = %v, want %v", err.Error(), tt.want)
			}

			// P0-1: Verify error message doesn't contain the actual API key
			if tt.apiKey != "" && contains(err.Error(), tt.apiKey) {
				t.Errorf("Error message should NOT contain API key value for security: %v", err)
			}
		})
	}
}

// P0-1 TEST: Test that valid API key format is accepted
func TestNewRanker_ValidAPIKeyFormat(t *testing.T) {
	// Use a valid format (but fake) API key
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key-12345")

	ranker, err := NewRanker()
	if err != nil {
		t.Errorf("NewRanker() should accept valid API key format: %v", err)
		return
	}

	if ranker == nil {
		t.Fatal("NewRanker() should return non-nil ranker with valid API key")
	}

	if ranker.rateLimiter == nil {
		t.Error("NewRanker() should initialize rate limiter")
	}

	if ranker.provider == nil {
		t.Error("NewRanker() should initialize provider")
	}
}

func TestRankingResult_Structure(t *testing.T) {
	// Test that RankingResult has expected fields
	result := RankingResult{
		Path:      "/some/path.ai.md",
		Relevance: 0.85,
		Reasoning: "Directly addresses the query",
	}

	if result.Path != "/some/path.ai.md" {
		t.Errorf("Path field not working correctly")
	}
	if result.Relevance != 0.85 {
		t.Errorf("Relevance field not working correctly")
	}
	if result.Reasoning != "Directly addresses the query" {
		t.Errorf("Reasoning field not working correctly")
	}
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func lookupEnv(key string) (string, bool) {
	// This is a test helper - in real code we'd use os.LookupEnv
	// For testing we just return empty
	return "", false
}

func TestRanker_Rank_EmptyCandidates(t *testing.T) {
	r := &Ranker{provider: nil, rateLimiter: nil}

	results, err := r.Rank(context.TODO(), "test query", []string{})

	if err != nil {
		t.Errorf("Rank() should not error on empty candidates: %v", err)
	}
	if results != nil {
		t.Error("Rank() should return nil for empty candidates")
	}
}
