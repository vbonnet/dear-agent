// Package hooks provides hooks-related functionality.
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// Aggregator collects and combines results from multiple verification hooks
type Aggregator struct {
	executor HookExecutor
}

// NewAggregator creates a new report aggregator
func NewAggregator(executor HookExecutor) *Aggregator {
	return &Aggregator{
		executor: executor,
	}
}

// AggregateResults runs all hooks for an event and aggregates results
func (a *Aggregator) AggregateResults(ctx context.Context, event HookEvent, hooks []Hook) (*AggregatedReport, error) {
	report := &AggregatedReport{
		Timestamp: time.Now(),
		Event:     event,
		Results:   make([]VerificationResult, 0),
		Warnings:  make([]HookWarning, 0),
	}

	// Sort hooks by priority (higher priority first)
	sortedHooks := make([]Hook, len(hooks))
	copy(sortedHooks, hooks)
	sort.Slice(sortedHooks, func(i, j int) bool {
		return sortedHooks[i].Priority > sortedHooks[j].Priority
	})

	// Execute each hook
	for _, hook := range sortedHooks {
		result, err := a.executor.Execute(ctx, hook)
		if err != nil {
			// Graceful degradation - log warning and continue
			warning := HookWarning{
				Hook:    hook.Name,
				Message: fmt.Sprintf("Hook execution failed: %v", err),
			}
			report.Warnings = append(report.Warnings, warning)
			continue
		}

		if result != nil {
			report.Results = append(report.Results, *result)
		}
	}

	// Calculate summary
	report.Summary = a.calculateSummary(report)

	return report, nil
}

// calculateSummary computes aggregated statistics
func (a *Aggregator) calculateSummary(report *AggregatedReport) Summary {
	summary := Summary{
		TotalHooks: len(report.Results) + len(report.Warnings),
	}

	for _, result := range report.Results {
		switch result.Status {
		case VerificationStatusPass:
			summary.PassedHooks++
		case VerificationStatusFail:
			summary.FailedHooks++
		case VerificationStatusWarning:
			summary.WarningHooks++
		}
		summary.TotalViolations += len(result.Violations)
	}

	summary.WarningHooks += len(report.Warnings)

	// Determine exit code
	switch {
	case summary.FailedHooks > 0:
		summary.ExitCode = 1 // Fail
	case summary.WarningHooks > 0:
		summary.ExitCode = 2 // Warnings
	default:
		summary.ExitCode = 0 // Pass
	}

	return summary
}

// FormatTerminal formats the report for terminal display with colors
//nolint:gocyclo // reason: linear formatter assembling many subsections of terminal output
func (a *Aggregator) FormatTerminal(report *AggregatedReport) string {
	var buf bytes.Buffer

	// Header
	fmt.Fprintf(&buf, "Verification Report - %s\n", report.Event)
	fmt.Fprintf(&buf, "Timestamp: %s\n\n", report.Timestamp.Format(time.RFC3339))

	// Group violations by severity
	highViolations := make([]violationWithHook, 0)
	mediumViolations := make([]violationWithHook, 0)
	lowViolations := make([]violationWithHook, 0)

	for _, result := range report.Results {
		for _, v := range result.Violations {
			vwh := violationWithHook{
				hook:      result.HookName,
				violation: v,
			}
			switch v.Severity {
			case "high":
				highViolations = append(highViolations, vwh)
			case "medium":
				mediumViolations = append(mediumViolations, vwh)
			case "low":
				lowViolations = append(lowViolations, vwh)
			}
		}
	}

	// Display violations by severity
	if len(highViolations) > 0 {
		buf.WriteString(colorRed("HIGH SEVERITY VIOLATIONS:\n"))
		for i, vwh := range highViolations {
			fmt.Fprintf(&buf, "\n%d. [%s] %s\n", i+1, vwh.hook, vwh.violation.Message)
			if len(vwh.violation.Files) > 0 {
				fmt.Fprintf(&buf, "   Files: %v\n", vwh.violation.Files)
			}
			if vwh.violation.Suggestion != "" {
				fmt.Fprintf(&buf, "   Fix: %s\n", vwh.violation.Suggestion)
			}
		}
		buf.WriteString("\n")
	}

	if len(mediumViolations) > 0 {
		buf.WriteString(colorYellow("MEDIUM SEVERITY VIOLATIONS:\n"))
		for i, vwh := range mediumViolations {
			fmt.Fprintf(&buf, "\n%d. [%s] %s\n", i+1, vwh.hook, vwh.violation.Message)
			if len(vwh.violation.Files) > 0 {
				fmt.Fprintf(&buf, "   Files: %v\n", vwh.violation.Files)
			}
			if vwh.violation.Suggestion != "" {
				fmt.Fprintf(&buf, "   Fix: %s\n", vwh.violation.Suggestion)
			}
		}
		buf.WriteString("\n")
	}

	if len(lowViolations) > 0 {
		buf.WriteString("LOW SEVERITY VIOLATIONS:\n")
		for i, vwh := range lowViolations {
			fmt.Fprintf(&buf, "\n%d. [%s] %s\n", i+1, vwh.hook, vwh.violation.Message)
			if len(vwh.violation.Files) > 0 {
				fmt.Fprintf(&buf, "   Files: %v\n", vwh.violation.Files)
			}
		}
		buf.WriteString("\n")
	}

	// Display warnings
	if len(report.Warnings) > 0 {
		buf.WriteString(colorYellow("WARNINGS:\n"))
		for _, w := range report.Warnings {
			fmt.Fprintf(&buf, "  - [%s] %s\n", w.Hook, w.Message)
		}
		buf.WriteString("\n")
	}

	// Summary
	buf.WriteString("SUMMARY:\n")
	fmt.Fprintf(&buf, "  Total Hooks: %d\n", report.Summary.TotalHooks)
	fmt.Fprintf(&buf, "  Passed: %s\n", colorGreen(fmt.Sprintf("%d", report.Summary.PassedHooks)))
	fmt.Fprintf(&buf, "  Failed: %s\n", colorRed(fmt.Sprintf("%d", report.Summary.FailedHooks)))
	fmt.Fprintf(&buf, "  Warnings: %s\n", colorYellow(fmt.Sprintf("%d", report.Summary.WarningHooks)))
	fmt.Fprintf(&buf, "  Total Violations: %d\n", report.Summary.TotalViolations)
	fmt.Fprintf(&buf, "  Exit Code: %d\n", report.Summary.ExitCode)

	return buf.String()
}

// FormatMarkdown formats the report as markdown
//nolint:gocyclo // reason: linear formatter assembling many subsections of markdown output
func (a *Aggregator) FormatMarkdown(report *AggregatedReport) string {
	var buf bytes.Buffer

	// Header
	fmt.Fprintf(&buf, "# Verification Report - %s\n\n", report.Event)
	fmt.Fprintf(&buf, "**Timestamp:** %s\n\n", report.Timestamp.Format(time.RFC3339))

	// Group violations by severity
	highViolations := make([]violationWithHook, 0)
	mediumViolations := make([]violationWithHook, 0)
	lowViolations := make([]violationWithHook, 0)

	for _, result := range report.Results {
		for _, v := range result.Violations {
			vwh := violationWithHook{
				hook:      result.HookName,
				violation: v,
			}
			switch v.Severity {
			case "high":
				highViolations = append(highViolations, vwh)
			case "medium":
				mediumViolations = append(mediumViolations, vwh)
			case "low":
				lowViolations = append(lowViolations, vwh)
			}
		}
	}

	// Display violations by severity
	if len(highViolations) > 0 {
		buf.WriteString("## High Severity Violations\n\n")
		for i, vwh := range highViolations {
			fmt.Fprintf(&buf, "%d. **[%s]** %s\n", i+1, vwh.hook, vwh.violation.Message)
			if len(vwh.violation.Files) > 0 {
				fmt.Fprintf(&buf, "   - **Files:** `%v`\n", vwh.violation.Files)
			}
			if vwh.violation.Suggestion != "" {
				fmt.Fprintf(&buf, "   - **Fix:** %s\n", vwh.violation.Suggestion)
			}
			buf.WriteString("\n")
		}
	}

	if len(mediumViolations) > 0 {
		buf.WriteString("## Medium Severity Violations\n\n")
		for i, vwh := range mediumViolations {
			fmt.Fprintf(&buf, "%d. **[%s]** %s\n", i+1, vwh.hook, vwh.violation.Message)
			if len(vwh.violation.Files) > 0 {
				fmt.Fprintf(&buf, "   - **Files:** `%v`\n", vwh.violation.Files)
			}
			if vwh.violation.Suggestion != "" {
				fmt.Fprintf(&buf, "   - **Fix:** %s\n", vwh.violation.Suggestion)
			}
			buf.WriteString("\n")
		}
	}

	if len(lowViolations) > 0 {
		buf.WriteString("## Low Severity Violations\n\n")
		for i, vwh := range lowViolations {
			fmt.Fprintf(&buf, "%d. **[%s]** %s\n", i+1, vwh.hook, vwh.violation.Message)
			if len(vwh.violation.Files) > 0 {
				fmt.Fprintf(&buf, "   - **Files:** `%v`\n", vwh.violation.Files)
			}
			buf.WriteString("\n")
		}
	}

	// Summary
	buf.WriteString("## Summary\n\n")
	fmt.Fprintf(&buf, "- **Total Hooks:** %d\n", report.Summary.TotalHooks)
	fmt.Fprintf(&buf, "- **Passed:** %d\n", report.Summary.PassedHooks)
	fmt.Fprintf(&buf, "- **Failed:** %d\n", report.Summary.FailedHooks)
	fmt.Fprintf(&buf, "- **Warnings:** %d\n", report.Summary.WarningHooks)
	fmt.Fprintf(&buf, "- **Total Violations:** %d\n", report.Summary.TotalViolations)
	fmt.Fprintf(&buf, "- **Exit Code:** %d\n", report.Summary.ExitCode)

	return buf.String()
}

// FormatJSON formats the report as JSON
func (a *Aggregator) FormatJSON(report *AggregatedReport) (string, error) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return string(data), nil
}

// violationWithHook pairs a violation with its source hook
type violationWithHook struct {
	hook      string
	violation Violation
}

// Terminal color helpers
func colorRed(s string) string {
	return fmt.Sprintf("\033[31m%s\033[0m", s)
}

func colorYellow(s string) string {
	return fmt.Sprintf("\033[33m%s\033[0m", s)
}

func colorGreen(s string) string {
	return fmt.Sprintf("\033[32m%s\033[0m", s)
}
