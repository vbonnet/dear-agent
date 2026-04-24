package scope

import (
	"fmt"
	"regexp"
	"strings"
)

// Validator validates Wayfinder phase documents against scope rules
type Validator struct {
	parser *Parser
}

// NewValidator creates a new scope validator
func NewValidator(parser *Parser) *Validator {
	return &Validator{
		parser: parser,
	}
}

// Validate performs validation on a phase document
func (v *Validator) Validate(phaseID PhaseID, document string, opts *ValidationOptions) ValidationResult {
	if opts == nil {
		opts = &ValidationOptions{}
	}

	// Set default fuzzy threshold
	if opts.FuzzyThreshold == 0 {
		opts.FuzzyThreshold = 0.75
	}

	// Parse document
	sections := v.parser.Parse(document)
	wordCount := v.countWords(document)
	violations := []Violation{}

	// Check anti-patterns (unless skipped)
	if !v.shouldSkip(opts, "anti-patterns") {
		violations = append(violations, v.checkAntiPatterns(phaseID, sections)...)
	}

	// Check required sections (unless skipped)
	if !v.shouldSkip(opts, "required-sections") {
		violations = append(violations, v.checkRequiredSections(phaseID, sections)...)
	}

	// Check document length (unless skipped)
	if !v.shouldSkip(opts, "length") {
		violations = append(violations, v.checkLength(phaseID, wordCount)...)
	}

	// Separate errors and warnings
	errors := []Violation{}
	warnings := []Violation{}
	for _, v := range violations {
		if v.Severity == SeverityError {
			errors = append(errors, v)
		} else {
			warnings = append(warnings, v)
		}
	}

	// Generate recommendations
	recommendations := v.generateRecommendations(violations)

	// Determine pass/fail
	passed := opts.Override || len(errors) == 0

	// Get expected word range
	expectedRange := PhaseLengthRanges[phaseID]

	return ValidationResult{
		Passed:          passed,
		Violations:      violations,
		Errors:          errors,
		Warnings:        warnings,
		Recommendations: recommendations,
		Metadata: ValidationMetadata{
			PhaseID:           phaseID,
			WordCount:         wordCount,
			SectionCount:      len(sections),
			ExpectedWordRange: expectedRange,
		},
	}
}

// FormatReport generates a human-readable validation report
func (v *Validator) FormatReport(result ValidationResult) string {
	var lines []string

	// Header
	lines = append(lines, "")
	lines = append(lines, strings.Repeat("━", 60))
	lines = append(lines, fmt.Sprintf("Phase Scope Validation: %s", result.Metadata.PhaseID))
	lines = append(lines, strings.Repeat("━", 60))
	lines = append(lines, "")

	// Metadata
	lines = append(lines, "📊 Document Stats:")
	lines = append(lines, fmt.Sprintf("   Word count: %d (expected: %d-%d)",
		result.Metadata.WordCount,
		result.Metadata.ExpectedWordRange.Min,
		result.Metadata.ExpectedWordRange.Max))
	lines = append(lines, fmt.Sprintf("   Sections: %d", result.Metadata.SectionCount))
	lines = append(lines, "")

	// Errors
	if len(result.Errors) > 0 {
		lines = append(lines, fmt.Sprintf("❌ Errors (%d):", len(result.Errors)))
		for _, err := range result.Errors {
			lines = append(lines, fmt.Sprintf("   %s", err.Message))
			if err.Line > 0 {
				lines = append(lines, fmt.Sprintf("      Location: Line %d", err.Line))
			}
			if err.Suggestion != "" {
				lines = append(lines, fmt.Sprintf("      → %s", err.Suggestion))
			}
		}
		lines = append(lines, "")
	}

	// Warnings
	if len(result.Warnings) > 0 {
		lines = append(lines, fmt.Sprintf("⚠️  Warnings (%d):", len(result.Warnings)))
		for _, warning := range result.Warnings {
			lines = append(lines, fmt.Sprintf("   %s", warning.Message))
			if warning.Suggestion != "" {
				lines = append(lines, fmt.Sprintf("      → %s", warning.Suggestion))
			}
		}
		lines = append(lines, "")
	}

	// Recommendations
	if len(result.Recommendations) > 0 {
		lines = append(lines, "📋 Recommendations:")
		for _, rec := range result.Recommendations {
			lines = append(lines, fmt.Sprintf("   • %s", rec))
		}
		lines = append(lines, "")
	}

	// Result
	if result.Passed {
		lines = append(lines, "✅ Validation passed")
	} else {
		lines = append(lines, "❌ Validation failed")
		lines = append(lines, fmt.Sprintf("   %d error(s) must be fixed before proceeding", len(result.Errors)))
	}

	lines = append(lines, "")
	lines = append(lines, strings.Repeat("━", 60))

	return strings.Join(lines, "\n")
}

// checkAntiPatterns detects sections from future phases
func (v *Validator) checkAntiPatterns(phaseID PhaseID, sections []Section) []Violation {
	antiPatterns := PhaseAntiPatterns[phaseID]
	violations := []Violation{}

	for _, pattern := range antiPatterns {
		// Check main pattern
		matches := v.parser.FindSections(sections, pattern.Section, true)

		// Check aliases
		for _, alias := range pattern.Aliases {
			aliasMatches := v.parser.FindSections(sections, alias, true)
			matches = append(matches, aliasMatches...)
		}

		// Deduplicate by line number (same section matched by multiple patterns/aliases)
		seen := make(map[int]bool)
		uniqueMatches := []Section{}
		for _, match := range matches {
			if !seen[match.StartLine] {
				seen[match.StartLine] = true
				uniqueMatches = append(uniqueMatches, match)
			}
		}

		// Create violations for all unique matches
		for _, match := range uniqueMatches {
			violations = append(violations, Violation{
				Type:      ViolationAntiPattern,
				Severity:  pattern.Severity,
				Section:   match.Heading,
				Line:      match.StartLine,
				BelongsIn: pattern.BelongsIn,
				Message: fmt.Sprintf("Section \"%s\" belongs in %s, not %s",
					match.Heading, pattern.BelongsIn, phaseID),
				Suggestion: fmt.Sprintf("Move \"%s\" section to %s phase",
					match.Heading, pattern.BelongsIn),
			})
		}
	}

	return violations
}

// checkRequiredSections warns if expected sections are missing
func (v *Validator) checkRequiredSections(phaseID PhaseID, sections []Section) []Violation {
	required := PhaseRequiredSections[phaseID]
	violations := []Violation{}

	for _, requiredSection := range required {
		found := v.parser.FindSections(sections, requiredSection, true)

		if len(found) == 0 {
			violations = append(violations, Violation{
				Type:     ViolationMissingSection,
				Severity: SeverityWarning,
				Section:  requiredSection,
				Message: fmt.Sprintf("Required section \"%s\" not found in %s",
					requiredSection, phaseID),
				Suggestion: fmt.Sprintf("Add \"%s\" section to document",
					requiredSection),
			})
		}
	}

	return violations
}

// checkLength validates document word count
func (v *Validator) checkLength(phaseID PhaseID, wordCount int) []Violation {
	lengthRange, exists := PhaseLengthRanges[phaseID]
	if !exists {
		return []Violation{}
	}

	violations := []Violation{}

	// Warn if too short
	if wordCount < lengthRange.Min {
		violations = append(violations, Violation{
			Type:     ViolationLength,
			Severity: SeverityWarning,
			Message: fmt.Sprintf("Document is %d words (expected %d-%d)",
				wordCount, lengthRange.Min, lengthRange.Max),
			Suggestion: fmt.Sprintf("Consider expanding analysis to meet minimum %d words",
				lengthRange.Min),
		})
	}

	// Warn if significantly too long (>1.5x max)
	if wordCount > int(float64(lengthRange.Max)*1.5) {
		percentOver := int((float64(wordCount)/float64(lengthRange.Max) - 1.0) * 100)
		violations = append(violations, Violation{
			Type:     ViolationLength,
			Severity: SeverityWarning,
			Message: fmt.Sprintf("Document is %d words (expected %d-%d, over by %d%%)",
				wordCount, lengthRange.Min, lengthRange.Max, percentOver),
			Suggestion: "Consider extracting content to future phases or summarizing",
		})
	}

	return violations
}

// generateRecommendations creates actionable recommendations from violations
func (v *Validator) generateRecommendations(violations []Violation) []string {
	recommendations := []string{}

	// Group anti-patterns by target phase
	antiPatternsByPhase := make(map[PhaseID][]Violation)
	for _, violation := range violations {
		if violation.Type == ViolationAntiPattern && violation.BelongsIn != "" {
			antiPatternsByPhase[violation.BelongsIn] = append(
				antiPatternsByPhase[violation.BelongsIn], violation)
		}
	}

	// Recommend moving sections
	for targetPhase, vList := range antiPatternsByPhase {
		sections := []string{}
		for _, v := range vList {
			sections = append(sections, fmt.Sprintf("\"%s\"", v.Section))
		}
		recommendations = append(recommendations,
			fmt.Sprintf("Move %s to %s phase where they belong",
				strings.Join(sections, ", "), targetPhase))
	}

	// Recommend adding missing sections
	missingSections := []string{}
	for _, v := range violations {
		if v.Type == ViolationMissingSection {
			missingSections = append(missingSections, fmt.Sprintf("\"%s\"", v.Section))
		}
	}
	if len(missingSections) > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Add missing sections: %s", strings.Join(missingSections, ", ")))
	}

	// Recommend length adjustments
	for _, v := range violations {
		if v.Type == ViolationLength && v.Suggestion != "" {
			recommendations = append(recommendations, v.Suggestion)
		}
	}

	return recommendations
}

// countWords counts words in document, excluding code blocks
func (v *Validator) countWords(document string) int {
	// Remove frontmatter
	cleaned := regexp.MustCompile(`(?m)^---[\s\S]*?^---`).ReplaceAllString(document, "")

	// Remove code blocks
	cleaned = regexp.MustCompile("```[\\s\\S]*?```").ReplaceAllString(cleaned, "")

	// Remove inline code
	cleaned = regexp.MustCompile("`[^`]+`").ReplaceAllString(cleaned, "")

	// Split by whitespace and count non-empty words
	words := strings.Fields(cleaned)
	count := 0
	for _, word := range words {
		if len(word) > 0 {
			count++
		}
	}

	return count
}

// shouldSkip checks if a validation type should be skipped
func (v *Validator) shouldSkip(opts *ValidationOptions, validationType string) bool {
	for _, skip := range opts.Skip {
		if skip == validationType {
			return true
		}
	}
	return false
}
