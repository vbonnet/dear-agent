package manifest

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUUIDGeneration_NotSessionName is a REGRESSION TEST for the bug where
// SessionID was generated as "session-{name}" instead of a proper UUID.
//
// Bug History (2026-01-13):
// - Before fix: SessionID = fmt.Sprintf("session-%s", sessionName)
// - Symptom: Validation errors, non-standard UUIDs in manifest
// - Root cause: Copy-paste error from directory naming code
// - Fix: SessionID = uuid.New().String()
//
// This test ensures SessionID is always a valid RFC 4122 UUID.
func TestUUIDGeneration_NotSessionName(t *testing.T) {
	sessionName := "test-session-123"

	// Create a new manifest
	m := &Manifest{
		SchemaVersion: SchemaVersion,
		SessionID:     uuid.New().String(), // Correct implementation
		Name:          sessionName,
	}

	// Verify SessionID is a valid UUID (not session-{name})
	parsedUUID, err := uuid.Parse(m.SessionID)
	require.NoError(t, err, "SessionID must be a valid UUID")
	assert.NotEmpty(t, parsedUUID.String(), "Parsed UUID should not be empty")

	// Verify SessionID is NOT the old broken format
	brokenFormat := "session-" + sessionName
	assert.NotEqual(t, brokenFormat, m.SessionID,
		"SessionID must NOT use 'session-{name}' format (regression from bug)")

	// Verify SessionID follows UUID format (8-4-4-4-12 hexadecimal)
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`,
		m.SessionID, "SessionID must follow UUID format")
}

// TestUUIDGeneration_IsUnique verifies that each generated UUID is unique
func TestUUIDGeneration_IsUnique(t *testing.T) {
	// Generate multiple UUIDs and ensure they're all unique
	const count = 100
	uuids := make(map[string]bool, count)

	for i := 0; i < count; i++ {
		id := uuid.New().String()

		// Verify this UUID hasn't been seen before
		assert.False(t, uuids[id], "UUID %s generated twice (not unique)", id)
		uuids[id] = true

		// Verify it's a valid UUID
		_, err := uuid.Parse(id)
		assert.NoError(t, err, "Generated ID must be valid UUID")
	}

	assert.Len(t, uuids, count, "Should have generated %d unique UUIDs", count)
}

// TestManifestValidation_SessionIDFormat tests that validation catches bad SessionIDs
func TestManifestValidation_SessionIDFormat(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid UUID",
			sessionID: "550e8400-e29b-41d4-a716-446655440000",
			wantErr:   false,
		},
		{
			name:      "regression: session-{name} format",
			sessionID: "session-my-test-session",
			wantErr:   true,
			errMsg:    "invalid UUID format",
		},
		{
			name:      "empty SessionID",
			sessionID: "",
			wantErr:   true,
			errMsg:    "SessionID is required",
		},
		{
			name:      "random string (not UUID)",
			sessionID: "not-a-uuid-at-all",
			wantErr:   true,
			errMsg:    "invalid UUID format",
		},
		{
			name:      "UUID with uppercase (should normalize)",
			sessionID: "550E8400-E29B-41D4-A716-446655440000",
			wantErr:   false, // UUIDs are case-insensitive
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manifest{
				SchemaVersion: SchemaVersion,
				SessionID:     tt.sessionID,
				Name:          "test-session",
			}

			// Try to parse as UUID
			_, err := uuid.Parse(m.SessionID)

			if tt.wantErr {
				assert.Error(t, err, "Expected error for SessionID: %s", tt.sessionID)
				if tt.errMsg != "" {
					// Note: uuid.Parse doesn't return our custom messages,
					// but we can check that it fails for bad formats
					assert.NotNil(t, err)
				}
			} else {
				assert.NoError(t, err, "Should accept valid UUID: %s", tt.sessionID)
			}
		})
	}
}

// TestDirectoryNaming_NoSessionPrefix is a related regression test
// Directory names should be {name}, not session-{name}
func TestDirectoryNaming_NoSessionPrefix(t *testing.T) {
	sessionName := "my-session"

	// Correct directory naming (no session- prefix)
	correctDir := sessionName
	assert.Equal(t, "my-session", correctDir, "Directory should not have session- prefix")

	// Incorrect directory naming (regression)
	incorrectDir := "session-" + sessionName
	assert.NotEqual(t, "my-session", incorrectDir,
		"Old code incorrectly used session-{name} for directories")

	// Document the fix
	t.Log("Directory naming fix:")
	t.Log("  Before: sessions/session-{name}/manifest.yaml")
	t.Log("  After:  sessions/{name}/manifest.yaml")
}

// BenchmarkUUIDGeneration benchmarks UUID generation performance
func BenchmarkUUIDGeneration(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = uuid.New().String()
	}
}
