package provider

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// CBState represents circuit breaker state.
type CBState int

// Circuit breaker state values.
const (
	CBClosed   CBState = iota // Normal — requests flow through
	CBOpen                    // Tripped — requests rejected
	CBHalfOpen                // Probing — one request allowed
)

func (s CBState) String() string {
	switch s {
	case CBClosed:
		return "closed"
	case CBOpen:
		return "open"
	case CBHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig configures the provider circuit breaker.
type CircuitBreakerConfig struct {
	FailureThreshold int           // Consecutive failures to trip (default 3)
	CooldownPeriod   time.Duration // Time in open state before half-open (default 30s)
	FallbackProvider Provider      // Optional provider to use when circuit is open
}

// CircuitBreaker wraps a Provider with circuit breaker protection.
type CircuitBreaker struct {
	primary  Provider
	fallback Provider

	mu               sync.Mutex
	state            CBState
	consecutiveFails int
	lastFailure      time.Time
	failureThreshold int
	cooldown         time.Duration
}

// NewCircuitBreaker wraps a provider with circuit breaker logic.
func NewCircuitBreaker(primary Provider, cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 3
	}
	if cfg.CooldownPeriod <= 0 {
		cfg.CooldownPeriod = 30 * time.Second
	}
	return &CircuitBreaker{
		primary:          primary,
		fallback:         cfg.FallbackProvider,
		state:            CBClosed,
		failureThreshold: cfg.FailureThreshold,
		cooldown:         cfg.CooldownPeriod,
	}
}

// Name returns the underlying primary provider's name.
func (cb *CircuitBreaker) Name() string {
	return cb.primary.Name()
}

// Capabilities returns the underlying primary provider's capabilities.
func (cb *CircuitBreaker) Capabilities() Capabilities {
	return cb.primary.Capabilities()
}

// Generate routes requests based on circuit state.
func (cb *CircuitBreaker) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	if cb.canProceed() {
		resp, err := cb.primary.Generate(ctx, req)
		if err != nil {
			cb.recordFailure()
			// If circuit just opened and we have a fallback, try it
			if cb.fallback != nil && cb.State() == CBOpen {
				return cb.fallback.Generate(ctx, req)
			}
			return nil, err
		}
		cb.recordSuccess()
		return resp, nil
	}

	// Circuit is open
	if cb.fallback != nil {
		return cb.fallback.Generate(ctx, req)
	}

	return nil, fmt.Errorf("circuit breaker open for provider %q: %d consecutive failures, cooldown %s",
		cb.primary.Name(), cb.failureThreshold, cb.cooldown)
}

func (cb *CircuitBreaker) canProceed() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CBClosed:
		return true
	case CBOpen:
		if time.Since(cb.lastFailure) >= cb.cooldown {
			cb.state = CBHalfOpen
			return true
		}
		return false
	case CBHalfOpen:
		return true
	}
	return true
}

func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFails++
	cb.lastFailure = time.Now()

	if cb.consecutiveFails >= cb.failureThreshold {
		cb.state = CBOpen
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFails = 0
	cb.state = CBClosed
}

// State returns the current circuit breaker state.
func (cb *CircuitBreaker) State() CBState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Reset clears circuit breaker state (for testing).
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CBClosed
	cb.consecutiveFails = 0
}
