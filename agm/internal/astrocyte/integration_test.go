package astrocyte

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
	"github.com/vbonnet/dear-agent/agm/internal/logging"
)

// TestFullIntegration tests the complete flow from incident file to event broadcast
func TestFullIntegration(t *testing.T) {
	// Create temporary incidents file
	tmpDir := t.TempDir()
	incidentsFile := filepath.Join(tmpDir, "incidents.jsonl")

	// Create a real hub
	hub := eventbus.NewHub()
	go hub.Run()
	defer hub.Shutdown()

	// Capture events using a channel
	eventChan := make(chan *eventbus.Event, 10)
	captureHub := &eventCaptureHub{
		hub:    hub,
		events: eventChan,
	}

	// Start watcher with short poll interval for faster testing
	testPollInterval := 100 * time.Millisecond
	watcher := &Watcher{
		hub:           captureHub,
		tracker:       NewEscalationTracker(15 * time.Minute),
		incidentsFile: incidentsFile,
		pollInterval:  testPollInterval,
		shutdown:      make(chan struct{}),
		logger:        logging.DefaultLogger(),
	}

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	// Write an incident
	incident := &AstrocyteIncident{
		Timestamp:          time.Now().Format(time.RFC3339),
		SessionName:        "integration-test",
		SessionID:          "session-integration",
		Symptom:            "stuck_mustering",
		DurationMinutes:    20,
		DetectionHeuristic: "mustering_timeout",
		PaneSnapshot:       "✻ Mustering...",
		RecoveryAttempted:  true,
	}

	incidentJSON, err := json.Marshal(incident)
	if err != nil {
		t.Fatalf("Failed to marshal incident: %v", err)
	}

	if err := os.WriteFile(incidentsFile, append(incidentJSON, '\n'), 0644); err != nil {
		t.Fatalf("Failed to write incident: %v", err)
	}

	// Wait for event to be processed
	select {
	case event := <-eventChan:
		if event.SessionID != incident.SessionID {
			t.Errorf("Expected session ID %s, got %s", incident.SessionID, event.SessionID)
		}
		if event.Type != eventbus.EventSessionEscalated {
			t.Errorf("Expected event type %s, got %s", eventbus.EventSessionEscalated, event.Type)
		}

		var payload eventbus.SessionEscalatedPayload
		if err := event.ParsePayload(&payload); err != nil {
			t.Fatalf("Failed to parse payload: %v", err)
		}

		if payload.Severity != "high" {
			t.Errorf("Expected severity 'high', got %s", payload.Severity)
		}

		if payload.Pattern != incident.DetectionHeuristic {
			t.Errorf("Expected pattern %s, got %s", incident.DetectionHeuristic, payload.Pattern)
		}

	case <-time.After(testPollInterval + 500*time.Millisecond):
		t.Fatal("Timeout waiting for event")
	}
}

// TestMultipleSessionsIntegration tests handling of multiple sessions concurrently
func TestMultipleSessionsIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	incidentsFile := filepath.Join(tmpDir, "incidents.jsonl")

	hub := eventbus.NewHub()
	go hub.Run()
	defer hub.Shutdown()

	eventChan := make(chan *eventbus.Event, 10)
	captureHub := &eventCaptureHub{
		hub:    hub,
		events: eventChan,
	}

	testPollInterval := 100 * time.Millisecond
	watcher := &Watcher{
		hub:           captureHub,
		tracker:       NewEscalationTracker(15 * time.Minute),
		incidentsFile: incidentsFile,
		pollInterval:  testPollInterval,
		shutdown:      make(chan struct{}),
		logger:        logging.DefaultLogger(),
	}

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	// Write incidents for multiple sessions
	sessions := []string{"session-1", "session-2", "session-3"}
	file, err := os.Create(incidentsFile)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	for _, sessionID := range sessions {
		incident := &AstrocyteIncident{
			Timestamp:          time.Now().Format(time.RFC3339),
			SessionName:        sessionID,
			SessionID:          sessionID,
			Symptom:            "permission_prompt",
			DetectionHeuristic: "permission_detected",
		}
		incidentJSON, _ := json.Marshal(incident)
		file.Write(append(incidentJSON, '\n'))
	}
	file.Close()

	// Collect events
	receivedSessions := make(map[string]bool)
	timeout := time.After(testPollInterval + 500*time.Millisecond)

	for i := 0; i < len(sessions); i++ {
		select {
		case event := <-eventChan:
			receivedSessions[event.SessionID] = true
		case <-timeout:
			t.Fatalf("Timeout waiting for events, got %d/%d", i, len(sessions))
		}
	}

	// Verify all sessions were processed
	for _, sessionID := range sessions {
		if !receivedSessions[sessionID] {
			t.Errorf("Did not receive event for session %s", sessionID)
		}
	}
}

// TestTimeWindowingIntegration verifies time-windowing prevents duplicates
func TestTimeWindowingIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	incidentsFile := filepath.Join(tmpDir, "incidents.jsonl")

	hub := eventbus.NewHub()
	go hub.Run()
	defer hub.Shutdown()

	eventChan := make(chan *eventbus.Event, 10)
	captureHub := &eventCaptureHub{
		hub:    hub,
		events: eventChan,
	}

	// Use a short poll interval (50ms) and short window (300ms) for testing
	testPollInterval := 50 * time.Millisecond
	testWindow := 300 * time.Millisecond
	watcher := &Watcher{
		hub:           captureHub,
		tracker:       NewEscalationTracker(testWindow),
		incidentsFile: incidentsFile,
		pollInterval:  testPollInterval,
		shutdown:      make(chan struct{}),
		logger:        logging.DefaultLogger(),
	}

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	sessionID := "session-windowing-test"

	// Write first incident
	incident1 := &AstrocyteIncident{
		Timestamp:          time.Now().Format(time.RFC3339),
		SessionName:        sessionID,
		SessionID:          sessionID,
		Symptom:            "stuck_mustering",
		DetectionHeuristic: "mustering_timeout",
	}

	file, err := os.Create(incidentsFile)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	incidentJSON, _ := json.Marshal(incident1)
	file.Write(append(incidentJSON, '\n'))
	file.Close()

	// Wait for first event
	select {
	case event := <-eventChan:
		if event.SessionID != sessionID {
			t.Errorf("Expected session %s, got %s", sessionID, event.SessionID)
		}
	case <-time.After(testPollInterval + 200*time.Millisecond):
		t.Fatal("Timeout waiting for first event")
	}

	// Immediately write second incident (should be skipped)
	incident2 := &AstrocyteIncident{
		Timestamp:          time.Now().Format(time.RFC3339),
		SessionName:        sessionID,
		SessionID:          sessionID,
		Symptom:            "stuck_mustering",
		DetectionHeuristic: "mustering_timeout",
	}

	file, err = os.OpenFile(incidentsFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}

	incidentJSON, _ = json.Marshal(incident2)
	file.Write(append(incidentJSON, '\n'))
	file.Close()

	// Should not receive second event (within window)
	select {
	case event := <-eventChan:
		t.Errorf("Received unexpected event within time window: %+v", event)
	case <-time.After(testPollInterval + 200*time.Millisecond):
		// Expected - no event should be received
	}

	// Wait for window to expire
	time.Sleep(testWindow + 100*time.Millisecond)

	// Write third incident (should be published)
	incident3 := &AstrocyteIncident{
		Timestamp:          time.Now().Format(time.RFC3339),
		SessionName:        sessionID,
		SessionID:          sessionID,
		Symptom:            "stuck_mustering",
		DetectionHeuristic: "mustering_timeout",
	}

	file, err = os.OpenFile(incidentsFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}

	incidentJSON, _ = json.Marshal(incident3)
	file.Write(append(incidentJSON, '\n'))
	file.Close()

	// Should receive third event (after window)
	select {
	case event := <-eventChan:
		if event.SessionID != sessionID {
			t.Errorf("Expected session %s, got %s", sessionID, event.SessionID)
		}
	case <-time.After(testPollInterval + 200*time.Millisecond):
		t.Fatal("Timeout waiting for third event after time window")
	}
}

// eventCaptureHub wraps a hub and captures broadcasted events
type eventCaptureHub struct {
	hub    *eventbus.Hub
	events chan *eventbus.Event
}

func (h *eventCaptureHub) Broadcast(event *eventbus.Event) {
	// Capture event
	select {
	case h.events <- event:
	default:
		// Channel full, drop event
	}

	// Also broadcast to real hub
	h.hub.Broadcast(event)
}
