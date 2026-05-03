package eventbus

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a test WebSocket client
func newTestClient(t *testing.T, url string) *websocket.Conn {
	t.Helper()

	wsURL := "ws" + strings.TrimPrefix(url, "http")
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	require.NoError(t, err, "Failed to dial WebSocket")

	return conn
}

// Helper function to read a JSON message from WebSocket
func readJSONMessage(t *testing.T, conn *websocket.Conn, timeout time.Duration) map[string]interface{} {
	t.Helper()

	conn.SetReadDeadline(time.Now().Add(timeout))
	_, data, err := conn.ReadMessage()
	require.NoError(t, err, "Failed to read message")

	var msg map[string]interface{}
	err = json.Unmarshal(data, &msg)
	require.NoError(t, err, "Failed to unmarshal message")

	return msg
}

// Helper function to send a JSON message to WebSocket
func sendJSONMessage(t *testing.T, conn *websocket.Conn, msg interface{}) {
	t.Helper()

	data, err := json.Marshal(msg)
	require.NoError(t, err, "Failed to marshal message")

	err = conn.WriteMessage(websocket.TextMessage, data)
	require.NoError(t, err, "Failed to send message")
}

func TestNewHub(t *testing.T) {
	hub := NewHub()

	assert.NotNil(t, hub)
	assert.NotNil(t, hub.clients)
	assert.NotNil(t, hub.broadcast)
	assert.NotNil(t, hub.register)
	assert.NotNil(t, hub.unregister)
	assert.Equal(t, defaultMaxClients, hub.maxClients)
}

func TestNewHub_CustomMaxClients(t *testing.T) {
	t.Setenv("AGM_EVENTBUS_MAX_CLIENTS", "50")
	defer os.Unsetenv("AGM_EVENTBUS_MAX_CLIENTS")

	hub := NewHub()
	assert.Equal(t, 50, hub.maxClients)
}

func TestGetPort(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected int
	}{
		{"Default port", "", defaultPort},
		{"Custom port", "9090", 9090},
		{"Invalid port (negative)", "-1", defaultPort},
		{"Invalid port (too large)", "70000", defaultPort},
		{"Invalid port (non-numeric)", "invalid", defaultPort},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("AGM_EVENTBUS_PORT", tt.envValue)
				defer os.Unsetenv("AGM_EVENTBUS_PORT")
			}

			port := GetPort()
			assert.Equal(t, tt.expected, port)
		})
	}
}

func TestHub_ClientRegistration(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Shutdown()

	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	// Connect client
	conn := newTestClient(t, server.URL)
	defer conn.Close()

	// Give hub time to register client
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 1, hub.ClientCount())
}

func TestHub_ClientUnregistration(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	conn := newTestClient(t, server.URL)

	// Give hub time to register
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, hub.ClientCount())

	// Close connection
	conn.Close()

	// Give hub time to unregister
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 0, hub.ClientCount())
}

func TestHub_EventBroadcast(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	conn := newTestClient(t, server.URL)
	defer conn.Close()

	// Give hub time to register
	time.Sleep(50 * time.Millisecond)

	// Create and broadcast an event
	event, err := NewEvent(EventSessionStuck, "session-123", SessionStuckPayload{
		Reason:   "Test reason",
		Duration: 5 * time.Minute,
	})
	require.NoError(t, err)

	hub.Broadcast(event)

	// Read the event from WebSocket
	msg := readJSONMessage(t, conn, 2*time.Second)

	assert.Equal(t, string(EventSessionStuck), msg["type"])
	assert.Equal(t, "session-123", msg["session_id"])
}

func TestHub_SessionFiltering(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	// Client 1: Subscribe to all sessions
	conn1 := newTestClient(t, server.URL)
	defer conn1.Close()

	// Client 2: Subscribe to specific session
	conn2 := newTestClient(t, server.URL)
	defer conn2.Close()

	// Give hub time to register
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 2, hub.ClientCount())

	// Subscribe client 2 to specific session
	sendJSONMessage(t, conn2, ClientMessage{
		Action:    "subscribe",
		SessionID: "session-specific",
	})

	time.Sleep(50 * time.Millisecond)

	// Broadcast event for specific session
	event1, err := NewEvent(EventSessionStuck, "session-specific", SessionStuckPayload{
		Reason: "Test",
	})
	require.NoError(t, err)
	hub.Broadcast(event1)

	// Both clients should receive it
	msg1 := readJSONMessage(t, conn1, 1*time.Second)
	assert.Equal(t, "session-specific", msg1["session_id"])

	msg2 := readJSONMessage(t, conn2, 1*time.Second)
	assert.Equal(t, "session-specific", msg2["session_id"])

	// Broadcast event for different session
	event2, err := NewEvent(EventSessionStuck, "session-other", SessionStuckPayload{
		Reason: "Test",
	})
	require.NoError(t, err)
	hub.Broadcast(event2)

	// Only client 1 should receive it
	msg3 := readJSONMessage(t, conn1, 1*time.Second)
	assert.Equal(t, "session-other", msg3["session_id"])

	// Client 2 should not receive it (timeout expected)
	conn2.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err = conn2.ReadMessage()
	assert.Error(t, err, "Client 2 should not receive message for different session")
}

func TestHub_Subscribe(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	conn := newTestClient(t, server.URL)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Subscribe to specific session
	sendJSONMessage(t, conn, ClientMessage{
		Action:    "subscribe",
		SessionID: "session-abc",
	})

	time.Sleep(50 * time.Millisecond)

	// Verify subscription by broadcasting event
	event, err := NewEvent(EventSessionStuck, "session-abc", SessionStuckPayload{
		Reason: "Test",
	})
	require.NoError(t, err)
	hub.Broadcast(event)

	msg := readJSONMessage(t, conn, 1*time.Second)
	assert.Equal(t, "session-abc", msg["session_id"])
}

func TestHub_Unsubscribe(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	conn := newTestClient(t, server.URL)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Unsubscribe
	sendJSONMessage(t, conn, ClientMessage{
		Action: "unsubscribe",
	})

	time.Sleep(50 * time.Millisecond)

	// Try to broadcast an event
	event, err := NewEvent(EventSessionStuck, "session-123", SessionStuckPayload{
		Reason: "Test",
	})
	require.NoError(t, err)
	hub.Broadcast(event)

	// Client should not receive it
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	assert.Error(t, err, "Client should not receive message after unsubscribe")
}

func TestHub_InvalidMessage(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	conn := newTestClient(t, server.URL)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Send invalid JSON
	err := conn.WriteMessage(websocket.TextMessage, []byte("invalid json"))
	require.NoError(t, err)

	// Should receive error message
	msg := readJSONMessage(t, conn, 1*time.Second)
	assert.Contains(t, msg["error"], "invalid message format")
}

func TestHub_UnknownAction(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	conn := newTestClient(t, server.URL)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Send message with unknown action
	sendJSONMessage(t, conn, ClientMessage{
		Action: "unknown",
	})

	// Should receive error message
	msg := readJSONMessage(t, conn, 1*time.Second)
	assert.Contains(t, msg["error"], "unknown action")
}

func TestHub_MaxClients(t *testing.T) {
	t.Setenv("AGM_EVENTBUS_MAX_CLIENTS", "2")
	defer os.Unsetenv("AGM_EVENTBUS_MAX_CLIENTS")

	hub := NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	// Connect 2 clients (should succeed)
	conn1 := newTestClient(t, server.URL)
	defer conn1.Close()

	conn2 := newTestClient(t, server.URL)
	defer conn2.Close()

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 2, hub.ClientCount())

	// Connect 3rd client (should be rejected)
	conn3 := newTestClient(t, server.URL)
	defer conn3.Close()

	time.Sleep(50 * time.Millisecond)

	// Should receive error message
	msg := readJSONMessage(t, conn3, 1*time.Second)
	assert.Contains(t, msg["error"], "maximum clients reached")

	// Client count should still be 2
	assert.Equal(t, 2, hub.ClientCount())
}

func TestHub_ConcurrentClients(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	const numClients = 10
	var wg sync.WaitGroup
	conns := make([]*websocket.Conn, numClients)

	// Connect multiple clients concurrently
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			conns[idx] = newTestClient(t, server.URL)
		}(i)
	}

	wg.Wait()

	// Give hub time to register all clients
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, numClients, hub.ClientCount())

	// Broadcast event to all
	event, err := NewEvent(EventSessionStuck, "session-123", SessionStuckPayload{
		Reason: "Test",
	})
	require.NoError(t, err)
	hub.Broadcast(event)

	// All clients should receive it
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(conn *websocket.Conn) {
			defer wg.Done()
			msg := readJSONMessage(t, conn, 2*time.Second)
			assert.Equal(t, "session-123", msg["session_id"])
		}(conns[i])
	}

	wg.Wait()

	// Close all connections
	for _, conn := range conns {
		conn.Close()
	}

	// Give hub time to unregister all
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 0, hub.ClientCount())
}

func TestHub_MessageOrdering(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	conn := newTestClient(t, server.URL)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Broadcast multiple events in order
	const numEvents = 5
	for i := 0; i < numEvents; i++ {
		event, err := NewEvent(EventSessionStuck, fmt.Sprintf("session-%d", i), SessionStuckPayload{
			Reason: fmt.Sprintf("Event %d", i),
		})
		require.NoError(t, err)
		hub.Broadcast(event)
	}

	// Verify events are received in order
	for i := 0; i < numEvents; i++ {
		msg := readJSONMessage(t, conn, 1*time.Second)
		assert.Equal(t, fmt.Sprintf("session-%d", i), msg["session_id"])
	}
}

func TestHub_PingPong(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	conn := newTestClient(t, server.URL)
	defer conn.Close()

	// Set up pong handler
	pongReceived := make(chan bool, 1)
	conn.SetPongHandler(func(string) error {
		select {
		case pongReceived <- true:
		default:
		}
		return nil
	})

	// Start reading messages (to process pong)
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()

	time.Sleep(50 * time.Millisecond)

	// Send ping manually
	err := conn.WriteMessage(websocket.PingMessage, nil)
	require.NoError(t, err)

	// Connection should remain alive
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 1, hub.ClientCount())
}

func TestHub_GracefulShutdown(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	// Connect multiple clients
	conn1 := newTestClient(t, server.URL)
	defer conn1.Close()

	conn2 := newTestClient(t, server.URL)
	defer conn2.Close()

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 2, hub.ClientCount())

	// Shutdown hub
	hub.Shutdown()

	// Give time for shutdown to complete
	time.Sleep(100 * time.Millisecond)

	// All clients should be disconnected
	assert.Equal(t, 0, hub.ClientCount())

	// Reading from connections should fail
	conn1.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, _, err1 := conn1.ReadMessage()
	assert.Error(t, err1)

	conn2.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, _, err2 := conn2.ReadMessage()
	assert.Error(t, err2)
}

func TestHub_BroadcastChannelFull(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Shutdown()

	// Fill the broadcast channel
	for i := 0; i < cap(hub.broadcast); i++ {
		event, err := NewEvent(EventSessionStuck, fmt.Sprintf("session-%d", i), SessionStuckPayload{
			Reason: "Test",
		})
		require.NoError(t, err)
		hub.Broadcast(event)
	}

	// Next broadcast should not block (should drop)
	event, err := NewEvent(EventSessionStuck, "session-dropped", SessionStuckPayload{
		Reason: "This should be dropped",
	})
	require.NoError(t, err)

	done := make(chan bool)
	go func() {
		hub.Broadcast(event)
		done <- true
	}()

	select {
	case <-done:
		// Broadcast completed (either queued or dropped)
	case <-time.After(1 * time.Second):
		t.Fatal("Broadcast blocked when channel was full")
	}
}

func TestHub_SubscribeDefaultSessionID(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	conn := newTestClient(t, server.URL)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Subscribe without session_id (should default to "*")
	sendJSONMessage(t, conn, ClientMessage{
		Action: "subscribe",
	})

	time.Sleep(50 * time.Millisecond)

	// Broadcast event
	event, err := NewEvent(EventSessionStuck, "any-session", SessionStuckPayload{
		Reason: "Test",
	})
	require.NoError(t, err)
	hub.Broadcast(event)

	// Client should receive it (subscribed to all)
	msg := readJSONMessage(t, conn, 1*time.Second)
	assert.Equal(t, "any-session", msg["session_id"])
}
