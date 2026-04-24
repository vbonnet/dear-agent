package ops

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// captureBus captures events broadcast to it for test assertions.
type captureBus struct {
	mu     sync.Mutex
	events []*eventbus.Event
}

func (b *captureBus) Broadcast(event *eventbus.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, event)
}

func (b *captureBus) Events() []*eventbus.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	cp := make([]*eventbus.Event, len(b.events))
	copy(cp, b.events)
	return cp
}

// --- StallDetector EventBus Tests ---

func TestStallDetector_PublishesStallDetected(t *testing.T) {
	now := time.Now()
	sessions := []*manifest.Manifest{
		testManifest("worker-1", manifest.StatePermissionPrompt, now.Add(-10*time.Minute)),
	}

	mockStore := &mockStorage{sessions: sessions}
	detector := NewStallDetector(&OpContext{Storage: mockStore})

	bus := &captureBus{}
	detector.SetBus(bus)

	events, err := detector.DetectStalls(context.Background())
	if err != nil {
		t.Fatalf("DetectStalls() error = %v", err)
	}

	if len(events) == 0 {
		t.Fatal("Expected at least one stall event")
	}

	busEvents := bus.Events()
	if len(busEvents) != len(events) {
		t.Fatalf("Expected %d bus events, got %d", len(events), len(busEvents))
	}

	evt := busEvents[0]
	if evt.Type != eventbus.EventStallDetected {
		t.Errorf("EventType = %v, want %v", evt.Type, eventbus.EventStallDetected)
	}
	if evt.SessionID != "worker-1" {
		t.Errorf("SessionID = %v, want worker-1", evt.SessionID)
	}

	var payload eventbus.StallDetectedPayload
	if err := evt.ParsePayload(&payload); err != nil {
		t.Fatalf("ParsePayload() error = %v", err)
	}
	if payload.StallType != "permission_prompt" {
		t.Errorf("StallType = %v, want permission_prompt", payload.StallType)
	}
	if payload.Severity != "critical" {
		t.Errorf("Severity = %v, want critical", payload.Severity)
	}
}

func TestStallDetector_NoBus_NoPublish(t *testing.T) {
	now := time.Now()
	sessions := []*manifest.Manifest{
		testManifest("worker-1", manifest.StatePermissionPrompt, now.Add(-10*time.Minute)),
	}

	mockStore := &mockStorage{sessions: sessions}
	detector := NewStallDetector(&OpContext{Storage: mockStore})
	// Don't set bus — should not panic

	events, err := detector.DetectStalls(context.Background())
	if err != nil {
		t.Fatalf("DetectStalls() error = %v", err)
	}
	if len(events) == 0 {
		t.Fatal("Expected at least one stall event even without bus")
	}
}

func TestStallDetector_NoStalls_NoEvents(t *testing.T) {
	now := time.Now()
	sessions := []*manifest.Manifest{
		testManifest("worker-1", manifest.StateReady, now),
	}

	mockStore := &mockStorage{sessions: sessions}
	detector := NewStallDetector(&OpContext{Storage: mockStore})

	bus := &captureBus{}
	detector.SetBus(bus)

	events, err := detector.DetectStalls(context.Background())
	if err != nil {
		t.Fatalf("DetectStalls() error = %v", err)
	}
	if len(events) != 0 {
		t.Errorf("Expected 0 stall events, got %d", len(events))
	}
	if len(bus.Events()) != 0 {
		t.Errorf("Expected 0 bus events, got %d", len(bus.Events()))
	}
}

// --- StallRecovery EventBus Tests ---

func TestStallRecovery_PublishesRecovered(t *testing.T) {
	setupTestRetryDir(t)
	recovery := NewStallRecovery(&OpContext{}, "")

	bus := &captureBus{}
	recovery.SetBus(bus)

	event := StallEvent{
		SessionName: "worker-1",
		StallType:   "error_loop",
		Evidence:    "Error: timeout appears 5 times",
	}

	// error_loop without orchestrator logs locally and succeeds
	action, err := recovery.Recover(context.Background(), event)
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if !action.Sent {
		t.Error("Expected action to be sent")
	}

	busEvents := bus.Events()
	if len(busEvents) != 1 {
		t.Fatalf("Expected 1 bus event, got %d", len(busEvents))
	}

	evt := busEvents[0]
	if evt.Type != eventbus.EventStallRecovered {
		t.Errorf("EventType = %v, want %v", evt.Type, eventbus.EventStallRecovered)
	}

	var payload eventbus.StallRecoveredPayload
	if err := evt.ParsePayload(&payload); err != nil {
		t.Fatalf("ParsePayload() error = %v", err)
	}
	if payload.StallType != "error_loop" {
		t.Errorf("StallType = %v, want error_loop", payload.StallType)
	}
	if payload.RecoveryAction != "log_diagnostic" {
		t.Errorf("RecoveryAction = %v, want log_diagnostic", payload.RecoveryAction)
	}
}

func TestStallRecovery_NoPublishOnFailure(t *testing.T) {
	setupTestRetryDir(t)
	recovery := NewStallRecovery(&OpContext{}, "")

	bus := &captureBus{}
	recovery.SetBus(bus)

	event := StallEvent{
		SessionName: "worker-1",
		StallType:   "permission_prompt",
		Duration:    10 * time.Minute,
	}

	// permission_prompt without orchestrator fails
	_, err := recovery.Recover(context.Background(), event)
	if err == nil {
		t.Fatal("Expected error for permission_prompt without orchestrator")
	}

	busEvents := bus.Events()
	if len(busEvents) != 0 {
		t.Errorf("Expected 0 bus events on failure, got %d", len(busEvents))
	}
}

func TestStallRecovery_NoBus_NoPublish(t *testing.T) {
	setupTestRetryDir(t)
	recovery := NewStallRecovery(&OpContext{}, "")
	// Don't set bus — should not panic

	event := StallEvent{
		SessionName: "worker-1",
		StallType:   "error_loop",
		Evidence:    "Error: timeout appears 5 times",
	}

	_, err := recovery.Recover(context.Background(), event)
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
}
