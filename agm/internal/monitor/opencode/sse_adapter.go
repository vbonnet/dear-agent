// Package opencode provides SSE-based monitoring for OpenCode agent sessions.
// It subscribes to OpenCode's Server-Sent Events stream and publishes state changes
// to AGM's EventBus, enabling real-time session state detection without tmux scraping.
package opencode

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SSE adapter specific errors (extends common errors from types.go)

// SSEAdapter manages SSE connection to OpenCode server and publishes events
type SSEAdapter struct {
	serverURL     string
	client        *http.Client
	parser        *EventParser
	publisher     *Publisher
	sessionID     string
	config        Config
	ctx           context.Context
	cancel        context.CancelFunc
	connected     atomic.Bool
	lastEvent     atomic.Value // time.Time
	lastHeartbeat atomic.Value // time.Time
	failureCount  atomic.Int64
	mu            sync.Mutex
	resp          *http.Response // Current HTTP response (for cleanup)
	wg            sync.WaitGroup // Wait group for goroutines
}

// NewSSEAdapter creates a new SSE adapter instance
func NewSSEAdapter(parser *EventParser, publisher *Publisher, config Config) *SSEAdapter {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize HTTP client with proper timeouts
	httpClient := &http.Client{
		Timeout: 0, // No overall timeout for streaming connection
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          10,
			MaxIdleConnsPerHost:   2,
		},
	}

	adapter := &SSEAdapter{
		serverURL: config.ServerURL,
		client:    httpClient,
		parser:    parser,
		publisher: publisher,
		sessionID: config.SessionID,
		config:    config,
		ctx:       ctx,
		cancel:    cancel,
	}

	// Initialize atomic values
	adapter.lastEvent.Store(time.Time{})
	adapter.lastHeartbeat.Store(time.Time{})

	return adapter
}

// Start begins the SSE adapter lifecycle
func (a *SSEAdapter) Start(ctx context.Context) error {
	// Replace internal context with provided one
	a.mu.Lock()
	a.cancel() // Cancel old context
	a.ctx, a.cancel = context.WithCancel(ctx)
	curCtx := a.ctx
	a.mu.Unlock()

	// Attempt initial connection
	if err := a.connect(curCtx); err != nil {
		// Initial connection failed - start reconnect loop in background
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			a.scheduleReconnect(curCtx)
		}()
		return fmt.Errorf("initial connection failed (will retry): %w", err)
	}

	return nil
}

// Stop gracefully shuts down the SSE adapter
func (a *SSEAdapter) Stop(ctx context.Context) error {
	// Cancel internal context to signal shutdown
	a.cancel()

	// Close HTTP response body if open
	a.mu.Lock()
	if a.resp != nil && a.resp.Body != nil {
		_ = a.resp.Body.Close()
		a.resp = nil
	}
	a.mu.Unlock()

	// Wait for goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown timeout: %w", ctx.Err())
	}
}

// Health returns the current health status of the adapter
func (a *SSEAdapter) Health() HealthStatus {
	connected := a.connected.Load()
	lastEvent := a.lastEvent.Load().(time.Time)
	lastHeartbeat := a.lastHeartbeat.Load().(time.Time)

	status := HealthStatus{
		Connected:     connected,
		LastEvent:     lastEvent,
		LastHeartbeat: lastHeartbeat,
		Metadata: map[string]interface{}{
			"server_url": a.serverURL,
			"session_id": a.sessionID,
		},
	}

	if !connected {
		status.Error = ErrNotConnected
		return status
	}

	// Use heartbeat, not event timestamp (prevents false positives for idle sessions)
	if !lastHeartbeat.IsZero() && time.Since(lastHeartbeat) > 5*time.Minute {
		status.Error = fmt.Errorf("no heartbeat for 5 minutes (connection may be dead)")
	}

	return status
}

// Name returns the adapter identifier
func (a *SSEAdapter) Name() string {
	return "opencode-sse"
}

// connect establishes the SSE connection to the OpenCode server.
// ctx is the lifecycle context captured at Start() time and used for both the
// HTTP request and the goroutines spawned here, so that subsequent Start/Stop
// cycles cannot rewrite the context out from under in-flight goroutines.
func (a *SSEAdapter) connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check circuit breaker
	if a.config.MaxRetries > 0 && a.failureCount.Load() >= int64(a.config.MaxRetries) {
		return ErrCircuitBreakerOpen
	}

	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.serverURL+"/event", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set SSE headers
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	// Execute request
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}

	// Validate response
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		_ = resp.Body.Close()
		return fmt.Errorf("%w: got %s", ErrInvalidContentType, contentType)
	}

	// Store response for cleanup
	a.resp = resp

	// Mark as connected
	a.connected.Store(true)
	a.failureCount.Store(0) // Reset failure count on successful connection

	// Start read pump in goroutine
	a.wg.Add(1)
	go a.readEvents(ctx, resp.Body)

	return nil
}

// readEvents reads and processes SSE events from the connection
func (a *SSEAdapter) readEvents(ctx context.Context, body io.ReadCloser) {
	defer func() {
		_ = body.Close()
		a.connected.Store(false)
		a.wg.Done()

		// Schedule reconnect if context not cancelled
		if ctx.Err() == nil {
			a.scheduleReconnect(ctx)
		}
	}()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 64KB initial, 1MB max

	for scanner.Scan() {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()

		// SSE format: "data: {json}"
		if strings.HasPrefix(line, "data: ") {
			eventData := strings.TrimPrefix(line, "data: ")
			a.handleEvent([]byte(eventData))
		} else if strings.HasPrefix(line, "event: ") {
			// Event type line (we'll handle this if needed)
			continue
		} else if strings.HasPrefix(line, ":") {
			// Comment line (often used for heartbeats)
			a.lastHeartbeat.Store(time.Now())
			continue
		} else if line == "" {
			// Empty line separates events
			continue
		}
	}

	// Check for scanner error
	if err := scanner.Err(); err != nil {
		// Only log if not context cancelled
		if ctx.Err() == nil {
			// Scanner error (connection issue)
			a.failureCount.Add(1)
		}
	}
}

// scheduleReconnect attempts to reconnect with exponential backoff
func (a *SSEAdapter) scheduleReconnect(ctx context.Context) {
	delay := a.config.Reconnect.InitialDelay
	attempts := 0

	for {
		select {
		case <-ctx.Done():
			return // Shutdown requested
		case <-time.After(delay):
			attempts++

			// Attempt reconnection
			err := a.connect(ctx)
			if err == nil {
				// Successfully reconnected
				return
			}

			// Check for circuit breaker
			if errors.Is(err, ErrCircuitBreakerOpen) {
				// Circuit breaker open, stop reconnecting
				return
			}

			// Exponential backoff
			delay = delay * time.Duration(a.config.Reconnect.Multiplier)
			if delay > a.config.Reconnect.MaxDelay {
				delay = a.config.Reconnect.MaxDelay
			}

			// Increment failure count
			a.failureCount.Add(1)
		}
	}
}

// handleEvent processes a single SSE event
func (a *SSEAdapter) handleEvent(data []byte) {
	// Update last event timestamp
	a.lastEvent.Store(time.Now())

	// Special handling for heartbeat events
	if len(data) > 0 && (data[0] == ':' || strings.HasPrefix(string(data), `{"type":"heartbeat"`)) {
		a.lastHeartbeat.Store(time.Now())
		return
	}

	// Parse the event using EventParser
	agmEvent, err := a.parser.Parse(data)
	if err != nil {
		slog.Error("Failed to parse OpenCode event", "error", err)
		return
	}

	// Publish to EventBus using Publisher
	if err := a.publisher.PublishWithBackpressure(agmEvent); err != nil {
		slog.Error("Failed to publish event", "error", err)
	}
}
