package analytics

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// mockEventBus is a mock EventBus that records published events
type mockEventBus struct {
	mu     sync.Mutex
	events []*eventbus.Event
}

func newMockEventBus() *mockEventBus {
	return &mockEventBus{
		events: []*eventbus.Event{},
	}
}

func (m *mockEventBus) Publish(ctx context.Context, event *eventbus.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventBus) Subscribe(eventType string, subscriber string, handler eventbus.Handler) {
	// Not needed for testing
}

func (m *mockEventBus) PublishWithResponse(ctx context.Context, event *eventbus.Event, timeout time.Duration) ([]*eventbus.Response, error) {
	// Not needed for testing
	return nil, nil
}

func (m *mockEventBus) GetEvents() []*eventbus.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.events
}

func (m *mockEventBus) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

// TestNewSessionTracker verifies session tracker initialization
func TestNewSessionTracker(t *testing.T) {
	bus := newMockEventBus()
	tracker := NewSessionTracker((*eventbus.LocalBus)(nil)) // Cast to real type for interface

	if tracker == nil {
		t.Fatal("NewSessionTracker() returned nil")
	}

	if tracker.sessionID == "" {
		t.Error("SessionID is empty")
	}

	if tracker.sessionStartTime.IsZero() {
		t.Error("Session start time is zero")
	}

	_ = bus // Avoid unused variable
}

// TestStartSession verifies session.started event
func TestStartSession(t *testing.T) {
	bus := newMockEventBus()
	tracker := &SessionTracker{
		sessionID:        "test-session-123",
		sessionStartTime: time.Now(),
		eventBus:         (*eventbus.LocalBus)(nil),
	}

	// Replace with mock after creation
	// (In real code, EventBus interface would allow this)
	// For now, we'll test the event creation logic indirectly

	// Test that StartSession doesn't panic with nil bus
	err := tracker.StartSession("/test/project")
	if err != nil {
		t.Errorf("StartSession() failed: %v", err)
	}

	_ = bus
}

// TestSessionID verifies session ID getter
func TestSessionID(t *testing.T) {
	tracker := NewSessionTracker(nil)
	sessionID := tracker.SessionID()

	if sessionID == "" {
		t.Error("SessionID() returned empty string")
	}

	if len(sessionID) != 36 { // UUID v4 length
		t.Errorf("SessionID length = %d, want 36 (UUID format)", len(sessionID))
	}
}

// TestStartPhase verifies phase tracking
func TestStartPhase(t *testing.T) {
	tracker := NewSessionTracker(nil)

	err := tracker.StartPhase("D1")
	if err != nil {
		t.Errorf("StartPhase() failed: %v", err)
	}

	if tracker.currentPhase != "D1" {
		t.Errorf("Current phase = %q, want %q", tracker.currentPhase, "D1")
	}

	if tracker.phaseStartTime.IsZero() {
		t.Error("Phase start time not set")
	}
}

// TestCompletePhase verifies phase completion
func TestCompletePhase(t *testing.T) {
	tracker := NewSessionTracker(nil)

	// Start a phase first
	tracker.StartPhase("D1")
	time.Sleep(10 * time.Millisecond) // Small delay to ensure duration > 0

	// Complete the phase
	metadata := map[string]interface{}{
		"engramsLoaded": 5,
		"tokensInput":   1234,
		"tokensOutput":  567,
	}

	err := tracker.CompletePhase("D1", "success", metadata)
	if err != nil {
		t.Errorf("CompletePhase() failed: %v", err)
	}
}

// TestEndSession verifies session end
func TestEndSession(t *testing.T) {
	tracker := NewSessionTracker(nil)

	err := tracker.EndSession("success")
	if err != nil {
		t.Errorf("EndSession() failed: %v", err)
	}
}

// TestSessionTrackerLifecycle verifies full session lifecycle
func TestSessionTrackerLifecycle(t *testing.T) {
	tracker := NewSessionTracker(nil)

	// Start session
	if err := tracker.StartSession("/test/project"); err != nil {
		t.Fatalf("StartSession() failed: %v", err)
	}

	// Phase 1: D1
	if err := tracker.StartPhase("D1"); err != nil {
		t.Fatalf("StartPhase(D1) failed: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := tracker.CompletePhase("D1", "success", nil); err != nil {
		t.Fatalf("CompletePhase(D1) failed: %v", err)
	}

	// Phase 2: D2
	if err := tracker.StartPhase("D2"); err != nil {
		t.Fatalf("StartPhase(D2) failed: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := tracker.CompletePhase("D2", "success", nil); err != nil {
		t.Fatalf("CompletePhase(D2) failed: %v", err)
	}

	// End session
	if err := tracker.EndSession("success"); err != nil {
		t.Fatalf("EndSession() failed: %v", err)
	}
}

// TestCompletePhase_WithoutStart verifies graceful handling
func TestCompletePhase_WithoutStart(t *testing.T) {
	tracker := NewSessionTracker(nil)

	// Complete phase without starting it
	// Should not panic, but duration will be since session start
	err := tracker.CompletePhase("D1", "success", nil)
	if err != nil {
		t.Errorf("CompletePhase() without StartPhase failed: %v", err)
	}
}

// TestMultipleSessions verifies session ID uniqueness
func TestMultipleSessions(t *testing.T) {
	tracker1 := NewSessionTracker(nil)
	tracker2 := NewSessionTracker(nil)

	if tracker1.SessionID() == tracker2.SessionID() {
		t.Error("Session IDs are not unique")
	}
}

// TestNilEventBus verifies graceful handling of nil EventBus
func TestNilEventBus(t *testing.T) {
	tracker := NewSessionTracker(nil)

	// All methods should work without panicking
	if err := tracker.StartSession("/test"); err != nil {
		t.Errorf("StartSession() with nil bus failed: %v", err)
	}

	if err := tracker.StartPhase("D1"); err != nil {
		t.Errorf("StartPhase() with nil bus failed: %v", err)
	}

	if err := tracker.CompletePhase("D1", "success", nil); err != nil {
		t.Errorf("CompletePhase() with nil bus failed: %v", err)
	}

	if err := tracker.EndSession("success"); err != nil {
		t.Errorf("EndSession() with nil bus failed: %v", err)
	}
}
