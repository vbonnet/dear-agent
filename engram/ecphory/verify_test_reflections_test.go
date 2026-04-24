package ecphory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/engram"
)

// TestVerifyTestReflections verifies the 10 sample reflections are valid
func TestVerifyTestReflections(t *testing.T) {
	reflectionsDir := setupReflectionFixtures(t)

	files, err := filepath.Glob(filepath.Join(reflectionsDir, "*.ai.md"))
	if err != nil {
		t.Fatalf("Failed to glob reflections: %v", err)
	}

	if len(files) != 10 {
		t.Fatalf("Expected 10 reflection files, got %d", len(files))
	}

	parser := engram.NewParser()
	categories := make(map[string]int)

	for _, file := range files {
		// Test parsing
		eg, err := parser.Parse(file)
		if err != nil {
			t.Errorf("Failed to parse %s: %v", filepath.Base(file), err)
			continue
		}

		// Test category extraction
		data, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("Failed to read %s: %v", filepath.Base(file), err)
			continue
		}

		category := extractErrorCategory(data)
		if category == "" {
			t.Errorf("No error_category in %s", filepath.Base(file))
			continue
		}

		// Verify it's one of the 5 valid categories
		validCategories := map[string]bool{
			"syntax_error":      true,
			"permission_denied": true,
			"timeout":           true,
			"tool_misuse":       true,
			"other":             true,
		}

		if !validCategories[category] {
			t.Errorf("Invalid category %q in %s", category, filepath.Base(file))
			continue
		}

		categories[category]++
		t.Logf("  %s: %s (title: %s)", filepath.Base(file), category, eg.Frontmatter.Title)
	}

	// Verify distribution
	if len(categories) != 5 {
		t.Errorf("Expected 5 categories, got %d: %v", len(categories), categories)
	}

	for cat, count := range categories {
		if count != 2 {
			t.Errorf("Expected 2 reflections per category, %s has %d", cat, count)
		}
	}

	t.Logf("All reflections valid: %d files, %d categories, 2 per category", len(files), len(categories))
}
