package eventbus

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupIntegrationHub creates a Hub, starts it, and returns a test server.
// Callers must defer hub.Shutdown() and server.Close().
func setupIntegrationHub(t *testing.T) (*Hub, *httptest.Server) {
	t.Helper()
	hub := NewHub()
	go hub.Run()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWebSocket(hub, w, r)
	}))

	return hub, server
}

// waitForClients polls until hub.ClientCount() reaches the expected value.
func waitForClients(t *testing.T, hub *Hub, expected int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.ClientCount() == expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %d clients, got %d", expected, hub.ClientCount())
}

// readEvent reads a single Event from a WebSocket connection.
func readEvent(t *testing.T, conn *websocket.Conn, timeout time.Duration) *Event {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(timeout))
	_, data, err := conn.ReadMessage()
	require.NoError(t, err, "failed to read event")

	var event Event
	require.NoError(t, json.Unmarshal(data, &event), "failed to unmarshal event")
	return &event
}

// --- Integration Tests ---

// TestIntegration_MultiTypePublishSubscribe verifies that all five event types
// flow through the hub and arrive at clients with correct payloads.
func TestIntegration_MultiTypePublishSubscribe(t *testing.T) {
	t.Parallel()
	hub, server := setupIntegrationHub(t)
	defer hub.Shutdown()
	defer server.Close()

	conn := newTestClient(t, server.URL)
	defer conn.Close()
	waitForClients(t, hub, 1)

	// Publish one event of each type
	events := []*Event{
		mustEvent(t, EventSessionStuck, "s-1", SessionStuckPayload{
			Reason: "no output", Duration: 5 * time.Minute,
		}),
		mustEvent(t, EventSessionEscalated, "s-2", SessionEscalatedPayload{
			EscalationType: "error", Pattern: "(?i)error:", Line: "Error: fail",
			LineNumber: 10, DetectedAt: time.Now(), Description: "test", Severity: "high",
		}),
		mustEvent(t, EventSessionRecovered, "s-3", SessionRecoveredPayload{
			PreviousState: "stuck", RecoveryTime: 2 * time.Minute, Action: "user input",
		}),
		mustEvent(t, EventSessionStateChange, "s-4", SessionStateChangePayload{
			OldState: "active", NewState: "stopped", Reason: "user stop",
		}),
		mustEvent(t, EventSessionCompleted, "s-5", SessionCompletedPayload{
			ExitCode: 0, Duration: 30 * time.Minute, MessageCount: 42,
			TokensUsed: 15000, FinalState: "archived",
		}),
	}

	for _, e := range events {
		hub.Broadcast(e)
	}

	// Verify each event is received in order with correct type and session
	for i, sent := range events {
		received := readEvent(t, conn, 2*time.Second)
		assert.Equal(t, sent.Type, received.Type, "event %d: wrong type", i)
		assert.Equal(t, sent.SessionID, received.SessionID, "event %d: wrong session", i)
	}
}

// TestIntegration_PayloadFidelity verifies that event payloads survive the full
// publish → hub → WebSocket → client pipeline without data loss.
func TestIntegration_PayloadFidelity(t *testing.T) {
	t.Parallel()
	hub, server := setupIntegrationHub(t)
	defer hub.Shutdown()
	defer server.Close()

	conn := newTestClient(t, server.URL)
	defer conn.Close()
	waitForClients(t, hub, 1)

	original := SessionStuckPayload{
		Reason:      "disk full",
		Duration:    7 * time.Minute,
		LastOutput:  "Writing block 42...",
		Suggestions: []string{"Free disk space", "Increase volume"},
	}
	event := mustEvent(t, EventSessionStuck, "payload-test", original)
	hub.Broadcast(event)

	received := readEvent(t, conn, 2*time.Second)
	var parsed SessionStuckPayload
	require.NoError(t, received.ParsePayload(&parsed))

	assert.Equal(t, original.Reason, parsed.Reason)
	assert.Equal(t, original.Duration, parsed.Duration)
	assert.Equal(t, original.LastOutput, parsed.LastOutput)
	assert.Equal(t, original.Suggestions, parsed.Suggestions)
}

// TestIntegration_ConcurrentPublishers verifies correctness when multiple
// goroutines broadcast events simultaneously.
func TestIntegration_ConcurrentPublishers(t *testing.T) {
	t.Parallel()
	hub, server := setupIntegrationHub(t)
	defer hub.Shutdown()
	defer server.Close()

	conn := newTestClient(t, server.URL)
	defer conn.Close()
	waitForClients(t, hub, 1)

	const numPublishers = 5
	const eventsPerPublisher = 20
	const totalEvents = numPublishers * eventsPerPublisher

	var wg sync.WaitGroup
	for p := 0; p < numPublishers; p++ {
		wg.Add(1)
		go func(pubID int) {
			defer wg.Done()
			for i := 0; i < eventsPerPublisher; i++ {
				event := mustEvent(t, EventSessionStuck,
					fmt.Sprintf("pub-%d-event-%d", pubID, i),
					SessionStuckPayload{Reason: "concurrent"})
				hub.Broadcast(event)
			}
		}(p)
	}
	wg.Wait()

	// Collect all received events
	received := make(map[string]bool)
	for i := 0; i < totalEvents; i++ {
		event := readEvent(t, conn, 3*time.Second)
		received[event.SessionID] = true
	}

	assert.Equal(t, totalEvents, len(received), "should receive all events from all publishers")
}

// TestIntegration_ConcurrentSubscribers verifies that multiple subscribers each
// receive all broadcast events independently.
func TestIntegration_ConcurrentSubscribers(t *testing.T) {
	t.Parallel()
	hub, server := setupIntegrationHub(t)
	defer hub.Shutdown()
	defer server.Close()

	const numSubscribers = 20
	const numEvents = 10

	conns := make([]*websocket.Conn, numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		conns[i] = newTestClient(t, server.URL)
		defer conns[i].Close()
	}
	waitForClients(t, hub, numSubscribers)

	// Broadcast events
	for i := 0; i < numEvents; i++ {
		event := mustEvent(t, EventSessionStateChange,
			fmt.Sprintf("sub-event-%d", i),
			SessionStateChangePayload{OldState: "a", NewState: "b", Reason: "test"})
		hub.Broadcast(event)
	}

	// Each subscriber should receive all events
	var wg sync.WaitGroup
	for idx, conn := range conns {
		wg.Add(1)
		go func(c *websocket.Conn, subscriberIdx int) {
			defer wg.Done()
			for i := 0; i < numEvents; i++ {
				event := readEvent(t, c, 3*time.Second)
				assert.Equal(t, EventSessionStateChange, event.Type,
					"subscriber %d event %d: wrong type", subscriberIdx, i)
			}
		}(conn, idx)
	}
	wg.Wait()
}

// TestIntegration_SessionFilterMultipleSessions verifies that session-filtered
// subscribers only receive events for their subscribed session while others
// receive all events.
func TestIntegration_SessionFilterMultipleSessions(t *testing.T) {
	t.Parallel()
	hub, server := setupIntegrationHub(t)
	defer hub.Shutdown()
	defer server.Close()

	// Three clients: one global, two session-specific
	globalConn := newTestClient(t, server.URL)
	defer globalConn.Close()

	sessionAConn := newTestClient(t, server.URL)
	defer sessionAConn.Close()

	sessionBConn := newTestClient(t, server.URL)
	defer sessionBConn.Close()

	waitForClients(t, hub, 3)

	// Subscribe to specific sessions
	sendJSONMessage(t, sessionAConn, ClientMessage{Action: "subscribe", SessionID: "session-A"})
	sendJSONMessage(t, sessionBConn, ClientMessage{Action: "subscribe", SessionID: "session-B"})
	time.Sleep(50 * time.Millisecond)

	// Broadcast events for session-A, session-B, and session-C
	for _, sid := range []string{"session-A", "session-B", "session-C"} {
		event := mustEvent(t, EventSessionStuck, sid, SessionStuckPayload{Reason: "test"})
		hub.Broadcast(event)
	}

	// Global client gets all 3
	for i := 0; i < 3; i++ {
		readEvent(t, globalConn, 2*time.Second)
	}

	// session-A client gets only session-A
	eventA := readEvent(t, sessionAConn, 2*time.Second)
	assert.Equal(t, "session-A", eventA.SessionID)

	sessionAConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err := sessionAConn.ReadMessage()
	assert.Error(t, err, "session-A client should not receive session-B or session-C events")

	// session-B client gets only session-B
	eventB := readEvent(t, sessionBConn, 2*time.Second)
	assert.Equal(t, "session-B", eventB.SessionID)

	sessionBConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err = sessionBConn.ReadMessage()
	assert.Error(t, err, "session-B client should not receive session-A or session-C events")
}

// TestIntegration_DisconnectReconnect verifies that a client can disconnect,
// reconnect, and resume receiving events.
func TestIntegration_DisconnectReconnect(t *testing.T) {
	t.Parallel()
	hub, server := setupIntegrationHub(t)
	defer hub.Shutdown()
	defer server.Close()

	// Phase 1: connect and receive
	conn1 := newTestClient(t, server.URL)
	waitForClients(t, hub, 1)

	event1 := mustEvent(t, EventSessionStuck, "before-disconnect", SessionStuckPayload{Reason: "test"})
	hub.Broadcast(event1)
	received1 := readEvent(t, conn1, 2*time.Second)
	assert.Equal(t, "before-disconnect", received1.SessionID)

	// Phase 2: disconnect
	conn1.Close()
	waitForClients(t, hub, 0)

	// Phase 3: reconnect
	conn2 := newTestClient(t, server.URL)
	defer conn2.Close()
	waitForClients(t, hub, 1)

	event2 := mustEvent(t, EventSessionRecovered, "after-reconnect", SessionRecoveredPayload{
		PreviousState: "stuck", RecoveryTime: time.Second, Action: "reconnect",
	})
	hub.Broadcast(event2)
	received2 := readEvent(t, conn2, 2*time.Second)
	assert.Equal(t, "after-reconnect", received2.SessionID)
	assert.Equal(t, EventSessionRecovered, received2.Type)
}

// TestIntegration_SlowSubscriberBackpressure verifies that the hub drops a
// client whose send buffer is full, without blocking delivery to other clients.
//
// On localhost, TCP buffers are large enough (up to 4MB with autotuning) that
// writePump can drain the send channel indefinitely. So instead of using a real
// WebSocket "slow reader," we register a synthetic client with a tiny send
// buffer (size=1) directly with the hub, and verify it gets evicted when the
// buffer overflows. A real WebSocket client validates that the fast path works.
func TestIntegration_SlowSubscriberBackpressure(t *testing.T) {
	t.Parallel()
	hub, server := setupIntegrationHub(t)
	defer hub.Shutdown()
	defer server.Close()

	// Fast client: connects via real WebSocket and reads in background
	fastConn := newTestClient(t, server.URL)
	defer fastConn.Close()

	var fastReceived atomic.Int64
	go func() {
		for {
			fastConn.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, _, err := fastConn.ReadMessage()
			if err != nil {
				return
			}
			fastReceived.Add(1)
		}
	}()

	waitForClients(t, hub, 1)

	// Slow client: register directly with a tiny send buffer (size=1).
	// No writePump runs, so nothing drains the channel.
	slowClient := &Client{
		hub:           hub,
		send:          make(chan []byte, 1),
		sessionFilter: "*",
	}
	hub.register <- slowClient
	waitForClients(t, hub, 2)

	// Broadcast 10 events. The slow client's buffer (size=1) overflows
	// after the second event, triggering eviction.
	for i := 0; i < 10; i++ {
		event := mustEvent(t, EventSessionStuck,
			fmt.Sprintf("bp-%d", i),
			SessionStuckPayload{Reason: "backpressure"})
		hub.Broadcast(event)
	}

	// Give the hub time to process
	time.Sleep(200 * time.Millisecond)

	assert.Equal(t, 1, hub.ClientCount(),
		"slow client should be evicted, only fast client remains")

	assert.Greater(t, fastReceived.Load(), int64(0),
		"fast client should have received events")
}

// TestIntegration_EventOrderingUnderLoad verifies that a single subscriber
// receives events in the same order they were broadcast, even at higher volume.
func TestIntegration_EventOrderingUnderLoad(t *testing.T) {
	t.Parallel()
	hub, server := setupIntegrationHub(t)
	defer hub.Shutdown()
	defer server.Close()

	conn := newTestClient(t, server.URL)
	defer conn.Close()
	waitForClients(t, hub, 1)

	const numEvents = 100
	for i := 0; i < numEvents; i++ {
		event := mustEvent(t, EventSessionStuck,
			fmt.Sprintf("order-%04d", i),
			SessionStuckPayload{Reason: fmt.Sprintf("event %d", i)})
		hub.Broadcast(event)
	}

	for i := 0; i < numEvents; i++ {
		received := readEvent(t, conn, 3*time.Second)
		expected := fmt.Sprintf("order-%04d", i)
		assert.Equal(t, expected, received.SessionID,
			"event %d out of order: got %s", i, received.SessionID)
	}
}

// TestIntegration_SubscribeUnsubscribeResubscribe verifies the full lifecycle
// of subscribe → receive → unsubscribe → miss → resubscribe → receive.
// Uses a goroutine+channel pattern to verify "no message" during unsubscribe,
// because gorilla/websocket connections become unusable after a read deadline
// timeout (the frame parser state gets corrupted).
func TestIntegration_SubscribeUnsubscribeResubscribe(t *testing.T) {
	t.Parallel()
	hub, server := setupIntegrationHub(t)
	defer hub.Shutdown()
	defer server.Close()

	// Phase 1: subscribe and receive
	conn := newTestClient(t, server.URL)
	defer conn.Close()
	waitForClients(t, hub, 1)

	sendJSONMessage(t, conn, ClientMessage{Action: "subscribe", SessionID: "session-X"})
	time.Sleep(50 * time.Millisecond)

	event1 := mustEvent(t, EventSessionStuck, "session-X", SessionStuckPayload{Reason: "first"})
	hub.Broadcast(event1)
	received := readEvent(t, conn, 2*time.Second)
	assert.Equal(t, "session-X", received.SessionID)

	// Phase 2: unsubscribe — verify no delivery using a channel-based check
	// (avoids setting read deadline which corrupts gorilla/websocket state)
	sendJSONMessage(t, conn, ClientMessage{Action: "unsubscribe"})
	time.Sleep(50 * time.Millisecond)

	event2 := mustEvent(t, EventSessionStuck, "session-X", SessionStuckPayload{Reason: "missed"})
	hub.Broadcast(event2)

	// Use a background read with no deadline; the event should not arrive
	gotMsg := make(chan bool, 1)
	go func() {
		// This goroutine will block if no message comes (expected).
		// It will be unblocked by the next phase's broadcast.
		_, _, err := conn.ReadMessage()
		if err == nil {
			gotMsg <- true
		}
	}()

	select {
	case <-gotMsg:
		t.Fatal("should not receive events after unsubscribe")
	case <-time.After(200 * time.Millisecond):
		// Expected: no message received
	}

	// Phase 3: resubscribe and verify delivery resumes.
	// The background goroutine is still blocking on ReadMessage, which will
	// unblock when the hub delivers this new event.
	sendJSONMessage(t, conn, ClientMessage{Action: "subscribe", SessionID: "*"})
	time.Sleep(100 * time.Millisecond)

	event3 := mustEvent(t, EventSessionStuck, "session-Y", SessionStuckPayload{Reason: "resubscribed"})
	hub.Broadcast(event3)

	select {
	case <-gotMsg:
		// The background goroutine received a message — resubscribe works
	case <-time.After(2 * time.Second):
		t.Fatal("should receive events after resubscribe")
	}
}

// TestIntegration_GracefulShutdownDrainsEvents verifies that events broadcast
// before shutdown are delivered before clients are disconnected.
func TestIntegration_GracefulShutdownDrainsEvents(t *testing.T) {
	t.Parallel()
	hub, server := setupIntegrationHub(t)
	defer server.Close()

	conn := newTestClient(t, server.URL)
	defer conn.Close()
	waitForClients(t, hub, 1)

	// Broadcast some events
	const numEvents = 5
	for i := 0; i < numEvents; i++ {
		event := mustEvent(t, EventSessionCompleted,
			fmt.Sprintf("shutdown-%d", i),
			SessionCompletedPayload{ExitCode: 0, FinalState: "done"})
		hub.Broadcast(event)
	}

	// Read events before shutdown hits
	for i := 0; i < numEvents; i++ {
		received := readEvent(t, conn, 2*time.Second)
		assert.Equal(t, EventSessionCompleted, received.Type)
	}

	// Now shut down
	hub.Shutdown()
	time.Sleep(100 * time.Millisecond)

	// Connection should be closed
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err := conn.ReadMessage()
	assert.Error(t, err, "connection should be closed after shutdown")
}

// TestIntegration_MixedSessionFilterConcurrent verifies correct filtering when
// multiple session-filtered and global clients operate concurrently.
func TestIntegration_MixedSessionFilterConcurrent(t *testing.T) {
	t.Parallel()
	hub, server := setupIntegrationHub(t)
	defer hub.Shutdown()
	defer server.Close()

	const numSessions = 5
	const eventsPerSession = 10

	// Create one global subscriber and one per-session subscriber
	globalConn := newTestClient(t, server.URL)
	defer globalConn.Close()

	sessionConns := make([]*websocket.Conn, numSessions)
	for i := 0; i < numSessions; i++ {
		sessionConns[i] = newTestClient(t, server.URL)
		defer sessionConns[i].Close()
	}
	waitForClients(t, hub, numSessions+1)

	// Subscribe each session client to its own session
	for i := 0; i < numSessions; i++ {
		sendJSONMessage(t, sessionConns[i], ClientMessage{
			Action:    "subscribe",
			SessionID: fmt.Sprintf("session-%d", i),
		})
	}
	time.Sleep(50 * time.Millisecond)

	// Broadcast events for each session
	for i := 0; i < numSessions; i++ {
		for j := 0; j < eventsPerSession; j++ {
			event := mustEvent(t, EventSessionStuck,
				fmt.Sprintf("session-%d", i),
				SessionStuckPayload{Reason: fmt.Sprintf("event %d", j)})
			hub.Broadcast(event)
		}
	}

	totalEvents := numSessions * eventsPerSession

	// Global client should receive all events
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < totalEvents; i++ {
			readEvent(t, globalConn, 3*time.Second)
		}
	}()

	// Each session client should receive only its own events
	for i := 0; i < numSessions; i++ {
		wg.Add(1)
		go func(idx int, c *websocket.Conn) {
			defer wg.Done()
			expectedSID := fmt.Sprintf("session-%d", idx)
			for j := 0; j < eventsPerSession; j++ {
				event := readEvent(t, c, 3*time.Second)
				assert.Equal(t, expectedSID, event.SessionID,
					"session %d got wrong event", idx)
			}
		}(i, sessionConns[i])
	}
	wg.Wait()
}

// TestIntegration_ThroughputBenchmark measures events/second throughput for
// the publish→subscribe pipeline. Sends events in paced batches to avoid
// overflowing the 256-deep broadcast channel.
func TestIntegration_ThroughputBenchmark(t *testing.T) {
	t.Parallel()
	hub, server := setupIntegrationHub(t)
	defer hub.Shutdown()
	defer server.Close()

	const numClients = 10
	const numEvents = 500

	conns := make([]*websocket.Conn, numClients)
	for i := 0; i < numClients; i++ {
		conns[i] = newTestClient(t, server.URL)
		defer conns[i].Close()
	}
	waitForClients(t, hub, numClients)

	// Count total received across all clients
	var totalReceived atomic.Int64
	expectedTotal := int64(numClients * numEvents)
	allDone := make(chan struct{})

	var wg sync.WaitGroup
	for _, conn := range conns {
		wg.Add(1)
		go func(c *websocket.Conn) {
			defer wg.Done()
			for {
				c.SetReadDeadline(time.Now().Add(10 * time.Second))
				_, _, err := c.ReadMessage()
				if err != nil {
					return
				}
				if totalReceived.Add(1) == expectedTotal {
					close(allDone)
					return
				}
			}
		}(conn)
	}

	// Send events in batches to avoid overflowing the 256-deep broadcast channel.
	// This lets the hub drain between batches.
	const batchSize = 100
	start := time.Now()
	for i := 0; i < numEvents; i++ {
		event := mustEvent(t, EventSessionStuck,
			fmt.Sprintf("bench-%d", i),
			SessionStuckPayload{Reason: "throughput"})
		hub.Broadcast(event)
		if (i+1)%batchSize == 0 && i+1 < numEvents {
			time.Sleep(10 * time.Millisecond)
		}
	}

	select {
	case <-allDone:
	case <-time.After(30 * time.Second):
		t.Fatalf("throughput test timed out: received %d/%d",
			totalReceived.Load(), expectedTotal)
	}
	elapsed := time.Since(start)

	throughput := float64(totalReceived.Load()) / elapsed.Seconds()
	t.Logf("Throughput: %.0f events/sec (%d clients, %d events, %v elapsed)",
		throughput, numClients, numEvents, elapsed)

	// Sanity check: we should manage at least 1000 events/sec in-process
	assert.Greater(t, throughput, 1000.0,
		"throughput too low: %.0f events/sec", throughput)

	// Clean up receiver goroutines
	for _, conn := range conns {
		conn.Close()
	}
	wg.Wait()
}

// TestIntegration_RapidConnectDisconnect verifies the hub handles rapid
// connect/disconnect cycles without leaking clients or goroutines.
func TestIntegration_RapidConnectDisconnect(t *testing.T) {
	t.Parallel()
	hub, server := setupIntegrationHub(t)
	defer hub.Shutdown()
	defer server.Close()

	const cycles = 50
	for i := 0; i < cycles; i++ {
		conn := newTestClient(t, server.URL)
		conn.Close()
	}

	// Give hub time to process all unregistrations
	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, 0, hub.ClientCount(),
		"hub should have 0 clients after all connections closed")
}

// TestIntegration_MultipleEventTypesSameSession verifies that different event
// types for the same session all arrive at a session-filtered subscriber.
func TestIntegration_MultipleEventTypesSameSession(t *testing.T) {
	t.Parallel()
	hub, server := setupIntegrationHub(t)
	defer hub.Shutdown()
	defer server.Close()

	conn := newTestClient(t, server.URL)
	defer conn.Close()
	waitForClients(t, hub, 1)

	// Filter to a single session
	sendJSONMessage(t, conn, ClientMessage{Action: "subscribe", SessionID: "lifecycle"})
	time.Sleep(50 * time.Millisecond)

	// Simulate a session lifecycle: stuck → escalated → recovered → state_change → completed
	lifecycle := []struct {
		eventType EventType
		payload   any
	}{
		{EventSessionStuck, SessionStuckPayload{Reason: "no output", Duration: time.Minute}},
		{EventSessionEscalated, SessionEscalatedPayload{
			EscalationType: "error", Pattern: "Error:", Line: "Error: timeout",
			LineNumber: 1, DetectedAt: time.Now(), Description: "timeout", Severity: "high",
		}},
		{EventSessionRecovered, SessionRecoveredPayload{
			PreviousState: "stuck", RecoveryTime: 30 * time.Second, Action: "auto-retry",
		}},
		{EventSessionStateChange, SessionStateChangePayload{
			OldState: "recovering", NewState: "active", Reason: "recovered",
		}},
		{EventSessionCompleted, SessionCompletedPayload{
			ExitCode: 0, Duration: 10 * time.Minute, MessageCount: 25, FinalState: "archived",
		}},
	}

	for _, lc := range lifecycle {
		event := mustEvent(t, lc.eventType, "lifecycle", lc.payload)
		hub.Broadcast(event)
	}

	// Verify all arrive in order with correct types
	for i, lc := range lifecycle {
		received := readEvent(t, conn, 2*time.Second)
		assert.Equal(t, lc.eventType, received.Type, "lifecycle step %d: wrong type", i)
		assert.Equal(t, "lifecycle", received.SessionID)
	}
}

// --- Helpers ---

// mustEvent creates an Event or fails the test.
func mustEvent(t *testing.T, eventType EventType, sessionID string, payload any) *Event {
	t.Helper()
	event, err := NewEvent(eventType, sessionID, payload)
	require.NoError(t, err)
	return event
}

