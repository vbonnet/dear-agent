package opencode

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Adapter is the top-level coordinator for the OpenCode SSE monitoring adapter.
// It implements the Adapter interface and manages the lifecycle of all sub-components.
type Adapter struct {
	sseClient *SSEAdapter
	parser    *EventParser
	publisher *Publisher
	config    Config
	mapper    *SessionMapper
}

// SessionMapper manages the mapping between OpenCode session IDs and AGM session IDs.
// This is necessary because OpenCode may use different internal session identifiers.
type SessionMapper struct {
	mu      sync.RWMutex
	mapping map[string]string // opencodeID → agmSessionID
}

// NewSessionMapper creates a new session ID mapper
func NewSessionMapper() *SessionMapper {
	return &SessionMapper{
		mapping: make(map[string]string),
	}
}

// Register adds a mapping from OpenCode session ID to AGM session ID
func (m *SessionMapper) Register(opencodeID, agmSessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mapping[opencodeID] = agmSessionID
}

// Lookup retrieves the AGM session ID for a given OpenCode session ID
func (m *SessionMapper) Lookup(opencodeID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	agmID, ok := m.mapping[opencodeID]
	return agmID, ok
}

// Remove deletes a session mapping
func (m *SessionMapper) Remove(opencodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.mapping, opencodeID)
}

// Count returns the number of active session mappings
func (m *SessionMapper) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.mapping)
}

// Clear removes all session mappings
func (m *SessionMapper) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mapping = make(map[string]string)
}

// NewAdapter creates a new OpenCode SSE adapter instance.
// The adapter coordinates the SSE client, event parser, and publisher components.
func NewAdapter(eventBus EventBusPublisher, config Config) (*Adapter, error) {
	// Validate configuration
	if eventBus == nil {
		return nil, fmt.Errorf("eventBus cannot be nil")
	}
	if config.ServerURL == "" {
		return nil, fmt.Errorf("serverURL cannot be empty")
	}
	if config.SessionID == "" {
		return nil, fmt.Errorf("sessionID cannot be empty")
	}

	// Apply defaults
	if config.HealthProbeURL == "" {
		config.HealthProbeURL = "/health"
	}
	if config.HealthTimeout == 0 {
		config.HealthTimeout = 5 * time.Second
	}

	// Create session mapper
	mapper := NewSessionMapper()

	// Create event parser
	parser := NewEventParser()

	// Create adapter instance (needed for publisher's circuit breaker)
	adapter := &Adapter{
		config: config,
		parser: parser,
		mapper: mapper,
	}

	// Create publisher (with reference to adapter for circuit breaker)
	publisher := NewPublisher(eventBus, config.SessionID, adapter)

	// Create SSE client with parser and publisher
	sseClient := NewSSEAdapter(parser, publisher, config)

	// Wire up components
	adapter.sseClient = sseClient
	adapter.publisher = publisher

	return adapter, nil
}

// Start begins the adapter lifecycle.
// It performs a health probe to the OpenCode server before starting the SSE client.
func (a *Adapter) Start(ctx context.Context) error {
	// Health probe to OpenCode server
	if err := a.healthProbe(ctx); err != nil {
		if a.config.FallbackTmux {
			slog.Warn("OpenCode SSE adapter health check failed", "error", err, "fallback", "tmux")
			return fmt.Errorf("health check failed (tmux fallback active): %w", err)
		}
		return fmt.Errorf("server health check failed: %w", err)
	}

	// Start SSE client
	if err := a.sseClient.Start(ctx); err != nil {
		return fmt.Errorf("failed to start SSE client: %w", err)
	}

	slog.Info("OpenCode SSE adapter started", "server", a.config.ServerURL, "session", a.config.SessionID)

	return nil
}

// Stop gracefully shuts down the adapter.
// It propagates the context cancellation to the SSE client and waits for cleanup.
func (a *Adapter) Stop(ctx context.Context) error {
	slog.Info("Stopping OpenCode SSE adapter", "session", a.config.SessionID)

	// Stop SSE client
	if err := a.sseClient.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop SSE client: %w", err)
	}

	// Clear session mappings
	a.mapper.Clear()

	return nil
}

// Health returns the current health status of the adapter.
// It delegates to the SSE client's health check.
func (a *Adapter) Health() HealthStatus {
	return a.sseClient.Health()
}

// Name returns the adapter identifier.
func (a *Adapter) Name() string {
	return "opencode-sse"
}

// healthProbe performs a health check against the OpenCode server.
// It verifies the server is reachable before attempting to establish an SSE connection.
func (a *Adapter) healthProbe(parentCtx context.Context) error {
	// Create timeout context for health probe
	ctx, cancel := context.WithTimeout(parentCtx, a.config.HealthTimeout)
	defer cancel()

	// Build health probe URL
	healthURL := a.config.ServerURL + a.config.HealthProbeURL

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health probe request: %w", err)
	}

	// Execute health probe
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("health probe failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned non-OK status: %d", resp.StatusCode)
	}

	return nil
}

// GetSessionMapper returns the session mapper (for testing and diagnostics)
func (a *Adapter) GetSessionMapper() *SessionMapper {
	return a.mapper
}

// GetPublisher returns the publisher (for testing)
func (a *Adapter) GetPublisher() *Publisher {
	return a.publisher
}

// GetParser returns the event parser (for testing)
func (a *Adapter) GetParser() *EventParser {
	return a.parser
}
