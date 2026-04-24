// Package enrichment provides event enrichment capabilities for telemetry.
//
// The enrichment layer adds contextual metadata to telemetry events through
// a decorator pattern, enabling retrospective analysis of plugin loading,
// version compatibility, and ecphory coverage.
//
// Architecture:
//   - Enricher interface: Core abstraction for event enrichment
//   - SafeEnricher wrapper: Timeout protection and graceful degradation
//   - CircuitBreaker: Fault isolation to prevent cascade failures
//
// Example usage:
//
//	enricher := NewPluginContextEnricher(pluginLoader)
//	safeEnricher := NewSafeEnricher(enricher, 500*time.Microsecond)
//
//	enrichedEvent, err := safeEnricher.Enrich(ctx, event, enrichmentContext)
//	if err != nil {
//	    // Enrichment failed, use original event
//	    log.Warn("Enrichment failed: %v", err)
//	    return event
//	}
package enrichment

import (
	"context"
	"time"
)

// Enricher enriches telemetry events with additional context
type Enricher interface {
	// Enrich adds contextual metadata to a telemetry event
	Enrich(ctx context.Context, event *TelemetryEvent, ec EnrichmentContext) (*TelemetryEvent, error)

	// Name returns the enricher name for logging and debugging
	Name() string
}

// TelemetryEvent represents a telemetry event to be enriched
type TelemetryEvent struct {
	// Event ID (UUID)
	ID string `json:"id"`

	// Event timestamp
	Timestamp time.Time `json:"timestamp"`

	// Event type (plugin_execution, ecphory_retrieval, session_end, etc.)
	Type string `json:"type"`

	// Agent platform (claude-code, cursor, etc.)
	Agent string `json:"agent"`

	// Schema version (1.0.0)
	SchemaVersion string `json:"schema_version"`

	// Event-specific data
	Data map[string]interface{} `json:"data,omitempty"`
}

// EnrichmentContext provides context for enrichment operations
type EnrichmentContext struct {
	// Prompt content (for pattern matching, not stored in telemetry)
	Prompt string

	// Available plugins (loaded in session)
	AvailablePlugins []Plugin

	// Loaded plugins (actually loaded for this prompt)
	LoadedPlugins []Plugin

	// Ecphory result (if applicable)
	EcphoryResult *EcphoryResult

	// Session salt (for privacy-preserving hashing)
	SessionSalt string
}

// Plugin represents a loaded plugin
type Plugin struct {
	// Plugin name
	Name string

	// Plugin version
	Version string

	// Plugin path
	Path string

	// Whether plugin is deprecated
	Deprecated bool
}

// EcphoryResult represents ecphory retrieval results
type EcphoryResult struct {
	// Engrams retrieved
	EngramsRetrieved int

	// Candidates filtered (before ranking)
	CandidatesFiltered int

	// Token budget used
	TokenBudgetUsed int

	// Retrieval strategy (api, inverted-tags, manual)
	Strategy string

	// Query prompt hash (privacy-preserving)
	PromptHash string
}

// Event type constants for enrichment
const (
	EventTypePluginExecution  = "plugin_execution"
	EventTypeEcphoryRetrieval = "ecphory_retrieval"
	EventTypeSessionEnd       = "session_end"
	EventTypeSanityCheck      = "sanity_check"
	EventTypeTelemetryHealth  = "telemetry_health"
)

// Schema version constant
const SchemaVersion = "1.0.0"
