package phaseisolation

import (
	"fmt"
	"regexp"
	"strings"
)

// Violation represents a validation violation.
type Violation struct {
	Type       string  // "anti-pattern", "missing-section", "length", "unexpected-section"
	Severity   string  // "error" or "warning"
	Section    string  // Section that triggered violation
	Line       int     // Line number in document
	BelongsIn  PhaseID // Phase where this section belongs (for anti-patterns)
	Message    string  // Human-readable message
	Suggestion string  // Suggestion for fixing
}

// ValidationResult holds the result of scope validation.
type ValidationResult struct {
	Passed          bool
	Violations      []Violation
	Warnings        []Violation
	Errors          []Violation
	Recommendations []string
	Metadata        ValidationMetadata
}

// ValidationMetadata holds metadata about the validated document.
type ValidationMetadata struct {
	PhaseID           PhaseID
	WordCount         int
	SectionCount      int
	ExpectedWordRange LengthRange
}

// ValidationOptions controls validation behavior.
type ValidationOptions struct {
	Override       bool     // Override validation (pass despite errors)
	FuzzyThreshold float64  // Custom fuzzy match threshold (default: 0.80)
	Skip           []string // Disable specific validation types
}

// ScopeValidator validates phase documents against scope rules.
type ScopeValidator struct {
	parser *SectionParser
}

// NewScopeValidator creates a new ScopeValidator.
func NewScopeValidator(parser *SectionParser) *ScopeValidator {
	return &ScopeValidator{parser: parser}
}

// Validate validates a phase document against scope rules.
func (sv *ScopeValidator) Validate(phaseID PhaseID, document string, opts ...ValidationOptions) ValidationResult {
	opt := ValidationOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	sections := sv.parser.Parse(document)
	wordCount := sv.countWords(document)
	var violations []Violation

	skipSet := make(map[string]bool)
	for _, s := range opt.Skip {
		skipSet[s] = true
	}

	if !skipSet["anti-patterns"] {
		violations = append(violations, sv.checkAntiPatterns(phaseID, sections)...)
	}

	if !skipSet["required-sections"] {
		violations = append(violations, sv.checkRequiredSections(phaseID, sections)...)
	}

	if !skipSet["length"] {
		violations = append(violations, sv.checkLength(phaseID, wordCount)...)
	}

	var errors, warnings []Violation
	for _, v := range violations {
		if v.Severity == "error" {
			errors = append(errors, v)
		} else {
			warnings = append(warnings, v)
		}
	}

	recommendations := sv.generateRecommendations(violations)

	passed := opt.Override || len(errors) == 0

	expectedRange := PhaseLengthRanges[phaseID]

	return ValidationResult{
		Passed:          passed,
		Violations:      violations,
		Warnings:        warnings,
		Errors:          errors,
		Recommendations: recommendations,
		Metadata: ValidationMetadata{
			PhaseID:           phaseID,
			WordCount:         wordCount,
			SectionCount:      len(sections),
			ExpectedWordRange: expectedRange,
		},
	}
}

// FormatReport generates a human-readable validation report.
func (sv *ScopeValidator) FormatReport(result ValidationResult) string {
	var lines []string

	lines = append(lines, "")
	lines = append(lines, strings.Repeat("\u2501", 60))
	lines = append(lines, fmt.Sprintf("Phase Scope Validation: %s", result.Metadata.PhaseID))
	lines = append(lines, strings.Repeat("\u2501", 60))
	lines = append(lines, "")

	lines = append(lines, "Document Stats:")
	lines = append(lines, fmt.Sprintf("   Word count: %d (expected: %d-%d)",
		result.Metadata.WordCount,
		result.Metadata.ExpectedWordRange.Min,
		result.Metadata.ExpectedWordRange.Max))
	lines = append(lines, fmt.Sprintf("   Sections: %d", result.Metadata.SectionCount))
	lines = append(lines, "")

	if len(result.Errors) > 0 {
		lines = append(lines, fmt.Sprintf("Errors (%d):", len(result.Errors)))
		for _, e := range result.Errors {
			lines = append(lines, "   "+e.Message)
			if e.Line > 0 {
				lines = append(lines, fmt.Sprintf("      Location: Line %d", e.Line))
			}
			if e.Suggestion != "" {
				lines = append(lines, "      -> "+e.Suggestion)
			}
		}
		lines = append(lines, "")
	}

	if len(result.Warnings) > 0 {
		lines = append(lines, fmt.Sprintf("Warnings (%d):", len(result.Warnings)))
		for _, w := range result.Warnings {
			lines = append(lines, "   "+w.Message)
			if w.Suggestion != "" {
				lines = append(lines, "      -> "+w.Suggestion)
			}
		}
		lines = append(lines, "")
	}

	if len(result.Recommendations) > 0 {
		lines = append(lines, "Recommendations:")
		for _, rec := range result.Recommendations {
			lines = append(lines, "   * "+rec)
		}
		lines = append(lines, "")
	}

	if result.Passed {
		lines = append(lines, "Validation passed")
	} else {
		lines = append(lines, "Validation failed")
		lines = append(lines, fmt.Sprintf("   %d error(s) must be fixed before proceeding", len(result.Errors)))
	}

	lines = append(lines, "")
	lines = append(lines, strings.Repeat("\u2501", 60))

	return strings.Join(lines, "\n")
}

func (sv *ScopeValidator) checkAntiPatterns(phaseID PhaseID, sections []Section) []Violation {
	antiPatterns := PhaseAntiPatterns[phaseID]
	var violations []Violation

	for _, pattern := range antiPatterns {
		matches := sv.parser.FindSections(sections, pattern.Section, true)

		for _, alias := range pattern.Aliases {
			matches = append(matches, sv.parser.FindSections(sections, alias, true)...)
		}

		// Deduplicate by line number
		seen := make(map[int]bool)
		for _, match := range matches {
			if seen[match.StartLine] {
				continue
			}
			seen[match.StartLine] = true

			violations = append(violations, Violation{
				Type:       "anti-pattern",
				Severity:   pattern.Severity,
				Section:    match.Heading,
				Line:       match.StartLine,
				BelongsIn:  pattern.BelongsIn,
				Message:    fmt.Sprintf("Section %q belongs in %s, not %s", match.Heading, pattern.BelongsIn, phaseID),
				Suggestion: fmt.Sprintf("Move %q section to %s phase", match.Heading, pattern.BelongsIn),
			})
		}
	}

	return violations
}

func (sv *ScopeValidator) checkRequiredSections(phaseID PhaseID, sections []Section) []Violation {
	required := PhaseRequiredSections[phaseID]
	var violations []Violation

	for _, reqSection := range required {
		found := sv.parser.FindSections(sections, reqSection, true)
		if len(found) == 0 {
			violations = append(violations, Violation{
				Type:       "missing-section",
				Severity:   "warning",
				Section:    reqSection,
				Message:    fmt.Sprintf("Required section %q not found in %s", reqSection, phaseID),
				Suggestion: fmt.Sprintf("Add %q section to document", reqSection),
			})
		}
	}

	return violations
}

func (sv *ScopeValidator) checkLength(phaseID PhaseID, wordCount int) []Violation {
	r, ok := PhaseLengthRanges[phaseID]
	if !ok {
		return nil
	}

	var violations []Violation

	if wordCount < r.Min {
		violations = append(violations, Violation{
			Type:       "length",
			Severity:   "warning",
			Message:    fmt.Sprintf("Document is %d words (expected %d-%d)", wordCount, r.Min, r.Max),
			Suggestion: fmt.Sprintf("Consider expanding analysis to meet minimum %d words", r.Min),
		})
	}

	if wordCount > r.Max*3/2 {
		percentOver := (wordCount * 100 / r.Max) - 100
		violations = append(violations, Violation{
			Type:       "length",
			Severity:   "warning",
			Message:    fmt.Sprintf("Document is %d words (expected %d-%d, over by %d%%)", wordCount, r.Min, r.Max, percentOver),
			Suggestion: "Consider extracting content to future phases or summarizing",
		})
	}

	return violations
}

func (sv *ScopeValidator) generateRecommendations(violations []Violation) []string {
	var recommendations []string

	// Group anti-patterns by target phase
	byPhase := make(map[PhaseID][]Violation)
	for _, v := range violations {
		if v.Type == "anti-pattern" && v.BelongsIn != "" {
			byPhase[v.BelongsIn] = append(byPhase[v.BelongsIn], v)
		}
	}

	for targetPhase, vList := range byPhase {
		var sections []string
		for _, v := range vList {
			sections = append(sections, fmt.Sprintf("%q", v.Section))
		}
		recommendations = append(recommendations,
			fmt.Sprintf("Move %s to %s phase where they belong", strings.Join(sections, ", "), targetPhase))
	}

	var missing []string
	for _, v := range violations {
		if v.Type == "missing-section" {
			missing = append(missing, fmt.Sprintf("%q", v.Section))
		}
	}
	if len(missing) > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Add missing sections: %s", strings.Join(missing, ", ")))
	}

	for _, v := range violations {
		if v.Type == "length" && v.Suggestion != "" {
			recommendations = append(recommendations, v.Suggestion)
		}
	}

	return recommendations
}

var (
	frontmatterRegex = regexp.MustCompile(`(?s)^---.*?---`)
	codeBlockRegex   = regexp.MustCompile("(?s)```.*?```")
	inlineCodeRegex  = regexp.MustCompile("`[^`]+`")
)

func (sv *ScopeValidator) countWords(document string) int {
	cleaned := frontmatterRegex.ReplaceAllString(document, "")
	cleaned = codeBlockRegex.ReplaceAllString(cleaned, "")
	cleaned = inlineCodeRegex.ReplaceAllString(cleaned, "")
	return len(strings.Fields(cleaned))
}
