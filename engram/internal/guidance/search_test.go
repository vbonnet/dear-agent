//go:build integration

package guidance

import (
	"path/filepath"
	"testing"
)

func TestSearch_BasicMatch(t *testing.T) {
	testDataPath, err := filepath.Abs("../../testdata/guidance")
	if err != nil {
		t.Fatalf("Failed to get testdata path: %v", err)
	}

	svc := NewSearchService(testDataPath)
	opts := SearchOptions{
		Query: "encryption",
		Limit: 10,
	}

	results, err := svc.Search(opts)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Expected at least one result for 'encryption'")
	}

	// Verify the encryption file was found
	found := false
	for _, result := range results {
		if result.Path == "test-encryption.ai.md" {
			found = true
			if result.Title != "HIPAA-Compliant Encryption Patterns" {
				t.Errorf("Expected title 'HIPAA-Compliant Encryption Patterns', got '%s'", result.Title)
			}
			if result.Score == 0 {
				t.Error("Expected non-zero score for encryption match")
			}
		}
	}

	if !found {
		t.Error("Expected to find test-encryption.ai.md in results")
	}
}

func TestSearch_DomainFilter(t *testing.T) {
	testDataPath, err := filepath.Abs("../../testdata/guidance")
	if err != nil {
		t.Fatalf("Failed to get testdata path: %v", err)
	}

	svc := NewSearchService(testDataPath)
	opts := SearchOptions{
		Query:  "patterns",
		Domain: "go",
		Limit:  10,
	}

	results, err := svc.Search(opts)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Should only return Go domain files
	for _, result := range results {
		if result.Domain != "go" {
			t.Errorf("Expected domain 'go', got '%s' for file %s", result.Domain, result.Path)
		}
	}

	// Should find the Go error handling file
	found := false
	for _, result := range results {
		if result.Path == "test-errors.ai.md" {
			found = true
		}
	}
	if !found {
		t.Error("Expected to find test-errors.ai.md with domain filter 'go'")
	}
}

func TestSearch_TypeFilter(t *testing.T) {
	testDataPath, err := filepath.Abs("../../testdata/guidance")
	if err != nil {
		t.Fatalf("Failed to get testdata path: %v", err)
	}

	svc := NewSearchService(testDataPath)
	opts := SearchOptions{
		Query: "testing",
		Type:  "workflow",
		Limit: 10,
	}

	results, err := svc.Search(opts)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Should only return workflow type files
	for _, result := range results {
		if result.Type != "workflow" {
			t.Errorf("Expected type 'workflow', got '%s' for file %s", result.Type, result.Path)
		}
	}
}

func TestSearch_TagFilter(t *testing.T) {
	testDataPath, err := filepath.Abs("../../testdata/guidance")
	if err != nil {
		t.Fatalf("Failed to get testdata path: %v", err)
	}

	svc := NewSearchService(testDataPath)
	opts := SearchOptions{
		Query: "test",
		Tag:   "security",
		Limit: 10,
	}

	results, err := svc.Search(opts)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Should only return files with security tag
	for _, result := range results {
		hasTag := false
		for _, tag := range result.Tags {
			if tag == "security" {
				hasTag = true
				break
			}
		}
		if !hasTag {
			t.Errorf("Expected 'security' tag for file %s, got tags: %v", result.Path, result.Tags)
		}
	}
}

func TestSearch_Limit(t *testing.T) {
	testDataPath, err := filepath.Abs("../../testdata/guidance")
	if err != nil {
		t.Fatalf("Failed to get testdata path: %v", err)
	}

	svc := NewSearchService(testDataPath)
	opts := SearchOptions{
		Query: "test",
		Limit: 2,
	}

	results, err := svc.Search(opts)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) > 2 {
		t.Errorf("Expected at most 2 results, got %d", len(results))
	}
}

func TestSearch_Sorting(t *testing.T) {
	testDataPath, err := filepath.Abs("../../testdata/guidance")
	if err != nil {
		t.Fatalf("Failed to get testdata path: %v", err)
	}

	svc := NewSearchService(testDataPath)
	opts := SearchOptions{
		Query: "patterns",
		Limit: 10,
	}

	results, err := svc.Search(opts)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Verify results are sorted by score (descending)
	for i := 0; i < len(results)-1; i++ {
		if results[i].Score < results[i+1].Score {
			t.Errorf("Results not sorted by score: result[%d] has score %d < result[%d] score %d",
				i, results[i].Score, i+1, results[i+1].Score)
		}
	}
}

func TestSearch_NoResults(t *testing.T) {
	testDataPath, err := filepath.Abs("../../testdata/guidance")
	if err != nil {
		t.Fatalf("Failed to get testdata path: %v", err)
	}

	svc := NewSearchService(testDataPath)
	opts := SearchOptions{
		Query: "nonexistent-topic-xyz",
		Limit: 10,
	}

	results, err := svc.Search(opts)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results for nonexistent query, got %d", len(results))
	}
}

func TestSearch_MalformedFrontmatter(t *testing.T) {
	testDataPath, err := filepath.Abs("../../testdata/guidance")
	if err != nil {
		t.Fatalf("Failed to get testdata path: %v", err)
	}

	svc := NewSearchService(testDataPath)
	opts := SearchOptions{
		Query: "malformed",
		Limit: 10,
	}

	// Should not crash, just skip the malformed file
	results, err := svc.Search(opts)
	if err != nil {
		t.Fatalf("Search should not fail on malformed file: %v", err)
	}

	// Malformed file should be skipped (not in results)
	for _, result := range results {
		if result.Path == "malformed.ai.md" {
			t.Error("Malformed file should have been skipped")
		}
	}
}

func TestParseFrontmatter(t *testing.T) {
	testFile := filepath.Join("../../testdata/guidance", "test-encryption.ai.md")

	fm, err := parseFrontmatter(testFile)
	if err != nil {
		t.Fatalf("Failed to parse frontmatter: %v", err)
	}

	if fm.Title != "HIPAA-Compliant Encryption Patterns" {
		t.Errorf("Expected title 'HIPAA-Compliant Encryption Patterns', got '%s'", fm.Title)
	}

	if fm.Description != "AES-256 encryption for healthcare data at rest" {
		t.Errorf("Unexpected description: %s", fm.Description)
	}

	if fm.Domain != "hipaa" {
		t.Errorf("Expected domain 'hipaa', got '%s'", fm.Domain)
	}

	if fm.Type != "pattern" {
		t.Errorf("Expected type 'pattern', got '%s'", fm.Type)
	}

	expectedTags := []string{"hipaa", "encryption", "security", "compliance"}
	if len(fm.Tags) != len(expectedTags) {
		t.Errorf("Expected %d tags, got %d", len(expectedTags), len(fm.Tags))
	}
}

func TestCalculateScore(t *testing.T) {
	tests := []struct {
		name     string
		fm       *Frontmatter
		query    string
		expected int
	}{
		{
			name: "title match",
			fm: &Frontmatter{
				Title:       "Encryption Patterns",
				Description: "AES-256 encryption for data at rest",
				Tags:        []string{"security"},
			},
			query:    "encryption",
			expected: 3 + 2, // title (3) + description (2)
		},
		{
			name: "description only",
			fm: &Frontmatter{
				Title:       "Security Guide",
				Description: "Encryption best practices",
				Tags:        []string{"security"},
			},
			query:    "encryption",
			expected: 2, // description (2)
		},
		{
			name: "tag only",
			fm: &Frontmatter{
				Title:       "Security Guide",
				Description: "Best practices",
				Tags:        []string{"encryption", "security"},
			},
			query:    "encryption",
			expected: 1, // tag (1)
		},
		{
			name: "no match",
			fm: &Frontmatter{
				Title:       "Testing Guide",
				Description: "How to test",
				Tags:        []string{"testing"},
			},
			query:    "encryption",
			expected: 0,
		},
		{
			name: "case insensitive",
			fm: &Frontmatter{
				Title:       "ENCRYPTION PATTERNS",
				Description: "AES-256 ENCRYPTION for data at rest",
				Tags:        []string{"SECURITY"},
			},
			query:    "encryption",
			expected: 3 + 2, // title (3) + description (2)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := calculateScore(tt.fm, tt.query)
			if score != tt.expected {
				t.Errorf("Expected score %d, got %d", tt.expected, score)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{
			name:     "exact match",
			slice:    []string{"go", "python", "rust"},
			item:     "go",
			expected: true,
		},
		{
			name:     "case insensitive match",
			slice:    []string{"Go", "Python", "Rust"},
			item:     "go",
			expected: true,
		},
		{
			name:     "no match",
			slice:    []string{"go", "python", "rust"},
			item:     "java",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			item:     "go",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.slice, tt.item)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
