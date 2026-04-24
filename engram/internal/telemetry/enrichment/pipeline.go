package enrichment

import (
	"context"
	"fmt"
	"time"
)

// Pipeline chains multiple enrichers together with fault isolation
type Pipeline struct {
	enrichers       []Enricher
	circuitBreakers map[string]*CircuitBreaker
	timeout         time.Duration
}

// NewPipeline creates a new enrichment pipeline
//
// Each enricher is wrapped with:
//   - SafeEnricher: Timeout protection (default 500μs per enricher)
//   - CircuitBreaker: Fault isolation (5 failures → open, 2 successes → close)
func NewPipeline(enrichers []Enricher, timeout time.Duration) *Pipeline {
	circuitBreakers := make(map[string]*CircuitBreaker)
	for _, e := range enrichers {
		// Create circuit breaker for each enricher
		// Thresholds: 5 failures to open, 2 successes to close, 30s timeout
		circuitBreakers[e.Name()] = NewCircuitBreaker(5, 2, 30*time.Second)
	}

	return &Pipeline{
		enrichers:       enrichers,
		circuitBreakers: circuitBreakers,
		timeout:         timeout,
	}
}

// Enrich runs all enrichers in sequence
//
// If an enricher fails (timeout or error), the original event is used
// and enrichment continues with the next enricher. This ensures that
// one failing enricher doesn't block event emission.
func (p *Pipeline) Enrich(ctx context.Context, event *TelemetryEvent, ec EnrichmentContext) *TelemetryEvent {
	currentEvent := event

	for _, enricher := range p.enrichers {
		// Wrap enricher with timeout protection
		safeEnricher := NewSafeEnricher(enricher, p.timeout)

		// Execute through circuit breaker
		cb := p.circuitBreakers[enricher.Name()]
		enrichedEvent, err := cb.Execute(ctx, safeEnricher, currentEvent, ec)

		if err != nil {
			// Enrichment failed - log but continue with original event
			// In production, this would use a proper logger
			fmt.Printf("WARN: Enrichment failed for %s: %v (circuit state: %s)\n",
				enricher.Name(), err, cb.State())
			// Keep current event and continue to next enricher
			continue
		}

		// Enrichment succeeded - use enriched event for next enricher
		currentEvent = enrichedEvent
	}

	return currentEvent
}

// GetCircuitBreakerState returns the state of a circuit breaker by enricher name
func (p *Pipeline) GetCircuitBreakerState(enricherName string) (State, bool) {
	cb, exists := p.circuitBreakers[enricherName]
	if !exists {
		return StateClosed, false
	}
	return cb.State(), true
}

// ResetCircuitBreaker resets a circuit breaker by enricher name
func (p *Pipeline) ResetCircuitBreaker(enricherName string) error {
	cb, exists := p.circuitBreakers[enricherName]
	if !exists {
		return fmt.Errorf("no circuit breaker found for enricher: %s", enricherName)
	}
	cb.Reset()
	return nil
}
