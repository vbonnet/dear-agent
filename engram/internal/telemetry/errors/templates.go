// Package errors provides tiered error messages for telemetry operations.
//
// This package implements a dual-template system for error messages:
//   - Technical messages: For developers/power users (paths, hashes, debug commands)
//   - Simple messages: For non-technical users (plain language, actionable steps)
//
// User type detection is automatic based on:
//   - Presence of ~/.engram/config.yml (technical user)
//   - CLI flag usage patterns (technical user)
//   - Default: simple (for alpha users)
//
// Example usage:
//
//	err := errors.PluginLoadingFailed(
//	    ctx,
//	    []string{"research", "personas"},
//	    "Plugin manifest not found: ~/.engram/plugins/research/plugin.yaml",
//	)
//	fmt.Println(err.Render())
package errors

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UserType represents the detected user expertise level
type UserType int

// User expertise level values.
const (
	UserTypeSimple UserType = iota
	UserTypeTechnical
)

// TelemetryError represents a telemetry operation error with tiered messages
type TelemetryError struct {
	Category         string
	TechnicalMsg     string
	SimpleMsg        string
	Recommendations  []string
	DebugCommand     string
	LearnMoreURL     string
	Context          map[string]interface{}
	DetectedUserType UserType
}

// Render renders the error message based on detected user type
func (e *TelemetryError) Render() string {
	if e.DetectedUserType == UserTypeTechnical {
		return e.renderTechnical()
	}
	return e.renderSimple()
}

// renderTechnical renders a technical error message
//
// Format:
//   - Problem statement
//   - Technical context (hashes, paths, configs)
//   - Debug command
//   - Learn more URL
func (e *TelemetryError) renderTechnical() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "[%s] %s\n\n", e.Category, e.TechnicalMsg)

	if len(e.Context) > 0 {
		sb.WriteString("Context:\n")
		for k, v := range e.Context {
			fmt.Fprintf(&sb, "  %s: %v\n", k, v)
		}
		sb.WriteString("\n")
	}

	if len(e.Recommendations) > 0 {
		sb.WriteString("Recommendations:\n")
		for i, rec := range e.Recommendations {
			fmt.Fprintf(&sb, "  %d. %s\n", i+1, rec)
		}
		sb.WriteString("\n")
	}

	if e.DebugCommand != "" {
		fmt.Fprintf(&sb, "Debug: %s\n", e.DebugCommand)
	}

	if e.LearnMoreURL != "" {
		fmt.Fprintf(&sb, "Learn more: %s\n", e.LearnMoreURL)
	}

	return sb.String()
}

// renderSimple renders a simple, non-technical error message
//
// Format:
//   - Plain language problem
//   - Why it matters
//   - Actionable steps (non-technical)
//   - Help contact
func (e *TelemetryError) renderSimple() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "%s\n\n", e.SimpleMsg)

	if len(e.Recommendations) > 0 {
		sb.WriteString("What you can do:\n")
		for i, rec := range e.Recommendations {
			// Simplify technical recommendations
			simplified := simplifyRecommendation(rec)
			fmt.Fprintf(&sb, "  %d. %s\n", i+1, simplified)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Need help? Contact your team's Engram administrator.\n")

	return sb.String()
}

// simplifyRecommendation converts technical recommendations to plain language
func simplifyRecommendation(rec string) string {
	// Simplify technical terms (check exact matches first to avoid double replacement)
	replacements := []struct {
		old string
		new string
	}{
		{"Check if plugins are enabled:", "Make sure the following plugins are enabled:"},
		{"Verify plugin.yaml configuration", "Check that plugins are properly configured"},
		{"Review plugin loading logs", "Check the error logs"},
		{"Check token budget limits", "Review retrieval settings"},
		{"Refine selection criteria", "Adjust your search filters"},
		{"Update plugins to latest version", "Update your plugins"},
		{"Check plugin manifest", "Check plugin settings"},
		{"Review migration guide", "See the update guide"},
	}

	for _, r := range replacements {
		if strings.Contains(rec, r.old) {
			rec = strings.ReplaceAll(rec, r.old, r.new)
			// After exact match replacement, skip file path cleanup to avoid double replacement
			return rec
		}
	}

	// Remove file paths (only if no exact match was found)
	rec = strings.ReplaceAll(rec, "~/.engram/", "")
	rec = strings.ReplaceAll(rec, "plugin.yaml", "plugin configuration")

	return rec
}

// ctxKey is a private type for context keys to avoid collisions across packages.
type ctxKey string

// CLIFlagsKey is the context key used to mark CLI-flag presence for user-type detection.
const CLIFlagsKey ctxKey = "cli_flags"

// DetectUserType detects whether the user is technical or non-technical
//
// Detection heuristics:
//  1. Check if ~/.engram/config.yml exists → technical
//  2. Check if context has CLI flags (from command) → technical
//  3. Default: simple (for alpha)
func DetectUserType(ctx context.Context) UserType {
	// Check for config file (technical users create this)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		configPath := filepath.Join(homeDir, ".engram", "config.yml")
		if _, err := os.Stat(configPath); err == nil {
			return UserTypeTechnical
		}
	}

	// Check context for CLI flag usage
	if ctx.Value(CLIFlagsKey) != nil {
		return UserTypeTechnical
	}

	// Default to simple for alpha users
	return UserTypeSimple
}

// PluginLoadingFailed creates an error for plugin loading failure
func PluginLoadingFailed(ctx context.Context, missingPlugins []string, technicalReason string) *TelemetryError {
	return &TelemetryError{
		Category: "Plugin Loading",
		TechnicalMsg: fmt.Sprintf("Failed to load expected plugins: %v. Reason: %s",
			missingPlugins, technicalReason),
		SimpleMsg: fmt.Sprintf("Some plugins didn't load: %s. Your Engram experience may be limited.",
			strings.Join(missingPlugins, ", ")),
		Recommendations: []string{
			fmt.Sprintf("Check if plugins are enabled: %v", missingPlugins),
			"Verify plugin.yaml configuration",
			"Review plugin loading logs for errors",
		},
		DebugCommand:     "engram telemetry health",
		LearnMoreURL:     "https://docs.engram.ai/troubleshooting/plugins",
		DetectedUserType: DetectUserType(ctx),
		Context: map[string]interface{}{
			"missing_plugins": missingPlugins,
		},
	}
}

// VersionCompatibilityWarning creates a warning for deprecated plugin versions
func VersionCompatibilityWarning(ctx context.Context, plugin string, currentVersion string, latestVersion string) *TelemetryError {
	return &TelemetryError{
		Category: "Version Compatibility",
		TechnicalMsg: fmt.Sprintf("Plugin %s version %s is deprecated (latest: %s)",
			plugin, currentVersion, latestVersion),
		SimpleMsg: fmt.Sprintf("Plugin %s needs updating (you have %s, latest is %s)",
			plugin, currentVersion, latestVersion),
		Recommendations: []string{
			fmt.Sprintf("Update %s plugin to version %s", plugin, latestVersion),
			"Check plugin manifest for breaking changes",
			"Review migration guide before updating",
		},
		DebugCommand:     fmt.Sprintf("engram plugin update %s", plugin),
		LearnMoreURL:     fmt.Sprintf("https://docs.engram.ai/plugins/%s/changelog", plugin),
		DetectedUserType: DetectUserType(ctx),
		Context: map[string]interface{}{
			"plugin":          plugin,
			"current_version": currentVersion,
			"latest_version":  latestVersion,
		},
	}
}

// EcphoryCoverageWarning creates a warning for high token utilization
func EcphoryCoverageWarning(ctx context.Context, utilizationPercent float64, tokenBudgetUsed int) *TelemetryError {
	return &TelemetryError{
		Category: "Ecphory Coverage",
		TechnicalMsg: fmt.Sprintf("Token utilization at %.1f%% (%d tokens used). Consider refining selection criteria.",
			utilizationPercent, tokenBudgetUsed),
		SimpleMsg: fmt.Sprintf("Your search is using a lot of resources (%.0f%% of available). Results may be limited.",
			utilizationPercent),
		Recommendations: []string{
			"Refine selection criteria to reduce token usage",
			"Check token budget limits in configuration",
			"Review ecphory retrieval strategy",
		},
		DebugCommand:     "engram telemetry analyze --week $(date +%Y-W%U)",
		LearnMoreURL:     "https://docs.engram.ai/ecphory/tuning",
		DetectedUserType: DetectUserType(ctx),
		Context: map[string]interface{}{
			"utilization_percent": utilizationPercent,
			"token_budget_used":   tokenBudgetUsed,
		},
	}
}

// StorageNearLimit creates a warning when telemetry storage is near limit
func StorageNearLimit(ctx context.Context, currentSize int64, maxSize int64) *TelemetryError {
	utilizationPercent := float64(currentSize) / float64(maxSize) * 100.0

	return &TelemetryError{
		Category: "Storage",
		TechnicalMsg: fmt.Sprintf("Telemetry storage at %.1f%% (%d/%d MB). Auto-rotation may occur.",
			utilizationPercent, currentSize/(1024*1024), maxSize/(1024*1024)),
		SimpleMsg: fmt.Sprintf("Storage is getting full (%.0f%% used). Old data may be automatically removed.",
			utilizationPercent),
		Recommendations: []string{
			"Review and clean up old telemetry files manually",
			"Adjust auto-rotation settings if needed",
			"Consider archiving important telemetry data",
		},
		DebugCommand:     "du -sh ~/.engram/telemetry/",
		LearnMoreURL:     "https://docs.engram.ai/telemetry/storage",
		DetectedUserType: DetectUserType(ctx),
		Context: map[string]interface{}{
			"current_size_mb":     currentSize / (1024 * 1024),
			"max_size_mb":         maxSize / (1024 * 1024),
			"utilization_percent": utilizationPercent,
		},
	}
}

// TelemetryDisabled creates an informational message when telemetry is disabled
func TelemetryDisabled(ctx context.Context) *TelemetryError {
	return &TelemetryError{
		Category:     "Telemetry",
		TechnicalMsg: "Telemetry is currently disabled. Enable in ~/.engram/config.yml (telemetry.enabled: true)",
		SimpleMsg:    "Telemetry is turned off. Usage insights and health checks are unavailable.",
		Recommendations: []string{
			"Enable telemetry to get weekly usage reports",
			"Health checks require telemetry to be enabled",
			"All data is stored locally on your machine",
		},
		DebugCommand:     "engram telemetry status",
		LearnMoreURL:     "https://docs.engram.ai/telemetry/getting-started",
		DetectedUserType: DetectUserType(ctx),
		Context:          map[string]interface{}{},
	}
}

// ParseError creates an error for JSONL parsing failure
func ParseError(ctx context.Context, filePath string, lineNum int, reason string) *TelemetryError {
	return &TelemetryError{
		Category:     "Parsing",
		TechnicalMsg: fmt.Sprintf("Failed to parse %s at line %d: %s", filePath, lineNum, reason),
		SimpleMsg:    fmt.Sprintf("Couldn't read telemetry data (file may be corrupted at line %d)", lineNum),
		Recommendations: []string{
			"Check if telemetry file is corrupted",
			"Try running telemetry health check",
			"Consider regenerating telemetry data",
		},
		DebugCommand:     fmt.Sprintf("tail -n +%d %s | head -5", lineNum-2, filePath),
		LearnMoreURL:     "https://docs.engram.ai/troubleshooting/telemetry",
		DetectedUserType: DetectUserType(ctx),
		Context: map[string]interface{}{
			"file_path": filePath,
			"line_num":  lineNum,
			"reason":    reason,
		},
	}
}
