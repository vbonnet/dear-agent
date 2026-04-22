package outputformatter

import (
	"fmt"
	"strings"
)

// SummaryGenerator creates summaries from result collections
type SummaryGenerator struct {
	iconMapper *IconMapper
}

// NewSummaryGenerator creates a new summary generator
func NewSummaryGenerator(iconMapper *IconMapper) *SummaryGenerator {
	return &SummaryGenerator{iconMapper: iconMapper}
}

// Generate creates a summary from a collection of results
func (g *SummaryGenerator) Generate(results []Result) Summary {
	summary := Summary{
		Total: len(results),
	}

	for _, r := range results {
		switch r.Status() {
		case StatusOK, StatusSuccess:
			summary.Passed++
		case StatusInfo:
			summary.Info++
		case StatusWarning:
			summary.Warnings++
		case StatusError, StatusFailed:
			summary.Errors++
		case StatusUnknown:
			summary.Unknown++
		}
	}

	return summary
}

// Format formats a summary as a human-readable string
func (g *SummaryGenerator) Format(summary Summary) string {
	var lines []string

	if summary.Passed > 0 {
		icon := g.iconMapper.GetIcon(StatusOK)
		lines = append(lines, fmt.Sprintf("  %s %d checks passed", icon, summary.Passed))
	}

	if summary.Info > 0 {
		icon := g.iconMapper.GetIcon(StatusInfo)
		lines = append(lines, fmt.Sprintf("  %s %d info", icon, summary.Info))
	}

	if summary.Warnings > 0 {
		icon := g.iconMapper.GetIcon(StatusWarning)
		lines = append(lines, fmt.Sprintf("  %s %d warnings", icon, summary.Warnings))
	}

	if summary.Errors > 0 {
		icon := g.iconMapper.GetIcon(StatusError)
		lines = append(lines, fmt.Sprintf("  %s %d errors", icon, summary.Errors))
	}

	if summary.Unknown > 0 {
		icon := g.iconMapper.GetIcon(StatusUnknown)
		lines = append(lines, fmt.Sprintf("  %s %d unknown", icon, summary.Unknown))
	}

	if len(lines) == 0 {
		return "  No results"
	}

	return strings.Join(lines, "\n")
}

// FormatCompact formats a summary as a single-line compact string
func (g *SummaryGenerator) FormatCompact(summary Summary) string {
	parts := []string{}

	if summary.Passed > 0 {
		parts = append(parts, fmt.Sprintf("%d passed", summary.Passed))
	}
	if summary.Info > 0 {
		parts = append(parts, fmt.Sprintf("%d info", summary.Info))
	}
	if summary.Warnings > 0 {
		parts = append(parts, fmt.Sprintf("%d warnings", summary.Warnings))
	}
	if summary.Errors > 0 {
		parts = append(parts, fmt.Sprintf("%d errors", summary.Errors))
	}
	if summary.Unknown > 0 {
		parts = append(parts, fmt.Sprintf("%d unknown", summary.Unknown))
	}

	if len(parts) == 0 {
		return "no results"
	}

	return strings.Join(parts, ", ")
}

// GetIssues filters results to only warnings and errors
func GetIssues(results []Result) []Result {
	issues := []Result{}
	for _, r := range results {
		status := r.Status()
		if status == StatusWarning || status == StatusError || status == StatusFailed {
			issues = append(issues, r)
		}
	}
	return issues
}
