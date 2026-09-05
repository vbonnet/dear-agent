package analytics

import (
	"context"
	"sync"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/engram"
	"github.com/vbonnet/dear-agent/pkg/eventbus"
	"github.com/vbonnet/dear-agent/pkg/trigger"
)

// TestWayfinderSessionTrackerEmissionMatchesTriggerRegistryLookup verifies that the
// event topics emitted by Wayfinder's SessionTracker match the topic strings expected
// by the trigger registry and trigger subscriber.
func TestWayfinderSessionTrackerEmissionMatchesTriggerRegistryLookup(t *testing.T) {
	bus := eventbus.NewBus(nil)
	defer bus.Close()

	var mu sync.Mutex
	var capturedEvents []*eventbus.Event

	// Subscribe to wayfinder.* matching the pattern used by TriggerSubscriber.
	bus.Subscribe("wayfinder.*", "parity-verifier", func(ctx context.Context, evt *eventbus.Event) (*eventbus.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		capturedEvents = append(capturedEvents, evt)
		return nil, nil
	})

	registry := trigger.NewTriggerRegistry()
	registry.Register("engrams/problem-start.ai.md", []engram.TriggerSpec{
		{
			On:       eventbus.TypeWayfinderPhaseStarted,
			Priority: 80,
		},
	})
	registry.Register("engrams/problem-complete.ai.md", []engram.TriggerSpec{
		{
			On:       eventbus.TypeWayfinderPhaseCompleted,
			Priority: 70,
		},
	})

	tracker := NewSessionTracker(bus)

	if err := tracker.StartPhase("PROBLEM"); err != nil {
		t.Fatalf("StartPhase failed: %v", err)
	}

	metadata := map[string]any{
		"outcome": "success",
	}
	if err := tracker.CompletePhase("PROBLEM", "success", metadata); err != nil {
		t.Fatalf("CompletePhase failed: %v", err)
	}

	mu.Lock()
	events := make([]*eventbus.Event, len(capturedEvents))
	copy(events, capturedEvents)
	mu.Unlock()

	if len(events) != 2 {
		t.Fatalf("expected 2 captured events, got %d", len(events))
	}

	startEvent := events[0]
	completeEvent := events[1]

	if startEvent.Type != eventbus.TypeWayfinderPhaseStarted {
		t.Errorf("startEvent.Type = %q, want %q", startEvent.Type, eventbus.TypeWayfinderPhaseStarted)
	}
	if completeEvent.Type != eventbus.TypeWayfinderPhaseCompleted {
		t.Errorf("completeEvent.Type = %q, want %q", completeEvent.Type, eventbus.TypeWayfinderPhaseCompleted)
	}

	// Verify that registry lookup with the emitted event type finds the registered engram.
	startEntries := registry.Lookup(startEvent.Type)
	if len(startEntries) != 1 {
		t.Fatalf("registry.Lookup(%q) returned %d entries, want 1", startEvent.Type, len(startEntries))
	}
	if startEntries[0].EngramPath != "engrams/problem-start.ai.md" {
		t.Errorf("start entry path = %q, want engrams/problem-start.ai.md", startEntries[0].EngramPath)
	}

	completeEntries := registry.Lookup(completeEvent.Type)
	if len(completeEntries) != 1 {
		t.Fatalf("registry.Lookup(%q) returned %d entries, want 1", completeEvent.Type, len(completeEntries))
	}
	if completeEntries[0].EngramPath != "engrams/problem-complete.ai.md" {
		t.Errorf("complete entry path = %q, want engrams/problem-complete.ai.md", completeEntries[0].EngramPath)
	}

	// Verify matcher also resolves matches for the emitted events.
	matcher := trigger.NewTriggerMatcher(registry)

	startMatches := matcher.Match(trigger.TriggerEvent{
		Type: startEvent.Type,
		Data: startEvent.Data,
	})
	if len(startMatches) != 1 {
		t.Fatalf("matcher.Match(startEvent) returned %d matches, want 1", len(startMatches))
	}

	completeMatches := matcher.Match(trigger.TriggerEvent{
		Type: completeEvent.Type,
		Data: completeEvent.Data,
	})
	if len(completeMatches) != 1 {
		t.Fatalf("matcher.Match(completeEvent) returned %d matches, want 1", len(completeMatches))
	}
}
