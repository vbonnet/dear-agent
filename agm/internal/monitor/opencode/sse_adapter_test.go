package opencode

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// mockAdapterController is a no-op adapter controller for testing
type mockAdapterController struct{}

func (m *mockAdapterController) Stop(ctx context.Context) error {
	return nil
}

// setupTestSSEAdapter creates an SSEAdapter with all required components for testing
func setupTestSSEAdapter(config Config) (*SSEAdapter, *mockEventBus) {
	eventBus := &mockEventBus{}
	parser := NewEventParser()
	publisher := NewPublisher(eventBus, config.SessionID, &mockAdapterController{})
	adapter := NewSSEAdapter(parser, publisher, config)
	return adapter, eventBus
}

// TestSSEAdapter_Connect tests successful SSE connection
func TestSSEAdapter_Connect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("Expected Accept: text/event-stream, got: %s", r.Header.Get("Accept"))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Send test event with timestamp
		_, _ = fmt.Fprintf(w, "data: {\"type\":\"session.created\",\"timestamp\":%d}\n\n", time.Now().Unix())
		w.(http.Flusher).Flush()

		// Keep connection open longer so test can check connected status
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	config := Config{
		ServerURL: server.URL,
		SessionID: "test-session",
		Reconnect: DefaultReconnectConfig(),
	}

	adapter, _ := setupTestSSEAdapter(config)
	defer func() { _ = adapter.Stop(context.Background()) }()

	err := adapter.Start(context.Background())
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Wait for connection to establish and event to be processed
	time.Sleep(50 * time.Millisecond)

	if !adapter.connected.Load() {
		t.Error("Expected adapter to be connected")
	}

	health := adapter.Health()
	if !health.Connected {
		t.Error("Health check should show connected")
	}
}

// TestSSEAdapter_ConnectionFailure tests handling of connection failures
func TestSSEAdapter_ConnectionFailure(t *testing.T) {
	// Server that returns 503
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	config := Config{
		ServerURL: server.URL,
		SessionID: "test-session",
		Reconnect: ReconnectConfig{
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     50 * time.Millisecond,
			Multiplier:   2,
		},
		MaxRetries: 0,
	}

	adapter, _ := setupTestSSEAdapter(config)
	defer func() { _ = adapter.Stop(context.Background()) }()

	err := adapter.Start(context.Background())
	if err == nil {
		t.Error("Expected error for connection to unavailable server")
	}

	// Verify not connected
	if adapter.connected.Load() {
		t.Error("Should not be connected to failing server")
	}

	health := adapter.Health()
	if health.Connected {
		t.Error("Health check should show disconnected")
	}
	if health.Error == nil {
		t.Error("Health check should report error")
	}
}

// TestSSEAdapter_InvalidContentType tests rejection of invalid content types
func TestSSEAdapter_InvalidContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json") // Wrong content type
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		ServerURL: server.URL,
		SessionID: "test-session",
		Reconnect: DefaultReconnectConfig(),
	}

	adapter, _ := setupTestSSEAdapter(config)
	defer func() { _ = adapter.Stop(context.Background()) }()

	err := adapter.Start(context.Background())
	if err == nil {
		t.Error("Expected error for invalid content type")
	}

	if !strings.Contains(err.Error(), "invalid content-type") {
		t.Errorf("Expected invalid content-type error, got: %v", err)
	}
}

// TestSSEAdapter_AutoReconnect tests automatic reconnection with exponential backoff
func TestSSEAdapter_AutoReconnect(t *testing.T) {
	callCount := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		if count < 3 {
			// Fail first 2 attempts
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Succeed on 3rd attempt - keep connection open
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
		w.(http.Flusher).Flush()
		// Keep connection alive so test can verify connected status
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	config := Config{
		ServerURL: server.URL,
		SessionID: "test-session",
		Reconnect: ReconnectConfig{
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2,
		},
		MaxRetries: 0,
	}

	adapter, _ := setupTestSSEAdapter(config)
	defer func() { _ = adapter.Stop(context.Background()) }()

	// Start (will fail initially)
	_ = adapter.Start(context.Background())

	// Wait for reconnection attempts and successful connect
	time.Sleep(150 * time.Millisecond)

	// Should eventually connect
	if callCount.Load() < 3 {
		t.Errorf("Expected at least 3 connection attempts, got %d", callCount.Load())
	}

	if !adapter.connected.Load() {
		t.Error("Expected adapter to be connected after retries")
	}
}

// TestSSEAdapter_GracefulShutdown tests clean shutdown
func TestSSEAdapter_GracefulShutdown(t *testing.T) {
	connClosed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Send heartbeats until connection closes
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_, err := fmt.Fprintf(w, ": heartbeat\n\n")
				if err != nil {
					close(connClosed)
					return
				}
				w.(http.Flusher).Flush()
			case <-r.Context().Done():
				close(connClosed)
				return
			}
		}
	}))
	defer server.Close()

	config := Config{
		ServerURL: server.URL,
		SessionID: "test-session",
		Reconnect: DefaultReconnectConfig(),
	}

	adapter, _ := setupTestSSEAdapter(config)

	// Start adapter
	err := adapter.Start(context.Background())
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Wait for connection
	time.Sleep(50 * time.Millisecond)

	// Stop adapter
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = adapter.Stop(ctx)
	if err != nil {
		t.Errorf("Stop() failed: %v", err)
	}

	// Verify connection closed
	select {
	case <-connClosed:
		// Good, connection closed
	case <-time.After(500 * time.Millisecond):
		t.Error("Connection not closed after Stop()")
	}

	// Verify not connected
	if adapter.connected.Load() {
		t.Error("Adapter should not be connected after Stop()")
	}
}

// TestSSEAdapter_HeartbeatTracking tests heartbeat vs event timestamp tracking
func TestSSEAdapter_HeartbeatTracking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Send heartbeat (comment line)
		_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
		w.(http.Flusher).Flush()

		// Wait a bit
		time.Sleep(50 * time.Millisecond)

		// Send actual event
		_, _ = fmt.Fprintf(w, "data: {\"type\":\"test.event\",\"timestamp\":%d}\n\n", time.Now().Unix())
		w.(http.Flusher).Flush()
	}))
	defer server.Close()

	config := Config{
		ServerURL: server.URL,
		SessionID: "test-session",
		Reconnect: DefaultReconnectConfig(),
	}

	adapter, _ := setupTestSSEAdapter(config)
	defer func() { _ = adapter.Stop(context.Background()) }()

	err := adapter.Start(context.Background())
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Wait for events to be processed
	time.Sleep(100 * time.Millisecond)

	health := adapter.Health()

	// Both should be set
	if health.LastHeartbeat.IsZero() {
		t.Error("LastHeartbeat should be set")
	}
	if health.LastEvent.IsZero() {
		t.Error("LastEvent should be set")
	}

	// Both heartbeat and event timestamps should be updated
	// (heartbeat may be after event if comment line came after data line)
}

// TestSSEAdapter_CircuitBreaker tests circuit breaker after max failures
func TestSSEAdapter_CircuitBreaker(t *testing.T) {
	callCount := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		// Always fail
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	config := Config{
		ServerURL: server.URL,
		SessionID: "test-session",
		Reconnect: ReconnectConfig{
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     50 * time.Millisecond,
			Multiplier:   2,
		},
		MaxRetries: 5, // Circuit breaker at 5 failures
	}

	adapter, _ := setupTestSSEAdapter(config)
	defer func() { _ = adapter.Stop(context.Background()) }()

	// Start (will fail)
	_ = adapter.Start(context.Background())

	// Wait for circuit breaker to open
	time.Sleep(500 * time.Millisecond)

	// Should stop trying after MaxRetries
	count := callCount.Load()
	if count > 10 {
		t.Errorf("Circuit breaker should limit attempts, got %d calls", count)
	}
}

// TestSSEAdapter_ContextCancellation tests proper handling of context cancellation
func TestSSEAdapter_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Keep sending events
		for {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
				_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
				w.(http.Flusher).Flush()
			}
		}
	}))
	defer server.Close()

	config := Config{
		ServerURL: server.URL,
		SessionID: "test-session",
		Reconnect: DefaultReconnectConfig(),
	}

	adapter, _ := setupTestSSEAdapter(config)

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	err := adapter.Start(ctx)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Wait for connection
	time.Sleep(50 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for shutdown
	time.Sleep(100 * time.Millisecond)

	// Should not be connected
	if adapter.connected.Load() {
		t.Error("Adapter should disconnect on context cancellation")
	}
}

// TestSSEAdapter_Name tests the Name() method
func TestSSEAdapter_Name(t *testing.T) {
	config := Config{
		ServerURL: "http://localhost:4096",
		SessionID: "test-session",
		Reconnect: DefaultReconnectConfig(),
	}

	adapter, _ := setupTestSSEAdapter(config)

	if adapter.Name() != "opencode-sse" {
		t.Errorf("Expected name 'opencode-sse', got: %s", adapter.Name())
	}
}

// TestSSEAdapter_HealthMetadata tests health status metadata
func TestSSEAdapter_HealthMetadata(t *testing.T) {
	config := Config{
		ServerURL: "http://localhost:4096",
		SessionID: "test-session-123",
		Reconnect: DefaultReconnectConfig(),
	}

	adapter, _ := setupTestSSEAdapter(config)

	health := adapter.Health()

	if health.Metadata["server_url"] != "http://localhost:4096" {
		t.Errorf("Expected server_url in metadata")
	}

	if health.Metadata["session_id"] != "test-session-123" {
		t.Errorf("Expected session_id in metadata")
	}
}

// TestSSEAdapter_MultipleStartStop tests multiple start/stop cycles
func TestSSEAdapter_MultipleStartStop(t *testing.T) {
	callCount := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Send heartbeat until client disconnects
		for {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
				_, err := fmt.Fprintf(w, ": heartbeat\n\n")
				if err != nil {
					return
				}
				w.(http.Flusher).Flush()
			}
		}
	}))
	defer server.Close()

	config := Config{
		ServerURL: server.URL,
		SessionID: "test-session",
		Reconnect: DefaultReconnectConfig(),
	}

	adapter, _ := setupTestSSEAdapter(config)

	// Start/Stop cycle 1
	ctx1, cancel1 := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel1()

	_ = adapter.Start(ctx1)
	time.Sleep(50 * time.Millisecond)
	_ = adapter.Stop(context.Background())

	// Start/Stop cycle 2
	ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel2()

	_ = adapter.Start(ctx2)
	time.Sleep(50 * time.Millisecond)
	_ = adapter.Stop(context.Background())

	// Should have connected twice
	if callCount.Load() < 2 {
		t.Errorf("Expected at least 2 connection attempts, got %d", callCount.Load())
	}
}

// TestDefaultReconnectConfig tests default configuration values
func TestDefaultReconnectConfig(t *testing.T) {
	config := DefaultReconnectConfig()

	if config.InitialDelay != 1*time.Second {
		t.Errorf("Expected InitialDelay=1s, got %v", config.InitialDelay)
	}

	if config.MaxDelay != 30*time.Second {
		t.Errorf("Expected MaxDelay=30s, got %v", config.MaxDelay)
	}

	if config.Multiplier != 2 {
		t.Errorf("Expected Multiplier=2, got %d", config.Multiplier)
	}
}

// Benchmark tests
func BenchmarkSSEAdapter_EventProcessing(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		for i := 0; i < b.N; i++ {
			_, _ = fmt.Fprintf(w, "data: {\"type\":\"test.event\",\"id\":%d,\"timestamp\":%d}\n\n", i, time.Now().Unix())
			w.(http.Flusher).Flush()
		}
	}))
	defer server.Close()

	config := Config{
		ServerURL: server.URL,
		SessionID: "benchmark-session",
		Reconnect: DefaultReconnectConfig(),
	}

	adapter, _ := setupTestSSEAdapter(config)
	defer func() { _ = adapter.Stop(context.Background()) }()

	b.ResetTimer()

	_ = adapter.Start(context.Background())
	time.Sleep(time.Duration(b.N) * time.Millisecond)
}
