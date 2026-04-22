package enrichment

import (
	"context"
)

// EcphoryCoverageEnricher enriches ecphory_retrieval events with coverage metadata
type EcphoryCoverageEnricher struct {
	// No shared mutable state - thread-safe
}

// NewEcphoryCoverageEnricher creates a new EcphoryCoverageEnricher
func NewEcphoryCoverageEnricher() *EcphoryCoverageEnricher {
	return &EcphoryCoverageEnricher{}
}

// Enrich adds ecphory coverage metadata to ecphory_retrieval events
func (e *EcphoryCoverageEnricher) Enrich(ctx context.Context, event *TelemetryEvent, ec EnrichmentContext) (*TelemetryEvent, error) {
	// Only enrich ecphory_retrieval events
	if event.Type != EventTypeEcphoryRetrieval {
		return event, nil
	}

	// Require ecphory result context
	if ec.EcphoryResult == nil {
		return event, nil
	}

	// Create copy of event to avoid modifying original
	enriched := *event
	if enriched.Data == nil {
		enriched.Data = make(map[string]interface{})
	}

	// Add ecphory metadata
	result := ec.EcphoryResult

	enriched.Data["prompt_hash"] = result.PromptHash
	enriched.Data["engrams_retrieved"] = result.EngramsRetrieved
	enriched.Data["candidates_filtered"] = result.CandidatesFiltered
	enriched.Data["token_budget_used"] = result.TokenBudgetUsed
	enriched.Data["retrieval_strategy"] = result.Strategy

	// Calculate token budget utilization percentage
	// Assuming a standard budget (e.g., 100k tokens)
	// This would be configurable in production
	const standardBudget = 100000
	if standardBudget > 0 {
		utilization := float64(result.TokenBudgetUsed) / float64(standardBudget) * 100.0
		enriched.Data["token_utilization_percent"] = utilization
	}

	return &enriched, nil
}

// Name returns the enricher name
func (e *EcphoryCoverageEnricher) Name() string {
	return "ecphory_coverage"
}
