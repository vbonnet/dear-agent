package engram

import (
	"testing"
)

func TestIsAvailable(t *testing.T) {
	cfg := EngramConfig{}
	client := NewClient(cfg)

	// With library integration, always available
	if !client.IsAvailable() {
		t.Error("IsAvailable() should return true with library integration")
	}
}

func TestFilterByScore(t *testing.T) {
	results := []EngramResult{
		{Score: 0.95, Title: "High"},
		{Score: 0.65, Title: "Low"},
		{Score: 0.75, Title: "Medium"},
	}

	filtered := filterByScore(results, 0.7)
	if len(filtered) != 2 {
		t.Errorf("Expected 2 results with score ≥0.7, got %d", len(filtered))
	}
	for _, r := range filtered {
		if r.Score < 0.7 {
			t.Errorf("Expected all filtered results to have score ≥0.7, got %.2f", r.Score)
		}
	}
}

func TestFilterByScore_AllBelowThreshold(t *testing.T) {
	results := []EngramResult{
		{Score: 0.5, Title: "Low1"},
		{Score: 0.6, Title: "Low2"},
	}

	filtered := filterByScore(results, 0.7)
	if len(filtered) != 0 {
		t.Errorf("Expected 0 results when all below threshold, got %d", len(filtered))
	}
}

func TestResolveEngramPath_Default(t *testing.T) {
	cfg := EngramConfig{BinaryPath: ""}
	client := NewClient(cfg).(*libClient)

	path := client.resolveEngramPath()
	if path != "engrams" {
		t.Errorf("Expected default path 'engrams', got %s", path)
	}
}

func TestResolveEngramPath_Custom(t *testing.T) {
	cfg := EngramConfig{BinaryPath: "/custom/path"}
	client := NewClient(cfg).(*libClient)

	path := client.resolveEngramPath()
	if path != "/custom/path" {
		t.Errorf("Expected custom path '/custom/path', got %s", path)
	}
}

// Integration test would require:
// - Real engrams directory
// - ANTHROPIC_API_KEY for ranking
// Example:
//
// func TestQuery_Integration(t *testing.T) {
//     if testing.Short() {
//         t.Skip("Skipping integration test")
//     }
//
//     cfg := LoadEngramConfig()
//     client := NewClient(cfg)
//
//     results, err := client.Query("error handling", []string{"go"})
//     if err != nil {
//         t.Fatalf("Query failed: %v", err)
//     }
//
//     if len(results) == 0 {
//         t.Error("Expected some results")
//     }
// }
