package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

// Circuit breaker state values.
const (
	CircuitClosed   CircuitState = iota // Normal operation
	CircuitOpen                         // Failures exceeded threshold, rejecting calls
	CircuitHalfOpen                     // Testing if service recovered
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker stops forwarding after N consecutive failures.
type CircuitBreaker struct {
	mu              sync.Mutex
	circuits        map[string]*circuit
	failureThreshold int
	resetTimeout    time.Duration
	logger          *slog.Logger
}

type circuit struct {
	state            CircuitState
	consecutiveFails int
	lastFailure      time.Time
}

// CircuitBreakerConfig holds circuit breaker configuration.
type CircuitBreakerConfig struct {
	FailureThreshold int           // consecutive failures before opening (default 5)
	ResetTimeout     time.Duration // time before trying half-open (default 30s)
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(cfg CircuitBreakerConfig, logger *slog.Logger) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.ResetTimeout <= 0 {
		cfg.ResetTimeout = 30 * time.Second
	}
	return &CircuitBreaker{
		circuits:         make(map[string]*circuit),
		failureThreshold: cfg.FailureThreshold,
		resetTimeout:     cfg.ResetTimeout,
		logger:           logger,
	}
}

// Middleware returns an MCP middleware that implements circuit breaking.
func (cb *CircuitBreaker) Middleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}

			toolName := extractToolName(req)
			if toolName == "" {
				return next(ctx, method, req)
			}

			if !cb.canProceed(toolName) {
				cb.logger.WarnContext(ctx, "gateway.circuitbreaker: circuit open", "tool", toolName)
				return nil, fmt.Errorf("circuit breaker open for tool %q: too many consecutive failures", toolName)
			}

			result, err := next(ctx, method, req)
			if err != nil {
				cb.recordFailure(toolName)
			} else {
				cb.recordSuccess(toolName)
			}

			return result, err
		}
	}
}

// canProceed checks if a request should be allowed through.
func (cb *CircuitBreaker) canProceed(tool string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	c, ok := cb.circuits[tool]
	if !ok {
		return true // No circuit = closed
	}

	switch c.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if reset timeout has elapsed
		if time.Since(c.lastFailure) >= cb.resetTimeout {
			c.state = CircuitHalfOpen
			return true
		}
		return false
	case CircuitHalfOpen:
		return true // Allow one probe
	default:
		return true
	}
}

// recordFailure records a failure for a tool.
func (cb *CircuitBreaker) recordFailure(tool string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	c, ok := cb.circuits[tool]
	if !ok {
		c = &circuit{state: CircuitClosed}
		cb.circuits[tool] = c
	}

	c.consecutiveFails++
	c.lastFailure = time.Now()

	if c.consecutiveFails >= cb.failureThreshold {
		c.state = CircuitOpen
		cb.logger.Warn("gateway.circuitbreaker: circuit opened", "tool", tool, "failures", c.consecutiveFails)
	}
}

// recordSuccess records a success and resets the circuit.
func (cb *CircuitBreaker) recordSuccess(tool string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	c, ok := cb.circuits[tool]
	if !ok {
		return
	}

	c.consecutiveFails = 0
	c.state = CircuitClosed
}

// State returns the current circuit state for a tool.
func (cb *CircuitBreaker) State(tool string) CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	c, ok := cb.circuits[tool]
	if !ok {
		return CircuitClosed
	}
	return c.state
}

// Reset clears all circuit breaker state (for testing).
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.circuits = make(map[string]*circuit)
}
