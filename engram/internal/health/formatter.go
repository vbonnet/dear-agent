package health

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	outputformatter "github.com/vbonnet/dear-agent/pkg/output-formatter"
)

// Formatter handles output formatting for health check results
type Formatter struct {
	results  []CheckResult
	duration time.Duration
}

// NewFormatter creates a new formatter instance
func NewFormatter(results []CheckResult, duration time.Duration) *Formatter {
	return &Formatter{
		results:  results,
		duration: duration,
	}
}

// FormatDefault returns the default formatted output
func (f *Formatter) FormatDefault() string {
	var output strings.Builder

	// Categorize and render sections
	categories := f.categorizeResults()
	f.renderCheckSections(&output, categories)
	f.renderSummary(&output)
	f.renderFooter(&output)

	return output.String()
}

// categorizeResults groups check results by category
func (f *Formatter) categorizeResults() map[string][]CheckResult {
	categories := make(map[string][]CheckResult)
	for _, r := range f.results {
		categories[r.Category] = append(categories[r.Category], r)
	}
	return categories
}

// renderCheckSections renders all check category sections
func (f *Formatter) renderCheckSections(output *strings.Builder, categories map[string][]CheckResult) {
	f.renderCategorySection(output, "Core Infrastructure", categories["core"])
	f.renderCategorySection(output, "Dependencies", categories["dependency"])
	f.renderCategorySection(output, "Hooks", categories["hooks"])
	f.renderCategorySection(output, "Marketplace", categories["marketplace"])
	f.renderCategorySection(output, "Security", categories["security"])
}

// renderCategorySection renders a single category section
func (f *Formatter) renderCategorySection(output *strings.Builder, title string, checks []CheckResult) {
	if len(checks) > 0 {
		output.WriteString(title + ":\n")
		for _, check := range checks {
			output.WriteString(f.formatCheck(check))
		}
		output.WriteString("\n")
	}
}

// renderSummary renders the summary section using output-formatter
func (f *Formatter) renderSummary(output *strings.Builder) {
	// Convert results to output-formatter interface
	formatterResults := make([]outputformatter.Result, len(f.results))
	for i, r := range f.results {
		formatterResults[i] = r.AsFormatterResult()
	}

	// Use output-formatter library for consistent summary rendering
	iconMapper := outputformatter.NewIconMapper(false) // Use emoji icons
	summaryGen := outputformatter.NewSummaryGenerator(iconMapper)
	libSummary := summaryGen.Generate(formatterResults)

	output.WriteString("Summary:\n")
	output.WriteString(summaryGen.Format(libSummary))
	output.WriteString("\n")
}

// renderFooter renders the health status and auto-fix hint
func (f *Formatter) renderFooter(output *strings.Builder) {
	summary := f.GetSummary()
	status := f.getHealthStatus()
	fmt.Fprintf(output, "Health Status: %s\n", status)

	if summary.Warnings > 0 || summary.Failed > 0 {
		output.WriteString("Run 'engram doctor --auto-fix' to apply safe fixes\n")
	}
}

// FormatQuiet returns quiet formatted output (only issues)
func (f *Formatter) FormatQuiet() string {
	issues := f.GetIssues()
	if len(issues) == 0 {
		return "" // Silent if healthy
	}

	var output strings.Builder
	fmt.Fprintf(&output, "⚠️  Engram Health Issues (%d):\n", len(issues))

	for _, issue := range issues {
		fmt.Fprintf(&output, "  - %s", issue.Message)
		if issue.Fix != "" {
			fmt.Fprintf(&output, " (fix: %s)", issue.Fix)
		}
		output.WriteString("\n")
	}

	return output.String()
}

// FormatJSON returns JSON formatted output
func (f *Formatter) FormatJSON() (string, error) {
	// Separate core checks from plugin checks
	plugins := make(map[string]CheckResult)
	fixesAvailable := 0
	for _, r := range f.results {
		if r.Category == "plugin" {
			plugins[r.Name] = r
		}
		if r.Fix != "" {
			fixesAvailable++
		}
	}

	report := HealthReport{
		Timestamp:      time.Now().Format(time.RFC3339),
		Tool:           "engram",
		Command:        "doctor",
		Version:        "0.1.0-prototype",
		Status:         f.getHealthStatus(),
		Summary:        f.GetSummary(),
		Checks:         f.results,
		Plugins:        plugins,
		FixesAvailable: fixesAvailable,
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal JSON: %w", err)
	}

	return string(data), nil
}

// GetExitCode returns the appropriate exit code
func (f *Formatter) GetExitCode() int {
	summary := f.GetSummary()
	if summary.Failed > 0 {
		return 2 // Errors present
	}
	if summary.Warnings > 0 {
		return 1 // Warnings present
	}
	return 0 // Healthy
}

// GetSummary calculates aggregated statistics
func (f *Formatter) GetSummary() Summary {
	summary := Summary{Total: len(f.results)}
	for _, r := range f.results {
		switch r.Status {
		case "ok":
			summary.Passed++
		case "warning":
			summary.Warnings++
		case "error":
			summary.Failed++
		case "info":
			summary.Info++
		}
	}
	return summary
}

// GetIssues returns only warnings and errors
func (f *Formatter) GetIssues() []CheckResult {
	issues := []CheckResult{}
	for _, r := range f.results {
		if r.Status == "warning" || r.Status == "error" {
			issues = append(issues, r)
		}
	}
	return issues
}

// formatCheck formats a single check result using output-formatter icons
func (f *Formatter) formatCheck(check CheckResult) string {
	// Use output-formatter for consistent icon mapping
	iconMapper := outputformatter.NewIconMapper(false) // Use emoji icons
	icon := iconMapper.GetIcon(outputformatter.StatusLevel(check.Status))

	var output string
	if check.Message != "" {
		output = fmt.Sprintf("  %s %s\n", icon, check.Message)
		if check.Fix != "" {
			output += fmt.Sprintf("      Fix: %s\n", check.Fix)
		}
	} else {
		// Just show check passed
		output = fmt.Sprintf("  %s %s\n", icon, f.getCheckDisplayName(check.Name))
	}

	return output
}

// getCheckDisplayName converts check name to human-readable display name
func (f *Formatter) getCheckDisplayName(name string) string {
	switch name {
	case "workspace_exists":
		return "Workspace exists (~/.engram/)"
	case "config_valid":
		return "Config valid (config.yaml)"
	case "logs_directory_writable":
		return "Logs directory writable"
	case "logs_size":
		return "Log size reasonable"
	case "core_engrams_accessible":
		return "Core engrams accessible"
	case "cache_directory_writable":
		return "Cache directory writable"
	case "yq_available":
		return "yq available"
	case "jq_available":
		return "jq available"
	case "python_available":
		return "python3 available"
	case "hooks_configured":
		return "Hooks configured"
	case "hook_scripts_executable":
		return "Hook scripts executable"
	case "file_permissions":
		return "File permissions correct"
	case "marketplace_config_valid":
		return "Marketplace config valid"
	case "marketplace_plugins_available":
		return "Plugin marketplaces registered"
	default:
		return name
	}
}

// getHealthStatus returns overall health status
func (f *Formatter) getHealthStatus() string {
	summary := f.GetSummary()
	if summary.Failed > 0 {
		return "critical"
	}
	if summary.Warnings > 0 {
		return "degraded"
	}
	return "healthy"
}
