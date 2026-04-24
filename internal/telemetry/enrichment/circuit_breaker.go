package enrichment

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// State represents circuit breaker state
type State int

const (
	// StateClosed - circuit is closed, requests flow through
	StateClosed State = iota
	// StateOpen - circuit is open, requests fail fast
	StateOpen
	// StateHalfOpen - circuit is testing if service recovered
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "Closed"
	case StateOpen:
		return "Open"
	case StateHalfOpen:
		return "HalfOpen"
	default:
		return "Unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern for fault isolation
//
// State machine:
//
//	Closed → Open (after failureThreshold consecutive failures)
//	Open → HalfOpen (after timeout duration)
//	HalfOpen → Closed (after successThreshold consecutive successes)
//	HalfOpen → Open (on any failure)
type CircuitBreaker struct {
	mu               sync.RWMutex // Protects all fields (S6 conditional fix)
	failureThreshold int
	successThreshold int
	timeout          time.Duration
	state            State
	failures         int
	successes        int
	lastFailure      time.Time
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(failureThreshold, successThreshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:          timeout,
		state:            StateClosed,
		failures:         0,
		successes:        0,
	}
}

// Execute runs the enricher through the circuit breaker
func (cb *CircuitBreaker) Execute(ctx context.Context, enricher Enricher, event *TelemetryEvent, ec EnrichmentContext) (*TelemetryEvent, error) {
	// Check if circuit should transition from Open to HalfOpen
	cb.mu.Lock()
	if cb.state == StateOpen && time.Now().After(cb.lastFailure.Add(cb.timeout)) {
		cb.state = StateHalfOpen
		cb.successes = 0
	}
	currentState := cb.state
	cb.mu.Unlock()

	// Fail fast if circuit is open
	if currentState == StateOpen {
		return event, fmt.Errorf("circuit breaker open for %s", enricher.Name())
	}

	// Execute enrichment
	enrichedEvent, err := enricher.Enrich(ctx, event, ec)

	// Update circuit breaker state based on result
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		// Enrichment failed
		cb.failures++
		cb.successes = 0
		cb.lastFailure = time.Now()

		// Transition to Open if threshold exceeded
		if cb.failures >= cb.failureThreshold {
			cb.state = StateOpen
		}

		return event, err // Return original event on failure
	}

	// Enrichment succeeded
	cb.successes++
	cb.failures = 0

	// Transition from HalfOpen to Closed if threshold met
	if cb.state == StateHalfOpen && cb.successes >= cb.successThreshold {
		cb.state = StateClosed
	}

	return enrichedEvent, nil
}

// State returns the current circuit breaker state (thread-safe)
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.failures = 0
	cb.successes = 0
}
