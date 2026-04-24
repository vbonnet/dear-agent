package enrichment

import (
	"context"
	"testing"
)

func TestEcphoryCoverageEnricher_EnrichEcphoryRetrieval(t *testing.T) {
	enricher := NewEcphoryCoverageEnricher()

	event := &TelemetryEvent{
		ID:   "test-id",
		Type: EventTypeEcphoryRetrieval,
	}

	ecphoryResult := &EcphoryResult{
		PromptHash:         "abc123",
		EngramsRetrieved:   15,
		CandidatesFiltered: 50,
		TokenBudgetUsed:    25000,
		Strategy:           "api",
	}

	ec := EnrichmentContext{
		EcphoryResult: ecphoryResult,
	}

	enrichedEvent, err := enricher.Enrich(context.Background(), event, ec)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// Check prompt hash
	if enrichedEvent.Data["prompt_hash"] != "abc123" {
		t.Errorf("Expected prompt_hash 'abc123', got: %v", enrichedEvent.Data["prompt_hash"])
	}

	// Check engrams retrieved
	if enrichedEvent.Data["engrams_retrieved"] != 15 {
		t.Errorf("Expected engrams_retrieved 15, got: %v", enrichedEvent.Data["engrams_retrieved"])
	}

	// Check candidates filtered
	if enrichedEvent.Data["candidates_filtered"] != 50 {
		t.Errorf("Expected candidates_filtered 50, got: %v", enrichedEvent.Data["candidates_filtered"])
	}

	// Check token budget used
	if enrichedEvent.Data["token_budget_used"] != 25000 {
		t.Errorf("Expected token_budget_used 25000, got: %v", enrichedEvent.Data["token_budget_used"])
	}

	// Check retrieval strategy
	if enrichedEvent.Data["retrieval_strategy"] != "api" {
		t.Errorf("Expected retrieval_strategy 'api', got: %v", enrichedEvent.Data["retrieval_strategy"])
	}

	// Check token utilization percentage (25000 / 100000 = 25%)
	utilization, ok := enrichedEvent.Data["token_utilization_percent"].(float64)
	if !ok {
		t.Fatalf("Expected token_utilization_percent to be float64, got: %T", enrichedEvent.Data["token_utilization_percent"])
	}

	if utilization != 25.0 {
		t.Errorf("Expected token_utilization_percent 25.0, got: %v", utilization)
	}
}

func TestEcphoryCoverageEnricher_NonEcphoryEvent(t *testing.T) {
	enricher := NewEcphoryCoverageEnricher()

	event := &TelemetryEvent{
		ID:   "test-id",
		Type: "other_event_type",
	}

	enrichedEvent, err := enricher.Enrich(context.Background(), event, EnrichmentContext{})
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// Non-ecphory_retrieval events should not be enriched
	if enrichedEvent.Data != nil && len(enrichedEvent.Data) > 0 {
		t.Errorf("Expected no enrichment for non-ecphory_retrieval event, got: %v", enrichedEvent.Data)
	}
}

func TestEcphoryCoverageEnricher_MissingEcphoryResult(t *testing.T) {
	enricher := NewEcphoryCoverageEnricher()

	event := &TelemetryEvent{
		ID:   "test-id",
		Type: EventTypeEcphoryRetrieval,
	}

	// No ecphory result in context
	ec := EnrichmentContext{
		EcphoryResult: nil,
	}

	enrichedEvent, err := enricher.Enrich(context.Background(), event, ec)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// Should not enrich if ecphory result is missing
	if enrichedEvent.Data != nil && len(enrichedEvent.Data) > 0 {
		t.Errorf("Expected no enrichment without ecphory result, got: %v", enrichedEvent.Data)
	}
}

func TestEcphoryCoverageEnricher_HighTokenUtilization(t *testing.T) {
	enricher := NewEcphoryCoverageEnricher()

	event := &TelemetryEvent{
		ID:   "test-id",
		Type: EventTypeEcphoryRetrieval,
	}

	ecphoryResult := &EcphoryResult{
		PromptHash:         "def456",
		EngramsRetrieved:   50,
		CandidatesFiltered: 200,
		TokenBudgetUsed:    85000, // 85% utilization
		Strategy:           "inverted-tags",
	}

	ec := EnrichmentContext{
		EcphoryResult: ecphoryResult,
	}

	enrichedEvent, err := enricher.Enrich(context.Background(), event, ec)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// Check high token utilization (85%)
	utilization, ok := enrichedEvent.Data["token_utilization_percent"].(float64)
	if !ok {
		t.Fatalf("Expected token_utilization_percent to be float64, got: %T", enrichedEvent.Data["token_utilization_percent"])
	}

	if utilization != 85.0 {
		t.Errorf("Expected token_utilization_percent 85.0, got: %v", utilization)
	}

	// Check retrieval strategy
	if enrichedEvent.Data["retrieval_strategy"] != "inverted-tags" {
		t.Errorf("Expected retrieval_strategy 'inverted-tags', got: %v", enrichedEvent.Data["retrieval_strategy"])
	}
}

func TestEcphoryCoverageEnricher_ThreadSafety(t *testing.T) {
	enricher := NewEcphoryCoverageEnricher()

	event := &TelemetryEvent{
		ID:   "test-id",
		Type: EventTypeEcphoryRetrieval,
	}

	ecphoryResult := &EcphoryResult{
		PromptHash:         "test",
		EngramsRetrieved:   10,
		CandidatesFiltered: 30,
		TokenBudgetUsed:    10000,
		Strategy:           "manual",
	}

	ec := EnrichmentContext{
		EcphoryResult: ecphoryResult,
	}

	// Run concurrent enrichments
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			_, err := enricher.Enrich(context.Background(), event, ec)
			if err != nil {
				t.Errorf("Enrich failed: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Test passes if no race condition detected with -race flag
}

func TestEcphoryCoverageEnricher_Name(t *testing.T) {
	enricher := NewEcphoryCoverageEnricher()
	if enricher.Name() != "ecphory_coverage" {
		t.Errorf("Expected name 'ecphory_coverage', got: %s", enricher.Name())
	}
}
