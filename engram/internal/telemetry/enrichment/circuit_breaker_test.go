package enrichment

import (
	"context"
	"errors"
	"testing"
	"time"
)

// errorEnricher always returns an error
type errorEnricher struct {
	name string
}

func (e *errorEnricher) Enrich(ctx context.Context, event *TelemetryEvent, ec EnrichmentContext) (*TelemetryEvent, error) {
	return nil, errors.New("enrichment failed")
}

func (e *errorEnricher) Name() string {
	return e.name
}

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 1*time.Second)
	enricher := &errorEnricher{name: "failing-enricher"}
	event := &TelemetryEvent{ID: "test"}

	// Initial state should be Closed
	if cb.State() != StateClosed {
		t.Errorf("Expected initial state Closed, got: %s", cb.State())
	}

	// Trigger failures until circuit opens
	for i := 0; i < 3; i++ {
		_, err := cb.Execute(context.Background(), enricher, event, EnrichmentContext{})
		if err == nil {
			t.Error("Expected error from failing enricher")
		}
	}

	// Circuit should now be Open
	if cb.State() != StateOpen {
		t.Errorf("Expected state Open after 3 failures, got: %s", cb.State())
	}
}

func TestCircuitBreaker_FailFast(t *testing.T) {
	cb := NewCircuitBreaker(1, 2, 1*time.Second)
	enricher := &errorEnricher{name: "failing-enricher"}
	event := &TelemetryEvent{ID: "test"}

	// Trigger failure to open circuit
	cb.Execute(context.Background(), enricher, event, EnrichmentContext{})

	// Circuit is now Open - next request should fail fast
	_, err := cb.Execute(context.Background(), enricher, event, EnrichmentContext{})
	if err == nil {
		t.Error("Expected fail-fast error when circuit is open")
	}

	if cb.State() != StateOpen {
		t.Errorf("Expected state Open, got: %s", cb.State())
	}
}

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(1, 2, 50*time.Millisecond)
	enricher := &errorEnricher{name: "failing-enricher"}
	event := &TelemetryEvent{ID: "test"}

	// Open the circuit
	cb.Execute(context.Background(), enricher, event, EnrichmentContext{})

	if cb.State() != StateOpen {
		t.Errorf("Expected state Open, got: %s", cb.State())
	}

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// Next request should transition to HalfOpen
	cb.Execute(context.Background(), enricher, event, EnrichmentContext{})

	if cb.State() != StateOpen { // Failed again, back to Open
		t.Errorf("Expected state Open after failure in HalfOpen, got: %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenToClosed(t *testing.T) {
	cb := NewCircuitBreaker(1, 2, 50*time.Millisecond)
	successEnricher := &mockEnricher{name: "success-enricher", delay: 1 * time.Microsecond}
	event := &TelemetryEvent{ID: "test"}

	// Open the circuit by manually setting state
	cb.mu.Lock()
	cb.state = StateHalfOpen
	cb.mu.Unlock()

	// Execute successful requests
	for i := 0; i < 2; i++ {
		_, err := cb.Execute(context.Background(), successEnricher, event, EnrichmentContext{})
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	}

	// Circuit should now be Closed
	if cb.State() != StateClosed {
		t.Errorf("Expected state Closed after 2 successes in HalfOpen, got: %s", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(1, 2, 1*time.Second)
	enricher := &errorEnricher{name: "failing-enricher"}
	event := &TelemetryEvent{ID: "test"}

	// Open the circuit
	cb.Execute(context.Background(), enricher, event, EnrichmentContext{})

	if cb.State() != StateOpen {
		t.Errorf("Expected state Open, got: %s", cb.State())
	}

	// Reset circuit
	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("Expected state Closed after reset, got: %s", cb.State())
	}
}

func TestCircuitBreaker_ThreadSafety(t *testing.T) {
	cb := NewCircuitBreaker(10, 2, 1*time.Second)
	enricher := &mockEnricher{name: "test-enricher", delay: 1 * time.Microsecond}
	event := &TelemetryEvent{ID: "test"}

	// Run concurrent executions
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			cb.Execute(context.Background(), enricher, event, EnrichmentContext{})
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Should not panic (test passes if no race condition detected with -race flag)
}
