package health

import (
	"context"
	"os"
	"path/filepath"

	outputformatter "github.com/vbonnet/dear-agent/pkg/output-formatter"
)

// CheckResult represents the outcome of a single health check
type CheckResult struct {
	Name     string `json:"name"`              // Check identifier (e.g., "workspace_exists")
	Category string `json:"category"`          // "core", "plugin", "dependency"
	Status   string `json:"status"`            // "ok", "warning", "error", "info"
	Message  string `json:"message,omitempty"` // Human-readable message (empty if ok)
	Fix      string `json:"fix,omitempty"`     // Command to fix the issue
}

// AsFormatterResult converts CheckResult to outputformatter.Result interface
func (c *CheckResult) AsFormatterResult() outputformatter.Result {
	return checkResultAdapter{c}
}

// checkResultAdapter adapts CheckResult to implement outputformatter.Result
type checkResultAdapter struct {
	result *CheckResult
}

func (a checkResultAdapter) Status() outputformatter.StatusLevel {
	return outputformatter.StatusLevel(a.result.Status)
}

func (a checkResultAdapter) Message() string {
	if a.result.Message != "" {
		return a.result.Message
	}
	// Return check name as fallback for passed checks
	return a.result.Name
}

func (a checkResultAdapter) Category() string {
	return a.result.Category
}

// HealthChecker runs health checks for Engram infrastructure
type HealthChecker struct {
	workspace string // Path to ~/.engram
	ctx       context.Context
}

// NewHealthChecker creates a new health checker instance
func NewHealthChecker(ctx context.Context) *HealthChecker {
	workspace := filepath.Join(os.Getenv("HOME"), ".engram")
	return &HealthChecker{
		workspace: workspace,
		ctx:       ctx,
	}
}

// Summary represents aggregated health check statistics
type Summary struct {
	Total    int `json:"total"`
	Passed   int `json:"passed"`
	Warnings int `json:"warnings"`
	Failed   int `json:"failed"` // renamed from "errors"
	Info     int `json:"info,omitempty"`
}

// HealthReport represents the complete health check results
type HealthReport struct {
	Timestamp      string                 `json:"timestamp"`
	Tool           string                 `json:"tool"`    // "engram"
	Command        string                 `json:"command"` // "doctor"
	Version        string                 `json:"version"`
	Status         string                 `json:"status"` // "healthy", "degraded", "critical"
	Summary        Summary                `json:"summary"`
	Checks         []CheckResult          `json:"checks"`
	Plugins        map[string]CheckResult `json:"plugins,omitempty"`
	FixesAvailable int                    `json:"fixes_available"`
}

// HealthCheckCache represents the cached health check results
type HealthCheckCache struct {
	Version   string                 `json:"version"`   // Cache schema version
	Timestamp string                 `json:"timestamp"` // ISO 8601 (RFC3339)
	TTL       int                    `json:"ttl"`       // Time-to-live in seconds
	Checks    map[string]CheckResult `json:"checks"`    // Core checks
	Plugins   map[string]CheckResult `json:"plugins"`   // Plugin checks
	Summary   Summary                `json:"summary"`   // Aggregated stats
}

// HealthCheckLog represents a JSONL log entry
type HealthCheckLog struct {
	Timestamp      string        `json:"timestamp"`
	SessionID      string        `json:"session_id"`
	Trigger        string        `json:"trigger"` // "manual", "session_start", "scheduled"
	DurationMS     int64         `json:"duration_ms"`
	ChecksRun      int           `json:"checks_run"`
	PluginsChecked int           `json:"plugins_checked"`
	Summary        Summary       `json:"summary"`
	Issues         []CheckResult `json:"issues"` // Only warnings/errors
	FixesApplied   int           `json:"fixes_applied"`
	CacheUsed      bool          `json:"cache_used"`
	Version        string        `json:"version"`
}

// Workspace returns the workspace path
func (hc *HealthChecker) Workspace() string {
	return hc.workspace
}

// Context returns the context
func (hc *HealthChecker) Context() context.Context {
	return hc.ctx
}
