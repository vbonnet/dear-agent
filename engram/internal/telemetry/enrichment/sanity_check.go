package enrichment

import (
	"context"
	"fmt"
	"time"
)

// CheckResult represents the result of a sanity check
type CheckResult struct {
	// Check type (plugin_loading, version_compatibility, ecphory_coverage)
	CheckType string

	// Result status (pass, warn, fail)
	Status string

	// Human-readable message
	Message string

	// Actionable recommendations
	Recommendations []string

	// Additional context data
	Context map[string]interface{}
}

// Check result status constants
const (
	StatusPass = "pass"
	StatusWarn = "warn"
	StatusFail = "fail"
)

// Check type constants
const (
	CheckTypePluginLoading        = "plugin_loading"
	CheckTypeVersionCompatibility = "version_compatibility"
	CheckTypeEcphoryCoverage      = "ecphory_coverage"
)

// SanityCheckEnricher runs sanity checks on session_end events and emits separate sanity_check events
type SanityCheckEnricher struct {
	// eventEmitter is a callback to emit new sanity_check events
	eventEmitter func(ctx context.Context, event *TelemetryEvent) error
}

// NewSanityCheckEnricher creates a new SanityCheckEnricher
func NewSanityCheckEnricher(eventEmitter func(ctx context.Context, event *TelemetryEvent) error) *SanityCheckEnricher {
	return &SanityCheckEnricher{
		eventEmitter: eventEmitter,
	}
}

// Enrich runs sanity checks on session_end events
func (s *SanityCheckEnricher) Enrich(ctx context.Context, event *TelemetryEvent, ec EnrichmentContext) (*TelemetryEvent, error) {
	// Only run on session_end events
	if event.Type != EventTypeSessionEnd {
		return event, nil
	}

	// Run sanity checks
	checkResults := runSanityChecks(ec)

	// Emit separate sanity_check event for each check
	for _, result := range checkResults {
		checkEvent := &TelemetryEvent{
			ID:            generateEventID(), // Would use UUID in production
			Timestamp:     time.Now(),
			Type:          EventTypeSanityCheck,
			Agent:         event.Agent,
			SchemaVersion: SchemaVersion,
			Data: map[string]interface{}{
				"check_type":      result.CheckType,
				"status":          result.Status,
				"message":         result.Message,
				"recommendations": result.Recommendations,
				"context":         result.Context,
			},
		}

		// Emit sanity check event
		if s.eventEmitter != nil {
			if err := s.eventEmitter(ctx, checkEvent); err != nil {
				// Log error but don't fail enrichment
				fmt.Printf("WARN: Failed to emit sanity check event: %v\n", err)
			}
		}
	}

	// Return original event unchanged (sanity checks emitted as separate events)
	return event, nil
}

// Name returns the enricher name
func (s *SanityCheckEnricher) Name() string {
	return "sanity_check"
}

// runSanityChecks executes all sanity checks and returns results
func runSanityChecks(ec EnrichmentContext) []CheckResult {
	results := make([]CheckResult, 0)

	// Check 1: Plugin Loading
	pluginLoadingCheck := checkPluginLoading(ec)
	results = append(results, pluginLoadingCheck)

	// Check 2: Version Compatibility
	versionCheck := checkVersionCompatibility(ec)
	results = append(results, versionCheck)

	// Check 3: Ecphory Coverage (if ecphory result available)
	if ec.EcphoryResult != nil {
		ecphoryCheck := checkEcphoryCoverage(ec)
		results = append(results, ecphoryCheck)
	}

	return results
}

// checkPluginLoading checks if expected plugins were loaded
func checkPluginLoading(ec EnrichmentContext) CheckResult {
	expectedPlugins := detectExpectedPlugins(ec.Prompt, ec.AvailablePlugins)
	loadedPluginNames := make([]string, len(ec.LoadedPlugins))
	for i, plugin := range ec.LoadedPlugins {
		loadedPluginNames[i] = plugin.Name
	}

	missingPlugins := calculateMissingPlugins(expectedPlugins, loadedPluginNames)

	if len(missingPlugins) == 0 {
		return CheckResult{
			CheckType: CheckTypePluginLoading,
			Status:    StatusPass,
			Message:   "All expected plugins loaded successfully",
			Context: map[string]interface{}{
				"expected_plugins": expectedPlugins,
				"loaded_plugins":   loadedPluginNames,
			},
		}
	}

	return CheckResult{
		CheckType: CheckTypePluginLoading,
		Status:    StatusFail,
		Message:   fmt.Sprintf("%d expected plugin(s) not loaded: %v", len(missingPlugins), missingPlugins),
		Recommendations: []string{
			fmt.Sprintf("Check if plugins are enabled: %v", missingPlugins),
			"Verify plugin.yaml configuration",
			"Review plugin loading logs for errors",
		},
		Context: map[string]interface{}{
			"expected_plugins": expectedPlugins,
			"loaded_plugins":   loadedPluginNames,
			"missing_plugins":  missingPlugins,
		},
	}
}

// checkVersionCompatibility checks if any loaded plugins are deprecated
func checkVersionCompatibility(ec EnrichmentContext) CheckResult {
	deprecatedPlugins := make([]string, 0)

	for _, plugin := range ec.LoadedPlugins {
		if plugin.Deprecated {
			deprecatedPlugins = append(deprecatedPlugins, fmt.Sprintf("%s@%s", plugin.Name, plugin.Version))
		}
	}

	if len(deprecatedPlugins) == 0 {
		return CheckResult{
			CheckType: CheckTypeVersionCompatibility,
			Status:    StatusPass,
			Message:   "All plugin versions are current",
			Context: map[string]interface{}{
				"loaded_plugins": len(ec.LoadedPlugins),
			},
		}
	}

	return CheckResult{
		CheckType: CheckTypeVersionCompatibility,
		Status:    StatusWarn,
		Message:   fmt.Sprintf("%d deprecated plugin(s) in use: %v", len(deprecatedPlugins), deprecatedPlugins),
		Recommendations: []string{
			"Update deprecated plugins to latest versions",
			"Check plugin manifest for version compatibility",
			"Review migration guide for deprecated plugins",
		},
		Context: map[string]interface{}{
			"deprecated_plugins": deprecatedPlugins,
			"total_plugins":      len(ec.LoadedPlugins),
		},
	}
}

// checkEcphoryCoverage checks if ecphory token utilization is healthy
func checkEcphoryCoverage(ec EnrichmentContext) CheckResult {
	if ec.EcphoryResult == nil {
		return CheckResult{
			CheckType: CheckTypeEcphoryCoverage,
			Status:    StatusPass,
			Message:   "No ecphory retrieval in this session",
		}
	}

	// Calculate token utilization percentage
	const standardBudget = 100000
	utilization := float64(ec.EcphoryResult.TokenBudgetUsed) / float64(standardBudget) * 100.0

	// Warn if utilization >80% (indicates potential retrieval coverage issues)
	if utilization > 80.0 {
		return CheckResult{
			CheckType: CheckTypeEcphoryCoverage,
			Status:    StatusWarn,
			Message:   fmt.Sprintf("High token budget utilization: %.1f%%", utilization),
			Recommendations: []string{
				"Consider refining engram selection criteria",
				"Review token budget limits",
				"Check if too many candidates are being ranked",
			},
			Context: map[string]interface{}{
				"token_utilization_percent": utilization,
				"engrams_retrieved":         ec.EcphoryResult.EngramsRetrieved,
				"candidates_filtered":       ec.EcphoryResult.CandidatesFiltered,
				"strategy":                  ec.EcphoryResult.Strategy,
			},
		}
	}

	return CheckResult{
		CheckType: CheckTypeEcphoryCoverage,
		Status:    StatusPass,
		Message:   fmt.Sprintf("Token budget utilization healthy: %.1f%%", utilization),
		Context: map[string]interface{}{
			"token_utilization_percent": utilization,
			"engrams_retrieved":         ec.EcphoryResult.EngramsRetrieved,
			"strategy":                  ec.EcphoryResult.Strategy,
		},
	}
}

// generateEventID generates a unique event ID (placeholder - would use UUID in production)
var generateEventID = func() string {
	return fmt.Sprintf("evt-%d", time.Now().UnixNano())
}
