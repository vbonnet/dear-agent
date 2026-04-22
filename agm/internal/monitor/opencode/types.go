package opencode

import (
	"context"
	"errors"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

// Shared types for OpenCode SSE Adapter components

// AGMEvent represents a parsed OpenCode event ready for publishing to EventBus
type AGMEvent struct {
	State     string                 // AGM state (IDLE, THINKING, AWAITING_PERMISSION, etc.)
	Timestamp int64                  // Unix timestamp from OpenCode event
	Metadata  map[string]interface{} // Event-specific metadata
}

// Config holds all configuration for the OpenCode adapter
type Config struct {
	Enabled        bool            // Whether adapter is enabled
	ServerURL      string          // OpenCode server URL (e.g., "http://localhost:4096")
	SessionID      string          // AGM session ID
	Reconnect      ReconnectConfig // Auto-reconnect configuration
	FallbackTmux   bool            // Fall back to tmux monitoring if SSE fails
	HealthProbeURL string          // Health probe endpoint (default: "/health")
	HealthTimeout  time.Duration   // Health probe timeout (default: 5s)
	MaxRetries     int             // Max reconnect attempts (0 = unlimited)
	CircuitBreaker bool            // Enable circuit breaker
}

// ReconnectConfig configures auto-reconnect behavior
type ReconnectConfig struct {
	InitialDelay time.Duration // Initial retry delay (e.g., 1s)
	MaxDelay     time.Duration // Maximum retry delay (e.g., 30s)
	Multiplier   int           // Backoff multiplier (e.g., 2)
}

// DefaultReconnectConfig returns sensible defaults for reconnection
func DefaultReconnectConfig() ReconnectConfig {
	return ReconnectConfig{
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2,
	}
}

// DefaultConfig returns sensible defaults for the adapter
func DefaultConfig() Config {
	return Config{
		Enabled:        false,
		ServerURL:      "http://localhost:4096",
		Reconnect:      DefaultReconnectConfig(),
		FallbackTmux:   true,
		HealthProbeURL: "/health",
		HealthTimeout:  5 * time.Second,
		MaxRetries:     0, // Unlimited
		CircuitBreaker: true,
	}
}

// EventBusPublisher is the interface for publishing events to AGM's EventBus
type EventBusPublisher interface {
	// Broadcast sends an event to all subscribers on the EventBus
	Broadcast(event *eventbus.Event)
}

// AdapterController provides control over the adapter lifecycle (used by circuit breaker)
type AdapterController interface {
	// Stop gracefully shuts down the adapter
	Stop(ctx context.Context) error
}

// HealthStatus represents the health status of the adapter
type HealthStatus struct {
	Connected     bool                   // Is SSE connection active?
	LastEvent     time.Time              // Last event received
	LastHeartbeat time.Time              // Last heartbeat received
	Error         error                  // Current error (if any)
	Metadata      map[string]interface{} // Additional metadata
}

// Common errors
var (
	ErrNotConnected       = errors.New("not connected to OpenCode server")
	ErrInvalidContentType = errors.New("invalid content-type: expected text/event-stream")
	ErrConnectionFailed   = errors.New("connection to OpenCode server failed")
	ErrCircuitBreakerOpen = errors.New("circuit breaker open: too many failures")
	ErrInvalidConfig      = errors.New("invalid configuration")
	ErrHealthProbeFailed  = errors.New("health probe failed")
)
