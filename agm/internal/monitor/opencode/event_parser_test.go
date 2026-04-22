package opencode

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventParser_ParsePermissionAsked(t *testing.T) {
	parser := NewEventParser()
	data := []byte(`{
		"type": "permission.asked",
		"timestamp": 1709654321,
		"properties": {
			"permission": {
				"id": "perm-123",
				"action": "file.write",
				"path": "~/main.go"
			}
		}
	}`)

	event, err := parser.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "AWAITING_PERMISSION", event.State)
	assert.Equal(t, int64(1709654321), event.Timestamp)
	assert.Equal(t, "perm-123", event.Metadata["permission_id"])
	assert.Equal(t, "file.write", event.Metadata["action"])
	assert.Equal(t, "~/main.go", event.Metadata["path"])
	assert.Equal(t, "permission.asked", event.Metadata["event_type"])
}

func TestEventParser_ParseToolExecuteBefore(t *testing.T) {
	parser := NewEventParser()
	data := []byte(`{
		"type": "tool.execute.before",
		"timestamp": 1709654322,
		"properties": {
			"tool": "Write",
			"args": {
				"file_path": "~/main.go"
			}
		}
	}`)

	event, err := parser.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "WORKING", event.State)
	assert.Equal(t, int64(1709654322), event.Timestamp)
	assert.Equal(t, "Write", event.Metadata["tool_name"])
	assert.Equal(t, "tool.execute.before", event.Metadata["event_type"])
}

func TestEventParser_ParseToolExecuteAfter(t *testing.T) {
	parser := NewEventParser()
	data := []byte(`{
		"type": "tool.execute.after",
		"timestamp": 1709654325,
		"properties": {
			"tool": "Write",
			"success": true
		}
	}`)

	event, err := parser.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "IDLE", event.State)
	assert.Equal(t, int64(1709654325), event.Timestamp)
	assert.Equal(t, "Write", event.Metadata["tool_name"])
	assert.Equal(t, "tool.execute.after", event.Metadata["event_type"])
}

func TestEventParser_ParseSessionCreated(t *testing.T) {
	parser := NewEventParser()
	data := []byte(`{
		"type": "session.created",
		"timestamp": 1709654320,
		"properties": {
			"session_id": "abc-123-def"
		}
	}`)

	event, err := parser.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "DONE", event.State)
	assert.Equal(t, int64(1709654320), event.Timestamp)
	assert.Equal(t, "abc-123-def", event.Metadata["session_id"])
	assert.Equal(t, "session.created", event.Metadata["event_type"])
}

func TestEventParser_ParseSessionClosed(t *testing.T) {
	parser := NewEventParser()
	data := []byte(`{
		"type": "session.closed",
		"timestamp": 1709654400,
		"properties": {}
	}`)

	event, err := parser.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "TERMINATED", event.State)
	assert.Equal(t, int64(1709654400), event.Timestamp)
	assert.Equal(t, "session.closed", event.Metadata["event_type"])
}

func TestEventParser_MalformedJSON(t *testing.T) {
	parser := NewEventParser()
	data := []byte(`{invalid json`)

	_, err := parser.Parse(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestEventParser_UnknownEventType(t *testing.T) {
	parser := NewEventParser()
	data := []byte(`{
		"type": "unknown.event",
		"timestamp": 1709654321,
		"properties": {}
	}`)

	event, err := parser.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "WORKING", event.State) // Default fallback
	assert.Equal(t, int64(1709654321), event.Timestamp)
	assert.Equal(t, "unknown.event", event.Metadata["event_type"])
}

func TestEventParser_MissingEventType(t *testing.T) {
	parser := NewEventParser()
	data := []byte(`{
		"timestamp": 1709654321,
		"properties": {}
	}`)

	_, err := parser.Parse(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing event type")
}

func TestEventParser_MissingTimestamp(t *testing.T) {
	parser := NewEventParser()
	data := []byte(`{
		"type": "permission.asked",
		"properties": {}
	}`)

	_, err := parser.Parse(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing timestamp")
}

func TestEventParser_EmptyProperties(t *testing.T) {
	parser := NewEventParser()
	data := []byte(`{
		"type": "tool.execute.before",
		"timestamp": 1709654322,
		"properties": {}
	}`)

	event, err := parser.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "WORKING", event.State)
	assert.Equal(t, int64(1709654322), event.Timestamp)
	assert.Equal(t, "tool.execute.before", event.Metadata["event_type"])
	// Should not have tool_name when tool property is missing
	_, hasToolName := event.Metadata["tool_name"]
	assert.False(t, hasToolName)
}

func TestEventParser_MalformedPermissionMetadata(t *testing.T) {
	parser := NewEventParser()
	data := []byte(`{
		"type": "permission.asked",
		"timestamp": 1709654321,
		"properties": {
			"permission": "invalid-not-a-map"
		}
	}`)

	event, err := parser.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "AWAITING_PERMISSION", event.State)
	// Should still parse successfully, just without permission metadata
	_, hasPermID := event.Metadata["permission_id"]
	assert.False(t, hasPermID)
}

func TestEventParser_PartialPermissionMetadata(t *testing.T) {
	parser := NewEventParser()
	data := []byte(`{
		"type": "permission.asked",
		"timestamp": 1709654321,
		"properties": {
			"permission": {
				"id": "perm-456"
			}
		}
	}`)

	event, err := parser.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "AWAITING_PERMISSION", event.State)
	assert.Equal(t, "perm-456", event.Metadata["permission_id"])
	// action and path should not be present
	_, hasAction := event.Metadata["action"]
	assert.False(t, hasAction)
	_, hasPath := event.Metadata["path"]
	assert.False(t, hasPath)
}

func TestEventParser_AllEventTypes(t *testing.T) {
	parser := NewEventParser()

	testCases := []struct {
		name          string
		eventType     string
		expectedState string
	}{
		{"permission.asked", "permission.asked", "AWAITING_PERMISSION"},
		{"tool.execute.before", "tool.execute.before", "WORKING"},
		{"tool.execute.after", "tool.execute.after", "IDLE"},
		{"session.created", "session.created", "DONE"},
		{"session.closed", "session.closed", "TERMINATED"},
		{"unknown.type", "unknown.type", "WORKING"},
		{"future.event", "future.event", "WORKING"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(fmt.Sprintf(`{
				"type": "%s",
				"timestamp": 1709654321,
				"properties": {}
			}`, tc.eventType))

			event, err := parser.Parse(data)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedState, event.State)
		})
	}
}

func TestEventParser_ZeroTimestamp(t *testing.T) {
	parser := NewEventParser()
	data := []byte(`{
		"type": "session.created",
		"timestamp": 0,
		"properties": {}
	}`)

	_, err := parser.Parse(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing timestamp")
}

func TestEventParser_NegativeTimestamp(t *testing.T) {
	parser := NewEventParser()
	data := []byte(`{
		"type": "session.created",
		"timestamp": -1,
		"properties": {}
	}`)

	// Negative timestamp should parse successfully (Unix epoch supports negative values)
	event, err := parser.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, int64(-1), event.Timestamp)
}

func TestEventParser_MetadataExtraction(t *testing.T) {
	parser := NewEventParser()

	t.Run("CompletePermissionMetadata", func(t *testing.T) {
		event := OpenCodeEvent{
			Type:      "permission.asked",
			Timestamp: 1709654321,
			Properties: map[string]interface{}{
				"permission": map[string]interface{}{
					"id":     "perm-789",
					"action": "file.read",
					"path":   "/etc/hosts",
				},
			},
		}

		metadata := parser.extractMetadata(event)
		assert.Equal(t, "perm-789", metadata["permission_id"])
		assert.Equal(t, "file.read", metadata["action"])
		assert.Equal(t, "/etc/hosts", metadata["path"])
	})

	t.Run("ToolMetadata", func(t *testing.T) {
		event := OpenCodeEvent{
			Type:      "tool.execute.before",
			Timestamp: 1709654321,
			Properties: map[string]interface{}{
				"tool": "Bash",
			},
		}

		metadata := parser.extractMetadata(event)
		assert.Equal(t, "Bash", metadata["tool_name"])
	})

	t.Run("SessionMetadata", func(t *testing.T) {
		event := OpenCodeEvent{
			Type:      "session.created",
			Timestamp: 1709654321,
			Properties: map[string]interface{}{
				"session_id": "xyz-456",
			},
		}

		metadata := parser.extractMetadata(event)
		assert.Equal(t, "xyz-456", metadata["session_id"])
	})
}

func TestEventParser_Validate(t *testing.T) {
	parser := NewEventParser()

	t.Run("ValidEvent", func(t *testing.T) {
		event := OpenCodeEvent{
			Type:       "permission.asked",
			Timestamp:  1709654321,
			Properties: map[string]interface{}{},
		}

		err := parser.validate(event)
		assert.NoError(t, err)
	})

	t.Run("MissingType", func(t *testing.T) {
		event := OpenCodeEvent{
			Timestamp:  1709654321,
			Properties: map[string]interface{}{},
		}

		err := parser.validate(event)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing event type")
	})

	t.Run("MissingTimestamp", func(t *testing.T) {
		event := OpenCodeEvent{
			Type:       "session.created",
			Properties: map[string]interface{}{},
		}

		err := parser.validate(event)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing timestamp")
	})
}

func TestEventParser_MapState(t *testing.T) {
	parser := NewEventParser()

	testCases := []struct {
		eventType     string
		expectedState string
	}{
		{"permission.asked", "AWAITING_PERMISSION"},
		{"tool.execute.before", "WORKING"},
		{"tool.execute.after", "IDLE"},
		{"session.created", "DONE"},
		{"session.closed", "TERMINATED"},
		{"unknown.event", "WORKING"},
		{"", "WORKING"},
	}

	for _, tc := range testCases {
		t.Run(tc.eventType, func(t *testing.T) {
			state := parser.mapState(tc.eventType)
			assert.Equal(t, tc.expectedState, state)
		})
	}
}
