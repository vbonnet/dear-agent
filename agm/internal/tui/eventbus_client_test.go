package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

// TestWebSocketServer is a test WebSocket server for testing
type TestWebSocketServer struct {
	server   *httptest.Server
	upgrader websocket.Upgrader
	clients  []*websocket.Conn
	events   chan *eventbus.Event
	done     chan struct{}
}

// NewTestWebSocketServer creates a new test WebSocket server
func NewTestWebSocketServer() *TestWebSocketServer {
	tws := &TestWebSocketServer{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		clients: make([]*websocket.Conn, 0),
		events:  make(chan *eventbus.Event, 10),
		done:    make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", tws.handleWebSocket)
	tws.server = httptest.NewServer(mux)

	return tws
}

// handleWebSocket handles WebSocket connections
func (tws *TestWebSocketServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := tws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	tws.clients = append(tws.clients, conn)

	// Read messages from client
	go func() {
		defer conn.Close()
		for {
			var msg map[string]string
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			// Handle subscribe/unsubscribe messages if needed
		}
	}()
}

// BroadcastEvent sends an event to all connected clients
func (tws *TestWebSocketServer) BroadcastEvent(event *eventbus.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	for _, conn := range tws.clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return err
		}
	}

	return nil
}

// Close shuts down the test server
func (tws *TestWebSocketServer) Close() {
	close(tws.done)
	for _, conn := range tws.clients {
		conn.Close()
	}
	tws.server.Close()
}

// URL returns the WebSocket URL of the test server
func (tws *TestWebSocketServer) URL() string {
	return "ws" + strings.TrimPrefix(tws.server.URL, "http") + "/ws"
}

func TestNewEventBusClient(t *testing.T) {
	client := NewEventBusClient("ws://localhost:8080/ws")

	assert.NotNil(t, client)
	assert.Equal(t, "ws://localhost:8080/ws", client.url)
	assert.NotNil(t, client.events)
	assert.NotNil(t, client.done)
	assert.True(t, client.reconnect)
}

func TestConnect(t *testing.T) {
	server := NewTestWebSocketServer()
	defer server.Close()

	client := NewEventBusClient(server.URL())
	defer client.Close()

	err := client.Connect(server.URL())
	require.NoError(t, err)

	// Wait a bit for connection to establish
	time.Sleep(100 * time.Millisecond)

	assert.True(t, client.IsConnected())
}

func TestConnectFailure(t *testing.T) {
	// Try to connect to a non-existent server
	client := NewEventBusClient("ws://localhost:9999/ws")
	defer client.Close()

	err := client.Connect("ws://localhost:9999/ws")
	// Should not return error because it falls back to HTTP polling
	assert.NoError(t, err)

	// Should be in HTTP fallback mode
	assert.True(t, client.httpFallback)
}

func TestSubscribe(t *testing.T) {
	server := NewTestWebSocketServer()
	defer server.Close()

	client := NewEventBusClient(server.URL())
	defer client.Close()

	err := client.Connect(server.URL())
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	err = client.Subscribe("test-session-123")
	assert.NoError(t, err)
	assert.Equal(t, "test-session-123", client.sessionID)
}

func TestEventReception(t *testing.T) {
	server := NewTestWebSocketServer()
	defer server.Close()

	client := NewEventBusClient(server.URL())
	defer client.Close()

	err := client.Connect(server.URL())
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	err = client.Subscribe("test-session-123")
	require.NoError(t, err)

	// Create a test event
	testEvent, err := eventbus.NewEvent(
		eventbus.EventSessionEscalated,
		"test-session-123",
		eventbus.SessionEscalatedPayload{
			EscalationType: "error",
			Pattern:        "(?i)error:",
			Line:           "Error: Test error",
			LineNumber:     42,
			DetectedAt:     time.Now(),
			Description:    "Test error detected",
			Severity:       "high",
		},
	)
	require.NoError(t, err)

	// Broadcast event
	err = server.BroadcastEvent(testEvent)
	require.NoError(t, err)

	// Wait for event to be received
	select {
	case receivedEvent := <-client.Listen():
		assert.Equal(t, eventbus.EventSessionEscalated, receivedEvent.Type)
		assert.Equal(t, "test-session-123", receivedEvent.SessionID)

		// Parse payload
		var payload eventbus.SessionEscalatedPayload
		err = receivedEvent.ParsePayload(&payload)
		require.NoError(t, err)
		assert.Equal(t, "error", payload.EscalationType)
		assert.Equal(t, "Test error detected", payload.Description)

	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for event")
	}
}

func TestMultipleEvents(t *testing.T) {
	server := NewTestWebSocketServer()
	defer server.Close()

	client := NewEventBusClient(server.URL())
	defer client.Close()

	err := client.Connect(server.URL())
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	err = client.Subscribe("test-session-456")
	require.NoError(t, err)

	// Create multiple test events
	events := []*eventbus.Event{}

	event1, _ := eventbus.NewEvent(
		eventbus.EventSessionStuck,
		"test-session-456",
		eventbus.SessionStuckPayload{
			Reason:   "No output for 5 minutes",
			Duration: 5 * time.Minute,
		},
	)
	events = append(events, event1)

	event2, _ := eventbus.NewEvent(
		eventbus.EventSessionRecovered,
		"test-session-456",
		eventbus.SessionRecoveredPayload{
			PreviousState: "stuck",
			RecoveryTime:  2 * time.Minute,
			Action:        "User provided input",
		},
	)
	events = append(events, event2)

	// Broadcast events
	for _, event := range events {
		err = server.BroadcastEvent(event)
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond) // Small delay between events
	}

	// Receive events
	receivedCount := 0
	timeout := time.After(2 * time.Second)

	for receivedCount < len(events) {
		select {
		case event := <-client.Listen():
			assert.NotNil(t, event)
			assert.Equal(t, "test-session-456", event.SessionID)
			receivedCount++

		case <-timeout:
			t.Fatalf("Timeout: received %d events, expected %d", receivedCount, len(events))
		}
	}

	assert.Equal(t, len(events), receivedCount)
}

func TestClose(t *testing.T) {
	server := NewTestWebSocketServer()
	defer server.Close()

	client := NewEventBusClient(server.URL())

	err := client.Connect(server.URL())
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	assert.True(t, client.IsConnected())

	// Close client
	err = client.Close()
	assert.NoError(t, err)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	assert.False(t, client.IsConnected())
}

func TestReconnect(t *testing.T) {
	server := NewTestWebSocketServer()
	defer server.Close()

	client := NewEventBusClient(server.URL())
	defer client.Close()

	err := client.Connect(server.URL())
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	assert.True(t, client.IsConnected())

	// Close server connection to trigger reconnect
	for _, conn := range server.clients {
		conn.Close()
	}
	server.clients = make([]*websocket.Conn, 0)

	// Wait for reconnect (should happen within a few seconds)
	time.Sleep(3 * time.Second)

	// Client should have reconnected
	assert.True(t, client.IsConnected())
}

func TestInvalidEvent(t *testing.T) {
	server := NewTestWebSocketServer()
	defer server.Close()

	client := NewEventBusClient(server.URL())
	defer client.Close()

	err := client.Connect(server.URL())
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	// Send invalid event (missing required fields)
	invalidEvent := &eventbus.Event{
		Type: "invalid.type",
		// Missing SessionID and other required fields
	}

	err = server.BroadcastEvent(invalidEvent)
	require.NoError(t, err)

	// Wait a bit - invalid event should be dropped
	time.Sleep(200 * time.Millisecond)

	// No event should be received
	select {
	case event := <-client.Listen():
		t.Fatalf("Received invalid event: %+v", event)
	case <-time.After(500 * time.Millisecond):
		// Expected - no event received
	}
}

func TestHTTPFallback(t *testing.T) {
	// Create a client that will fail to connect via WebSocket
	client := NewEventBusClient("ws://localhost:9999/ws")
	defer client.Close()

	err := client.Connect("ws://localhost:9999/ws")
	assert.NoError(t, err) // Should not error, falls back to HTTP

	// Should be in HTTP fallback mode
	assert.True(t, client.httpFallback)

	err = client.Subscribe("test-session")
	assert.NoError(t, err)
}

func TestConcurrentAccess(t *testing.T) {
	server := NewTestWebSocketServer()
	defer server.Close()

	client := NewEventBusClient(server.URL())
	defer client.Close()

	err := client.Connect(server.URL())
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	// Start multiple goroutines that check connection status
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = client.IsConnected()
				time.Sleep(1 * time.Millisecond)
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Client should still be connected
	assert.True(t, client.IsConnected())
}

func TestEventChannelOverflow(t *testing.T) {
	server := NewTestWebSocketServer()
	defer server.Close()

	client := NewEventBusClient(server.URL())
	defer client.Close()

	err := client.Connect(server.URL())
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	// Fill up the event channel by sending many events without reading
	for i := 0; i < 300; i++ { // More than channel capacity (256)
		event, _ := eventbus.NewEvent(
			eventbus.EventSessionStuck,
			"test-session",
			eventbus.SessionStuckPayload{
				Reason:   "Test",
				Duration: 1 * time.Minute,
			},
		)
		server.BroadcastEvent(event)
		time.Sleep(1 * time.Millisecond)
	}

	// Wait a bit
	time.Sleep(200 * time.Millisecond)

	// Should still be connected (events are dropped when channel is full)
	assert.True(t, client.IsConnected())

	// Drain the channel
	drained := 0
	for {
		select {
		case <-client.Listen():
			drained++
		case <-time.After(100 * time.Millisecond):
			// No more events
			return
		}
	}
}

func TestResubscribeAfterReconnect(t *testing.T) {
	server := NewTestWebSocketServer()
	defer server.Close()

	client := NewEventBusClient(server.URL())
	defer client.Close()

	err := client.Connect(server.URL())
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	// Subscribe to a session
	err = client.Subscribe("test-session-789")
	require.NoError(t, err)

	// Close server connection to trigger reconnect
	for _, conn := range server.clients {
		conn.Close()
	}
	server.clients = make([]*websocket.Conn, 0)

	// Wait for reconnect
	time.Sleep(3 * time.Second)

	// Client should still have the session ID
	assert.Equal(t, "test-session-789", client.sessionID)

	// Should be reconnected
	assert.True(t, client.IsConnected())
}
