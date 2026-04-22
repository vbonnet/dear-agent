package trace

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

func TestRecordFromEvent(t *testing.T) {
	payload := map[string]interface{}{
		"reason": "test stuck",
	}
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	ev := &eventbus.Event{
		Type:      eventbus.EventSessionStuck,
		Timestamp: time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC),
		SessionID: "sess-123",
		Payload:   payloadBytes,
	}

	rec, err := RecordFromEvent(ev)
	require.NoError(t, err)
	assert.Equal(t, eventbus.EventSessionStuck, rec.EventType)
	assert.Equal(t, "sess-123", rec.SessionID)
	assert.Equal(t, "test stuck", rec.Payload["reason"])
}

func TestRecordFromEvent_EmptyPayload(t *testing.T) {
	ev := &eventbus.Event{
		Type:      eventbus.EventSessionCompleted,
		Timestamp: time.Now(),
		SessionID: "sess-456",
	}

	rec, err := RecordFromEvent(ev)
	require.NoError(t, err)
	assert.Nil(t, rec.Payload)
}
