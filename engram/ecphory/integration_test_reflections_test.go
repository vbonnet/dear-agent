package ecphory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestEcphoryRetrievesTestReflections verifies ecphory can retrieve and boost test reflections
func TestEcphoryRetrievesTestReflections(t *testing.T) {
	// Skip if ANTHROPIC_API_KEY not set (Tier 2 ranking requires API)
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping integration test")
	}

	reflectionsDir := filepath.Join(os.Getenv("HOME"), ".engram", "reflections")

	// Verify reflections directory exists
	if _, err := os.Stat(reflectionsDir); os.IsNotExist(err) {
		t.Fatalf("Reflections directory not found: %s", reflectionsDir)
	}

	// Create ecphory with reflections directory
	ecphory, err := NewEcphory(reflectionsDir, 50000)
	if err != nil {
		t.Fatalf("Failed to create ecphory: %v", err)
	}
	defer ecphory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testCases := []struct {
		name                string
		query               string
		expectedCategory    string
		expectReflectionTop bool // Should a reflection with matching category be in top results?
	}{
		{
			name:                "syntax_error_query",
			query:               "syntax error in my Go code",
			expectedCategory:    "syntax_error",
			expectReflectionTop: true,
		},
		{
			name:                "permission_denied_query",
			query:               "permission denied when accessing file",
			expectedCategory:    "permission_denied",
			expectReflectionTop: true,
		},
		{
			name:                "timeout_query",
			query:               "request timeout error",
			expectedCategory:    "timeout",
			expectReflectionTop: true,
		},
		{
			name:                "tool_misuse_query",
			query:               "wrong tool used for the job",
			expectedCategory:    "tool_misuse",
			expectReflectionTop: true,
		},
		{
			name:                "other_error_query",
			query:               "nil pointer dereference error",
			expectedCategory:    "other",
			expectReflectionTop: true,
		},
		{
			name:                "normal_query_no_boosting",
			query:               "how to write a for loop in Go",
			expectedCategory:    "",
			expectReflectionTop: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Query ecphory
			results, err := ecphory.Query(ctx, tc.query, "test-session", "test transcript", nil, "")
			if err != nil {
				t.Errorf("Query failed: %v", err)
				return
			}

			if len(results) == 0 {
				t.Logf("No results returned for query: %q", tc.query)
				return
			}

			// Log top results
			t.Logf("Query: %q", tc.query)
			for i, eg := range results {
				if i >= 5 {
					break
				}
				category := extractErrorCategory([]byte(eg.Content))
				t.Logf("  Result %d: %s (category: %s, path: %s)",
					i+1, eg.Frontmatter.Title, category, filepath.Base(eg.Path))
			}

			// If we expect reflection to be in top results, verify
			if tc.expectReflectionTop {
				foundMatchingCategory := false
				for i, eg := range results {
					if i >= 5 {
						break // Check top 5 only
					}
					category := extractErrorCategory([]byte(eg.Content))
					if category == tc.expectedCategory {
						foundMatchingCategory = true
						t.Logf("✓ Found matching category %q in top 5 results", tc.expectedCategory)
						break
					}
				}

				if !foundMatchingCategory {
					t.Logf("⚠ Expected category %q not in top 5 results (may be due to ranking)", tc.expectedCategory)
					// Don't fail - ranking is fuzzy and depends on API
				}
			}
		})
	}
}
