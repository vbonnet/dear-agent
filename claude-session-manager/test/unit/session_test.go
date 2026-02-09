package unit_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"gopkg.in/yaml.v3"
)

// TestSession_CreationWithValidInput tests creating a manifest with valid fields
func TestSession_CreationWithValidInput(t *testing.T) {
	// Create a valid manifest
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "test-session-123",
		Name:          "My Test Session",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "", // Empty = active/stopped
		Context: manifest.Context{
			Project: "/home/user/projects/myapp",
			Purpose: "Testing session creation",
			Tags:    []string{"test", "development"},
			Notes:   "This is a test session",
		},
		Claude: manifest.Claude{
			UUID: "claude-uuid-123",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-session",
		},
		Agent: "claude",
	}

	// Verify all fields are set correctly
	assert.Equal(t, "2", m.SchemaVersion)
	assert.Equal(t, "test-session-123", m.SessionID)
	assert.Equal(t, "My Test Session", m.Name)
	assert.Equal(t, "", m.Lifecycle)
	assert.Equal(t, "/home/user/projects/myapp", m.Context.Project)
	assert.Equal(t, "Testing session creation", m.Context.Purpose)
	assert.Len(t, m.Context.Tags, 2)
	assert.Contains(t, m.Context.Tags, "test")
	assert.Contains(t, m.Context.Tags, "development")
	assert.Equal(t, "This is a test session", m.Context.Notes)
	assert.Equal(t, "claude-uuid-123", m.Claude.UUID)
	assert.Equal(t, "test-session", m.Tmux.SessionName)
	assert.Equal(t, "claude", m.Agent)
	assert.False(t, m.CreatedAt.IsZero())
	assert.False(t, m.UpdatedAt.IsZero())
}

// TestSession_SerializationToYAML tests marshaling manifest to YAML
func TestSession_SerializationToYAML(t *testing.T) {
	// Create manifest
	createdAt := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)

	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "test-123",
		Name:          "Test Session",
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
		Lifecycle:     "", // Empty = active/stopped
		Context: manifest.Context{
			Project: "/tmp/project",
			Tags:    []string{"tag1", "tag2"},
		},
		Claude: manifest.Claude{
			UUID: "uuid-123",
		},
		Tmux: manifest.Tmux{
			SessionName: "tmux-test",
		},
		Agent: "claude",
	}

	// Serialize to YAML
	data, err := yaml.Marshal(m)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Verify YAML contains expected fields
	yamlStr := string(data)
	assert.Contains(t, yamlStr, "schema_version: \"2\"")
	assert.Contains(t, yamlStr, "session_id: test-123")
	assert.Contains(t, yamlStr, "name: Test Session")
	// Empty lifecycle might not be serialized (YAML omits empty strings)
	assert.Contains(t, yamlStr, "project: /tmp/project")
	assert.Contains(t, yamlStr, "- tag1")
	assert.Contains(t, yamlStr, "- tag2")
	assert.Contains(t, yamlStr, "uuid: uuid-123")
	assert.Contains(t, yamlStr, "session_name: tmux-test")
	assert.Contains(t, yamlStr, "agent: claude")
}

// TestSession_DeserializationFromYAML tests unmarshaling YAML to manifest
func TestSession_DeserializationFromYAML(t *testing.T) {
	// Sample YAML
	yamlData := `schema_version: "2"
session_id: deserialized-session
name: Deserialized Session
created_at: 2024-01-01T12:00:00Z
updated_at: 2024-01-02T12:00:00Z
lifecycle: ""
context:
  project: /home/user/project
  purpose: Testing deserialization
  tags:
    - test
    - yaml
  notes: Test notes
claude:
  uuid: claude-uuid-456
tmux:
  session_name: deserialized-tmux
agent: claude
`

	// Deserialize from YAML
	var m manifest.Manifest
	err := yaml.Unmarshal([]byte(yamlData), &m)
	require.NoError(t, err)

	// Verify all fields deserialized correctly
	assert.Equal(t, "2", m.SchemaVersion)
	assert.Equal(t, "deserialized-session", m.SessionID)
	assert.Equal(t, "Deserialized Session", m.Name)
	assert.Equal(t, "", m.Lifecycle)
	assert.Equal(t, "/home/user/project", m.Context.Project)
	assert.Equal(t, "Testing deserialization", m.Context.Purpose)
	assert.Len(t, m.Context.Tags, 2)
	assert.Contains(t, m.Context.Tags, "test")
	assert.Contains(t, m.Context.Tags, "yaml")
	assert.Equal(t, "Test notes", m.Context.Notes)
	assert.Equal(t, "claude-uuid-456", m.Claude.UUID)
	assert.Equal(t, "deserialized-tmux", m.Tmux.SessionName)
	assert.Equal(t, "claude", m.Agent)

	// Verify timestamps
	assert.Equal(t, 2024, m.CreatedAt.Year())
	assert.Equal(t, time.January, m.CreatedAt.Month())
	assert.Equal(t, 1, m.CreatedAt.Day())
}

// TestSession_RoundTripSerialization tests serialization + deserialization
func TestSession_RoundTripSerialization(t *testing.T) {
	// Create original manifest
	original := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "roundtrip-test",
		Name:          "Round Trip Test",
		CreatedAt:     time.Now().Round(time.Second), // Round to avoid nanosecond precision issues
		UpdatedAt:     time.Now().Round(time.Second),
		Lifecycle:     "", // Empty = active/stopped
		Context: manifest.Context{
			Project: "/tmp/roundtrip",
			Tags:    []string{"roundtrip", "test"},
		},
		Claude: manifest.Claude{
			UUID: "roundtrip-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "roundtrip-tmux",
		},
		Agent: "claude",
	}

	// Serialize to YAML
	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	// Deserialize back
	var restored manifest.Manifest
	err = yaml.Unmarshal(data, &restored)
	require.NoError(t, err)

	// Verify all fields match
	assert.Equal(t, original.SchemaVersion, restored.SchemaVersion)
	assert.Equal(t, original.SessionID, restored.SessionID)
	assert.Equal(t, original.Name, restored.Name)
	assert.Equal(t, original.Lifecycle, restored.Lifecycle)
	assert.Equal(t, original.Context.Project, restored.Context.Project)
	assert.Equal(t, original.Context.Tags, restored.Context.Tags)
	assert.Equal(t, original.Claude.UUID, restored.Claude.UUID)
	assert.Equal(t, original.Tmux.SessionName, restored.Tmux.SessionName)
	assert.Equal(t, original.Agent, restored.Agent)
	assert.True(t, original.CreatedAt.Equal(restored.CreatedAt))
	assert.True(t, original.UpdatedAt.Equal(restored.UpdatedAt))
}

// TestSession_ValidationLogic tests manifest validation
func TestSession_ValidationLogic(t *testing.T) {
	t.Run("valid manifest passes validation", func(t *testing.T) {
		m := &manifest.Manifest{
			SchemaVersion: "2",
			SessionID:     "valid-session-123",
			Name:          "Valid Session",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Lifecycle:     "", // Empty = active/stopped
			Context: manifest.Context{
				Project: "/tmp/project",
			},
			Tmux: manifest.Tmux{
				SessionName: "valid-tmux",
			},
		}

		err := m.Validate()
		assert.NoError(t, err, "Valid manifest should pass validation")
	})

	t.Run("missing session ID fails validation", func(t *testing.T) {
		m := &manifest.Manifest{
			SchemaVersion: "2",
			SessionID:     "", // Missing
			Name:          "Invalid Session",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Context: manifest.Context{
				Project: "/tmp/project",
			},
			Tmux: manifest.Tmux{
				SessionName: "invalid-tmux",
			},
		}

		err := m.Validate()
		assert.Error(t, err, "Manifest with missing session_id should fail validation")
		assert.Contains(t, err.Error(), "session_id")
	})

	t.Run("missing tmux session name fails validation", func(t *testing.T) {
		m := &manifest.Manifest{
			SchemaVersion: "2",
			SessionID:     "session-123",
			Name:          "Invalid Session",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Context: manifest.Context{
				Project: "/tmp/project",
			},
			Tmux: manifest.Tmux{
				SessionName: "", // Missing
			},
		}

		err := m.Validate()
		assert.Error(t, err, "Manifest with missing tmux session_name should fail validation")
		assert.Contains(t, err.Error(), "session_name")
	})

	t.Run("invalid lifecycle fails validation", func(t *testing.T) {
		m := &manifest.Manifest{
			SchemaVersion: "2",
			SessionID:     "session-123",
			Name:          "Invalid Session",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Lifecycle:     "invalid-state", // Invalid lifecycle
			Context: manifest.Context{
				Project: "/tmp/project",
			},
			Tmux: manifest.Tmux{
				SessionName: "test-tmux",
			},
		}

		err := m.Validate()
		assert.Error(t, err, "Manifest with invalid lifecycle should fail validation")
		assert.Contains(t, err.Error(), "lifecycle")
	})
}

// TestSession_WriteAndRead tests writing manifest to file and reading it back
func TestSession_WriteAndRead(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")

	// Create manifest
	original := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "write-read-test",
		Name:          "Write Read Test",
		CreatedAt:     time.Now().Round(time.Second),
		UpdatedAt:     time.Now().Round(time.Second),
		Lifecycle:     "", // Empty = active/stopped
		Context: manifest.Context{
			Project: "/tmp/writeread",
			Tags:    []string{"write", "read"},
		},
		Claude: manifest.Claude{
			UUID: "write-read-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "write-read-tmux",
		},
		Agent: "claude",
	}

	// Write manifest
	err := manifest.Write(manifestPath, original)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(manifestPath)
	require.NoError(t, err)

	// Read manifest back
	restored, err := manifest.Read(manifestPath)
	require.NoError(t, err)
	require.NotNil(t, restored)

	// Verify fields match
	assert.Equal(t, original.SessionID, restored.SessionID)
	assert.Equal(t, original.Name, restored.Name)
	assert.Equal(t, original.Lifecycle, restored.Lifecycle)
	assert.Equal(t, original.Context.Project, restored.Context.Project)
	assert.Equal(t, original.Context.Tags, restored.Context.Tags)
	assert.Equal(t, original.Claude.UUID, restored.Claude.UUID)
	assert.Equal(t, original.Tmux.SessionName, restored.Tmux.SessionName)
	assert.Equal(t, original.Agent, restored.Agent)
}

// TestSession_EngramMetadata tests Engram metadata handling
func TestSession_EngramMetadata(t *testing.T) {
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "engram-test",
		Name:          "Engram Test",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "", // Empty = active/stopped
		Context: manifest.Context{
			Project: "/tmp/engram",
		},
		Tmux: manifest.Tmux{
			SessionName: "engram-tmux",
		},
		EngramMetadata: &manifest.EngramMetadata{
			Enabled:   true,
			Query:     "test query",
			EngramIDs: []string{"id1", "id2", "id3"},
			LoadedAt:  time.Now(),
			Count:     3,
		},
	}

	// Verify Engram metadata is set
	require.NotNil(t, m.EngramMetadata)
	assert.True(t, m.EngramMetadata.Enabled)
	assert.Equal(t, "test query", m.EngramMetadata.Query)
	assert.Len(t, m.EngramMetadata.EngramIDs, 3)
	assert.Contains(t, m.EngramMetadata.EngramIDs, "id1")
	assert.Equal(t, 3, m.EngramMetadata.Count)

	// Serialize and deserialize with Engram metadata
	data, err := yaml.Marshal(m)
	require.NoError(t, err)

	var restored manifest.Manifest
	err = yaml.Unmarshal(data, &restored)
	require.NoError(t, err)

	// Verify Engram metadata survived round trip
	require.NotNil(t, restored.EngramMetadata)
	assert.Equal(t, m.EngramMetadata.Enabled, restored.EngramMetadata.Enabled)
	assert.Equal(t, m.EngramMetadata.Query, restored.EngramMetadata.Query)
	assert.Equal(t, m.EngramMetadata.EngramIDs, restored.EngramMetadata.EngramIDs)
	assert.Equal(t, m.EngramMetadata.Count, restored.EngramMetadata.Count)
}

// TestSession_LifecycleStates tests different lifecycle states
func TestSession_LifecycleStates(t *testing.T) {
	tests := []struct {
		name      string
		lifecycle string
	}{
		{"active/stopped state (empty)", ""},
		{"archived state", manifest.LifecycleArchived},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manifest.Manifest{
				SchemaVersion: "2",
				SessionID:     "lifecycle-test",
				Name:          "Lifecycle Test",
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				Lifecycle:     tt.lifecycle,
				Context: manifest.Context{
					Project: "/tmp/lifecycle",
				},
				Tmux: manifest.Tmux{
					SessionName: "lifecycle-tmux",
				},
			}

			// Verify lifecycle is set correctly
			assert.Equal(t, tt.lifecycle, m.Lifecycle)

			// Serialize and deserialize
			data, err := yaml.Marshal(m)
			require.NoError(t, err)

			var restored manifest.Manifest
			err = yaml.Unmarshal(data, &restored)
			require.NoError(t, err)

			// Verify lifecycle survived round trip
			assert.Equal(t, tt.lifecycle, restored.Lifecycle)
		})
	}
}
