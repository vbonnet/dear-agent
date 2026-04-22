package errormemory

import (
	"strings"
	"testing"
	"time"
)

func TestConsolidateToEngrams(t *testing.T) {
	now := time.Now()

	records := []ErrorRecord{
		{
			ID:            "high1",
			Pattern:       "DANGEROUS_RM",
			ErrorCategory: "tool_misuse",
			CommandSample: "rm -rf /",
			Remediation:   "Use targeted rm with specific paths",
			Count:         15,
			FirstSeen:     now.Add(-48 * time.Hour),
			LastSeen:      now.Add(-1 * time.Hour),
		},
		{
			ID:            "low1",
			Pattern:       "MINOR_ISSUE",
			ErrorCategory: "syntax_error",
			CommandSample: "echo {bad",
			Remediation:   "Fix syntax",
			Count:         2,
			FirstSeen:     now.Add(-24 * time.Hour),
			LastSeen:      now.Add(-2 * time.Hour),
		},
		{
			ID:            "high2",
			Pattern:       "DOCKER_PRIVILEGED",
			ErrorCategory: "tool_misuse",
			CommandSample: "docker run --privileged",
			Remediation:   "Use --cap-add for specific capabilities",
			Count:         10,
			FirstSeen:     now.Add(-72 * time.Hour),
			LastSeen:      now.Add(-3 * time.Hour),
		},
	}

	// Filter with minCount=5 — should get 2 engrams
	engrams := ConsolidateToEngrams(records, 5)

	if len(engrams) != 2 {
		t.Fatalf("expected 2 engrams with minCount=5, got %d", len(engrams))
	}

	// Verify first engram fields
	e := engrams[0]
	if e.Title != "Error Pattern: DANGEROUS_RM" {
		t.Errorf("expected title 'Error Pattern: DANGEROUS_RM', got %q", e.Title)
	}
	if e.Description != "Use targeted rm with specific paths" {
		t.Errorf("expected description to match remediation, got %q", e.Description)
	}
	if e.ErrorCategory != "tool_misuse" {
		t.Errorf("expected error category 'tool_misuse', got %q", e.ErrorCategory)
	}
	if e.Count != 15 {
		t.Errorf("expected count 15, got %d", e.Count)
	}
	if len(e.Tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(e.Tags))
	}
	if e.LessonLearned != "Use targeted rm with specific paths" {
		t.Errorf("expected lesson learned to match remediation, got %q", e.LessonLearned)
	}
	if e.Content == "" {
		t.Error("expected non-empty content")
	}

	// Verify second engram
	e2 := engrams[1]
	if e2.Title != "Error Pattern: DOCKER_PRIVILEGED" {
		t.Errorf("expected second engram to be DOCKER_PRIVILEGED, got %q", e2.Title)
	}

	// Filter with minCount=20 — should get 0 engrams
	engrams = ConsolidateToEngrams(records, 20)
	if len(engrams) != 0 {
		t.Errorf("expected 0 engrams with minCount=20, got %d", len(engrams))
	}
}

func TestConsolidateEmpty(t *testing.T) {
	engrams := ConsolidateToEngrams(nil, 1)
	if len(engrams) != 0 {
		t.Errorf("expected 0 engrams for nil records, got %d", len(engrams))
	}

	engrams = ConsolidateToEngrams([]ErrorRecord{}, 1)
	if len(engrams) != 0 {
		t.Errorf("expected 0 engrams for empty records, got %d", len(engrams))
	}
}

func TestFormatEngramContent(t *testing.T) {
	now := time.Now()
	rec := ErrorRecord{
		Pattern:       "DANGEROUS_RM",
		ErrorCategory: "tool_misuse",
		CommandSample: "rm -rf /",
		Remediation:   "Use targeted rm with specific paths",
		Count:         50,
		FirstSeen:     now.Add(-48 * time.Hour),
		LastSeen:      now.Add(-1 * time.Hour),
	}

	content := formatEngramContent(rec)

	// Verify frontmatter
	if !strings.Contains(content, "---") {
		t.Error("expected frontmatter delimiters")
	}
	if !strings.Contains(content, `title: "Error Pattern: DANGEROUS_RM"`) {
		t.Error("expected title in frontmatter")
	}
	if !strings.Contains(content, "type: reflection") {
		t.Error("expected type in frontmatter")
	}
	if !strings.Contains(content, "tags: [error-memory, auto-generated, tool_misuse]") {
		t.Error("expected tags in frontmatter")
	}
	if !strings.Contains(content, "error_category: tool_misuse") {
		t.Error("expected error_category in frontmatter")
	}
	if !strings.Contains(content, "encoding_strength:") {
		t.Error("expected encoding_strength in frontmatter")
	}

	// Verify body sections
	if !strings.Contains(content, "# Error Pattern: DANGEROUS_RM") {
		t.Error("expected heading")
	}
	if !strings.Contains(content, "## Problem") {
		t.Error("expected Problem section")
	}
	if !strings.Contains(content, "## Solution") {
		t.Error("expected Solution section")
	}
	if !strings.Contains(content, "## Lesson Learned") {
		t.Error("expected Lesson Learned section")
	}
	if !strings.Contains(content, "**Occurrences**: 50 times") {
		t.Error("expected occurrence count")
	}
	if !strings.Contains(content, "`rm -rf /`") {
		t.Error("expected command sample")
	}
	if !strings.Contains(content, "Use targeted rm with specific paths") {
		t.Error("expected remediation text")
	}
}
