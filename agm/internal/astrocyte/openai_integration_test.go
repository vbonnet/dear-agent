package astrocyte

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

// TestOpenAIIncidentProcessing tests that manually-created OpenAI incidents
// are properly processed by Astrocyte watcher
func TestOpenAIIncidentProcessing(t *testing.T) {
	// Create temporary incidents file
	tmpDir := t.TempDir()
	incidentsFile := filepath.Join(tmpDir, "incidents.jsonl")

	// Create mock EventBus
	hub := eventbus.NewHub()
	go hub.Run()
	defer hub.Shutdown()

	// Create watcher with short poll interval for testing
	watcher := NewWatcherWithPollInterval(hub, incidentsFile, 15*time.Minute, 100*time.Millisecond)

	// Start watcher
	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	// Create a manual incident for an OpenAI session
	incident := AstrocyteIncident{
		Timestamp:               time.Now().Format(time.RFC3339),
		SessionID:               "openai-test-session-001",
		SessionName:             "my-openai-session",
		Symptom:                 "api_timeout",
		DurationMinutes:         5,
		DetectionHeuristic:      "manual",
		PaneSnapshot:            "API request timeout",
		CursorPosition:          "",
		RecoveryAttempted:       false,
		RecoveryMethod:          nil,
		RecoverySuccess:         nil,
		RecoveryDurationSeconds: nil,
		DiagnosisFiled:          false,
		DiagnosisFile:           nil,
		CascadeDepth:            0,
		CircuitBreakerTriggered: false,
	}

	// Write incident to file
	data, err := json.Marshal(incident)
	if err != nil {
		t.Fatalf("Failed to marshal incident: %v", err)
	}

	file, err := os.OpenFile(incidentsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open incidents file: %v", err)
	}
	defer file.Close()

	if _, err := file.Write(append(data, '\n')); err != nil {
		t.Fatalf("Failed to write incident: %v", err)
	}
	file.Sync()

	// Wait for watcher to process the incident
	time.Sleep(200 * time.Millisecond)

	// Verify incident was processed
	// Note: In a real test, we'd subscribe to the EventBus and verify the event was published
	// For now, we just verify the watcher didn't crash and the file was read
	if watcher.LastPosition() == 0 {
		t.Error("Watcher did not process the incidents file")
	}
}

// TestMixedAgentIncidents tests that Astrocyte can handle incidents from both
// tmux-based (Claude/Gemini) and API-based (OpenAI) sessions
func TestMixedAgentIncidents(t *testing.T) {
	// Create temporary incidents file
	tmpDir := t.TempDir()
	incidentsFile := filepath.Join(tmpDir, "incidents.jsonl")

	// Create mock EventBus
	hub := eventbus.NewHub()
	go hub.Run()
	defer hub.Shutdown()

	// Create watcher
	watcher := NewWatcherWithPollInterval(hub, incidentsFile, 15*time.Minute, 100*time.Millisecond)

	// Start watcher
	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	// Create incidents for different agent types
	incidents := []AstrocyteIncident{
		{
			Timestamp:          time.Now().Format(time.RFC3339),
			SessionID:          "claude-session-001",
			SessionName:        "my-claude-session",
			Symptom:            "stuck_mustering",
			DurationMinutes:    20,
			DetectionHeuristic: "mustering_timeout",
			PaneSnapshot:       "✻ Mustering...",
			RecoveryAttempted:  true,
			RecoveryMethod:     strPtr("escape"),
			RecoverySuccess:    boolPtr(true),
		},
		{
			Timestamp:          time.Now().Add(1 * time.Second).Format(time.RFC3339),
			SessionID:          "openai-session-001",
			SessionName:        "my-openai-session",
			Symptom:            "api_timeout",
			DurationMinutes:    5,
			DetectionHeuristic: "manual",
			PaneSnapshot:       "API request timeout",
			RecoveryAttempted:  false,
		},
		{
			Timestamp:          time.Now().Add(2 * time.Second).Format(time.RFC3339),
			SessionID:          "gemini-session-001",
			SessionName:        "my-gemini-session",
			Symptom:            "permission_prompt",
			DurationMinutes:    10,
			DetectionHeuristic: "directory_auth_prompt",
			PaneSnapshot:       "Allow directory access?",
			RecoveryAttempted:  true,
			RecoveryMethod:     strPtr("yes"),
			RecoverySuccess:    boolPtr(true),
		},
	}

	// Write all incidents to file
	file, err := os.OpenFile(incidentsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open incidents file: %v", err)
	}
	defer file.Close()

	for _, incident := range incidents {
		data, err := json.Marshal(incident)
		if err != nil {
			t.Fatalf("Failed to marshal incident: %v", err)
		}

		if _, err := file.Write(append(data, '\n')); err != nil {
			t.Fatalf("Failed to write incident: %v", err)
		}
	}
	file.Sync()

	// Wait for watcher to process all incidents
	time.Sleep(300 * time.Millisecond)

	// Verify all incidents were processed
	if watcher.LastPosition() == 0 {
		t.Error("Watcher did not process the incidents file")
	}

	// In a real test, we'd verify that:
	// 1. All three escalation events were published to EventBus
	// 2. Each event had correct escalation type and severity
	// 3. Time-windowing prevented duplicate escalations for same session
}

// TestOpenAIRecoveryMessage tests sending a recovery message to an OpenAI session
// Note: This test would require a mock OpenAI adapter or integration with the AGM send command
func TestOpenAIRecoveryMessage(t *testing.T) {
	t.Skip("Requires AGM send command integration - tested in cmd/agm/send_integration_test.go")

	// This test is documented here for completeness but implemented in
	// the AGM send command test suite where we can properly mock the
	// Agent interface and verify message delivery
}

// Helper functions

func strPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}
