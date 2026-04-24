# EventBus Performance Test Suite

This directory contains comprehensive performance tests for the AGM EventBus WebSocket implementation.

## Overview

The performance test suite validates that the EventBus meets the <1s event delivery latency requirement under various load conditions.

## Test Files

- **`eventbus_load_test.go`** - Complete load testing suite

## Test Scenarios

### 1. Baseline Test (`TestBaseline`)

**Purpose:** Establish baseline performance with minimal load

**Configuration:**
- 1 WebSocket client
- 100 events
- Measures single-client latency

**Duration:** ~0.5 seconds

**Run:**
```bash
go test -v ./test/performance/... -run TestBaseline
```

---

### 2. Load Test (`TestLoad`)

**Purpose:** Validate performance under target concurrent connection load

**Configuration:**
- 100 concurrent WebSocket clients
- 10 events per client (1,000 total)
- Measures connection time and event delivery latency

**Duration:** ~2 seconds

**Run:**
```bash
go test -v ./test/performance/... -run TestLoad
```

---

### 3. Burst Test (`TestBurst`)

**Purpose:** Test system behavior under sudden high-volume event bursts

**Configuration:**
- 100 concurrent WebSocket clients
- 100 events per client (10,000 total)
- Events broadcast as fast as possible

**Duration:** ~5 seconds

**Run:**
```bash
go test -v ./test/performance/... -run TestBurst
```

---

### 4. Sustained Test (`TestSustained`)

**Purpose:** Validate sustained performance over extended operation

**Configuration:**
- 50 concurrent WebSocket clients
- 5-minute test duration
- 10 events/second per client (~15,000 total events)

**Duration:** 5 minutes

**Run:**
```bash
go test -v ./test/performance/... -run TestSustained
```

---

## Running Tests

### Run All Performance Tests

```bash
cd main/agm
go test -v ./test/performance/... -timeout 30m
```

### Run Individual Tests

```bash
# Baseline
go test -v ./test/performance/... -run TestBaseline

# Load
go test -v ./test/performance/... -run TestLoad

# Burst
go test -v ./test/performance/... -run TestBurst

# Sustained (takes 5 minutes)
go test -v ./test/performance/... -run TestSustained
```

### Skip Long Tests

Use `-short` flag to skip sustained test:

```bash
go test -v -short ./test/performance/...
```

### Run Benchmarks

```bash
go test -bench=BenchmarkEventBusLoad ./test/performance/... -benchtime=10s
```

---

## Performance Metrics

Each test reports the following metrics:

### Connection Time (where applicable)
- **Min/Max/Mean:** Connection establishment time range
- **p50/p95/p99:** Percentile latencies

### Event Delivery Latency
- **Min/Max/Mean:** Event delivery time range
- **p50/p95/p99:** Percentile latencies
- **p99 Requirement:** Must be <1s

### Throughput
- **Events/second:** Total events divided by test duration

### Memory Usage
- **Alloc:** Current heap allocation
- **TotalAlloc:** Cumulative allocation
- **Sys:** Total memory obtained from OS
- **NumGC:** Number of garbage collections

---

## Example Output

```
================================================================================
Performance Report: Load (100 clients)
================================================================================

Test Configuration:
  Clients:             100
  Events per client:   10
  Total events:        1,000
  Test duration:       1.847s

Connection Time:
  Min:                 842µs
  Max:                 24.3ms
  Mean:                5.2ms
  p50:                 4.1ms
  p95:                 12.8ms
  p99:                 18.9ms

Event Delivery Latency:
  Min:                 234µs
  Max:                 8.7ms
  Mean:                1.9ms
  p50:                 1.6ms
  p95:                 4.2ms
  p99:                 6.8ms ⭐ (requirement: <1s)

Throughput:
  Events/second:       541.47

Memory Usage:
  Alloc:               8 MB
  TotalAlloc:          24 MB
  Sys:                 28 MB
  NumGC:               5

Performance Status:
  ✅ PASS: p99 latency (6.8ms) is below 1s requirement
================================================================================
```

---

## Test Architecture

### WebSocket Client Creation

Tests use `httptest.NewServer()` to create an in-memory HTTP server with WebSocket upgrade capability:

```go
hub := eventbus.NewHub()
go hub.Run()

server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    eventbus.ServeWebSocket(hub, w, r)
}))

conn := newTestClient(t, server.URL)
```

### Latency Measurement

Latency is measured from event creation to client reception:

```go
// Create event with timestamp
event, _ := eventbus.NewEvent(eventType, sessionID, payload)
// event.Timestamp is set to time.Now()

// Broadcast event
hub.Broadcast(event)

// Client receives event
receiveTime := time.Now()

// Calculate latency
latency := receiveTime.Sub(event.Timestamp)
```

This measures true end-to-end latency including:
- Event creation and marshaling
- Hub broadcast distribution
- WebSocket transmission
- Client reception and unmarshaling

### Concurrent Client Pattern

Tests use goroutines and sync.WaitGroup for concurrent clients:

```go
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
```

### Latency Collection

Thread-safe latency collection with mutex:

```go
var latencies []time.Duration
var mu sync.Mutex

// In receiver goroutine
mu.Lock()
latencies = append(latencies, latency)
mu.Unlock()

// After test
metrics := calculateLatencyMetrics(latencies)
```

---

## Percentile Calculation

Latency percentiles are calculated using sorted duration slices:

```go
func calculatePercentiles(latencies []time.Duration) (p50, p95, p99 time.Duration) {
    sort.Slice(latencies, func(i, j int) bool {
        return latencies[i] < latencies[j]
    })

    n := len(latencies)
    p50 = latencies[n*50/100]
    p95 = latencies[n*95/100]
    p99 = latencies[n*99/100]
    return
}
```

Standard deviation calculation:

```go
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
stddev := time.Duration(math.Sqrt(variance))
```

---

## Performance Requirements

### Primary Requirement

**Event Delivery Latency (p99) < 1 second**

This is the critical SLA for the EventBus. All tests verify that p99 latency remains below 1 second.

### Secondary Goals

- **Concurrent Connections:** Support 100+ simultaneous WebSocket clients
- **Throughput:** Handle 500+ events/second
- **Stability:** No memory leaks or degradation over 5-minute sustained test
- **Reliability:** Zero dropped connections under normal load

---

## Test Results Summary

See `PHASE-3-PERFORMANCE.md` for detailed results.

**Quick Summary:**

| Test      | Clients | Events | p99 Latency | Throughput | Status |
|-----------|---------|--------|-------------|------------|--------|
| Baseline  | 1       | 100    | 1.1ms       | 408/s      | ✅ PASS |
| Load      | 100     | 1,000  | 6.8ms       | 541/s      | ✅ PASS |
| Burst     | 100     | 10,000 | 28.7ms      | 2,923/s    | ✅ PASS |
| Sustained | 50      | 150,000| 7.3ms       | 500/s      | ✅ PASS |

**All tests pass with p99 latency 35-909x better than 1s requirement.**

---

## Troubleshooting

### Tests Fail with Connection Errors

**Symptom:** `Failed to dial WebSocket` errors

**Solution:** Ensure no other process is using the test port. Tests use `httptest` which should auto-select available ports.

### Tests Timeout

**Symptom:** Test times out waiting for events

**Solution:**
- Check for deadlocks in event receiving goroutines
- Verify hub is running (`go hub.Run()`)
- Increase timeout in test code

### High Latency Results

**Symptom:** p99 latency higher than expected

**Solution:**
- Ensure test machine is not under heavy load
- Close other applications consuming CPU/memory
- Run tests with `-count=1` to disable test caching
- Check for CPU throttling or resource limits

### Memory Leaks Detected

**Symptom:** Memory usage grows continuously in sustained test

**Solution:**
- Verify all WebSocket connections are properly closed
- Check for goroutine leaks (use `runtime.NumGoroutine()`)
- Ensure clients unregister from hub on disconnect

---

## Adding New Tests

To add a new performance test:

1. Create test function following pattern:

```go
func TestMyScenario(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping test in short mode")
    }

    // Setup hub and server
    hub := eventbus.NewHub()
    go hub.Run()
    defer hub.Shutdown()

    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        eventbus.ServeWebSocket(hub, w, r)
    }))
    defer server.Close()

    // Test implementation
    // ...

    // Calculate metrics
    metrics := calculateLatencyMetrics(latencies)

    // Print report
    report := PerformanceReport{
        TestName: "My Scenario",
        // ... fill in metrics
    }
    printReport(t, report)

    // Verify requirement
    if metrics.P99 > time.Second {
        t.Errorf("p99 latency %v exceeds 1s", metrics.P99)
    }
}
```

2. Update this README with test description

3. Update `PHASE-3-PERFORMANCE.md` with results

---

## Dependencies

- `github.com/gorilla/websocket` - WebSocket client/server
- `github.com/stretchr/testify/require` - Test assertions
- `net/http/httptest` - HTTP test server
- Standard library: `sync`, `time`, `sort`, `runtime`

---

## Integration with CI/CD

### Recommended CI Configuration

```yaml
# Example GitHub Actions config
- name: Run Performance Tests
  run: |
    cd agm
    go test -v ./test/performance/... -timeout 30m
  timeout-minutes: 35
```

### Test Isolation

Performance tests should run on dedicated CI runners to avoid interference from other jobs.

### Performance Regression Detection

Consider failing CI if p99 latency exceeds threshold:

```go
const maxP99Latency = 100 * time.Millisecond // 10% of requirement

if metrics.P99 > maxP99Latency {
    t.Errorf("Performance regression: p99 %v exceeds %v", metrics.P99, maxP99Latency)
}
```

---

## Related Documentation

- **PHASE-3-PERFORMANCE.md** - Detailed performance test results
- **internal/eventbus/websocket.go** - EventBus implementation
- **internal/eventbus/websocket_test.go** - Unit tests
- **internal/eventbus/schema.go** - Event schema definitions

---

**Last Updated:** 2026-02-14
**Maintainer:** AGM Team
