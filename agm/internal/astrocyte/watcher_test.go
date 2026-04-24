package astrocyte

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
	"github.com/vbonnet/dear-agent/agm/internal/logging"
)

// mockHub is a test helper that captures broadcasted events
type mockHub struct {
	events []*eventbus.Event
	mu     sync.Mutex
}

func (h *mockHub) Broadcast(event *eventbus.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, event)
}

func (h *mockHub) GetEvents() []*eventbus.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]*eventbus.Event{}, h.events...)
}

func (h *mockHub) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = nil
}

func TestEscalationTracker_ShouldPublish(t *testing.T) {
	tests := []struct {
		name      string
		window    time.Duration
		sessionID string
		delay     time.Duration
		want      bool
	}{
		{
			name:      "first escalation should publish",
			window:    5 * time.Second,
			sessionID: "session-1",
			delay:     0,
			want:      true,
		},
		{
			name:      "within window should not publish",
			window:    5 * time.Second,
			sessionID: "session-2",
			delay:     2 * time.Second,
			want:      false,
		},
		{
			name:      "after window should publish",
			window:    1 * time.Second,
			sessionID: "session-3",
			delay:     2 * time.Second,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := NewEscalationTracker(tt.window)

			// First call should always return true (except for tests checking within window)
			if tt.delay == 0 {
				got := tracker.ShouldPublish(tt.sessionID)
				if got != tt.want {
					t.Errorf("ShouldPublish() = %v, want %v", got, tt.want)
				}
				return
			}

			// Record first escalation
			tracker.RecordEscalation(tt.sessionID)

			// Wait for delay
			time.Sleep(tt.delay)

			// Check if should publish again
			got := tracker.ShouldPublish(tt.sessionID)
			if got != tt.want {
				t.Errorf("ShouldPublish() after %v = %v, want %v", tt.delay, got, tt.want)
			}
		})
	}
}

func TestEscalationTracker_ConcurrentSessions(t *testing.T) {
	tracker := NewEscalationTracker(100 * time.Millisecond)

	sessions := []string{"session-1", "session-2", "session-3", "session-4", "session-5"}
	var wg sync.WaitGroup

	// Concurrently record escalations for multiple sessions
	for _, sessionID := range sessions {
		wg.Add(1)
		go func(sid string) {
			defer wg.Done()

			// First call should return true
			if !tracker.ShouldPublish(sid) {
				t.Errorf("First ShouldPublish(%s) should return true", sid)
			}

			tracker.RecordEscalation(sid)

			// Immediate second call should return false
			if tracker.ShouldPublish(sid) {
				t.Errorf("Immediate ShouldPublish(%s) should return false", sid)
			}

			// Wait for window to expire
			time.Sleep(150 * time.Millisecond)

			// Should now return true
			if !tracker.ShouldPublish(sid) {
				t.Errorf("ShouldPublish(%s) after window should return true", sid)
			}
		}(sessionID)
	}

	wg.Wait()
}

func TestWatcher_ProcessIncident(t *testing.T) {
	tests := []struct {
		name           string
		incident       *AstrocyteIncident
		wantEventCount int
		wantEventType  eventbus.EventType
		wantSeverity   string
	}{
		{
			name: "permission prompt incident",
			incident: &AstrocyteIncident{
				Timestamp:          time.Now().Format(time.RFC3339),
				SessionName:        "test-session",
				SessionID:          "session-123",
				Symptom:            "permission_prompt",
				DurationMinutes:    5,
				DetectionHeuristic: "permission_prompt_detected",
				PaneSnapshot:       "Allow permission? [y/n]",
				RecoveryAttempted:  true,
			},
			wantEventCount: 1,
			wantEventType:  eventbus.EventSessionEscalated,
			wantSeverity:   "medium",
		},
		{
			name: "stuck mustering incident",
			incident: &AstrocyteIncident{
				Timestamp:          time.Now().Format(time.RFC3339),
				SessionName:        "test-session",
				SessionID:          "session-456",
				Symptom:            "stuck_mustering",
				DurationMinutes:    20,
				DetectionHeuristic: "mustering_timeout",
				PaneSnapshot:       "✻ Mustering...",
				RecoveryAttempted:  true,
			},
			wantEventCount: 1,
			wantEventType:  eventbus.EventSessionEscalated,
			wantSeverity:   "high",
		},
		{
			name: "ask question violation",
			incident: &AstrocyteIncident{
				Timestamp:          time.Now().Format(time.RFC3339),
				SessionName:        "test-session",
				SessionID:          "session-789",
				Symptom:            "ask_question_violation",
				DurationMinutes:    10,
				DetectionHeuristic: "question_pattern",
				PaneSnapshot:       "Would you like to proceed?",
				RecoveryAttempted:  true,
			},
			wantEventCount: 1,
			wantEventType:  eventbus.EventSessionEscalated,
			wantSeverity:   "low",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := &mockHub{}
			watcher := &Watcher{
				hub:     hub,
				tracker: NewEscalationTracker(15 * time.Minute),
				logger:  logging.DefaultLogger(),
			}

			err := watcher.processIncident(tt.incident)
			if err != nil {
				t.Fatalf("processIncident() error = %v", err)
			}

			events := hub.GetEvents()
			if len(events) != tt.wantEventCount {
				t.Errorf("got %d events, want %d", len(events), tt.wantEventCount)
			}

			if len(events) > 0 {
				event := events[0]

				if event.Type != tt.wantEventType {
					t.Errorf("event type = %v, want %v", event.Type, tt.wantEventType)
				}

				if event.SessionID != tt.incident.SessionID {
					t.Errorf("session ID = %v, want %v", event.SessionID, tt.incident.SessionID)
				}

				var payload eventbus.SessionEscalatedPayload
				if err := event.ParsePayload(&payload); err != nil {
					t.Fatalf("failed to parse payload: %v", err)
				}

				if payload.Severity != tt.wantSeverity {
					t.Errorf("severity = %v, want %v", payload.Severity, tt.wantSeverity)
				}

				if payload.Pattern != tt.incident.DetectionHeuristic {
					t.Errorf("pattern = %v, want %v", payload.Pattern, tt.incident.DetectionHeuristic)
				}
			}
		})
	}
}

func TestWatcher_TimeWindowing(t *testing.T) {
	hub := &mockHub{}
	watcher := &Watcher{
		hub:     hub,
		tracker: NewEscalationTracker(200 * time.Millisecond),
		logger:  logging.DefaultLogger(),
	}

	incident := &AstrocyteIncident{
		Timestamp:          time.Now().Format(time.RFC3339),
		SessionName:        "test-session",
		SessionID:          "session-windowing",
		Symptom:            "stuck_mustering",
		DurationMinutes:    20,
		DetectionHeuristic: "mustering_timeout",
		PaneSnapshot:       "✻ Mustering...",
	}

	// First incident should be published
	if err := watcher.processIncident(incident); err != nil {
		t.Fatalf("processIncident() error = %v", err)
	}

	events := hub.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event after first incident, got %d", len(events))
	}

	hub.Clear()

	// Immediate second incident should be skipped (within window)
	if err := watcher.processIncident(incident); err != nil {
		t.Fatalf("processIncident() error = %v", err)
	}

	events = hub.GetEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 events within time window, got %d", len(events))
	}

	// Wait for window to expire
	time.Sleep(250 * time.Millisecond)

	// Third incident should be published (after window)
	if err := watcher.processIncident(incident); err != nil {
		t.Fatalf("processIncident() error = %v", err)
	}

	events = hub.GetEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event after time window, got %d", len(events))
	}
}

func TestWatcher_EmptySessionID(t *testing.T) {
	hub := &mockHub{}
	watcher := &Watcher{
		hub:     hub,
		tracker: NewEscalationTracker(15 * time.Minute),
		logger:  logging.DefaultLogger(),
	}

	incident := &AstrocyteIncident{
		Timestamp:          time.Now().Format(time.RFC3339),
		SessionName:        "test-session",
		SessionID:          "", // Empty session ID
		Symptom:            "stuck_mustering",
		DetectionHeuristic: "mustering_timeout",
	}

	if err := watcher.processIncident(incident); err != nil {
		t.Fatalf("processIncident() should not error on empty session ID: %v", err)
	}

	events := hub.GetEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 events for empty session ID, got %d", len(events))
	}
}

func TestWatcher_FileWatching(t *testing.T) {
	// Create a temporary incidents file
	tmpDir := t.TempDir()
	incidentsFile := filepath.Join(tmpDir, "incidents.jsonl")

	hub := &mockHub{}
	testPollInterval := 100 * time.Millisecond
	watcher := NewWatcherWithPollInterval(hub, incidentsFile, 15*time.Minute, testPollInterval)

	// Start the watcher
	if err := watcher.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer watcher.Stop()

	// Write an incident to the file
	incident := &AstrocyteIncident{
		Timestamp:          time.Now().Format(time.RFC3339),
		SessionName:        "test-session",
		SessionID:          "session-file-watch",
		Symptom:            "stuck_mustering",
		DurationMinutes:    20,
		DetectionHeuristic: "mustering_timeout",
		PaneSnapshot:       "✻ Mustering...",
	}

	incidentJSON, err := json.Marshal(incident)
	if err != nil {
		t.Fatalf("failed to marshal incident: %v", err)
	}

	if err := os.WriteFile(incidentsFile, append(incidentJSON, '\n'), 0644); err != nil {
		t.Fatalf("failed to write incidents file: %v", err)
	}

	// Wait for the watcher to process the file
	time.Sleep(testPollInterval + 100*time.Millisecond)

	// Check that the event was published
	events := hub.GetEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event from file watching, got %d", len(events))
	}

	if len(events) > 0 {
		if events[0].SessionID != incident.SessionID {
			t.Errorf("event session ID = %v, want %v", events[0].SessionID, incident.SessionID)
		}
	}
}

func TestWatcher_MultipleIncidents(t *testing.T) {
	tmpDir := t.TempDir()
	incidentsFile := filepath.Join(tmpDir, "incidents.jsonl")

	hub := &mockHub{}
	testPollInterval := 100 * time.Millisecond
	watcher := NewWatcherWithPollInterval(hub, incidentsFile, 15*time.Minute, testPollInterval)

	if err := watcher.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer watcher.Stop()

	// Write multiple incidents
	incidents := []*AstrocyteIncident{
		{
			Timestamp:          time.Now().Format(time.RFC3339),
			SessionName:        "session-1",
			SessionID:          "session-multi-1",
			Symptom:            "permission_prompt",
			DetectionHeuristic: "permission_detected",
		},
		{
			Timestamp:          time.Now().Format(time.RFC3339),
			SessionName:        "session-2",
			SessionID:          "session-multi-2",
			Symptom:            "stuck_mustering",
			DetectionHeuristic: "mustering_timeout",
		},
		{
			Timestamp:          time.Now().Format(time.RFC3339),
			SessionName:        "session-3",
			SessionID:          "session-multi-3",
			Symptom:            "ask_question_violation",
			DetectionHeuristic: "question_pattern",
		},
	}

	file, err := os.Create(incidentsFile)
	if err != nil {
		t.Fatalf("failed to create incidents file: %v", err)
	}

	for _, incident := range incidents {
		incidentJSON, err := json.Marshal(incident)
		if err != nil {
			t.Fatalf("failed to marshal incident: %v", err)
		}
		if _, err := file.Write(append(incidentJSON, '\n')); err != nil {
			t.Fatalf("failed to write incident: %v", err)
		}
	}
	file.Close()

	// Wait for the watcher to process
	time.Sleep(testPollInterval + 100*time.Millisecond)

	// Check that all events were published
	events := hub.GetEvents()
	if len(events) != len(incidents) {
		t.Errorf("expected %d events, got %d", len(incidents), len(events))
	}
}

func TestMapSymptomToEscalationType(t *testing.T) {
	tests := []struct {
		symptom string
		want    string
	}{
		{"permission_prompt", "prompt"},
		{"ask_question_violation", "warning"},
		{"bash_violation", "error"},
		{"stuck_mustering", "error"},
		{"stuck_waiting", "error"},
		{"cursor_frozen", "error"},
		{"unknown_symptom", "warning"},
	}

	for _, tt := range tests {
		t.Run(tt.symptom, func(t *testing.T) {
			got := mapSymptomToEscalationType(tt.symptom)
			if got != tt.want {
				t.Errorf("mapSymptomToEscalationType(%q) = %q, want %q", tt.symptom, got, tt.want)
			}
		})
	}
}

func TestMapSymptomToSeverity(t *testing.T) {
	tests := []struct {
		symptom string
		want    string
	}{
		{"stuck_mustering", "high"},
		{"stuck_waiting", "high"},
		{"cursor_frozen", "high"},
		{"permission_prompt", "medium"},
		{"ask_question_violation", "low"},
		{"bash_violation", "low"},
		{"unknown_symptom", "medium"},
	}

	for _, tt := range tests {
		t.Run(tt.symptom, func(t *testing.T) {
			got := mapSymptomToSeverity(tt.symptom)
			if got != tt.want {
				t.Errorf("mapSymptomToSeverity(%q) = %q, want %q", tt.symptom, got, tt.want)
			}
		})
	}
}

func TestFormatDescription(t *testing.T) {
	successRecovery := true
	failedRecovery := false
	recoveryMethod := "escape"

	tests := []struct {
		name         string
		incident     *AstrocyteIncident
		wantContains []string
	}{
		{
			name: "basic incident",
			incident: &AstrocyteIncident{
				Symptom:         "stuck_mustering",
				DurationMinutes: 20,
			},
			wantContains: []string{"stuck_mustering", "20 min"},
		},
		{
			name: "successful recovery",
			incident: &AstrocyteIncident{
				Symptom:           "permission_prompt",
				RecoveryAttempted: true,
				RecoveryMethod:    &recoveryMethod,
				RecoverySuccess:   &successRecovery,
			},
			wantContains: []string{"permission_prompt", "escape", "success"},
		},
		{
			name: "failed recovery",
			incident: &AstrocyteIncident{
				Symptom:           "stuck_waiting",
				RecoveryAttempted: true,
				RecoveryMethod:    &recoveryMethod,
				RecoverySuccess:   &failedRecovery,
			},
			wantContains: []string{"stuck_waiting", "escape", "failed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := formatDescription(tt.incident)
			for _, substr := range tt.wantContains {
				if !contains(desc, substr) {
					t.Errorf("formatDescription() = %q, want to contain %q", desc, substr)
				}
			}
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name      string
		timestamp string
		wantValid bool
	}{
		{
			name:      "RFC3339 format",
			timestamp: "2026-02-14T12:30:00Z",
			wantValid: true,
		},
		{
			name:      "without timezone",
			timestamp: "2026-02-14T12:30:00",
			wantValid: true,
		},
		{
			name:      "invalid format",
			timestamp: "invalid-timestamp",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTimestamp(tt.timestamp)
			if tt.wantValid && result.IsZero() {
				t.Errorf("parseTimestamp(%q) returned zero time, expected valid time", tt.timestamp)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "needs truncation",
			input:  "hello world, this is a long string",
			maxLen: 10,
			want:   "hello worl...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
