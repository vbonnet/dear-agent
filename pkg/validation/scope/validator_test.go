package scope

import (
	"strings"
	"testing"
	"time"
)

func TestValidator_Validate_AntiPatterns(t *testing.T) {
	parser := NewParser()
	validator := NewValidator(parser)

	t.Run("detects SPEC content in DESIGN document (Acceptance Criteria)", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Decision Matrix

| Criteria | Approach A | Approach B |
|----------|------------|------------|
| Cost     | Low        | High       |

## Chosen Approach

We chose approach A.

## Risk Assessment

Minimal risk.

## Acceptance Criteria

- Users can do X
- System supports Y
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		if result.Passed {
			t.Error("validation should fail with anti-pattern")
		}
		// Should report anti-pattern error
		foundAntiPattern := false
		for _, err := range result.Errors {
			if err.Type == ViolationAntiPattern && err.Section == "Acceptance Criteria" {
				foundAntiPattern = true
				if err.BelongsIn != PhaseSpec {
					t.Errorf("expected belongs in SPEC, got %s", err.BelongsIn)
				}
			}
		}
		if !foundAntiPattern {
			t.Error("expected anti-pattern violation for 'Acceptance Criteria'")
		}
	})

	t.Run("detects SETUP content in DESIGN document (Task Breakdown)", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Summary

We chose approach A.

## Task Breakdown

**Phase 1**: Setup (2 hours)
- Task 1.1: Install dependencies
- Task 1.2: Configure environment
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		if result.Passed {
			t.Error("validation should fail with anti-pattern")
		}
		if len(result.Errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(result.Errors))
		}
		if result.Errors[0].BelongsIn != PhaseSetup {
			t.Errorf("expected belongs in SETUP, got %s", result.Errors[0].BelongsIn)
		}
	})

	t.Run("detects multiple anti-patterns in same document", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Acceptance Criteria

Criteria here

## Requirements

Requirements here

## Task Breakdown

Tasks here
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		if result.Passed {
			t.Error("validation should fail with multiple anti-patterns")
		}
		if len(result.Errors) < 2 {
			t.Errorf("expected at least 2 errors, got %d", len(result.Errors))
		}

		antiPatternCount := 0
		for _, err := range result.Errors {
			if err.Type == ViolationAntiPattern {
				antiPatternCount++
			}
		}
		if antiPatternCount < 2 {
			t.Errorf("expected at least 2 anti-pattern errors, got %d", antiPatternCount)
		}
	})

	t.Run("detects anti-pattern aliases (Accept Criteria → Acceptance Criteria)", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Decision Matrix

Comparison table

## Chosen Approach

Selected approach

## Risk Assessment

Low risk

## Accept Criteria

Criteria here
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		if result.Passed {
			t.Error("validation should fail with anti-pattern alias")
		}
		// Should report anti-pattern error for "Accept Criteria"
		foundAntiPattern := false
		for _, err := range result.Errors {
			if err.Type == ViolationAntiPattern && err.Section == "Accept Criteria" {
				foundAntiPattern = true
				if err.BelongsIn != PhaseSpec {
					t.Errorf("expected belongs in SPEC, got %s", err.BelongsIn)
				}
			}
		}
		if !foundAntiPattern {
			t.Error("expected anti-pattern violation for 'Accept Criteria'")
		}
	})

	t.Run("passes validation with no anti-patterns", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Summary

We chose approach A.

## Comparison

Approach A vs B vs C.

## Decision Rationale

Why we chose A.
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		if !result.Passed {
			t.Error("validation should pass with no anti-patterns")
		}
		if len(result.Errors) != 0 {
			t.Errorf("expected 0 errors, got %d", len(result.Errors))
		}
	})

	t.Run("detects SPEC content in PROBLEM document (Solution Proposal)", func(t *testing.T) {
		markdown := `
# PROBLEM: Problem Validation

## Problem Statement

Problem is X.

## Solution Proposal

We should build Y.
`
		result := validator.Validate(PhaseProblem, markdown, nil)

		if result.Passed {
			t.Error("validation should fail with anti-pattern")
		}
		if len(result.Errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(result.Errors))
		}
		if result.Errors[0].BelongsIn != PhaseSpec {
			t.Errorf("expected belongs in SPEC, got %s", result.Errors[0].BelongsIn)
		}
	})

	t.Run("provides line numbers for anti-patterns", func(t *testing.T) {
		markdown := `Line 1
Line 2
# DESIGN: Approach Decision

Line 5

## Acceptance Criteria

Criteria here
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		if len(result.Errors) == 0 {
			t.Fatal("expected at least 1 error")
		}
		if result.Errors[0].Line != 7 {
			t.Errorf("expected line 7, got %d", result.Errors[0].Line)
		}
	})
}

func TestValidator_Validate_RequiredSections(t *testing.T) {
	parser := NewParser()
	validator := NewValidator(parser)

	t.Run("warns when required sections are missing", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Summary

Just a summary, missing comparison.
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		if len(result.Warnings) == 0 {
			t.Error("expected warnings for missing sections")
		}

		missingSectionCount := 0
		for _, w := range result.Warnings {
			if w.Type == ViolationMissingSection {
				missingSectionCount++
			}
		}
		if missingSectionCount == 0 {
			t.Error("expected at least one missing section warning")
		}
	})

	t.Run("warnings do not fail validation", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Summary

Missing some required sections.
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		if !result.Passed {
			t.Error("warnings should not fail validation")
		}
	})
}

func TestValidator_Validate_Length(t *testing.T) {
	parser := NewParser()
	validator := NewValidator(parser)

	t.Run("warns when document is too short", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

Short document.
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		lengthWarningCount := 0
		for _, w := range result.Warnings {
			if w.Type == ViolationLength {
				lengthWarningCount++
			}
		}
		if lengthWarningCount == 0 {
			t.Error("expected length warning for short document")
		}
	})

	t.Run("warns when document is too long", func(t *testing.T) {
		longContent := strings.Repeat("Word ", 5000) // 5000 words
		markdown := `
# DESIGN: Approach Decision

` + longContent
		result := validator.Validate(PhaseDesign, markdown, nil)

		lengthWarningCount := 0
		for _, w := range result.Warnings {
			if w.Type == ViolationLength {
				lengthWarningCount++
			}
		}
		if lengthWarningCount == 0 {
			t.Error("expected length warning for long document")
		}
	})

	t.Run("passes when document length is within range", func(t *testing.T) {
		normalContent := strings.Repeat("Word ", 1500) // 1500 words (DESIGN range: 1000-2500)
		markdown := `
# DESIGN: Approach Decision

` + normalContent
		result := validator.Validate(PhaseDesign, markdown, nil)

		lengthWarningCount := 0
		for _, w := range result.Warnings {
			if w.Type == ViolationLength {
				lengthWarningCount++
			}
		}
		if lengthWarningCount != 0 {
			t.Error("expected no length warnings for normal-length document")
		}
	})

	t.Run("excludes code blocks from word count", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

Normal text here.

` + "```" + `javascript
// This code should not count toward word count
function test() {
  return "This is a lot of text in code";
}
` + "```" + `

More normal text.
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		if result.Metadata.WordCount >= 100 {
			t.Errorf("word count too high: %d (code should be excluded)", result.Metadata.WordCount)
		}
	})
}

func TestValidator_Validate_Options(t *testing.T) {
	parser := NewParser()
	validator := NewValidator(parser)

	t.Run("override option makes validation pass despite errors", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Decision Matrix

Comparison

## Chosen Approach

Selected

## Risk Assessment

Low risk

## Acceptance Criteria

This should fail normally.
`
		result := validator.Validate(PhaseDesign, markdown, &ValidationOptions{Override: true})

		if !result.Passed {
			t.Error("override should force validation to pass")
		}
		// Errors should still be reported
		if len(result.Errors) == 0 {
			t.Error("errors should still be reported even with override")
		}
		// Should include anti-pattern error for "Acceptance Criteria"
		foundAntiPattern := false
		for _, err := range result.Errors {
			if err.Type == ViolationAntiPattern && err.Section == "Acceptance Criteria" {
				foundAntiPattern = true
			}
		}
		if !foundAntiPattern {
			t.Error("expected anti-pattern error to be reported")
		}
	})

	t.Run("skip anti-patterns option", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Acceptance Criteria

This should normally fail.
`
		result := validator.Validate(PhaseDesign, markdown, &ValidationOptions{
			Skip: []string{"anti-patterns"},
		})

		if !result.Passed {
			t.Error("validation should pass when anti-patterns skipped")
		}
		if len(result.Errors) != 0 {
			t.Errorf("expected 0 errors, got %d", len(result.Errors))
		}
	})

	t.Run("skip required-sections option", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Summary

Just summary.
`
		result := validator.Validate(PhaseDesign, markdown, &ValidationOptions{
			Skip: []string{"required-sections"},
		})

		missingSectionCount := 0
		for _, w := range result.Warnings {
			if w.Type == ViolationMissingSection {
				missingSectionCount++
			}
		}
		if missingSectionCount != 0 {
			t.Error("missing section warnings should be skipped")
		}
	})

	t.Run("skip length option", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

Short.
`
		result := validator.Validate(PhaseDesign, markdown, &ValidationOptions{
			Skip: []string{"length"},
		})

		lengthWarningCount := 0
		for _, w := range result.Warnings {
			if w.Type == ViolationLength {
				lengthWarningCount++
			}
		}
		if lengthWarningCount != 0 {
			t.Error("length warnings should be skipped")
		}
	})

	t.Run("skip multiple validation types", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

Short.
`
		result := validator.Validate(PhaseDesign, markdown, &ValidationOptions{
			Skip: []string{"anti-patterns", "required-sections", "length"},
		})

		if len(result.Errors) != 0 {
			t.Errorf("expected 0 errors, got %d", len(result.Errors))
		}
		if len(result.Warnings) != 0 {
			t.Errorf("expected 0 warnings, got %d", len(result.Warnings))
		}
	})
}

func TestValidator_Validate_ValidationResult(t *testing.T) {
	parser := NewParser()
	validator := NewValidator(parser)

	t.Run("returns correct metadata", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Section 1

Content here
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		if result.Metadata.PhaseID != PhaseDesign {
			t.Errorf("expected phase DESIGN, got %s", result.Metadata.PhaseID)
		}
		if result.Metadata.WordCount == 0 {
			t.Error("word count should be non-zero")
		}
		if result.Metadata.SectionCount == 0 {
			t.Error("section count should be non-zero")
		}
		if result.Metadata.ExpectedWordRange.Min == 0 {
			t.Error("expected word range min should be non-zero")
		}
		if result.Metadata.ExpectedWordRange.Max == 0 {
			t.Error("expected word range max should be non-zero")
		}
	})

	t.Run("separates errors and warnings", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Acceptance Criteria

Error here

Missing required sections (warnings)
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		if len(result.Violations) == 0 {
			t.Error("expected violations")
		}
		if len(result.Errors) == 0 {
			t.Error("expected errors")
		}
		if len(result.Warnings) == 0 {
			t.Error("expected warnings")
		}
	})

	t.Run("generates recommendations", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Acceptance Criteria

Error here
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		if len(result.Recommendations) == 0 {
			t.Error("expected recommendations")
		}
		if !strings.Contains(result.Recommendations[0], "SPEC") {
			t.Error("recommendation should mention SPEC phase")
		}
	})

	t.Run("counts sections correctly", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Section 1

Content

## Section 2

Content

## Section 3

Content
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		if result.Metadata.SectionCount != 4 {
			t.Errorf("expected 4 sections (1 H1 + 3 H2), got %d", result.Metadata.SectionCount)
		}
	})
}

func TestValidator_FormatReport(t *testing.T) {
	parser := NewParser()
	validator := NewValidator(parser)

	t.Run("generates readable report for passed validation", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Summary

Valid document.

## Comparison

Approach A vs B.
`
		result := validator.Validate(PhaseDesign, markdown, nil)
		report := validator.FormatReport(result)

		if !strings.Contains(report, "Phase Scope Validation: DESIGN") {
			t.Error("report should contain phase ID")
		}
		if !strings.Contains(report, "✅ Validation passed") {
			t.Error("report should show passed status")
		}
	})

	t.Run("generates readable report for failed validation", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Acceptance Criteria

Error here
`
		result := validator.Validate(PhaseDesign, markdown, nil)
		report := validator.FormatReport(result)

		if !strings.Contains(report, "Phase Scope Validation: DESIGN") {
			t.Error("report should contain phase ID")
		}
		if !strings.Contains(report, "❌ Validation failed") {
			t.Error("report should show failed status")
		}
		if !strings.Contains(report, "Acceptance Criteria") {
			t.Error("report should mention violating section")
		}
		if !strings.Contains(report, "SPEC") {
			t.Error("report should mention target phase")
		}
	})

	t.Run("includes line numbers in report", func(t *testing.T) {
		markdown := `Line 1
Line 2
# DESIGN: Approach Decision

Line 5

## Acceptance Criteria

Criteria
`
		result := validator.Validate(PhaseDesign, markdown, nil)
		report := validator.FormatReport(result)

		if !strings.Contains(report, "Line 7") {
			t.Error("report should include line number")
		}
	})

	t.Run("includes suggestions in report", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Acceptance Criteria

Error here
`
		result := validator.Validate(PhaseDesign, markdown, nil)
		report := validator.FormatReport(result)

		if !strings.Contains(report, "→ Move") {
			t.Error("report should include suggestion")
		}
	})

	t.Run("includes word count in report", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

Content here
`
		result := validator.Validate(PhaseDesign, markdown, nil)
		report := validator.FormatReport(result)

		if !strings.Contains(report, "Word count:") {
			t.Error("report should include word count")
		}
	})
}

func TestValidator_EdgeCases(t *testing.T) {
	parser := NewParser()
	validator := NewValidator(parser)

	t.Run("handles empty document", func(t *testing.T) {
		result := validator.Validate(PhaseDesign, "", nil)

		if result.Passed != true {
			t.Error("empty document should pass (no errors, just warnings)")
		}
	})

	t.Run("handles document with only frontmatter", func(t *testing.T) {
		markdown := `---
title: Test
---
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		if result.Metadata.WordCount != 0 {
			t.Errorf("expected 0 word count, got %d", result.Metadata.WordCount)
		}
	})

	t.Run("handles document with multiple anti-patterns on same section name", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Requirements

Content

## Functional Requirements

More content
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		reqErrorCount := 0
		for _, err := range result.Errors {
			if strings.Contains(strings.ToLower(err.Section), "requirements") {
				reqErrorCount++
			}
		}
		if reqErrorCount == 0 {
			t.Error("both requirements sections should be detected")
		}
	})

	t.Run("handles very large document", func(t *testing.T) {
		var sections []string
		for i := range 100 {
			sections = append(sections, "## Section "+string(rune(i)))
			sections = append(sections, "Content here")
		}
		markdown := "# DESIGN: Approach Decision\n\n" + strings.Join(sections, "\n")

		start := time.Now()
		result := validator.Validate(PhaseDesign, markdown, nil)
		duration := time.Since(start)

		if result.Metadata.PhaseID != PhaseDesign {
			t.Error("validation should complete")
		}
		if duration > 100*time.Millisecond {
			t.Errorf("validation too slow: %v (expected <100ms)", duration)
		}
	})

	t.Run("handles invalid phase ID gracefully", func(t *testing.T) {
		markdown := `# Document

## Content`
		result := validator.Validate("INVALID", markdown, nil)

		if result.Metadata.PhaseID != "INVALID" {
			t.Error("should preserve invalid phase ID")
		}
	})
}

func TestBoundary_WordCount(t *testing.T) {
	parser := NewParser()
	validator := NewValidator(parser)

	// DESIGN range: Min=1000, Max=2500
	// checkLength warns when wordCount < Min and when wordCount > int(float64(Max)*1.5)
	// 1.5 * 2500 = 3750, so int(3750) = 3750

	countLengthViolations := func(result ValidationResult) int {
		count := 0
		for _, w := range result.Violations {
			if w.Type == ViolationLength {
				count++
			}
		}
		return count
	}

	generateDoc := func(wordCount int) string {
		// Header words: "DESIGN" "Approach" "Decision" = 3 words from heading
		// We skip anti-pattern and required-section checks to focus on length
		words := strings.Repeat("word ", wordCount)
		return words
	}

	t.Run("exactly at Min (1000 words) produces no too-short warning", func(t *testing.T) {
		doc := generateDoc(1000)
		result := validator.Validate(PhaseDesign, doc, &ValidationOptions{
			Skip: []string{"anti-patterns", "required-sections"},
		})
		violations := countLengthViolations(result)
		if violations != 0 {
			t.Errorf("expected 0 length violations at exact Min, got %d (word count: %d)",
				violations, result.Metadata.WordCount)
		}
	})

	t.Run("one below Min (999 words) produces too-short warning", func(t *testing.T) {
		doc := generateDoc(999)
		result := validator.Validate(PhaseDesign, doc, &ValidationOptions{
			Skip: []string{"anti-patterns", "required-sections"},
		})
		violations := countLengthViolations(result)
		if violations == 0 {
			t.Errorf("expected length violation at Min-1, got 0 (word count: %d)",
				result.Metadata.WordCount)
		}
	})

	t.Run("one above Min (1001 words) produces no too-short warning", func(t *testing.T) {
		doc := generateDoc(1001)
		result := validator.Validate(PhaseDesign, doc, &ValidationOptions{
			Skip: []string{"anti-patterns", "required-sections"},
		})
		violations := countLengthViolations(result)
		if violations != 0 {
			t.Errorf("expected 0 length violations at Min+1, got %d (word count: %d)",
				violations, result.Metadata.WordCount)
		}
	})

	t.Run("at Max (2500 words) produces no warning", func(t *testing.T) {
		doc := generateDoc(2500)
		result := validator.Validate(PhaseDesign, doc, &ValidationOptions{
			Skip: []string{"anti-patterns", "required-sections"},
		})
		violations := countLengthViolations(result)
		if violations != 0 {
			t.Errorf("expected 0 length violations at Max, got %d (word count: %d)",
				violations, result.Metadata.WordCount)
		}
	})

	t.Run("at Max+1 (2501 words) produces no warning (only 1.5x triggers)", func(t *testing.T) {
		doc := generateDoc(2501)
		result := validator.Validate(PhaseDesign, doc, &ValidationOptions{
			Skip: []string{"anti-patterns", "required-sections"},
		})
		violations := countLengthViolations(result)
		if violations != 0 {
			t.Errorf("expected 0 length violations at Max+1, got %d (word count: %d)",
				violations, result.Metadata.WordCount)
		}
	})

	t.Run("at exactly 1.5x Max (3750 words) produces no too-long warning", func(t *testing.T) {
		doc := generateDoc(3750)
		result := validator.Validate(PhaseDesign, doc, &ValidationOptions{
			Skip: []string{"anti-patterns", "required-sections"},
		})
		violations := countLengthViolations(result)
		if violations != 0 {
			t.Errorf("expected 0 length violations at 1.5x Max, got %d (word count: %d)",
				violations, result.Metadata.WordCount)
		}
	})

	t.Run("at 1.5x Max + 1 (3751 words) produces too-long warning", func(t *testing.T) {
		doc := generateDoc(3751)
		result := validator.Validate(PhaseDesign, doc, &ValidationOptions{
			Skip: []string{"anti-patterns", "required-sections"},
		})
		violations := countLengthViolations(result)
		if violations == 0 {
			t.Errorf("expected length violation at 1.5x Max + 1, got 0 (word count: %d)",
				result.Metadata.WordCount)
		}
	})
}

func TestValidator_Recommendations(t *testing.T) {
	parser := NewParser()
	validator := NewValidator(parser)

	t.Run("groups anti-patterns by target phase", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Acceptance Criteria

D4 content

## Requirements

D4 content

## Task Breakdown

S7 content
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		if len(result.Recommendations) == 0 {
			t.Error("expected recommendations")
		}
	})

	t.Run("recommends adding missing sections", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

## Summary

Very short document.
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		foundAddRecommendation := false
		for _, rec := range result.Recommendations {
			if strings.Contains(strings.ToLower(rec), "add") {
				foundAddRecommendation = true
				break
			}
		}
		if !foundAddRecommendation {
			t.Error("expected recommendation to add missing sections")
		}
	})

	t.Run("recommends adjusting length", func(t *testing.T) {
		markdown := `
# DESIGN: Approach Decision

Short.
`
		result := validator.Validate(PhaseDesign, markdown, nil)

		foundLengthRecommendation := false
		for _, rec := range result.Recommendations {
			if strings.Contains(strings.ToLower(rec), "expand") ||
				strings.Contains(strings.ToLower(rec), "words") {
				foundLengthRecommendation = true
				break
			}
		}
		if !foundLengthRecommendation {
			t.Error("expected recommendation to adjust length")
		}
	})
}
