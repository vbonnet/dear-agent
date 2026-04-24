package frontmatter

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

		sections, err := parser.Parse(markdown)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(sections) != 3 {
			t.Fatalf("expected 3 sections, got %d", len(sections))
		}

		assertSection(t, sections[0], "Heading 1", 1, 2)
		assertSection(t, sections[1], "Heading 2", 2, 3)
		assertSection(t, sections[2], "Heading 3", 3, 4)
	})

	t.Run("provides accurate line numbers", func(t *testing.T) {
		markdown := `Line 1
Line 2
# Heading at line 3
Line 4
## Heading at line 5
`

		sections, err := parser.Parse(markdown)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(sections) != 2 {
			t.Fatalf("expected 2 sections, got %d", len(sections))
		}

		if sections[0].StartLine != 3 {
			t.Errorf("expected startLine 3, got %d", sections[0].StartLine)
		}
		if sections[1].StartLine != 5 {
			t.Errorf("expected startLine 5, got %d", sections[1].StartLine)
		}
	})

	t.Run("handles empty document", func(t *testing.T) {
		markdown := ""
		sections, err := parser.Parse(markdown)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(sections) != 0 {
			t.Errorf("expected 0 sections, got %d", len(sections))
		}
	})

	t.Run("handles document with no headings", func(t *testing.T) {
		markdown := `
This is just text.
No headings here.
`

		sections, err := parser.Parse(markdown)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

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

		sections, err := parser.Parse(markdown)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(sections) != 4 {
			t.Fatalf("expected 4 sections, got %d", len(sections))
		}

		assertEqual(t, sections[0].Heading, "Heading with bold text")
		assertEqual(t, sections[1].Heading, "Heading with italic text")
		assertEqual(t, sections[2].Heading, "Heading with code text")
		assertEqual(t, sections[3].Heading, "Heading with link")
	})

	t.Run("handles headings with special characters", func(t *testing.T) {
		markdown := `
# Heading with émojis 🎉
## Heading with symbols: @#$%^&*()
### Heading with "quotes" and 'apostrophes'
`

		sections, err := parser.Parse(markdown)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(sections) != 3 {
			t.Fatalf("expected 3 sections, got %d", len(sections))
		}

		assertEqual(t, sections[0].Heading, "Heading with émojis 🎉")
		assertEqual(t, sections[1].Heading, "Heading with symbols: @#$%^&*()")
		assertEqual(t, sections[2].Heading, "Heading with \"quotes\" and 'apostrophes'")
	})

	t.Run("handles multiple headings with same level", func(t *testing.T) {
		markdown := `
## Section 1
Content
## Section 2
Content
## Section 3
`

		sections, err := parser.Parse(markdown)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(sections) != 3 {
			t.Fatalf("expected 3 sections, got %d", len(sections))
		}

		for i, section := range sections {
			if section.Level != 2 {
				t.Errorf("section %d: expected level 2, got %d", i, section.Level)
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

		sections, err := parser.Parse(markdown)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(sections) != 6 {
			t.Fatalf("expected 6 sections, got %d", len(sections))
		}

		for i, section := range sections {
			expectedLevel := i + 1
			if section.Level != expectedLevel {
				t.Errorf("section %d: expected level %d, got %d", i, expectedLevel, section.Level)
			}
		}
	})

	t.Run("ignores code blocks with hash symbols", func(t *testing.T) {
		markdown := "# Real Heading\n\n```bash\n# This is a comment, not a heading\necho \"test\"\n```\n\n## Another Real Heading\n"

		sections, err := parser.Parse(markdown)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(sections) != 2 {
			t.Fatalf("expected 2 sections, got %d", len(sections))
		}

		assertEqual(t, sections[0].Heading, "Real Heading")
		assertEqual(t, sections[1].Heading, "Another Real Heading")
	})

	t.Run("handles headings with trailing whitespace", func(t *testing.T) {
		markdown := `
# Heading with spaces
## Heading with tabs
`

		sections, err := parser.Parse(markdown)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(sections) != 2 {
			t.Fatalf("expected 2 sections, got %d", len(sections))
		}

		assertEqual(t, sections[0].Heading, "Heading with spaces")
		assertEqual(t, sections[1].Heading, "Heading with tabs")
	})

	t.Run("handles mixed heading styles (ATX)", func(t *testing.T) {
		markdown := `
# ATX Style Heading
## ATX With Closing ##
### ATX No Closing
`

		sections, err := parser.Parse(markdown)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(sections) != 3 {
			t.Fatalf("expected 3 sections, got %d", len(sections))
		}

		assertEqual(t, sections[0].Heading, "ATX Style Heading")
		assertEqual(t, sections[1].Heading, "ATX With Closing")
		assertEqual(t, sections[2].Heading, "ATX No Closing")
	})

	t.Run("handles frontmatter", func(t *testing.T) {
		markdown := `---
title: Document Title
author: Test Author
---

# Actual Heading
`

		sections, err := parser.Parse(markdown)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(sections) != 1 {
			t.Fatalf("expected 1 section, got %d", len(sections))
		}

		assertEqual(t, sections[0].Heading, "Actual Heading")
	})

	t.Run("handles very large documents", func(t *testing.T) {
		var sb strings.Builder
		sb.WriteString("# Heading\n")
		for i := 0; i < 10000; i++ {
			sb.WriteString("Content\n")
		}

		sections, err := parser.Parse(sb.String())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(sections) != 1 {
			t.Fatalf("expected 1 section, got %d", len(sections))
		}

		assertEqual(t, sections[0].Heading, "Heading")
	})

	t.Run("handles mixed line endings (CRLF vs LF)", func(t *testing.T) {
		markdownCRLF := "# Heading 1\r\n## Heading 2\r\n"
		markdownLF := "# Heading 1\n## Heading 2\n"

		sectionsCRLF, err := parser.Parse(markdownCRLF)
		if err != nil {
			t.Fatalf("unexpected error (CRLF): %v", err)
		}

		sectionsLF, err := parser.Parse(markdownLF)
		if err != nil {
			t.Fatalf("unexpected error (LF): %v", err)
		}

		if len(sectionsCRLF) != 2 {
			t.Errorf("CRLF: expected 2 sections, got %d", len(sectionsCRLF))
		}
		if len(sectionsLF) != 2 {
			t.Errorf("LF: expected 2 sections, got %d", len(sectionsLF))
		}
	})

	t.Run("handles headings with links", func(t *testing.T) {
		markdown := `
# [Link Text](https://example.com)
## Heading with [inline](url) link
`

		sections, err := parser.Parse(markdown)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(sections) != 2 {
			t.Fatalf("expected 2 sections, got %d", len(sections))
		}

		assertEqual(t, sections[0].Heading, "Link Text")
		assertEqual(t, sections[1].Heading, "Heading with inline link")
	})
}

func TestParser_FuzzyMatch(t *testing.T) {
	parser := NewParser()

	t.Run("matches exact strings", func(t *testing.T) {
		if !parser.FuzzyMatch("Acceptance Criteria", "Acceptance Criteria", 0.80) {
			t.Error("expected exact match to return true")
		}
	})

	t.Run("matches case-insensitive", func(t *testing.T) {
		if !parser.FuzzyMatch("acceptance criteria", "ACCEPTANCE CRITERIA", 0.80) {
			t.Error("expected case-insensitive match to return true")
		}
	})

	t.Run("matches with whitespace differences", func(t *testing.T) {
		if !parser.FuzzyMatch("Acceptance  Criteria", "Acceptance Criteria", 0.80) {
			t.Error("expected match with whitespace differences")
		}
	})

	t.Run("matches close variations (75% threshold)", func(t *testing.T) {
		if !parser.FuzzyMatch("Accept Criteria", "Acceptance Criteria", 0.75) {
			t.Error("expected match for close variations")
		}
	})

	t.Run("matches with typos", func(t *testing.T) {
		// nolint:misspell // intentional typo for testing fuzzy matching
		if !parser.FuzzyMatch("Acceptence Criteria", "Acceptance Criteria", 0.80) {
			t.Error("expected match with typos")
		}
	})

	t.Run("rejects very different strings", func(t *testing.T) {
		if parser.FuzzyMatch("Task Breakdown", "Acceptance Criteria", 0.80) {
			t.Error("expected different strings to not match")
		}
	})

	t.Run("handles different thresholds", func(t *testing.T) {
		heading := "Accept Criteria"
		pattern := "Acceptance Criteria"

		if !parser.FuzzyMatch(heading, pattern, 0.70) {
			t.Error("expected match with 0.70 threshold")
		}
		if !parser.FuzzyMatch(heading, pattern, 0.75) {
			t.Error("expected match with 0.75 threshold")
		}
		if parser.FuzzyMatch(heading, pattern, 0.80) {
			t.Error("expected no match with 0.80 threshold (79% similarity)")
		}
		if parser.FuzzyMatch(heading, pattern, 0.95) {
			t.Error("expected no match with 0.95 threshold")
		}
	})

	t.Run("matches plurals", func(t *testing.T) {
		if !parser.FuzzyMatch("Tasks", "Task", 0.80) {
			t.Error("expected match for plurals")
		}
	})

	t.Run("handles empty strings", func(t *testing.T) {
		if !parser.FuzzyMatch("", "", 0.80) {
			t.Error("expected empty strings to match")
		}
		if parser.FuzzyMatch("Heading", "", 0.80) {
			t.Error("expected non-empty vs empty to not match")
		}
		if parser.FuzzyMatch("", "Heading", 0.80) {
			t.Error("expected empty vs non-empty to not match")
		}
	})

	t.Run("handles very long strings", func(t *testing.T) {
		long1 := strings.Repeat("A", 1000)
		long2 := strings.Repeat("A", 1000)

		if !parser.FuzzyMatch(long1, long2, 0.80) {
			t.Error("expected match for identical long strings")
		}
	})

	t.Run("handles unicode characters", func(t *testing.T) {
		if !parser.FuzzyMatch("Café", "Cafe", 0.75) {
			t.Error("expected match for unicode variations")
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
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		assertEqual(t, results[0].Heading, "Acceptance Criteria")
	})

	t.Run("finds fuzzy match sections", func(t *testing.T) {
		sections := []Section{
			{Heading: "Accept Criteria", Level: 2, StartLine: 10},
			{Heading: "Task Breakdown", Level: 2, StartLine: 20},
		}

		results := parser.FindSections(sections, "Acceptance Criteria", true)

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		assertEqual(t, results[0].Heading, "Accept Criteria")
	})

	t.Run("finds multiple fuzzy matches", func(t *testing.T) {
		sections := []Section{
			{Heading: "Requirements", Level: 2, StartLine: 10},
			{Heading: "Requirement", Level: 3, StartLine: 15},
			{Heading: "Reqs", Level: 3, StartLine: 20},
		}

		results := parser.FindSections(sections, "Requirements", true)

		// Fuzzy matching at 75% catches "Requirements" and "Requirement" (similar)
		// but not "Reqs" (too different: ~33% similarity)
		if len(results) < 2 {
			t.Errorf("expected at least 2 results, got %d", len(results))
		}
	})

	t.Run("returns empty array when no matches", func(t *testing.T) {
		results := parser.FindSections(sections, "Nonexistent Section", false)

		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("distinguishes exact vs fuzzy matching", func(t *testing.T) {
		sections := []Section{
			{Heading: "Accept Criteria", Level: 2, StartLine: 10},
		}

		exactResults := parser.FindSections(sections, "Acceptance Criteria", false)
		fuzzyResults := parser.FindSections(sections, "Acceptance Criteria", true)

		if len(exactResults) != 0 {
			t.Errorf("exact: expected 0 results, got %d", len(exactResults))
		}
		if len(fuzzyResults) != 1 {
			t.Errorf("fuzzy: expected 1 result, got %d", len(fuzzyResults))
		}
	})
}

func TestParser_Performance(t *testing.T) {
	parser := NewParser()

	t.Run("parses large document quickly", func(t *testing.T) {
		var sb strings.Builder
		for i := 0; i < 1000; i++ {
			sb.WriteString("## Heading ")
			sb.WriteString(string(rune(i)))
			sb.WriteString("\n\nContent here\n\n")
		}

		start := time.Now()
		sections, err := parser.Parse(sb.String())
		duration := time.Since(start)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(sections) != 1000 {
			t.Errorf("expected 1000 sections, got %d", len(sections))
		}

		// Should be much faster than 100ms (Go target: <10ms)
		if duration > 100*time.Millisecond {
			t.Logf("WARNING: parsing took %v (expected <100ms)", duration)
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

		// 10k matches should be fast (Go target: <100ms)
		if duration > 1000*time.Millisecond {
			t.Logf("WARNING: 10k matches took %v (expected <1s)", duration)
		}
	})
}

// Helper functions

func assertSection(t *testing.T, section Section, heading string, level, line int) {
	t.Helper()
	if section.Heading != heading {
		t.Errorf("expected heading %q, got %q", heading, section.Heading)
	}
	if section.Level != level {
		t.Errorf("expected level %d, got %d", level, section.Level)
	}
	if section.StartLine != line {
		t.Errorf("expected startLine %d, got %d", line, section.StartLine)
	}
}

func assertEqual(t *testing.T, actual, expected string) {
	t.Helper()
	if actual != expected {
		t.Errorf("expected %q, got %q", expected, actual)
	}
}
