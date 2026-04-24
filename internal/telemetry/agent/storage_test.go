package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewStorage(t *testing.T) {
	// Use temp database for testing
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewStorageAt(dbPath)
	if err != nil {
		t.Fatalf("NewStorageAt() failed: %v", err)
	}
	defer storage.Close()

	// Verify database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}
}

func TestLogLaunch(t *testing.T) {
	storage := setupTestStorage(t)
	defer storage.Close()

	ctx := context.Background()
	prompt := "Create a function calculateTotal() with limit 100"
	model := "claude-sonnet-4.5"
	features := ExtractFeatures(prompt)

	id, err := storage.LogLaunch(ctx, prompt, model, features)
	if err != nil {
		t.Fatalf("LogLaunch() failed: %v", err)
	}

	if id == 0 {
		t.Error("Expected non-zero launch ID")
	}

	// Verify launch was stored
	filters := QueryFilters{Limit: 10}
	launches, err := storage.Query(ctx, filters)
	if err != nil {
		t.Fatalf("Query() failed: %v", err)
	}

	if len(launches) != 1 {
		t.Errorf("Expected 1 launch, got %d", len(launches))
	}

	launch := launches[0]
	if launch.PromptText != prompt {
		t.Errorf("PromptText = %q, want %q", launch.PromptText, prompt)
	}

	if launch.Model != model {
		t.Errorf("Model = %q, want %q", launch.Model, model)
	}

	if launch.WordCount != features.WordCount {
		t.Errorf("WordCount = %d, want %d", launch.WordCount, features.WordCount)
	}
}

func TestUpdateOutcome(t *testing.T) {
	storage := setupTestStorage(t)
	defer storage.Close()

	ctx := context.Background()

	// Log a launch
	features := Features{WordCount: 10, TokenCount: 10, SpecificityScore: 0.5}
	id, err := storage.LogLaunch(ctx, "test prompt", "test-model", features)
	if err != nil {
		t.Fatalf("LogLaunch() failed: %v", err)
	}

	// Update outcome
	err = storage.UpdateOutcome(ctx, id, "success", 1500)
	if err != nil {
		t.Fatalf("UpdateOutcome() failed: %v", err)
	}

	// Verify outcome was updated
	filters := QueryFilters{Outcome: "success", Limit: 10}
	launches, err := storage.Query(ctx, filters)
	if err != nil {
		t.Fatalf("Query() failed: %v", err)
	}

	if len(launches) != 1 {
		t.Errorf("Expected 1 launch with success outcome, got %d", len(launches))
	}

	launch := launches[0]
	if launch.Outcome != "success" {
		t.Errorf("Outcome = %q, want %q", launch.Outcome, "success")
	}

	if launch.TokensUsed != 1500 {
		t.Errorf("TokensUsed = %d, want %d", launch.TokensUsed, 1500)
	}
}

func TestQuery_Filters(t *testing.T) {
	storage := setupTestStorage(t)
	defer storage.Close()

	ctx := context.Background()

	// Insert test data
	features1 := Features{WordCount: 10, TokenCount: 10}
	id1, _ := storage.LogLaunch(ctx, "prompt 1", "model-1", features1)
	storage.UpdateOutcome(ctx, id1, "success", 1000)

	features2 := Features{WordCount: 20, TokenCount: 20}
	id2, _ := storage.LogLaunch(ctx, "prompt 2", "model-2", features2)
	storage.UpdateOutcome(ctx, id2, "failure", 2000)

	features3 := Features{WordCount: 15, TokenCount: 15}
	storage.LogLaunch(ctx, "prompt 3", "model-1", features3)

	tests := []struct {
		name       string
		filters    QueryFilters
		wantCount  int
		checkFirst func(t *testing.T, launch AgentLaunch)
	}{
		{
			name:      "filter by outcome success",
			filters:   QueryFilters{Outcome: "success", Limit: 10},
			wantCount: 1,
			checkFirst: func(t *testing.T, launch AgentLaunch) {
				if launch.Outcome != "success" {
					t.Errorf("Outcome = %q, want success", launch.Outcome)
				}
			},
		},
		{
			name:      "filter by model",
			filters:   QueryFilters{Model: "model-1", Limit: 10},
			wantCount: 2,
			checkFirst: func(t *testing.T, launch AgentLaunch) {
				if launch.Model != "model-1" {
					t.Errorf("Model = %q, want model-1", launch.Model)
				}
			},
		},
		{
			name:      "filter by outcome failure",
			filters:   QueryFilters{Outcome: "failure", Limit: 10},
			wantCount: 1,
			checkFirst: func(t *testing.T, launch AgentLaunch) {
				if launch.Outcome != "failure" {
					t.Errorf("Outcome = %q, want failure", launch.Outcome)
				}
			},
		},
		{
			name:      "no filters",
			filters:   QueryFilters{Limit: 10},
			wantCount: 3,
		},
		{
			name:      "limit",
			filters:   QueryFilters{Limit: 2},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			launches, err := storage.Query(ctx, tt.filters)
			if err != nil {
				t.Fatalf("Query() failed: %v", err)
			}

			if len(launches) != tt.wantCount {
				t.Errorf("Query() returned %d launches, want %d", len(launches), tt.wantCount)
			}

			if len(launches) > 0 && tt.checkFirst != nil {
				tt.checkFirst(t, launches[0])
			}
		})
	}
}

func TestStats(t *testing.T) {
	storage := setupTestStorage(t)
	defer storage.Close()

	ctx := context.Background()

	// Insert test data
	features1 := Features{WordCount: 10, TokenCount: 10, SpecificityScore: 0.5, ContextEmbeddingScore: 0.8}
	id1, _ := storage.LogLaunchFull(ctx, "prompt 1", "test-model", "", "", "", features1)
	storage.UpdateOutcome(ctx, id1, "success", 1000)

	features2 := Features{WordCount: 20, TokenCount: 20, SpecificityScore: 0.7, ContextEmbeddingScore: 0.9}
	id2, _ := storage.LogLaunchFull(ctx, "prompt 2", "test-model", "", "", "", features2)
	storage.UpdateOutcome(ctx, id2, "success", 2000)

	features3 := Features{WordCount: 15, TokenCount: 15, SpecificityScore: 0.3, ContextEmbeddingScore: 0.6}
	id3, _ := storage.LogLaunchFull(ctx, "prompt 3", "test-model", "", "", "", features3)
	storage.UpdateOutcome(ctx, id3, "failure", 500)

	stats, err := storage.Stats(ctx, "test-model")
	if err != nil {
		t.Fatalf("Stats() failed: %v", err)
	}

	if stats.Total != 3 {
		t.Errorf("Total = %d, want 3", stats.Total)
	}

	if stats.SuccessCount != 2 {
		t.Errorf("SuccessCount = %d, want 2", stats.SuccessCount)
	}

	wantSuccessRate := 2.0 / 3.0
	if abs(stats.SuccessRate-wantSuccessRate) > 0.01 {
		t.Errorf("SuccessRate = %.2f, want %.2f", stats.SuccessRate, wantSuccessRate)
	}

	wantAvgTokens := (1000.0 + 2000.0 + 500.0) / 3.0
	if abs(stats.AvgTokensUsed-wantAvgTokens) > 1.0 {
		t.Errorf("AvgTokensUsed = %.2f, want %.2f", stats.AvgTokensUsed, wantAvgTokens)
	}

	wantAvgSpec := (0.5 + 0.7 + 0.3) / 3.0
	if abs(stats.AvgSpecificityScore-wantAvgSpec) > 0.01 {
		t.Errorf("AvgSpecificityScore = %.2f, want %.2f", stats.AvgSpecificityScore, wantAvgSpec)
	}
}

func TestBoolConversion(t *testing.T) {
	tests := []struct {
		name string
		b    bool
		want int
	}{
		{"true", true, 1},
		{"false", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := boolToInt(tt.b); got != tt.want {
				t.Errorf("boolToInt() = %v, want %v", got, tt.want)
			}

			if got := intToBool(tt.want); got != tt.b {
				t.Errorf("intToBool() = %v, want %v", got, tt.b)
			}
		})
	}
}

// Helper functions

func setupTestStorage(t *testing.T) *Storage {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewStorageAt(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test storage: %v", err)
	}

	return storage
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
