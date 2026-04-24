package review

import (
	"fmt"
	"strings"
	"time"
)

// GenerateReviewReport creates a human-readable review report
func GenerateReviewReport(result *ReviewResult) string {
	var sb strings.Builder

	// Header
	sb.WriteString("=" + strings.Repeat("=", 78) + "\n")
	sb.WriteString("  MULTI-PERSONA REVIEW REPORT\n")
	sb.WriteString("=" + strings.Repeat("=", 78) + "\n\n")

	// Overview
	sb.WriteString("OVERVIEW\n")
	sb.WriteString(strings.Repeat("-", 79) + "\n")
	sb.WriteString(fmt.Sprintf("Task ID:       %s\n", result.TaskID))
	sb.WriteString(fmt.Sprintf("Risk Level:    %s\n", result.RiskLevel))
	sb.WriteString(fmt.Sprintf("Review Type:   %s\n", result.ReviewType))
	sb.WriteString(fmt.Sprintf("Timestamp:     %s\n", result.Timestamp.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Status:        %s\n", reviewStatus(result.Passed)))
	sb.WriteString(fmt.Sprintf("Score:         %.1f/100\n", result.AggregateScore))
	sb.WriteString("\n")

	// Metrics Summary
	sb.WriteString("METRICS SUMMARY\n")
	sb.WriteString(strings.Repeat("-", 79) + "\n")
	sb.WriteString(fmt.Sprintf("Total Issues:         %d\n", result.Metrics.TotalIssues))
	sb.WriteString(fmt.Sprintf("  P0 (Critical):      %d\n", result.Metrics.P0Issues))
	sb.WriteString(fmt.Sprintf("  P1 (High):          %d\n", result.Metrics.P1Issues))
	sb.WriteString(fmt.Sprintf("  P2 (Medium):        %d\n", result.Metrics.P2Issues))
	sb.WriteString(fmt.Sprintf("  P3 (Low):           %d\n", result.Metrics.P3Issues))
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("Security Score:       %.1f/100\n", result.Metrics.SecurityScore))
	sb.WriteString(fmt.Sprintf("Performance Score:    %.1f/100\n", result.Metrics.PerformanceScore))
	sb.WriteString(fmt.Sprintf("Maintainability:      %.1f/100\n", result.Metrics.MaintainabilityScore))
	sb.WriteString(fmt.Sprintf("UX Score:             %.1f/100\n", result.Metrics.UXScore))
	sb.WriteString(fmt.Sprintf("Reliability Score:    %.1f/100\n", result.Metrics.ReliabilityScore))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Review Duration:      %dms\n", result.Metrics.ReviewDurationMS))
	sb.WriteString("\n")

	// Persona Results
	sb.WriteString("PERSONA REVIEWS\n")
	sb.WriteString(strings.Repeat("-", 79) + "\n")

	for _, persona := range result.PersonaResults {
		sb.WriteString(fmt.Sprintf("\n%s (%s)\n", strings.ToUpper(string(persona.Persona)), persona.Confidence))
		sb.WriteString(fmt.Sprintf("Score: %.1f/100\n", persona.Score))

		if persona.Summary != "" {
			sb.WriteString(fmt.Sprintf("Summary: %s\n", persona.Summary))
		}

		if len(persona.Issues) > 0 {
			sb.WriteString(fmt.Sprintf("Issues Found: %d\n", len(persona.Issues)))
			for i, issue := range persona.Issues {
				if i >= 5 { // Limit to first 5 issues per persona
					sb.WriteString(fmt.Sprintf("  ... and %d more issues\n", len(persona.Issues)-5))
					break
				}
				sb.WriteString(formatIssue(issue, "  "))
			}
		} else {
			sb.WriteString("No issues found.\n")
		}
	}

	sb.WriteString("\n")

	// Blocking Issues
	if len(result.BlockingIssues) > 0 {
		sb.WriteString("BLOCKING ISSUES\n")
		sb.WriteString(strings.Repeat("-", 79) + "\n")
		sb.WriteString(fmt.Sprintf("Found %d blocking issue(s) that MUST be resolved:\n\n", len(result.BlockingIssues)))

		for i, issue := range result.BlockingIssues {
			sb.WriteString(fmt.Sprintf("%d. ", i+1))
			sb.WriteString(formatIssue(issue, "   "))
		}
		sb.WriteString("\n")
	}

	// Recommendations
	if len(result.Recommendations) > 0 {
		sb.WriteString("RECOMMENDATIONS\n")
		sb.WriteString(strings.Repeat("-", 79) + "\n")

		for i, rec := range result.Recommendations {
			if i >= 10 { // Limit to first 10 recommendations
				sb.WriteString(fmt.Sprintf("... and %d more recommendations\n", len(result.Recommendations)-10))
				break
			}
			sb.WriteString(fmt.Sprintf("- %s\n", rec))
		}
		sb.WriteString("\n")
	}

	// Footer
	sb.WriteString("=" + strings.Repeat("=", 78) + "\n")
	if result.Passed {
		sb.WriteString("  REVIEW PASSED - No blocking issues found\n")
	} else {
		sb.WriteString("  REVIEW FAILED - Blocking issues must be resolved\n")
	}
	sb.WriteString("=" + strings.Repeat("=", 78) + "\n")

	return sb.String()
}

// formatIssue formats a single issue for display
func formatIssue(issue ReviewIssue, indent string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", issue.Severity, issue.Category, issue.Message))

	if issue.FilePath != "" {
		sb.WriteString(fmt.Sprintf("%sFile: %s", indent, issue.FilePath))
		if issue.LineNumber > 0 {
			sb.WriteString(fmt.Sprintf(":%d", issue.LineNumber))
		}
		sb.WriteString("\n")
	}

	if issue.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("%sSuggestion: %s\n", indent, issue.Suggestion))
	}

	if issue.CodeSnippet != "" {
		sb.WriteString(fmt.Sprintf("%sCode:\n%s\n", indent, indentText(issue.CodeSnippet, indent+"  ")))
	}

	return sb.String()
}

// reviewStatus returns a colored status string
func reviewStatus(passed bool) string {
	if passed {
		return "PASSED ✓"
	}
	return "FAILED ✗"
}

// indentText adds indentation to each line of text
func indentText(text, indent string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

// GenerateMarkdownReport creates a Markdown-formatted review report
func GenerateMarkdownReport(result *ReviewResult) string {
	var sb strings.Builder

	// Header
	sb.WriteString("# Multi-Persona Review Report\n\n")

	// Overview
	sb.WriteString("## Overview\n\n")
	sb.WriteString(fmt.Sprintf("- **Task ID**: %s\n", result.TaskID))
	sb.WriteString(fmt.Sprintf("- **Risk Level**: %s\n", result.RiskLevel))
	sb.WriteString(fmt.Sprintf("- **Review Type**: %s\n", result.ReviewType))
	sb.WriteString(fmt.Sprintf("- **Timestamp**: %s\n", result.Timestamp.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("- **Status**: %s\n", reviewStatusMarkdown(result.Passed)))
	sb.WriteString(fmt.Sprintf("- **Aggregate Score**: %.1f/100\n\n", result.AggregateScore))

	// Metrics
	sb.WriteString("## Metrics Summary\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Total Issues | %d |\n", result.Metrics.TotalIssues))
	sb.WriteString(fmt.Sprintf("| P0 (Critical) | %d |\n", result.Metrics.P0Issues))
	sb.WriteString(fmt.Sprintf("| P1 (High) | %d |\n", result.Metrics.P1Issues))
	sb.WriteString(fmt.Sprintf("| P2 (Medium) | %d |\n", result.Metrics.P2Issues))
	sb.WriteString(fmt.Sprintf("| P3 (Low) | %d |\n", result.Metrics.P3Issues))
	sb.WriteString(fmt.Sprintf("| Security Score | %.1f/100 |\n", result.Metrics.SecurityScore))
	sb.WriteString(fmt.Sprintf("| Performance Score | %.1f/100 |\n", result.Metrics.PerformanceScore))
	sb.WriteString(fmt.Sprintf("| Maintainability Score | %.1f/100 |\n", result.Metrics.MaintainabilityScore))
	sb.WriteString(fmt.Sprintf("| UX Score | %.1f/100 |\n", result.Metrics.UXScore))
	sb.WriteString(fmt.Sprintf("| Reliability Score | %.1f/100 |\n", result.Metrics.ReliabilityScore))
	sb.WriteString(fmt.Sprintf("| Review Duration | %dms |\n\n", result.Metrics.ReviewDurationMS))

	// Persona Results
	sb.WriteString("## Persona Reviews\n\n")

	for _, persona := range result.PersonaResults {
		sb.WriteString(fmt.Sprintf("### %s\n\n", strings.Title(string(persona.Persona))))
		sb.WriteString(fmt.Sprintf("- **Score**: %.1f/100\n", persona.Score))
		sb.WriteString(fmt.Sprintf("- **Confidence**: %s\n", persona.Confidence))

		if persona.Summary != "" {
			sb.WriteString(fmt.Sprintf("- **Summary**: %s\n", persona.Summary))
		}

		if len(persona.Issues) > 0 {
			sb.WriteString(fmt.Sprintf("\n**Issues Found**: %d\n\n", len(persona.Issues)))

			for _, issue := range persona.Issues {
				sb.WriteString(formatIssueMarkdown(issue))
			}
		} else {
			sb.WriteString("\n✓ No issues found.\n")
		}

		sb.WriteString("\n")
	}

	// Blocking Issues
	if len(result.BlockingIssues) > 0 {
		sb.WriteString("## ⚠️ Blocking Issues\n\n")
		sb.WriteString(fmt.Sprintf("Found **%d blocking issue(s)** that MUST be resolved:\n\n", len(result.BlockingIssues)))

		for i, issue := range result.BlockingIssues {
			sb.WriteString(fmt.Sprintf("%d. ", i+1))
			sb.WriteString(formatIssueMarkdown(issue))
		}
	}

	// Recommendations
	if len(result.Recommendations) > 0 {
		sb.WriteString("## Recommendations\n\n")

		for _, rec := range result.Recommendations {
			sb.WriteString(fmt.Sprintf("- %s\n", rec))
		}
		sb.WriteString("\n")
	}

	// Footer
	if result.Passed {
		sb.WriteString("---\n\n✅ **REVIEW PASSED** - No blocking issues found\n")
	} else {
		sb.WriteString("---\n\n❌ **REVIEW FAILED** - Blocking issues must be resolved\n")
	}

	return sb.String()
}

// formatIssueMarkdown formats an issue for Markdown
func formatIssueMarkdown(issue ReviewIssue) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("**[%s]** %s: %s\n", issue.Severity, issue.Category, issue.Message))

	if issue.FilePath != "" {
		location := fmt.Sprintf("`%s`", issue.FilePath)
		if issue.LineNumber > 0 {
			location = fmt.Sprintf("`%s:%d`", issue.FilePath, issue.LineNumber)
		}
		sb.WriteString(fmt.Sprintf("   - File: %s\n", location))
	}

	if issue.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("   - Suggestion: %s\n", issue.Suggestion))
	}

	if issue.CodeSnippet != "" {
		sb.WriteString("   - Code:\n```\n" + issue.CodeSnippet + "\n```\n")
	}

	sb.WriteString("\n")

	return sb.String()
}

// reviewStatusMarkdown returns a Markdown-formatted status
func reviewStatusMarkdown(passed bool) string {
	if passed {
		return "✅ PASSED"
	}
	return "❌ FAILED"
}

// GenerateSummary creates a concise one-line summary
func GenerateSummary(result *ReviewResult) string {
	status := "PASSED"
	if !result.Passed {
		status = "FAILED"
	}

	return fmt.Sprintf("%s: Task %s (Risk: %s, Score: %.1f/100, Issues: P0:%d P1:%d P2:%d P3:%d)",
		status,
		result.TaskID,
		result.RiskLevel,
		result.AggregateScore,
		result.Metrics.P0Issues,
		result.Metrics.P1Issues,
		result.Metrics.P2Issues,
		result.Metrics.P3Issues,
	)
}
