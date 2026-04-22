package enrichment

import (
	"context"
	"fmt"
	"time"
)

// SafeEnricher wraps an enricher with timeout protection and graceful degradation
type SafeEnricher struct {
	enricher Enricher
	timeout  time.Duration
}

// NewSafeEnricher creates a new SafeEnricher with timeout protection
func NewSafeEnricher(enricher Enricher, timeout time.Duration) *SafeEnricher {
	return &SafeEnricher{
		enricher: enricher,
		timeout:  timeout,
	}
}

// Enrich enriches the event with timeout protection
//
// If enrichment exceeds the timeout, the original event is returned.
// This prevents slow enrichers from blocking event emission.
func (s *SafeEnricher) Enrich(ctx context.Context, event *TelemetryEvent, ec EnrichmentContext) (*TelemetryEvent, error) {
	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// Channel for enrichment result
	type result struct {
		event *TelemetryEvent
		err   error
	}
	resultCh := make(chan result, 1)

	// Run enrichment in goroutine
	go func() {
		enrichedEvent, err := s.enricher.Enrich(timeoutCtx, event, ec)
		resultCh <- result{event: enrichedEvent, err: err}
	}()

	// Wait for result or timeout
	select {
	case res := <-resultCh:
		return res.event, res.err
	case <-timeoutCtx.Done():
		// Timeout - return original event (graceful degradation)
		return event, fmt.Errorf("enrichment timeout after %v for %s", s.timeout, s.enricher.Name())
	}
}

// Name returns the enricher name
func (s *SafeEnricher) Name() string {
	return s.enricher.Name()
}
