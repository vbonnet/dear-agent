package eventbus

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateEventType(t *testing.T) {
	tests := []struct {
		name      string
		eventType EventType
		wantErr   bool
	}{
		{
			name:      "valid session.stuck",
			eventType: EventSessionStuck,
			wantErr:   false,
		},
		{
			name:      "valid session.escalated",
			eventType: EventSessionEscalated,
			wantErr:   false,
		},
		{
			name:      "valid session.recovered",
			eventType: EventSessionRecovered,
			wantErr:   false,
		},
		{
			name:      "valid session.state_change",
			eventType: EventSessionStateChange,
			wantErr:   false,
		},
		{
			name:      "valid session.completed",
			eventType: EventSessionCompleted,
			wantErr:   false,
		},
		{
			name:      "valid stall.detected",
			eventType: EventStallDetected,
			wantErr:   false,
		},
		{
			name:      "valid stall.recovered",
			eventType: EventStallRecovered,
			wantErr:   false,
		},
		{
			name:      "valid stall.escalated",
			eventType: EventStallEscalated,
			wantErr:   false,
		},
		{
			name:      "invalid event type",
			eventType: "session.invalid",
			wantErr:   true,
		},
		{
			name:      "empty event type",
			eventType: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEventType(tt.eventType)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewEvent(t *testing.T) {
	tests := []struct {
		name      string
		eventType EventType
		sessionID string
		payload   any
		wantErr   bool
	}{
		{
			name:      "valid event with payload",
			eventType: EventSessionStuck,
			sessionID: "session-123",
			payload: SessionStuckPayload{
				Reason:   "No output",
				Duration: 5 * time.Minute,
			},
			wantErr: false,
		},
		{
			name:      "valid event with empty payload",
			eventType: EventSessionCompleted,
			sessionID: "session-456",
			payload:   SessionCompletedPayload{},
			wantErr:   false,
		},
		{
			name:      "invalid event type",
			eventType: "invalid.type",
			sessionID: "session-789",
			payload:   nil,
			wantErr:   true,
		},
		{
			name:      "empty session ID",
			eventType: EventSessionStuck,
			sessionID: "",
			payload:   SessionStuckPayload{},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := NewEvent(tt.eventType, tt.sessionID, tt.payload)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, event)
			} else {
				require.NoError(t, err)
				require.NotNil(t, event)
				assert.Equal(t, tt.eventType, event.Type)
				assert.Equal(t, tt.sessionID, event.SessionID)
				assert.False(t, event.Timestamp.IsZero())
			}
		})
	}
}

func TestEvent_ParsePayload(t *testing.T) {
	// Create event with SessionStuckPayload
	originalPayload := SessionStuckPayload{
		Reason:      "Test reason",
		Duration:    10 * time.Minute,
		LastOutput:  "Test output",
		Suggestions: []string{"suggestion1", "suggestion2"},
	}

	event, err := NewEvent(EventSessionStuck, "session-test", originalPayload)
	require.NoError(t, err)

	// Parse payload back
	var parsedPayload SessionStuckPayload
	err = event.ParsePayload(&parsedPayload)
	require.NoError(t, err)

	// Verify payload matches
	assert.Equal(t, originalPayload.Reason, parsedPayload.Reason)
	assert.Equal(t, originalPayload.Duration, parsedPayload.Duration)
	assert.Equal(t, originalPayload.LastOutput, parsedPayload.LastOutput)
	assert.Equal(t, originalPayload.Suggestions, parsedPayload.Suggestions)
}

func TestEvent_Validate(t *testing.T) {
	tests := []struct {
		name    string
		event   *Event
		wantErr bool
	}{
		{
			name: "valid event",
			event: &Event{
				Type:      EventSessionStuck,
				Timestamp: time.Now(),
				SessionID: "session-123",
				Payload:   json.RawMessage(`{"reason": "test"}`),
			},
			wantErr: false,
		},
		{
			name: "invalid event type",
			event: &Event{
				Type:      "invalid.type",
				Timestamp: time.Now(),
				SessionID: "session-123",
				Payload:   json.RawMessage(`{}`),
			},
			wantErr: true,
		},
		{
			name: "empty session ID",
			event: &Event{
				Type:      EventSessionStuck,
				Timestamp: time.Now(),
				SessionID: "",
				Payload:   json.RawMessage(`{}`),
			},
			wantErr: true,
		},
		{
			name: "zero timestamp",
			event: &Event{
				Type:      EventSessionStuck,
				Timestamp: time.Time{},
				SessionID: "session-123",
				Payload:   json.RawMessage(`{}`),
			},
			wantErr: true,
		},
		{
			name: "invalid payload JSON",
			event: &Event{
				Type:      EventSessionStuck,
				Timestamp: time.Now(),
				SessionID: "session-123",
				Payload:   json.RawMessage(`{invalid json`),
			},
			wantErr: true,
		},
		{
			name: "empty payload is valid",
			event: &Event{
				Type:      EventSessionStuck,
				Timestamp: time.Now(),
				SessionID: "session-123",
				Payload:   json.RawMessage(``),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExampleEvents(t *testing.T) {
	examples := ExampleEvents()

	// Verify all event types have examples
	expectedTypes := []EventType{
		EventSessionStuck,
		EventSessionEscalated,
		EventSessionRecovered,
		EventSessionStateChange,
		EventSessionCompleted,
		EventStallDetected,
		EventStallRecovered,
		EventStallEscalated,
	}

	for _, eventType := range expectedTypes {
		t.Run(string(eventType), func(t *testing.T) {
			event, exists := examples[eventType]
			require.True(t, exists, "Example not found for event type: %s", eventType)
			require.NotNil(t, event)

			// Validate event
			err := event.Validate()
			assert.NoError(t, err, "Example event should be valid")

			// Verify event type matches
			assert.Equal(t, eventType, event.Type)
		})
	}
}

func TestPayloadSerializationRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		eventType EventType
		payload   any
	}{
		{
			name:      "SessionStuckPayload",
			eventType: EventSessionStuck,
			payload: SessionStuckPayload{
				Reason:      "Timeout",
				Duration:    15 * time.Minute,
				LastOutput:  "Processing...",
				Suggestions: []string{"Retry", "Cancel"},
			},
		},
		{
			name:      "SessionEscalatedPayload",
			eventType: EventSessionEscalated,
			payload: SessionEscalatedPayload{
				EscalationType: "error",
				Pattern:        "(?i)fatal",
				Line:           "Fatal error occurred",
				LineNumber:     100,
				DetectedAt:     time.Now(),
				Description:    "Critical error",
				Severity:       "critical",
			},
		},
		{
			name:      "SessionRecoveredPayload",
			eventType: EventSessionRecovered,
			payload: SessionRecoveredPayload{
				PreviousState: "stuck",
				RecoveryTime:  3 * time.Minute,
				Action:        "Manual intervention",
			},
		},
		{
			name:      "SessionStateChangePayload",
			eventType: EventSessionStateChange,
			payload: SessionStateChangePayload{
				OldState: "running",
				NewState: "paused",
				Reason:   "User requested pause",
			},
		},
		{

			name:      "SessionCompletedPayload",

			eventType: EventSessionCompleted,

			payload: SessionCompletedPayload{

				ExitCode:     0,

				Duration:     45 * time.Minute,

				MessageCount: 100,

				TokensUsed:   25000,

				FinalState:   "success",

			},

		},

		{

			name:      "StallDetectedPayload",

			eventType: EventStallDetected,

			payload: StallDetectedPayload{

				StallType: "permission_prompt",

				Session:   "worker-1",

				Duration:  10 * time.Minute,

				Details:   "Permission dialog open for 10m",

				Severity:  "critical",

			},

		},

		{

			name:      "StallRecoveredPayload",

			eventType: EventStallRecovered,

			payload: StallRecoveredPayload{

				StallType:      "no_commit",

				Session:        "worker-1",

				RecoveryAction: "nudge",

				Duration:       15 * time.Minute,

			},

		},

		{

			name:      "StallEscalatedPayload",

			eventType: EventStallEscalated,

			payload: StallEscalatedPayload{

				StallType:    "error_loop",

				Session:      "worker-2",

				Reason:       "max retries exceeded",

				AttemptCount: 3,

			},

		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create event with payload
			event, err := NewEvent(tt.eventType, "session-test", tt.payload)
			require.NoError(t, err)

			// Serialize to JSON
			eventJSON, err := json.Marshal(event)
			require.NoError(t, err)

			// Deserialize from JSON
			var deserializedEvent Event
			err = json.Unmarshal(eventJSON, &deserializedEvent)
			require.NoError(t, err)

			// Verify event fields match
			assert.Equal(t, event.Type, deserializedEvent.Type)
			assert.Equal(t, event.SessionID, deserializedEvent.SessionID)
			assert.WithinDuration(t, event.Timestamp, deserializedEvent.Timestamp, time.Second)

			// Verify payload can be parsed
			err = deserializedEvent.Validate()
			assert.NoError(t, err)
		})
	}
}
