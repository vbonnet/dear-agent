package scope

import (
	"strings"
	"testing"
	"time"
)

func TestParser_Parse(t *testing.T) {
	parser := NewParser()

	t.Run("parses simple markdown headings", func(t *testing.T) {
		markdown := `
# Heading 1
## Heading 2
### Heading 3
`
		sections := parser.Parse(markdown)

		if len(sections) != 3 {
			t.Errorf("expected 3 sections, got %d", len(sections))
		}

		expected := []struct {
			heading string
			level   int
		}{
			{"Heading 1", 1},
			{"Heading 2", 2},
			{"Heading 3", 3},
		}

		for i, exp := range expected {
			if sections[i].Heading != exp.heading {
				t.Errorf("section %d: expected heading %q, got %q", i, exp.heading, sections[i].Heading)
			}
			if sections[i].Level != exp.level {
				t.Errorf("section %d: expected level %d, got %d", i, exp.level, sections[i].Level)
			}
		}
	})

	t.Run("provides accurate line numbers", func(t *testing.T) {
		markdown := `Line 1
Line 2
# Heading at line 3
Line 4
## Heading at line 5
`
		sections := parser.Parse(markdown)

		if len(sections) != 2 {
			t.Errorf("expected 2 sections, got %d", len(sections))
		}

		if sections[0].StartLine != 3 {
			t.Errorf("expected line 3, got %d", sections[0].StartLine)
		}
		if sections[1].StartLine != 5 {
			t.Errorf("expected line 5, got %d", sections[1].StartLine)
		}
	})

	t.Run("handles empty document", func(t *testing.T) {
		sections := parser.Parse("")
		if len(sections) != 0 {
			t.Errorf("expected 0 sections, got %d", len(sections))
		}
	})

	t.Run("handles document with no headings", func(t *testing.T) {
		markdown := `
This is just text.
No headings here.
`
		sections := parser.Parse(markdown)
		if len(sections) != 0 {
			t.Errorf("expected 0 sections, got %d", len(sections))
		}
	})

	t.Run("handles headings with inline formatting", func(t *testing.T) {
		markdown := `
# Heading with **bold** text
## Heading with *italic* text
### Heading with ` + "`code`" + ` text
#### Heading with [link](url)
`
		sections := parser.Parse(markdown)

		expected := []string{
			"Heading with bold text",
			"Heading with italic text",
			"Heading with text",
			"Heading with link",
		}

		for i, exp := range expected {
			if sections[i].Heading != exp {
				t.Errorf("section %d: expected %q, got %q", i, exp, sections[i].Heading)
			}
		}
	})

	t.Run("handles headings with special characters", func(t *testing.T) {
		markdown := `
# Heading with symbols: @#$%^&*()
## Heading with "quotes" and 'apostrophes'
`
		sections := parser.Parse(markdown)

		if sections[0].Heading != "Heading with symbols: @#$%^&*()" {
			t.Errorf("unexpected heading: %q", sections[0].Heading)
		}
		if sections[1].Heading != "Heading with \"quotes\" and 'apostrophes'" {
			t.Errorf("unexpected heading: %q", sections[1].Heading)
		}
	})

	t.Run("handles multiple headings with same level", func(t *testing.T) {
		markdown := `
## Section 1
Content
## Section 2
Content
## Section 3
`
		sections := parser.Parse(markdown)

		if len(sections) != 3 {
			t.Errorf("expected 3 sections, got %d", len(sections))
		}

		for _, section := range sections {
			if section.Level != 2 {
				t.Errorf("expected level 2, got %d", section.Level)
			}
		}
	})

	t.Run("handles deeply nested headings", func(t *testing.T) {
		markdown := `
# Level 1
## Level 2
### Level 3
#### Level 4
##### Level 5
###### Level 6
`
		sections := parser.Parse(markdown)

		if len(sections) != 6 {
			t.Errorf("expected 6 sections, got %d", len(sections))
		}

		for i, section := range sections {
			expectedLevel := i + 1
			if section.Level != expectedLevel {
				t.Errorf("section %d: expected level %d, got %d", i, expectedLevel, section.Level)
			}
		}
	})

	t.Run("handles headings with trailing whitespace", func(t *testing.T) {
		markdown := "# Heading with spaces   \n## Heading with tabs\t\t\n"
		sections := parser.Parse(markdown)

		if sections[0].Heading != "Heading with spaces" {
			t.Errorf("expected trimmed heading, got %q", sections[0].Heading)
		}
		if sections[1].Heading != "Heading with tabs" {
			t.Errorf("expected trimmed heading, got %q", sections[1].Heading)
		}
	})

	t.Run("handles ATX style with closing hashes", func(t *testing.T) {
		markdown := `
# ATX Style Heading
## ATX With Closing ##
### ATX No Closing
`
		sections := parser.Parse(markdown)

		if len(sections) != 3 {
			t.Errorf("expected 3 sections, got %d", len(sections))
		}

		if sections[1].Heading != "ATX With Closing" {
			t.Errorf("expected 'ATX With Closing', got %q", sections[1].Heading)
		}
	})
}

func TestParser_FuzzyMatch(t *testing.T) {
	parser := NewParser()

	t.Run("matches exact strings", func(t *testing.T) {
		if !parser.FuzzyMatch("Acceptance Criteria", "Acceptance Criteria", 0.75) {
			t.Error("exact match should return true")
		}
	})

	t.Run("matches case-insensitive", func(t *testing.T) {
		if !parser.FuzzyMatch("acceptance criteria", "ACCEPTANCE CRITERIA", 0.75) {
			t.Error("case-insensitive match should return true")
		}
	})

	t.Run("matches with whitespace differences", func(t *testing.T) {
		if !parser.FuzzyMatch("Acceptance  Criteria", "Acceptance Criteria", 0.75) {
			t.Error("whitespace variation should match")
		}
	})

	t.Run("matches close variations (75% threshold)", func(t *testing.T) {
		if !parser.FuzzyMatch("Accept Criteria", "Acceptance Criteria", 0.75) {
			t.Error("similar strings should match at 75% threshold")
		}
	})

	t.Run("matches with typos", func(t *testing.T) {
		if !parser.FuzzyMatch("Acceptence Criteria", "Acceptance Criteria", 0.80) {
			t.Error("typo should match at reasonable threshold")
		}
	})

	t.Run("rejects very different strings", func(t *testing.T) {
		if parser.FuzzyMatch("Task Breakdown", "Acceptance Criteria", 0.80) {
			t.Error("very different strings should not match")
		}
	})

	t.Run("handles different thresholds", func(t *testing.T) {
		heading := "Accept Criteria"
		pattern := "Acceptance Criteria"

		if !parser.FuzzyMatch(heading, pattern, 0.70) {
			t.Error("should match at 0.70 threshold")
		}
		if !parser.FuzzyMatch(heading, pattern, 0.75) {
			t.Error("should match at 0.75 threshold")
		}
		if parser.FuzzyMatch(heading, pattern, 0.80) {
			t.Error("should not match at 0.80 threshold (79% similarity)")
		}
	})

	t.Run("matches plurals", func(t *testing.T) {
		if !parser.FuzzyMatch("Tasks", "Task", 0.80) {
			t.Error("plural should match singular")
		}
	})

	t.Run("handles empty strings", func(t *testing.T) {
		if !parser.FuzzyMatch("", "", 0.80) {
			t.Error("empty strings should match")
		}
		if parser.FuzzyMatch("Heading", "", 0.80) {
			t.Error("non-empty should not match empty")
		}
		if parser.FuzzyMatch("", "Heading", 0.80) {
			t.Error("empty should not match non-empty")
		}
	})

	t.Run("handles very long strings", func(t *testing.T) {
		long1 := strings.Repeat("A", 1000)
		long2 := strings.Repeat("A", 1000)
		if !parser.FuzzyMatch(long1, long2, 0.80) {
			t.Error("identical long strings should match")
		}
	})
}

func TestParser_FindSections(t *testing.T) {
	parser := NewParser()

	sections := []Section{
		{Heading: "Introduction", Level: 1, StartLine: 1},
		{Heading: "Acceptance Criteria", Level: 2, StartLine: 10},
		{Heading: "Conclusion", Level: 1, StartLine: 20},
	}

	t.Run("finds exact match sections", func(t *testing.T) {
		results := parser.FindSections(sections, "Acceptance Criteria", false)

		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
		}
		if results[0].Heading != "Acceptance Criteria" {
			t.Errorf("unexpected heading: %q", results[0].Heading)
		}
	})

	t.Run("finds fuzzy match sections", func(t *testing.T) {
		fuzzySections := []Section{
			{Heading: "Accept Criteria", Level: 2, StartLine: 10},
			{Heading: "Task Breakdown", Level: 2, StartLine: 20},
		}

		results := parser.FindSections(fuzzySections, "Acceptance Criteria", true)

		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
		}
		if results[0].Heading != "Accept Criteria" {
			t.Errorf("unexpected heading: %q", results[0].Heading)
		}
	})

	t.Run("finds multiple fuzzy matches", func(t *testing.T) {
		multiSections := []Section{
			{Heading: "Requirements", Level: 2, StartLine: 10},
			{Heading: "Requirement", Level: 3, StartLine: 15},
			{Heading: "Reqs", Level: 3, StartLine: 20},
		}

		results := parser.FindSections(multiSections, "Requirements", true)

		if len(results) < 2 {
			t.Errorf("expected at least 2 results, got %d", len(results))
		}
	})

	t.Run("returns empty array when no matches", func(t *testing.T) {
		results := parser.FindSections(sections, "Nonexistent", false)

		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("distinguishes exact vs fuzzy matching", func(t *testing.T) {
		fuzzySections := []Section{
			{Heading: "Accept Criteria", Level: 2, StartLine: 10},
		}

		exactResults := parser.FindSections(fuzzySections, "Acceptance Criteria", false)
		fuzzyResults := parser.FindSections(fuzzySections, "Acceptance Criteria", true)

		if len(exactResults) != 0 {
			t.Error("exact match should find nothing")
		}
		if len(fuzzyResults) != 1 {
			t.Error("fuzzy match should find one result")
		}
	})
}

func TestParser_Performance(t *testing.T) {
	parser := NewParser()

	t.Run("parses large document quickly", func(t *testing.T) {
		// Generate document with 1000 headings
		var lines []string
		for i := 0; i < 1000; i++ {
			lines = append(lines, "## Heading "+string(rune(i)))
			lines = append(lines, "Content here")
		}
		markdown := strings.Join(lines, "\n")

		start := time.Now()
		sections := parser.Parse(markdown)
		duration := time.Since(start)

		if len(sections) != 1000 {
			t.Errorf("expected 1000 sections, got %d", len(sections))
		}

		// Threshold set to 500ms to account for CI variability and race detector overhead
		if duration > 500*time.Millisecond {
			t.Errorf("parsing too slow: %v (expected <500ms)", duration)
		}
	})

	t.Run("fuzzy matching is fast", func(t *testing.T) {
		heading := "Acceptance Criteria for Testing"
		pattern := "Accept Criteria for Test"

		start := time.Now()
		for i := 0; i < 10000; i++ {
			parser.FuzzyMatch(heading, pattern, 0.80)
		}
		duration := time.Since(start)

		if duration > 1*time.Second {
			t.Errorf("fuzzy matching too slow: %v (expected <1s for 10k iterations)", duration)
		}
	})
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		s1       string
		s2       string
		expected int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "adc", 1},
		{"kitten", "sitting", 3},
		{"saturday", "sunday", 3},
		{"Accept Criteria", "Acceptance Criteria", 4},
	}

	for _, tt := range tests {
		t.Run(tt.s1+"_vs_"+tt.s2, func(t *testing.T) {
			result := levenshteinDistance(tt.s1, tt.s2)
			if result != tt.expected {
				t.Errorf("levenshteinDistance(%q, %q) = %d, want %d",
					tt.s1, tt.s2, result, tt.expected)
			}
		})
	}
}
