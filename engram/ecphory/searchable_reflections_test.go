package ecphory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/engram/internal/reflection"
)

// TestReflectionsSearchable verifies test reflections are indexed and searchable
func TestReflectionsSearchable(t *testing.T) {
	reflectionsDir := setupReflectionFixtures(t)

	// Build index
	idx := NewIndex()
	if err := idx.Build(reflectionsDir); err != nil {
		t.Fatalf("Failed to build index: %v", err)
	}

	// Verify all reflections indexed
	allFiles := idx.All()
	if len(allFiles) != 10 {
		t.Errorf("Expected 10 indexed files, got %d", len(allFiles))
	}

	// Test filtering by tags (all reflections should have failure-related tags)
	goFiles := idx.FilterByTags([]string{"go"})
	if len(goFiles) < 1 {
		t.Logf("Note: Expected at least 1 reflection with 'go' tag, got %d", len(goFiles))
	}

	// Verify boosting logic works
	detector := NewContextDetector()

	testCases := []struct {
		query            string
		expectedCategory reflection.ErrorCategory
	}{
		{"syntax error in code", reflection.ErrorCategorySyntax},
		{"permission denied", reflection.ErrorCategoryPermissionDenied},
		{"request timeout", reflection.ErrorCategoryTimeout},
		{"wrong tool", reflection.ErrorCategoryToolMisuse},
		{"nil pointer error", reflection.ErrorCategoryOther},
	}

	for _, tc := range testCases {
		t.Run(tc.query, func(t *testing.T) {
			isDebugging, category := detector.DetectContext(tc.query)

			if !isDebugging {
				t.Errorf("Query %q not detected as debugging", tc.query)
			}

			if category != tc.expectedCategory {
				t.Errorf("Query %q: category = %q, want %q", tc.query, string(category), string(tc.expectedCategory))
			}

			// Simulate boosting
			ranked := []RankingResult{
				{Path: filepath.Join(reflectionsDir, "syntax-error-missing-bracket.ai.md"), Relevance: 50.0},
				{Path: filepath.Join(reflectionsDir, "permission-denied-file-access.ai.md"), Relevance: 50.0},
				{Path: filepath.Join(reflectionsDir, "timeout-database-query.ai.md"), Relevance: 50.0},
				{Path: filepath.Join(reflectionsDir, "tool-misuse-wrong-git-branch.ai.md"), Relevance: 50.0},
				{Path: filepath.Join(reflectionsDir, "other-error-nil-pointer.ai.md"), Relevance: 50.0},
			}

			// Apply boosting
			ecphory := &Ecphory{
				contextDetector: detector,
			}
			ecphory.applyFailureBoosting(tc.query, ranked)

			// Verify correct reflection was boosted
			foundBoosted := false
			for _, r := range ranked {
				if r.Relevance > 50.0 {
					data, err := os.ReadFile(r.Path)
					if err != nil {
						t.Errorf("Failed to read %s: %v", r.Path, err)
						continue
					}

					actualCategory := extractErrorCategory(data)
					if actualCategory == string(tc.expectedCategory) {
						foundBoosted = true
						t.Logf("  Correctly boosted %s from 50.0 to %.1f", filepath.Base(r.Path), r.Relevance)
					} else {
						t.Errorf("Boosted wrong reflection: expected %q, got %q", string(tc.expectedCategory), actualCategory)
					}
				}
			}

			if !foundBoosted {
				t.Errorf("No reflection boosted for query %q (category %q)", tc.query, tc.expectedCategory)
			}
		})
	}

	t.Logf("All reflections searchable and boostable")
}
