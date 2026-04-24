package enrichment

import (
	"context"
	"testing"
	"time"
)

func TestPipeline_MultipleEnrichers(t *testing.T) {
	// Create pipeline with multiple enrichers
	enrichers := []Enricher{
		&mockEnricher{name: "enricher-1", delay: 1 * time.Millisecond},
		&mockEnricher{name: "enricher-2", delay: 1 * time.Millisecond},
		&mockEnricher{name: "enricher-3", delay: 1 * time.Millisecond},
	}

	pipeline := NewPipeline(enrichers, 100*time.Millisecond)

	event := &TelemetryEvent{
		ID:   "test-id",
		Type: "test",
		Data: make(map[string]interface{}),
	}

	ctx := context.Background()
	enrichedEvent := pipeline.Enrich(ctx, event, EnrichmentContext{})

	// Check that all enrichers processed the event
	// (Last enricher wins for "enriched_by" field)
	if enrichedEvent.Data["enriched_by"] != "enricher-3" {
		t.Errorf("Expected last enricher to process event, got: %v", enrichedEvent.Data)
	}
}

func TestPipeline_GracefulDegradation(t *testing.T) {
	// Create pipeline with one failing enricher
	enrichers := []Enricher{
		&mockEnricher{name: "enricher-1", delay: 1 * time.Millisecond},
		&errorEnricher{name: "failing-enricher"}, // This one fails
		&mockEnricher{name: "enricher-3", delay: 1 * time.Millisecond},
	}

	pipeline := NewPipeline(enrichers, 100*time.Millisecond)

	event := &TelemetryEvent{
		ID:   "test-id",
		Type: "test",
		Data: make(map[string]interface{}),
	}

	ctx := context.Background()
	enrichedEvent := pipeline.Enrich(ctx, event, EnrichmentContext{})

	// Pipeline should continue even with failing enricher
	// enricher-1 should succeed, failing-enricher skipped, enricher-3 should succeed
	if enrichedEvent.Data["enriched_by"] != "enricher-3" {
		t.Errorf("Expected enricher-3 to process event despite enricher-2 failure, got: %v", enrichedEvent.Data)
	}
}

func TestPipeline_CircuitBreakerIntegration(t *testing.T) {
	// Create pipeline with failing enricher
	enrichers := []Enricher{
		&errorEnricher{name: "failing-enricher"},
	}

	pipeline := NewPipeline(enrichers, 500*time.Microsecond)

	event := &TelemetryEvent{
		ID:   "test-id",
		Type: "test",
	}

	ctx := context.Background()

	// Trigger failures to open circuit breaker (threshold: 5)
	for i := 0; i < 5; i++ {
		pipeline.Enrich(ctx, event, EnrichmentContext{})
	}

	// Check circuit breaker state
	state, exists := pipeline.GetCircuitBreakerState("failing-enricher")
	if !exists {
		t.Fatal("Circuit breaker not found for failing-enricher")
	}

	if state != StateOpen {
		t.Errorf("Expected circuit breaker to be Open after 5 failures, got: %s", state)
	}
}

func TestPipeline_ResetCircuitBreaker(t *testing.T) {
	enrichers := []Enricher{
		&errorEnricher{name: "failing-enricher"},
	}

	pipeline := NewPipeline(enrichers, 500*time.Microsecond)

	event := &TelemetryEvent{ID: "test"}
	ctx := context.Background()

	// Open circuit breaker
	for i := 0; i < 5; i++ {
		pipeline.Enrich(ctx, event, EnrichmentContext{})
	}

	state, _ := pipeline.GetCircuitBreakerState("failing-enricher")
	if state != StateOpen {
		t.Errorf("Expected Open state, got: %s", state)
	}

	// Reset circuit breaker
	err := pipeline.ResetCircuitBreaker("failing-enricher")
	if err != nil {
		t.Fatalf("Failed to reset circuit breaker: %v", err)
	}

	state, _ = pipeline.GetCircuitBreakerState("failing-enricher")
	if state != StateClosed {
		t.Errorf("Expected Closed state after reset, got: %s", state)
	}
}

func TestPipeline_PerformanceBudget(t *testing.T) {
	// Create pipeline with 3 enrichers, each with generous timeout
	// Total enrichment should complete reasonably fast
	enrichers := []Enricher{
		&mockEnricher{name: "enricher-1", delay: 1 * time.Microsecond},
		&mockEnricher{name: "enricher-2", delay: 1 * time.Microsecond},
		&mockEnricher{name: "enricher-3", delay: 1 * time.Microsecond},
	}

	pipeline := NewPipeline(enrichers, 10*time.Millisecond) // Generous timeout

	event := &TelemetryEvent{
		ID:   "test-id",
		Type: "test",
	}

	ctx := context.Background()
	start := time.Now()
	pipeline.Enrich(ctx, event, EnrichmentContext{})
	elapsed := time.Since(start)

	// Should complete within reasonable time (generous budget for test environment)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Enrichment exceeded performance budget: %v (expected <100ms)", elapsed)
	}
}
