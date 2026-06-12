package guidance

import (
	"testing"
)

func TestCalculateScore_TitleMatch(t *testing.T) {
	fm := &Frontmatter{Title: "Go error handling"}
	score := calculateScore(fm, "error")
	if score < 3 {
		t.Errorf("calculateScore title match = %d, want >= 3", score)
	}
}

func TestCalculateScore_DescMatch(t *testing.T) {
	fm := &Frontmatter{Title: "Patterns", Description: "error handling best practices"}
	score := calculateScore(fm, "error")
	if score < 2 {
		t.Errorf("calculateScore desc match = %d, want >= 2", score)
	}
}

func TestCalculateScore_TagMatch(t *testing.T) {
	fm := &Frontmatter{Title: "Other", Tags: []string{"errors", "go"}}
	score := calculateScore(fm, "errors")
	if score < 1 {
		t.Errorf("calculateScore tag match = %d, want >= 1", score)
	}
}

func TestCalculateScore_NoMatch_ReturnsZero(t *testing.T) {
	fm := &Frontmatter{Title: "Testing", Description: "unit tests", Tags: []string{"testing"}}
	score := calculateScore(fm, "database")
	if score != 0 {
		t.Errorf("calculateScore no match = %d, want 0", score)
	}
}

func TestContains_Found(t *testing.T) {
	if !contains([]string{"foo", "bar", "baz"}, "bar") {
		t.Error("contains should find 'bar'")
	}
}

func TestContains_CaseInsensitive(t *testing.T) {
	if !contains([]string{"FOO", "Bar"}, "bar") {
		t.Error("contains should be case-insensitive")
	}
}

func TestContains_NotFound(t *testing.T) {
	if contains([]string{"a", "b"}, "c") {
		t.Error("contains should return false for missing element")
	}
}

func TestNewSearchService_NotNil(t *testing.T) {
	svc := NewSearchService("/some/path")
	if svc == nil {
		t.Fatal("NewSearchService returned nil")
	}
}
