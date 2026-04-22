package dashboard

import (
	"context"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/telemetry/agent"
)

func setupTestDB(t *testing.T) *agent.Storage {
	t.Helper()

	// Create in-memory SQLite database
	storage, err := agent.NewStorageAt(":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Insert test data with varying characteristics
	ctx := context.Background()

	// High specificity, with examples, success
	features1 := agent.Features{
		WordCount:             50,
		TokenCount:            1200,
		SpecificityScore:      0.8, // High
		HasExamples:           true,
		HasConstraints:        true,
		ContextEmbeddingScore: 0.9,
	}
	id1, _ := storage.LogLaunch(ctx, "Test prompt 1", "claude-sonnet-4.5", features1)
	storage.UpdateOutcome(ctx, id1, "success", 1500)

	// Medium specificity, without examples, success
	features2 := agent.Features{
		WordCount:             30,
		TokenCount:            800,
		SpecificityScore:      0.5, // Medium
		HasExamples:           false,
		HasConstraints:        false,
		ContextEmbeddingScore: 0.6,
	}
	id2, _ := storage.LogLaunch(ctx, "Test prompt 2", "claude-sonnet-4.5", features2)
	storage.UpdateOutcome(ctx, id2, "success", 900)

	// Low specificity, without examples, failure
	features3 := agent.Features{
		WordCount:             20,
		TokenCount:            500,
		SpecificityScore:      0.2, // Low
		HasExamples:           false,
		HasConstraints:        false,
		ContextEmbeddingScore: 0.3,
	}
	id3, _ := storage.LogLaunch(ctx, "Test prompt 3", "claude-sonnet-4.5", features3)
	storage.UpdateOutcomeFull(ctx, id3, "failure", 600, 2, "error", 3000)

	// High specificity, with examples, success (for trends test)
	features4 := agent.Features{
		WordCount:             60,
		TokenCount:            1300,
		SpecificityScore:      0.9, // High
		HasExamples:           true,
		HasConstraints:        true,
		ContextEmbeddingScore: 0.95,
	}
	id4, _ := storage.LogLaunch(ctx, "Test prompt 4", "claude-sonnet-4.5", features4)
	storage.UpdateOutcome(ctx, id4, "success", 1600)

	return storage
}

func TestQuerySuccessBySpecificity(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	metrics, err := QuerySuccessBySpecificity(ctx, storage.DB(), time.Time{}, time.Time{})

	if err != nil {
		t.Fatalf("QuerySuccessBySpecificity() error = %v", err)
	}

	if len(metrics) == 0 {
		t.Fatal("Expected metrics, got empty result")
	}

	// Verify we have 3 buckets (High, Medium, Low)
	if len(metrics) != 3 {
		t.Errorf("Expected 3 specificity buckets, got %d", len(metrics))
	}

	// Verify High bucket
	highFound := false
	for _, m := range metrics {
		if m.Level == "High (>0.7)" {
			highFound = true
			if m.Total != 2 {
				t.Errorf("High specificity: expected 2 total, got %d", m.Total)
			}
			if m.Successes != 2 {
				t.Errorf("High specificity: expected 2 successes, got %d", m.Successes)
			}
			if m.SuccessRate != 100.0 {
				t.Errorf("High specificity: expected 100%% success rate, got %.1f%%", m.SuccessRate)
			}
		}
	}

	if !highFound {
		t.Error("High specificity bucket not found in results")
	}
}

func TestQuerySuccessByExamples(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	metrics, err := QuerySuccessByExamples(ctx, storage.DB(), time.Time{}, time.Time{})

	if err != nil {
		t.Fatalf("QuerySuccessByExamples() error = %v", err)
	}

	if len(metrics) == 0 {
		t.Fatal("Expected metrics, got empty result")
	}

	// Verify we have 2 groups (With Examples, Without Examples)
	if len(metrics) != 2 {
		t.Errorf("Expected 2 example groups, got %d", len(metrics))
	}

	// Verify "With Examples" group
	withFound := false
	for _, m := range metrics {
		if m.Status == "With Examples" {
			withFound = true
			if m.Total != 2 {
				t.Errorf("With Examples: expected 2 total, got %d", m.Total)
			}
			if m.Successes != 2 {
				t.Errorf("With Examples: expected 2 successes, got %d", m.Successes)
			}
		}
	}

	if !withFound {
		t.Error("With Examples group not found in results")
	}
}

func TestQueryTokenEfficiency(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	metrics, err := QueryTokenEfficiency(ctx, storage.DB(), time.Time{}, time.Time{})

	if err != nil {
		t.Fatalf("QueryTokenEfficiency() error = %v", err)
	}

	if len(metrics) == 0 {
		t.Fatal("Expected metrics, got empty result")
	}

	// Verify we have 2 groups (Specific, Vague)
	if len(metrics) != 2 {
		t.Errorf("Expected 2 prompt types, got %d", len(metrics))
	}

	// Verify "Specific" group has higher success rate
	specificFound := false
	for _, m := range metrics {
		if m.PromptType == "Specific (>0.7)" {
			specificFound = true
			if m.AvgTokens <= 0 {
				t.Errorf("Specific prompts: expected positive avg tokens, got %.0f", m.AvgTokens)
			}
			// Specific prompts should have 100% success (2/2)
			if m.SuccessRate != 100.0 {
				t.Errorf("Specific prompts: expected 100%% success rate, got %.1f%%", m.SuccessRate)
			}
		}
	}

	if !specificFound {
		t.Error("Specific prompt type not found in results")
	}
}

func TestQueryTrendsOverTime(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	metrics, err := QueryTrendsOverTime(ctx, storage.DB(), time.Time{}, time.Time{})

	if err != nil {
		t.Fatalf("QueryTrendsOverTime() error = %v", err)
	}

	if len(metrics) == 0 {
		t.Fatal("Expected metrics, got empty result")
	}

	// Verify metrics are ordered by date DESC (most recent first)
	if len(metrics) > 1 {
		for i := 0; i < len(metrics)-1; i++ {
			if metrics[i].Date < metrics[i+1].Date {
				t.Errorf("Trends not ordered by date DESC: %s comes before %s", metrics[i].Date, metrics[i+1].Date)
			}
		}
	}

	// Verify each metric has required fields
	for _, m := range metrics {
		if m.Date == "" {
			t.Error("Trend metric missing date")
		}
		if m.TotalLaunches <= 0 {
			t.Error("Trend metric has invalid total launches")
		}
	}
}

func TestQuerySuccessBySpecificity_WithDateFilter(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	// Query with future date (should return empty)
	ctx := context.Background()
	futureDate := time.Now().Add(24 * time.Hour)
	metrics, err := QuerySuccessBySpecificity(ctx, storage.DB(), futureDate, time.Time{})

	if err != nil {
		t.Fatalf("QuerySuccessBySpecificity() with date filter error = %v", err)
	}

	// Should be empty since all test data is from "now"
	if len(metrics) != 0 {
		t.Errorf("Expected empty result with future date filter, got %d metrics", len(metrics))
	}
}

func TestQuerySuccessByExamples_EmptyDatabase(t *testing.T) {
	// Create empty database
	storage, err := agent.NewStorageAt(":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	metrics, err := QuerySuccessByExamples(ctx, storage.DB(), time.Time{}, time.Time{})

	if err != nil {
		t.Fatalf("QuerySuccessByExamples() on empty database error = %v", err)
	}

	// Should return empty slice (not error)
	if len(metrics) != 0 {
		t.Errorf("Expected empty result for empty database, got %d metrics", len(metrics))
	}
}

func TestQueryTokenEfficiency_WithDateFilter(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	since := time.Now().Add(-24 * time.Hour)
	until := time.Now().Add(24 * time.Hour)

	metrics, err := QueryTokenEfficiency(ctx, storage.DB(), since, until)
	if err != nil {
		t.Fatalf("QueryTokenEfficiency() with date filter error = %v", err)
	}

	// Should return results within date range
	if len(metrics) == 0 {
		t.Error("Expected metrics with date filter, got empty result")
	}
}

func TestQueryTrendsOverTime_WithDateFilter(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	since := time.Now().Add(-24 * time.Hour)
	until := time.Now().Add(24 * time.Hour)

	metrics, err := QueryTrendsOverTime(ctx, storage.DB(), since, until)
	if err != nil {
		t.Fatalf("QueryTrendsOverTime() with date filter error = %v", err)
	}

	// Should return results within date range
	if len(metrics) == 0 {
		t.Error("Expected trends with date filter, got empty result")
	}
}

func TestQuerySuccessByExamples_WithDateFilter(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	since := time.Now().Add(-24 * time.Hour)
	until := time.Now().Add(24 * time.Hour)

	metrics, err := QuerySuccessByExamples(ctx, storage.DB(), since, until)
	if err != nil {
		t.Fatalf("QuerySuccessByExamples() with date filter error = %v", err)
	}

	// Should return results
	if len(metrics) == 0 {
		t.Error("Expected results with date filter")
	}
}
