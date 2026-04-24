package events

import (
	"encoding/json"
	"testing"
)

func TestEventSerialization(t *testing.T) {
	event := PhaseStartedEvent{
		EventBase: EventBase{
			TraceID:   "trace-123",
			Timestamp: 1700000000000,
		},
		Phase:     "D1-problem-validation",
		FeatureID: "feat-1",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded PhaseStartedEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.TraceID != "trace-123" {
		t.Errorf("expected traceId 'trace-123', got %q", decoded.TraceID)
	}
	if decoded.Phase != "D1-problem-validation" {
		t.Errorf("expected phase 'D1-problem-validation', got %q", decoded.Phase)
	}
}

func TestW0EventSerialization(t *testing.T) {
	event := W0StartedEvent{
		EventBase: EventBase{
			TraceID:   "trace-456",
			Timestamp: 1700000000000,
		},
		SessionID:               "session-1",
		ProjectPath:             "/tmp/project",
		TriggeredBy:             "vague_request",
		InitialRequest:          "make it better",
		InitialRequestWordCount: 3,
		VaguenessSignals:        []string{"short", "no_specifics"},
		VaguenessScore:          0.6,
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded W0StartedEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.SessionID != "session-1" {
		t.Errorf("expected sessionId 'session-1', got %q", decoded.SessionID)
	}
	if len(decoded.VaguenessSignals) != 2 {
		t.Errorf("expected 2 signals, got %d", len(decoded.VaguenessSignals))
	}
}

func TestEventConstants(t *testing.T) {
	if EventPersonaInvoked != "wayfinder.persona.invoked" {
		t.Error("wrong event name for PersonaInvoked")
	}
	if EventW0Completed != "wayfinder.w0.completed" {
		t.Error("wrong event name for W0Completed")
	}
}

func TestValidPhases(t *testing.T) {
	if len(ValidPhases) != 12 {
		t.Errorf("expected 12 valid phases, got %d", len(ValidPhases))
	}
}

func TestFeatureCompletedEvent(t *testing.T) {
	event := FeatureCompletedEvent{
		EventBase: EventBase{
			TraceID:   "trace-789",
			Timestamp: 1700000000000,
		},
		FeatureID:       "feat-2",
		TotalDurationMs: 60000,
		PhasesCompleted: 12,
		Outcome:         "success",
		SkippedPhases:   []string{"S9-validation"},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded FeatureCompletedEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.PhasesCompleted != 12 {
		t.Errorf("expected 12 phases, got %d", decoded.PhasesCompleted)
	}
	if len(decoded.SkippedPhases) != 1 {
		t.Errorf("expected 1 skipped phase, got %d", len(decoded.SkippedPhases))
	}
}
