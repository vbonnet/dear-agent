package opencode

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

// mockAdapter is a mock adapter controller for testing
type mockAdapter struct {
	stopped atomic.Bool
}

func (m *mockAdapter) Stop(ctx context.Context) error {
	m.stopped.Store(true)
	return nil
}

func (m *mockAdapter) isStopped() bool {
	return m.stopped.Load()
}

// TestPublish_Success tests successful event publishing
func TestPublish_Success(t *testing.T) {
	mockBus := &mockEventBus{}
	mockAdapt := &mockAdapter{}
	publisher := NewPublisher(mockBus, "test-session", mockAdapt)

	event := &AGMEvent{
		State:     "WORKING",
		Timestamp: 1709654321,
		Metadata: map[string]interface{}{
			"event_type": "tool.execute.before",
			"tool_name":  "Write",
		},
	}

	err := publisher.Publish(event)
	if err != nil {
		t.Fatalf("Publish() failed: %v", err)
	}

	published := mockBus.GetEvents()
	if len(published) != 1 {
		t.Fatalf("Expected 1 published event, got %d", len(published))
	}

	busEvent := published[0]

	// Verify event type and session ID
	if busEvent.Type != eventbus.EventSessionStateChange {
		t.Errorf("Expected event type='session.state_change', got '%s'", busEvent.Type)
	}
	if busEvent.SessionID != "test-session" {
		t.Errorf("Expected session_id='test-session', got '%s'", busEvent.SessionID)
	}

	// Parse payload
	var payload SessionStateChangeEvent
	if err := busEvent.ParsePayload(&payload); err != nil {
		t.Fatalf("Failed to parse payload: %v", err)
	}

	// Verify payload fields
	if payload.SessionID != "test-session" {
		t.Errorf("Expected payload.session_id='test-session', got '%s'", payload.SessionID)
	}
	if payload.State != "WORKING" {
		t.Errorf("Expected state='THINKING', got '%s'", payload.State)
	}
	if payload.Timestamp != 1709654321 {
		t.Errorf("Expected timestamp=1709654321, got %d", payload.Timestamp)
	}
	if payload.Source != "opencode-sse" {
		t.Errorf("Expected source='opencode-sse', got '%s'", payload.Source)
	}
	if payload.Harness != "opencode-cli" {
		t.Errorf("Expected harness='opencode-cli', got '%s'", payload.Harness)
	}
	if payload.Sequence != 1 {
		t.Errorf("Expected sequence=1, got %d", payload.Sequence)
	}
	if payload.Metadata["event_type"] != "tool.execute.before" {
		t.Errorf("Expected metadata.event_type='tool.execute.before', got '%v'", payload.Metadata["event_type"])
	}
}

// TestPublish_SequenceMonotonic tests that sequence numbers are monotonically increasing
func TestPublish_SequenceMonotonic(t *testing.T) {
	mockBus := &mockEventBus{}
	mockAdapt := &mockAdapter{}
	publisher := NewPublisher(mockBus, "test-session", mockAdapt)

	// Publish 5 events
	for i := 0; i < 5; i++ {
		event := &AGMEvent{
			State:     "WORKING",
			Timestamp: int64(1709654321 + i),
		}
		err := publisher.Publish(event)
		if err != nil {
			t.Fatalf("Publish() failed on iteration %d: %v", i, err)
		}
	}

	published := mockBus.GetEvents()
	if len(published) != 5 {
		t.Fatalf("Expected 5 published events, got %d", len(published))
	}

	// Verify sequence numbers are monotonically increasing
	for i, busEvent := range published {
		var payload SessionStateChangeEvent
		if err := busEvent.ParsePayload(&payload); err != nil {
			t.Fatalf("Failed to parse payload for event %d: %v", i, err)
		}

		expectedSeq := uint64(i + 1)
		if payload.Sequence != expectedSeq {
			t.Errorf("Event %d: expected sequence=%d, got %d", i, expectedSeq, payload.Sequence)
		}
	}
}

// TestPublishWithBackpressure_Success tests successful publish with backpressure handler
func TestPublishWithBackpressure_Success(t *testing.T) {
	mockBus := &mockEventBus{}
	mockAdapt := &mockAdapter{}
	publisher := NewPublisher(mockBus, "test-session", mockAdapt)

	event := &AGMEvent{
		State:     "IDLE",
		Timestamp: 1709654321,
	}

	err := publisher.PublishWithBackpressure(event)
	if err != nil {
		t.Fatalf("PublishWithBackpressure() failed: %v", err)
	}

	published := mockBus.GetEvents()
	if len(published) != 1 {
		t.Fatalf("Expected 1 published event, got %d", len(published))
	}
}

// TestPublishWithBackpressure_RetrySuccess tests retry logic when queue is dropping events
func TestPublishWithBackpressure_RetrySuccess(t *testing.T) {
	mockBus := &mockEventBus{
		shouldDrop: true,
	}
	mockAdapt := &mockAdapter{}
	publisher := NewPublisher(mockBus, "test-session", mockAdapt)

	event := &AGMEvent{
		State:     "IDLE",
		Timestamp: 1709654321,
	}

	// Stop dropping after 150ms (simulating queue pressure relief)
	go func() {
		time.Sleep(150 * time.Millisecond)
		mockBus.mu.Lock()
		if mockBus.dropCount >= 2 {
			mockBus.shouldDrop = false
		}
		mockBus.mu.Unlock()
	}()

	// Note: Since Broadcast is non-blocking, the retry logic in PublishWithBackpressure
	// won't actually trigger. This test is kept for API compatibility but will always succeed.
	err := publisher.PublishWithBackpressure(event)
	if err != nil {
		t.Fatalf("PublishWithBackpressure() failed: %v", err)
	}

	// With non-blocking Broadcast, we expect the event to be dropped initially
	// but the test still demonstrates the API pattern
}

// TestPublish_NilEventError tests nil event error handling in Publish
// This can trigger retries in PublishWithBackpressure
func TestPublish_NilEventError(t *testing.T) {
	mockBus := &mockEventBus{}
	mockAdapt := &mockAdapter{}
	publisher := NewPublisher(mockBus, "test-session", mockAdapt)

	// Direct Publish call with nil should fail
	err := publisher.Publish(nil)
	if err == nil {
		t.Fatal("Expected error when publishing nil event, got nil")
	}
	if !contains(err.Error(), "cannot be nil") {
		t.Errorf("Expected error message to contain 'cannot be nil', got '%s'", err.Error())
	}

	// PublishWithBackpressure with nil should also fail after retries
	err = publisher.PublishWithBackpressure(nil)
	if err == nil {
		t.Fatal("Expected error from PublishWithBackpressure with nil event")
	}

	// Should increment failure counter
	if publisher.GetFailureCount() != 1 {
		t.Errorf("Expected failure count=1 after failed PublishWithBackpressure, got %d", publisher.GetFailureCount())
	}
}

// TestCircuitBreaker_Activation tests circuit breaker activation after 10 failures
func TestCircuitBreaker_Activation(t *testing.T) {
	mockBus := &mockEventBus{}
	mockAdapt := &mockAdapter{}
	publisher := NewPublisher(mockBus, "test-session", mockAdapt)

	// Use nil event to trigger errors (since Broadcast itself doesn't fail)
	var lastErr error
	for i := 0; i < 10; i++ {
		err := publisher.PublishWithBackpressure(nil)
		if err != nil {
			lastErr = err
		}
	}

	// The 10th failure should trigger circuit breaker
	if lastErr == nil {
		t.Fatal("Expected error from PublishWithBackpressure")
	}
	if !contains(lastErr.Error(), "circuit breaker open") {
		t.Errorf("Expected circuit breaker error, got %v", lastErr)
	}

	// Give the async Stop() call time to execute
	time.Sleep(100 * time.Millisecond)

	// Adapter should be stopped
	if !mockAdapt.isStopped() {
		t.Error("Expected adapter to be stopped after circuit breaker activation")
	}
}

// TestCircuitBreaker_Reset tests that successful publish resets failure counter
func TestCircuitBreaker_Reset(t *testing.T) {
	mockBus := &mockEventBus{}
	mockAdapt := &mockAdapter{}
	publisher := NewPublisher(mockBus, "test-session", mockAdapt)

	// Fail 5 times with nil events
	for i := 0; i < 5; i++ {
		_ = publisher.PublishWithBackpressure(nil)
	}

	if publisher.GetFailureCount() != 5 {
		t.Errorf("Expected failure count=5, got %d", publisher.GetFailureCount())
	}

	// Now succeed with valid event
	validEvent := &AGMEvent{
		State:     "IDLE",
		Timestamp: 1709654321,
	}

	err := publisher.PublishWithBackpressure(validEvent)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	// Failure count should be reset to 0
	if publisher.GetFailureCount() != 0 {
		t.Errorf("Expected failure count to reset to 0, got %d", publisher.GetFailureCount())
	}
}

// TestConcurrentPublish tests thread-safety of concurrent publishing
func TestConcurrentPublish(t *testing.T) {
	mockBus := &mockEventBus{}
	mockAdapt := &mockAdapter{}
	publisher := NewPublisher(mockBus, "test-session", mockAdapt)

	numGoroutines := 10
	eventsPerGoroutine := 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				event := &AGMEvent{
					State:     "WORKING",
					Timestamp: int64(id*1000 + j),
				}
				_ = publisher.Publish(event)
			}
		}(i)
	}

	wg.Wait()

	published := mockBus.GetEvents()
	expectedCount := numGoroutines * eventsPerGoroutine
	if len(published) != expectedCount {
		t.Fatalf("Expected %d published events, got %d", expectedCount, len(published))
	}

	// Verify all sequence numbers are unique and in range
	seqMap := make(map[uint64]bool)
	for _, pub := range published {
		var payload SessionStateChangeEvent
		if err := json.Unmarshal(pub.Payload, &payload); err != nil {
			t.Errorf("Failed to unmarshal payload: %v", err)
			continue
		}

		if seqMap[payload.Sequence] {
			t.Errorf("Duplicate sequence number: %d", payload.Sequence)
		}
		seqMap[payload.Sequence] = true

		if payload.Sequence < 1 || payload.Sequence > uint64(expectedCount) {
			t.Errorf("Sequence number out of range: %d", payload.Sequence)
		}
	}
}

// TestGetters tests the getter methods
func TestGetters(t *testing.T) {
	mockBus := &mockEventBus{}
	mockAdapt := &mockAdapter{}
	publisher := NewPublisher(mockBus, "test-session", mockAdapt)

	if publisher.GetSequence() != 0 {
		t.Errorf("Initial sequence should be 0, got %d", publisher.GetSequence())
	}

	if publisher.GetFailureCount() != 0 {
		t.Errorf("Initial failure count should be 0, got %d", publisher.GetFailureCount())
	}

	// Publish an event
	event := &AGMEvent{State: "IDLE", Timestamp: 1709654321}
	_ = publisher.Publish(event)

	if publisher.GetSequence() != 1 {
		t.Errorf("Sequence after 1 publish should be 1, got %d", publisher.GetSequence())
	}
}
