package ecphory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/engram/internal/reflection"
)

// TestValidateEcphoryBoosting_TopResults validates that failure reflections
// appear in top 5 results during debugging queries (Task 1.4.2)
func TestValidateEcphoryBoosting_TopResults(t *testing.T) {
	reflectionsDir := setupReflectionFixtures(t)

	// Build index
	idx := NewIndex()
	if err := idx.Build(reflectionsDir); err != nil {
		t.Fatalf("Failed to build index: %v", err)
	}

	detector := NewContextDetector()
	ecphory := &Ecphory{
		index:           idx,
		contextDetector: detector,
	}

	testCases := []struct {
		name                 string
		query                string
		expectedCategory     reflection.ErrorCategory
		minReflectionsInTop5 int
	}{
		{
			name:                 "syntax_error_debugging",
			query:                "syntax error in my Go code - missing bracket",
			expectedCategory:     reflection.ErrorCategorySyntax,
			minReflectionsInTop5: 1, // At least 1 syntax_error reflection in top 5
		},
		{
			name:                 "permission_denied_debugging",
			query:                "permission denied when trying to access config file",
			expectedCategory:     reflection.ErrorCategoryPermissionDenied,
			minReflectionsInTop5: 1,
		},
		{
			name:                 "timeout_debugging",
			query:                "database query timeout - operation timed out",
			expectedCategory:     reflection.ErrorCategoryTimeout,
			minReflectionsInTop5: 1,
		},
		{
			name:                 "tool_misuse_debugging",
			query:                "wrong tool used - committed to wrong branch",
			expectedCategory:     reflection.ErrorCategoryToolMisuse,
			minReflectionsInTop5: 1,
		},
		{
			name:                 "other_error_debugging",
			query:                "nil pointer dereference error in production",
			expectedCategory:     reflection.ErrorCategoryOther,
			minReflectionsInTop5: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Tier 1: Filter candidates
			candidates := ecphory.fastFilter(nil, "")

			if len(candidates) == 0 {
				t.Fatal("No candidates found")
			}

			// Create mock ranking results (simulate all candidates having same base score)
			ranked := make([]RankingResult, 0, len(candidates))
			for _, path := range candidates {
				ranked = append(ranked, RankingResult{
					Path:      path,
					Relevance: 50.0, // Base score before boosting
				})
			}

			// Apply failure boosting (this is what we're validating)
			ecphory.applyFailureBoosting(tc.query, ranked)

			// Sort by relevance (simulate Tier 2 sorting)
			sortByRelevance(ranked)

			// Validate: Count matching reflections in top 5
			matchingInTop5 := 0
			top5Results := ranked
			if len(ranked) > 5 {
				top5Results = ranked[:5]
			}

			for i, r := range top5Results {
				data, err := os.ReadFile(r.Path)
				if err != nil {
					continue
				}

				category := extractErrorCategory(data)
				if category == string(tc.expectedCategory) {
					matchingInTop5++
					t.Logf("  Position %d: %s (score: %.1f, category: %s)",
						i+1, filepath.Base(r.Path), r.Relevance, category)
				}
			}

			if matchingInTop5 < tc.minReflectionsInTop5 {
				t.Errorf("Expected at least %d %s reflections in top 5, got %d",
					tc.minReflectionsInTop5, tc.expectedCategory, matchingInTop5)

				// Show what was in top 5 for debugging
				t.Logf("Top 5 results:")
				for i, r := range top5Results {
					data, _ := os.ReadFile(r.Path)
					category := extractErrorCategory(data)
					t.Logf("  %d. %s (score: %.1f, category: %s)",
						i+1, filepath.Base(r.Path), r.Relevance, category)
				}
			} else {
				t.Logf("Found %d matching reflections in top 5 (expected: %d)",
					matchingInTop5, tc.minReflectionsInTop5)
			}
		})
	}
}

// TestValidateEcphoryBoosting_RankingOrder validates that boosted reflections
// rank higher than non-boosted reflections
func TestValidateEcphoryBoosting_RankingOrder(t *testing.T) {
	reflectionsDir := setupReflectionFixtures(t)

	detector := NewContextDetector()
	ecphory := &Ecphory{
		contextDetector: detector,
	}

	// Test case: syntax error query should boost syntax_error reflections
	query := "syntax error in my code"
	expectedCategory := reflection.ErrorCategorySyntax

	// Create mixed ranking results
	syntaxReflection := filepath.Join(reflectionsDir, "syntax-error-missing-bracket.ai.md")
	otherReflection := filepath.Join(reflectionsDir, "permission-denied-file-access.ai.md")

	ranked := []RankingResult{
		{Path: syntaxReflection, Relevance: 50.0}, // Will be boosted to 75.0
		{Path: otherReflection, Relevance: 50.0},  // Will stay 50.0
	}

	// Apply boosting
	ecphory.applyFailureBoosting(query, ranked)

	// Verify boosting occurred
	if ranked[0].Relevance != 75.0 {
		t.Errorf("Syntax reflection should be boosted to 75.0, got %.1f", ranked[0].Relevance)
	}

	if ranked[1].Relevance != 50.0 {
		t.Errorf("Non-matching reflection should stay at 50.0, got %.1f", ranked[1].Relevance)
	}

	// Sort and verify order
	sortByRelevance(ranked)

	// After sorting, syntax reflection should be first
	data, err := os.ReadFile(ranked[0].Path)
	if err != nil {
		t.Fatalf("Failed to read top result: %v", err)
	}

	category := extractErrorCategory(data)
	if category != string(expectedCategory) {
		t.Errorf("Top result should be %s, got %s", expectedCategory, category)
	}

	t.Logf("Boosted reflection correctly ranked #1 (score: %.1f)", ranked[0].Relevance)
	t.Logf("Non-boosted reflection ranked #2 (score: %.1f)", ranked[1].Relevance)
}

// TestValidateEcphoryBoosting_AllCategories validates boosting works for all 5 categories
func TestValidateEcphoryBoosting_AllCategories(t *testing.T) {
	reflectionsDir := setupReflectionFixtures(t)

	detector := NewContextDetector()
	ecphory := &Ecphory{
		contextDetector: detector,
	}

	categories := []struct {
		category reflection.ErrorCategory
		query    string
		file     string
	}{
		{reflection.ErrorCategorySyntax, "syntax error", "syntax-error-missing-bracket.ai.md"},
		{reflection.ErrorCategoryPermissionDenied, "permission denied", "permission-denied-file-access.ai.md"},
		{reflection.ErrorCategoryTimeout, "timeout error", "timeout-database-query.ai.md"},
		{reflection.ErrorCategoryToolMisuse, "wrong tool", "tool-misuse-wrong-git-branch.ai.md"},
		{reflection.ErrorCategoryOther, "nil pointer error", "other-error-nil-pointer.ai.md"},
	}

	for _, cat := range categories {
		t.Run(string(cat.category), func(t *testing.T) {
			targetPath := filepath.Join(reflectionsDir, cat.file)
			otherPath := filepath.Join(reflectionsDir, "syntax-error-json-parsing.ai.md")
			if cat.category == reflection.ErrorCategorySyntax {
				otherPath = filepath.Join(reflectionsDir, "timeout-http-request.ai.md")
			}

			ranked := []RankingResult{
				{Path: targetPath, Relevance: 50.0},
				{Path: otherPath, Relevance: 50.0},
			}

			ecphory.applyFailureBoosting(cat.query, ranked)

			// Verify target was boosted
			if ranked[0].Relevance != 75.0 {
				t.Errorf("Category %s: reflection not boosted (score: %.1f)", cat.category, ranked[0].Relevance)
			}

			// Verify other was not boosted
			if ranked[1].Relevance != 50.0 {
				t.Errorf("Category %s: wrong reflection boosted (score: %.1f)", cat.category, ranked[1].Relevance)
			}

			t.Logf("Category %s: correctly boosted", cat.category)
		})
	}

	t.Logf("All 5 categories validated")
}

// TestValidateEcphoryBoosting_SimulatedDebugging simulates real debugging workflow
func TestValidateEcphoryBoosting_SimulatedDebugging(t *testing.T) {
	reflectionsDir := setupReflectionFixtures(t)

	// Create ecphory instance (without API key, just for boosting validation)
	idx := NewIndex()
	if err := idx.Build(reflectionsDir); err != nil {
		t.Fatalf("Failed to build index: %v", err)
	}

	ecphory := &Ecphory{
		index:           idx,
		contextDetector: NewContextDetector(),
		tokenBudget:     50000,
	}

	// Scenario: Developer encounters syntax error
	t.Run("scenario_syntax_error", func(t *testing.T) {
		query := "I'm getting a syntax error - missing closing bracket in my Go code"

		// Simulate debugging query
		candidates := ecphory.fastFilter(nil, "")

		ranked := make([]RankingResult, 0, len(candidates))
		for _, path := range candidates {
			ranked = append(ranked, RankingResult{Path: path, Relevance: 50.0})
		}

		// Apply boosting
		ecphory.applyFailureBoosting(query, ranked)
		sortByRelevance(ranked)

		// Check top 5
		top5 := ranked
		if len(ranked) > 5 {
			top5 = ranked[:5]
		}

		foundSyntaxError := false
		for i, r := range top5 {
			data, _ := os.ReadFile(r.Path)
			category := extractErrorCategory(data)
			if category == "syntax_error" {
				foundSyntaxError = true
				t.Logf("  Position %d: syntax_error reflection (score: %.1f)", i+1, r.Relevance)
			}
		}

		if !foundSyntaxError {
			t.Error("Syntax error reflection not in top 5 during debugging")
		} else {
			t.Log("Syntax error reflection successfully retrieved during debugging")
		}
	})

	// Scenario: Developer encounters permission denied
	t.Run("scenario_permission_denied", func(t *testing.T) {
		query := "permission denied error when trying to read config file"

		candidates := ecphory.fastFilter(nil, "")
		ranked := make([]RankingResult, 0, len(candidates))
		for _, path := range candidates {
			ranked = append(ranked, RankingResult{Path: path, Relevance: 50.0})
		}

		ecphory.applyFailureBoosting(query, ranked)
		sortByRelevance(ranked)

		top5 := ranked
		if len(ranked) > 5 {
			top5 = ranked[:5]
		}

		foundPermissionDenied := false
		for i, r := range top5 {
			data, _ := os.ReadFile(r.Path)
			category := extractErrorCategory(data)
			if category == "permission_denied" {
				foundPermissionDenied = true
				t.Logf("  Position %d: permission_denied reflection (score: %.1f)", i+1, r.Relevance)
			}
		}

		if !foundPermissionDenied {
			t.Error("Permission denied reflection not in top 5 during debugging")
		} else {
			t.Log("Permission denied reflection successfully retrieved during debugging")
		}
	})

	t.Log("Simulated debugging scenarios validated")
}

// Helper function to sort by relevance (descending)
func sortByRelevance(ranked []RankingResult) {
	// Simple bubble sort (fine for small test datasets)
	for i := 0; i < len(ranked); i++ {
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].Relevance > ranked[i].Relevance {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}
}

// TestValidateEcphoryBoosting_NormalQueries validates no boosting for normal queries
func TestValidateEcphoryBoosting_NormalQueries(t *testing.T) {
	reflectionsDir := setupReflectionFixtures(t)

	detector := NewContextDetector()
	ecphory := &Ecphory{
		contextDetector: detector,
	}

	normalQueries := []string{
		"how to write a for loop in Go",
		"best practices for error handling",
		"what is dependency injection",
	}

	for _, query := range normalQueries {
		t.Run(query, func(t *testing.T) {
			ranked := []RankingResult{
				{Path: filepath.Join(reflectionsDir, "syntax-error-missing-bracket.ai.md"), Relevance: 50.0},
				{Path: filepath.Join(reflectionsDir, "timeout-database-query.ai.md"), Relevance: 50.0},
			}

			ecphory.applyFailureBoosting(query, ranked)

			// Verify no boosting occurred
			for i, r := range ranked {
				if r.Relevance != 50.0 {
					t.Errorf("Normal query boosted reflection %d: score %.1f, expected 50.0", i, r.Relevance)
				}
			}

			t.Logf("No boosting for normal query: %q", query)
		})
	}

	t.Log("Normal queries correctly ignored (no boosting)")
}

// TestValidateEcphoryBoosting_BoostEffectiveness measures boosting effectiveness
func TestValidateEcphoryBoosting_BoostEffectiveness(t *testing.T) {
	reflectionsDir := setupReflectionFixtures(t)

	detector := NewContextDetector()
	ecphory := &Ecphory{
		contextDetector: detector,
	}

	// Measure effectiveness: How often does boosting move target reflection into top 5?
	queries := []struct {
		query    string
		category string
	}{
		{"syntax error in code", "syntax_error"},
		{"permission denied", "permission_denied"},
		{"timeout error", "timeout"},
		{"wrong tool used", "tool_misuse"},
		{"nil pointer error", "other"},
	}

	successCount := 0
	totalQueries := len(queries)

	for _, q := range queries {
		// Build all reflections
		files, _ := filepath.Glob(filepath.Join(reflectionsDir, "*.ai.md"))
		ranked := make([]RankingResult, 0, len(files))
		for _, file := range files {
			ranked = append(ranked, RankingResult{Path: file, Relevance: 50.0})
		}

		// Apply boosting
		ecphory.applyFailureBoosting(q.query, ranked)
		sortByRelevance(ranked)

		// Check if target category in top 5
		top5 := ranked
		if len(ranked) > 5 {
			top5 = ranked[:5]
		}

		foundInTop5 := false
		for _, r := range top5 {
			data, _ := os.ReadFile(r.Path)
			category := extractErrorCategory(data)
			if category == q.category {
				foundInTop5 = true
				break
			}
		}

		if foundInTop5 {
			successCount++
		}
	}

	effectiveness := float64(successCount) / float64(totalQueries) * 100.0

	t.Logf("Boosting effectiveness: %.1f%% (%d/%d queries)", effectiveness, successCount, totalQueries)

	if effectiveness < 80.0 {
		t.Errorf("Boosting effectiveness too low: %.1f%% (expected >= 80%%)", effectiveness)
	} else {
		t.Logf("Boosting effectiveness: %.1f%% (meets 80%% threshold)", effectiveness)
	}
}
