package performance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

// LatencyMetrics holds latency measurements
type LatencyMetrics struct {
	Min    time.Duration
	Max    time.Duration
	Mean   time.Duration
	P50    time.Duration
	P95    time.Duration
	P99    time.Duration
	StdDev time.Duration
}

// PerformanceReport holds complete performance test results
type PerformanceReport struct {
	TestName             string
	NumClients           int
	EventsPerClient      int
	TotalEvents          int
	ConnectionTime       LatencyMetrics
	EventDeliveryLatency LatencyMetrics
	Throughput           float64 // events/second
	DroppedConnections   int
	TestDuration         time.Duration
	MemoryUsage          runtime.MemStats
}


// calculateLatencyMetrics computes comprehensive latency statistics
func calculateLatencyMetrics(latencies []time.Duration) LatencyMetrics {
	if len(latencies) == 0 {
		return LatencyMetrics{}
	}

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	n := len(latencies)
	min := latencies[0]
	max := latencies[n-1]
	p50 := latencies[n*50/100]
	p95 := latencies[n*95/100]
	p99 := latencies[n*99/100]

	// Calculate mean
	var sum time.Duration
	for _, lat := range latencies {
		sum += lat
	}
	mean := sum / time.Duration(n)

	// Calculate standard deviation
	var variance float64
	for _, lat := range latencies {
		diff := float64(lat - mean)
		variance += diff * diff
	}
	variance /= float64(n)
	stddev := time.Duration(float64(time.Nanosecond) * (variance))

	return LatencyMetrics{
		Min:    min,
		Max:    max,
		Mean:   mean,
		P50:    p50,
		P95:    p95,
		P99:    p99,
		StdDev: stddev,
	}
}

// newTestClient creates a WebSocket client for testing
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

// EventWithTimestamp wraps an event with creation timestamp for latency tracking
type EventWithTimestamp struct {
	Event     *eventbus.Event
	CreatedAt time.Time
}

// TestBaseline tests a single client receiving events
func TestBaseline(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping baseline test in short mode")
	}

	hub := eventbus.NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eventbus.ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	// Connect single client
	conn := newTestClient(t, server.URL)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond) // Let client register

	const numEvents = 100
	latencies := make([]time.Duration, 0, numEvents)
	var mu sync.Mutex

	// Start receiving events
	done := make(chan bool)
	receivedEvents := make(map[string]time.Time)
	go func() {
		for i := 0; i < numEvents; i++ {
			_, data, err := conn.ReadMessage()
			if err != nil {
				t.Errorf("Failed to read message: %v", err)
				return
			}

			var event eventbus.Event
			if err := json.Unmarshal(data, &event); err != nil {
				t.Errorf("Failed to unmarshal event: %v", err)
				return
			}

			mu.Lock()
			receivedEvents[event.SessionID] = time.Now()
			mu.Unlock()
		}
		done <- true
	}()

	// Send events and measure latency
	testStart := time.Now()
	eventTimestamps := make(map[string]time.Time)

	for i := 0; i < numEvents; i++ {
		sessionID := fmt.Sprintf("session-%d", i)
		event, err := eventbus.NewEvent(
			eventbus.EventSessionStuck,
			sessionID,
			eventbus.SessionStuckPayload{
				Reason:   "Test event",
				Duration: 5 * time.Minute,
			},
		)
		require.NoError(t, err)

		sendTime := time.Now()
		eventTimestamps[sessionID] = sendTime
		hub.Broadcast(event)
	}

	// Wait for all events to be received
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for events")
	}

	testDuration := time.Since(testStart)

	// Calculate latencies
	mu.Lock()
	for sessionID, receiveTime := range receivedEvents {
		if sendTime, ok := eventTimestamps[sessionID]; ok {
			latency := receiveTime.Sub(sendTime)
			latencies = append(latencies, latency)
		}
	}
	mu.Unlock()

	// Calculate metrics
	metrics := calculateLatencyMetrics(latencies)
	throughput := float64(numEvents) / testDuration.Seconds()

	// Memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	report := PerformanceReport{
		TestName:             "Baseline",
		NumClients:           1,
		EventsPerClient:      numEvents,
		TotalEvents:          numEvents,
		EventDeliveryLatency: metrics,
		Throughput:           throughput,
		TestDuration:         testDuration,
		MemoryUsage:          memStats,
	}

	printReport(t, report)

	// Verify performance
	if metrics.P99 > time.Second {
		t.Errorf("Baseline p99 latency %v exceeds 1s requirement", metrics.P99)
	}
}

// TestLoad tests 100 concurrent clients
func TestLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	hub := eventbus.NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eventbus.ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	const numClients = 100
	const eventsPerClient = 10
	const totalEvents = numClients * eventsPerClient

	// Track connection times
	connectionTimes := make([]time.Duration, numClients)
	var connMu sync.Mutex

	// Connect clients
	var wg sync.WaitGroup
	clients := make([]*websocket.Conn, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			start := time.Now()
			conn := newTestClient(t, server.URL)
			connTime := time.Since(start)

			connMu.Lock()
			clients[idx] = conn
			connectionTimes[idx] = connTime
			connMu.Unlock()
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond) // Let all clients register

	// Verify all clients connected
	require.Equal(t, numClients, hub.ClientCount(), "Not all clients connected")

	// Track event latencies
	latencies := make([]time.Duration, 0, totalEvents)
	var latMu sync.Mutex
	receivedCount := 0
	allReceived := make(chan bool)

	// Start receivers
	for _, conn := range clients {
		wg.Add(1)
		go func(c *websocket.Conn) {
			defer wg.Done()
			for i := 0; i < eventsPerClient; i++ {
				_, data, err := c.ReadMessage()
				if err != nil {
					t.Errorf("Failed to read message: %v", err)
					return
				}

				receiveTime := time.Now()

				var event eventbus.Event
				if err := json.Unmarshal(data, &event); err != nil {
					t.Errorf("Failed to unmarshal event: %v", err)
					return
				}

				// Calculate latency from event timestamp
				latency := receiveTime.Sub(event.Timestamp)

				latMu.Lock()
				latencies = append(latencies, latency)
				receivedCount++
				if receivedCount == totalEvents {
					close(allReceived)
				}
				latMu.Unlock()
			}
		}(conn)
	}

	// Broadcast events
	testStart := time.Now()
	for i := 0; i < totalEvents; i++ {
		event, err := eventbus.NewEvent(
			eventbus.EventSessionStuck,
			fmt.Sprintf("session-%d", i),
			eventbus.SessionStuckPayload{
				Reason:   "Load test event",
				Duration: 5 * time.Minute,
			},
		)
		require.NoError(t, err)
		hub.Broadcast(event)
	}

	// Wait for all events to be received
	select {
	case <-allReceived:
	case <-time.After(30 * time.Second):
		latMu.Lock()
		received := receivedCount
		latMu.Unlock()
		t.Fatalf("Timeout waiting for events: received %d/%d", received, totalEvents)
	}

	testDuration := time.Since(testStart)

	// Close all clients
	for _, conn := range clients {
		if conn != nil {
			conn.Close()
		}
	}

	wg.Wait()

	// Calculate metrics
	connMetrics := calculateLatencyMetrics(connectionTimes)
	latencyMetrics := calculateLatencyMetrics(latencies)
	throughput := float64(totalEvents) / testDuration.Seconds()

	// Memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	report := PerformanceReport{
		TestName:             "Load (100 clients)",
		NumClients:           numClients,
		EventsPerClient:      eventsPerClient,
		TotalEvents:          totalEvents,
		ConnectionTime:       connMetrics,
		EventDeliveryLatency: latencyMetrics,
		Throughput:           throughput,
		TestDuration:         testDuration,
		MemoryUsage:          memStats,
	}

	printReport(t, report)

	// Verify performance requirements
	if latencyMetrics.P99 > time.Second {
		t.Errorf("Load test p99 latency %v exceeds 1s requirement", latencyMetrics.P99)
	}
}

// TestBurst tests 100 clients receiving 100 events in rapid succession
func TestBurst(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping burst test in short mode")
	}

	hub := eventbus.NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eventbus.ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	const numClients = 100
	const eventsPerClient = 100
	const totalEvents = numClients * eventsPerClient

	// Connect clients
	var wg sync.WaitGroup
	clients := make([]*websocket.Conn, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			clients[idx] = newTestClient(t, server.URL)
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	require.Equal(t, numClients, hub.ClientCount())

	// Track latencies
	latencies := make([]time.Duration, 0, totalEvents)
	var latMu sync.Mutex
	receivedCount := 0
	allReceived := make(chan bool)

	// Start receivers
	for _, conn := range clients {
		wg.Add(1)
		go func(c *websocket.Conn) {
			defer wg.Done()
			for i := 0; i < eventsPerClient; i++ {
				_, data, err := c.ReadMessage()
				if err != nil {
					return
				}

				receiveTime := time.Now()

				var event eventbus.Event
				json.Unmarshal(data, &event)

				latency := receiveTime.Sub(event.Timestamp)

				latMu.Lock()
				latencies = append(latencies, latency)
				receivedCount++
				if receivedCount == totalEvents {
					close(allReceived)
				}
				latMu.Unlock()
			}
		}(conn)
	}

	// Burst send all events as fast as possible
	testStart := time.Now()
	for i := 0; i < totalEvents; i++ {
		event, _ := eventbus.NewEvent(
			eventbus.EventSessionStuck,
			fmt.Sprintf("session-%d", i),
			eventbus.SessionStuckPayload{
				Reason:   "Burst test event",
				Duration: 5 * time.Minute,
			},
		)
		hub.Broadcast(event)
	}

	// Wait for all events
	select {
	case <-allReceived:
	case <-time.After(60 * time.Second):
		latMu.Lock()
		received := receivedCount
		latMu.Unlock()
		t.Fatalf("Timeout in burst test: received %d/%d", received, totalEvents)
	}

	testDuration := time.Since(testStart)

	// Close clients
	for _, conn := range clients {
		if conn != nil {
			conn.Close()
		}
	}

	wg.Wait()

	// Calculate metrics
	latencyMetrics := calculateLatencyMetrics(latencies)
	throughput := float64(totalEvents) / testDuration.Seconds()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	report := PerformanceReport{
		TestName:             "Burst (100 clients, 100 events)",
		NumClients:           numClients,
		EventsPerClient:      eventsPerClient,
		TotalEvents:          totalEvents,
		EventDeliveryLatency: latencyMetrics,
		Throughput:           throughput,
		TestDuration:         testDuration,
		MemoryUsage:          memStats,
	}

	printReport(t, report)

	// Verify performance
	if latencyMetrics.P99 > time.Second {
		t.Errorf("Burst test p99 latency %v exceeds 1s requirement", latencyMetrics.P99)
	}
}

// TestSustained tests sustained load over 5 minutes
func TestSustained(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping sustained test in short mode")
	}

	hub := eventbus.NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eventbus.ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	const numClients = 50
	const testDuration = 5 * time.Minute
	const eventInterval = 100 * time.Millisecond // 10 events/sec per client

	// Connect clients
	var wg sync.WaitGroup
	clients := make([]*websocket.Conn, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			clients[idx] = newTestClient(t, server.URL)
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	require.Equal(t, numClients, hub.ClientCount())

	// Track latencies
	var latencies []time.Duration
	var latMu sync.Mutex

	// Start receivers — goroutines block on ReadMessage and exit when the
	// connection is closed during shutdown. Do NOT use SetReadDeadline here:
	// gorilla/websocket treats deadline-exceeded as a permanent connection
	// failure and panics on subsequent reads.
	for _, conn := range clients {
		wg.Add(1)
		go func(c *websocket.Conn) {
			defer wg.Done()
			for {
				_, data, err := c.ReadMessage()
				if err != nil {
					return
				}

				receiveTime := time.Now()

				var event eventbus.Event
				if json.Unmarshal(data, &event) == nil {
					latency := receiveTime.Sub(event.Timestamp)
					latMu.Lock()
					latencies = append(latencies, latency)
					latMu.Unlock()
				}
			}
		}(conn)
	}

	// Send events continuously
	testStart := time.Now()
	eventCount := 0
	ticker := time.NewTicker(eventInterval)
	defer ticker.Stop()

	timeout := time.After(testDuration)

loop:
	for {
		select {
		case <-timeout:
			break loop
		case <-ticker.C:
			for i := 0; i < numClients; i++ {
				event, _ := eventbus.NewEvent(
					eventbus.EventSessionStuck,
					fmt.Sprintf("session-%d-%d", i, eventCount),
					eventbus.SessionStuckPayload{
						Reason:   "Sustained test event",
						Duration: 5 * time.Minute,
					},
				)
				hub.Broadcast(event)
				eventCount++
			}
		}
	}

	actualDuration := time.Since(testStart)

	// Close client connections to unblock ReadMessage in receiver goroutines.
	for _, conn := range clients {
		if conn != nil {
			conn.Close()
		}
	}

	wg.Wait()

	// Calculate metrics
	latMu.Lock()
	latenciesCopy := make([]time.Duration, len(latencies))
	copy(latenciesCopy, latencies)
	latMu.Unlock()

	latencyMetrics := calculateLatencyMetrics(latenciesCopy)
	throughput := float64(eventCount) / actualDuration.Seconds()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	report := PerformanceReport{
		TestName:             "Sustained (50 clients, 5 minutes)",
		NumClients:           numClients,
		EventsPerClient:      eventCount / numClients,
		TotalEvents:          eventCount,
		EventDeliveryLatency: latencyMetrics,
		Throughput:           throughput,
		TestDuration:         actualDuration,
		MemoryUsage:          memStats,
	}

	printReport(t, report)

	// Verify performance
	if latencyMetrics.P99 > time.Second {
		t.Errorf("Sustained test p99 latency %v exceeds 1s requirement", latencyMetrics.P99)
	}
}

// printReport outputs a formatted performance report
func printReport(t *testing.T, report PerformanceReport) {
	t.Helper()

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Printf("Performance Report: %s\n", report.TestName)
	fmt.Println(strings.Repeat("=", 80))

	fmt.Printf("\nTest Configuration:\n")
	fmt.Printf("  Clients:             %d\n", report.NumClients)
	fmt.Printf("  Events per client:   %d\n", report.EventsPerClient)
	fmt.Printf("  Total events:        %d\n", report.TotalEvents)
	fmt.Printf("  Test duration:       %v\n", report.TestDuration)

	if report.ConnectionTime.Mean > 0 {
		fmt.Printf("\nConnection Time:\n")
		fmt.Printf("  Min:                 %v\n", report.ConnectionTime.Min)
		fmt.Printf("  Max:                 %v\n", report.ConnectionTime.Max)
		fmt.Printf("  Mean:                %v\n", report.ConnectionTime.Mean)
		fmt.Printf("  p50:                 %v\n", report.ConnectionTime.P50)
		fmt.Printf("  p95:                 %v\n", report.ConnectionTime.P95)
		fmt.Printf("  p99:                 %v\n", report.ConnectionTime.P99)
	}

	fmt.Printf("\nEvent Delivery Latency:\n")
	fmt.Printf("  Min:                 %v\n", report.EventDeliveryLatency.Min)
	fmt.Printf("  Max:                 %v\n", report.EventDeliveryLatency.Max)
	fmt.Printf("  Mean:                %v\n", report.EventDeliveryLatency.Mean)
	fmt.Printf("  p50:                 %v\n", report.EventDeliveryLatency.P50)
	fmt.Printf("  p95:                 %v\n", report.EventDeliveryLatency.P95)
	fmt.Printf("  p99:                 %v ⭐ (requirement: <1s)\n", report.EventDeliveryLatency.P99)

	fmt.Printf("\nThroughput:\n")
	fmt.Printf("  Events/second:       %.2f\n", report.Throughput)

	fmt.Printf("\nMemory Usage:\n")
	fmt.Printf("  Alloc:               %d MB\n", report.MemoryUsage.Alloc/1024/1024)
	fmt.Printf("  TotalAlloc:          %d MB\n", report.MemoryUsage.TotalAlloc/1024/1024)
	fmt.Printf("  Sys:                 %d MB\n", report.MemoryUsage.Sys/1024/1024)
	fmt.Printf("  NumGC:               %d\n", report.MemoryUsage.NumGC)

	fmt.Printf("\nPerformance Status:\n")
	if report.EventDeliveryLatency.P99 < time.Second {
		fmt.Printf("  ✅ PASS: p99 latency (%v) is below 1s requirement\n", report.EventDeliveryLatency.P99)
	} else {
		fmt.Printf("  ❌ FAIL: p99 latency (%v) exceeds 1s requirement\n", report.EventDeliveryLatency.P99)
	}

	fmt.Println(strings.Repeat("=", 80))
}

// TestFilteredLoad tests performance when clients use session filters.
// Verifies that filtering doesn't degrade p99 latency.
func TestFilteredLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping filtered load test in short mode")
	}

	hub := eventbus.NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eventbus.ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	const numSessions = 10
	const clientsPerSession = 10
	const numClients = numSessions * clientsPerSession
	const eventsPerSession = 50
	// Each client receives only events for its session
	const eventsPerClient = eventsPerSession
	const totalDelivered = numClients * eventsPerClient

	// Connect and subscribe clients
	var wg sync.WaitGroup
	clients := make([]*websocket.Conn, numClients)
	connectionTimes := make([]time.Duration, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			start := time.Now()
			conn := newTestClient(t, server.URL)
			connTime := time.Since(start)

			clients[idx] = conn
			connectionTimes[idx] = connTime

			// Subscribe to specific session
			sessionID := fmt.Sprintf("session-%d", idx%numSessions)
			msg, _ := json.Marshal(map[string]string{
				"action":     "subscribe",
				"session_id": sessionID,
			})
			conn.WriteMessage(websocket.TextMessage, msg)
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, numClients, hub.ClientCount())

	// Track latencies
	latencies := make([]time.Duration, 0, totalDelivered)
	var latMu sync.Mutex
	receivedCount := 0
	allReceived := make(chan bool)

	// Start receivers
	for _, conn := range clients {
		wg.Add(1)
		go func(c *websocket.Conn) {
			defer wg.Done()
			for i := 0; i < eventsPerClient; i++ {
				_, data, err := c.ReadMessage()
				if err != nil {
					return
				}
				receiveTime := time.Now()
				var event eventbus.Event
				if json.Unmarshal(data, &event) == nil {
					latency := receiveTime.Sub(event.Timestamp)
					latMu.Lock()
					latencies = append(latencies, latency)
					receivedCount++
					if receivedCount == totalDelivered {
						close(allReceived)
					}
					latMu.Unlock()
				}
			}
		}(conn)
	}

	// Broadcast events for each session, paced to avoid overflowing the
	// 256-deep broadcast channel. Send in rounds: one event per session
	// per round, with brief pauses between rounds.
	testStart := time.Now()
	for i := 0; i < eventsPerSession; i++ {
		for s := 0; s < numSessions; s++ {
			event, _ := eventbus.NewEvent(
				eventbus.EventSessionStuck,
				fmt.Sprintf("session-%d", s),
				eventbus.SessionStuckPayload{
					Reason:   "Filtered load test",
					Duration: time.Minute,
				},
			)
			hub.Broadcast(event)
		}
		if (i+1)%10 == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}

	select {
	case <-allReceived:
	case <-time.After(30 * time.Second):
		latMu.Lock()
		received := receivedCount
		latMu.Unlock()
		t.Fatalf("Timeout: received %d/%d", received, totalDelivered)
	}
	testDuration := time.Since(testStart)

	for _, conn := range clients {
		if conn != nil {
			conn.Close()
		}
	}
	wg.Wait()

	connMetrics := calculateLatencyMetrics(connectionTimes)
	latencyMetrics := calculateLatencyMetrics(latencies)
	throughput := float64(totalDelivered) / testDuration.Seconds()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	report := PerformanceReport{
		TestName:             fmt.Sprintf("Filtered (%d sessions, %d clients)", numSessions, numClients),
		NumClients:           numClients,
		EventsPerClient:      eventsPerClient,
		TotalEvents:          totalDelivered,
		ConnectionTime:       connMetrics,
		EventDeliveryLatency: latencyMetrics,
		Throughput:           throughput,
		TestDuration:         testDuration,
		MemoryUsage:          memStats,
	}
	printReport(t, report)

	if latencyMetrics.P99 > time.Second {
		t.Errorf("Filtered load p99 latency %v exceeds 1s", latencyMetrics.P99)
	}
}

// TestConnectionChurn tests event delivery while clients connect and disconnect.
func TestConnectionChurn(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping connection churn test in short mode")
	}

	hub := eventbus.NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eventbus.ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	const stableClients = 20
	const churnCycles = 50
	const eventsPerCycle = 10
	const totalEvents = churnCycles * eventsPerCycle

	// Connect stable clients that stay for the entire test.
	// Connections are closed explicitly after the churn loop.
	stableConns := make([]*websocket.Conn, stableClients)
	for i := 0; i < stableClients; i++ {
		stableConns[i] = newTestClient(t, server.URL)
	}
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, stableClients, hub.ClientCount())

	// Collect latencies from stable clients
	latencies := make([]time.Duration, 0, stableClients*totalEvents)
	var latMu sync.Mutex
	var wg sync.WaitGroup
	for _, conn := range stableConns {
		wg.Add(1)
		go func(c *websocket.Conn) {
			defer wg.Done()
			for {
				_, data, err := c.ReadMessage()
				if err != nil {
					return
				}
				receiveTime := time.Now()
				var event eventbus.Event
				if json.Unmarshal(data, &event) == nil {
					latency := receiveTime.Sub(event.Timestamp)
					latMu.Lock()
					latencies = append(latencies, latency)
					latMu.Unlock()
				}
			}
		}(conn)
	}

	// Run churn cycles: connect a client, broadcast events, disconnect
	testStart := time.Now()
	for cycle := 0; cycle < churnCycles; cycle++ {
		// Connect ephemeral client
		ephConn := newTestClient(t, server.URL)

		// Broadcast events
		for i := 0; i < eventsPerCycle; i++ {
			event, _ := eventbus.NewEvent(
				eventbus.EventSessionStateChange,
				fmt.Sprintf("churn-%d-%d", cycle, i),
				eventbus.SessionStateChangePayload{
					OldState: "active", NewState: "idle", Reason: "churn",
				},
			)
			hub.Broadcast(event)
		}

		// Disconnect ephemeral client
		ephConn.Close()
		time.Sleep(5 * time.Millisecond) // brief pause between cycles
	}
	testDuration := time.Since(testStart)

	// Give stable clients time to receive remaining events, then close
	// connections to unblock ReadMessage in receiver goroutines.
	time.Sleep(500 * time.Millisecond)
	for _, conn := range stableConns {
		conn.Close()
	}
	wg.Wait()

	// Calculate metrics
	latMu.Lock()
	latenciesCopy := make([]time.Duration, len(latencies))
	copy(latenciesCopy, latencies)
	latMu.Unlock()

	latencyMetrics := calculateLatencyMetrics(latenciesCopy)
	throughput := float64(len(latenciesCopy)) / testDuration.Seconds()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	report := PerformanceReport{
		TestName:             fmt.Sprintf("Connection Churn (%d stable + %d ephemeral cycles)", stableClients, churnCycles),
		NumClients:           stableClients,
		EventsPerClient:      len(latenciesCopy) / stableClients,
		TotalEvents:          len(latenciesCopy),
		EventDeliveryLatency: latencyMetrics,
		Throughput:           throughput,
		TestDuration:         testDuration,
		MemoryUsage:          memStats,
	}
	printReport(t, report)

	if latencyMetrics.P99 > time.Second {
		t.Errorf("Connection churn p99 latency %v exceeds 1s", latencyMetrics.P99)
	}

	// Verify hub cleaned up ephemeral clients
	time.Sleep(200 * time.Millisecond)
	assert.LessOrEqual(t, hub.ClientCount(), stableClients,
		"hub should have cleaned up all ephemeral clients")
}

// BenchmarkEventBusLoad is a standard Go benchmark
func BenchmarkEventBusLoad(b *testing.B) {
	hub := eventbus.NewHub()
	go hub.Run()
	defer hub.Shutdown()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eventbus.ServeWebSocket(hub, w, r)
	}))
	defer server.Close()

	const numClients = 100

	// Connect clients
	clients := make([]*websocket.Conn, numClients)
	for i := 0; i < numClients; i++ {
		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if err != nil {
			b.Fatalf("Failed to connect client %d: %v", i, err)
		}
		clients[i] = conn
		defer conn.Close()
	}

	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		event, _ := eventbus.NewEvent(
			eventbus.EventSessionStuck,
			fmt.Sprintf("session-%d", i),
			eventbus.SessionStuckPayload{
				Reason:   "Benchmark event",
				Duration: 5 * time.Minute,
			},
		)
		hub.Broadcast(event)
	}
}
