package transcript

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionMapSetName(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		uuid        string
		sessionName string
		wantErr     bool
	}{
		{
			name:        "valid UUID and name",
			uuid:        "12345678-1234-1234-1234-123456789abc",
			sessionName: "Feature: Dark Mode",
			wantErr:     false,
		},
		{
			name:        "valid UUID with special characters in name",
			uuid:        "aaaabbbb-cccc-dddd-eeee-ffffffffffff",
			sessionName: "Bug Fix: Auth Flow (Critical)",
			wantErr:     false,
		},
		{
			name:        "empty session name (should be allowed)",
			uuid:        "11111111-2222-3333-4444-555555555555",
			sessionName: "",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, err := NewSessionMap(tempDir)
			require.NoError(t, err)

			err = sm.SetName(tt.uuid, tt.sessionName)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify file was created
			mapPath := filepath.Join(tempDir, "transcripts", "session_map.json")
			assert.FileExists(t, mapPath)

			// Verify content
			data, err := os.ReadFile(mapPath)
			require.NoError(t, err)

			var sessionMap map[string]string
			require.NoError(t, json.Unmarshal(data, &sessionMap))

			assert.Equal(t, tt.sessionName, sessionMap[tt.uuid])
		})
	}
}

func TestSessionMapGetName(t *testing.T) {
	tempDir := t.TempDir()

	// Setup: Create session map with test data
	mapPath := filepath.Join(tempDir, "transcripts", "session_map.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(mapPath), 0755))

	testMap := map[string]string{
		"12345678-1234-1234-1234-123456789abc": "Feature: Dark Mode",
		"aaaabbbb-cccc-dddd-eeee-ffffffffffff": "Bug Fix: Auth",
	}

	data, err := json.Marshal(testMap)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(mapPath, data, 0644))

	sm, err := NewSessionMap(tempDir)
	require.NoError(t, err)

	tests := []struct {
		name     string
		uuid     string
		wantName string
	}{
		{
			name:     "existing session",
			uuid:     "12345678-1234-1234-1234-123456789abc",
			wantName: "Feature: Dark Mode",
		},
		{
			name:     "another existing session",
			uuid:     "aaaabbbb-cccc-dddd-eeee-ffffffffffff",
			wantName: "Bug Fix: Auth",
		},
		{
			name:     "non-existent session",
			uuid:     "99999999-9999-9999-9999-999999999999",
			wantName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := sm.GetName(tt.uuid)
			assert.Equal(t, tt.wantName, name)
		})
	}
}

func TestSessionMapNoMapFile(t *testing.T) {
	// Test behavior when session_map.json doesn't exist
	tempDir := t.TempDir()

	sm, err := NewSessionMap(tempDir)
	require.NoError(t, err)

	uuid := "12345678-1234-1234-1234-123456789abc"
	name := sm.GetName(uuid)

	// Should return empty string
	assert.Empty(t, name)
}

func TestSessionMapDeleteName(t *testing.T) {
	tempDir := t.TempDir()

	// Setup: Create session map with test data
	mapPath := filepath.Join(tempDir, "transcripts", "session_map.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(mapPath), 0755))

	testMap := map[string]string{
		"12345678-1234-1234-1234-123456789abc": "Feature: Dark Mode",
		"aaaabbbb-cccc-dddd-eeee-ffffffffffff": "Bug Fix: Auth",
	}

	data, err := json.Marshal(testMap)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(mapPath, data, 0644))

	sm, err := NewSessionMap(tempDir)
	require.NoError(t, err)

	tests := []struct {
		name    string
		uuid    string
		wantErr bool
	}{
		{
			name:    "delete existing session",
			uuid:    "12345678-1234-1234-1234-123456789abc",
			wantErr: false,
		},
		{
			name:    "delete non-existent session (should not error)",
			uuid:    "99999999-9999-9999-9999-999999999999",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sm.DeleteName(tt.uuid)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify deletion
			name := sm.GetName(tt.uuid)
			assert.Empty(t, name, "Session should be deleted from map")
		})
	}
}

func TestSessionMapListAll(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name      string
		setupMap  map[string]string
		wantCount int
	}{
		{
			name: "multiple sessions",
			setupMap: map[string]string{
				"12345678-1234-1234-1234-123456789abc": "Feature: Dark Mode",
				"aaaabbbb-cccc-dddd-eeee-ffffffffffff": "Bug Fix: Auth",
				"11111111-2222-3333-4444-555555555555": "Refactor: Database",
			},
			wantCount: 3,
		},
		{
			name:      "empty map",
			setupMap:  map[string]string{},
			wantCount: 0,
		},
		{
			name: "single session",
			setupMap: map[string]string{
				"12345678-1234-1234-1234-123456789abc": "Only Session",
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			testDir := filepath.Join(tempDir, tt.name)
			mapPath := filepath.Join(testDir, "transcripts", "session_map.json")
			require.NoError(t, os.MkdirAll(filepath.Dir(mapPath), 0755))

			data, err := json.Marshal(tt.setupMap)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(mapPath, data, 0644))

			// Create SessionMap and list sessions
			sm, err := NewSessionMap(testDir)
			require.NoError(t, err)

			sessions := sm.ListAll()
			assert.Len(t, sessions, tt.wantCount)

			// Verify content
			for uuid, name := range tt.setupMap {
				assert.Equal(t, name, sessions[uuid])
			}
		})
	}
}

func TestSessionMapListAllEmptyMap(t *testing.T) {
	// Test behavior when session_map.json doesn't exist
	tempDir := t.TempDir()

	sm, err := NewSessionMap(tempDir)
	require.NoError(t, err)

	sessions := sm.ListAll()

	// Should return empty map
	assert.NotNil(t, sessions)
	assert.Empty(t, sessions)
}

func TestConcurrentSessionMapAccess(t *testing.T) {
	// Test thread-safety of session map operations
	tempDir := t.TempDir()

	sm, err := NewSessionMap(tempDir)
	require.NoError(t, err)

	// Run concurrent set operations
	done := make(chan bool)
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			// Generate unique UUIDs
			uuid := fmt.Sprintf("%08d-1234-1234-1234-123456789abc", id)
			name := fmt.Sprintf("Session %d", id)

			err := sm.SetName(uuid, name)
			assert.NoError(t, err)

			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify all sessions were written
	sessions := sm.ListAll()
	assert.Equal(t, numGoroutines, len(sessions), "All sessions should be written")
}

func TestSessionMapJSONFormat(t *testing.T) {
	// Test that session map maintains valid JSON format
	tempDir := t.TempDir()

	sm, err := NewSessionMap(tempDir)
	require.NoError(t, err)

	uuid1 := "12345678-1234-1234-1234-123456789abc"
	uuid2 := "aaaabbbb-cccc-dddd-eeee-ffffffffffff"

	// Set multiple sessions
	require.NoError(t, sm.SetName(uuid1, "Session 1"))
	require.NoError(t, sm.SetName(uuid2, "Session 2"))

	// Read and validate JSON
	mapPath := filepath.Join(tempDir, "transcripts", "session_map.json")
	data, err := os.ReadFile(mapPath)
	require.NoError(t, err)

	// Should be valid JSON
	var sessionMap map[string]string
	require.NoError(t, json.Unmarshal(data, &sessionMap))

	// Verify both UUIDs are in the map
	assert.NotEmpty(t, data)
	assert.Contains(t, string(data), uuid1)
	assert.Contains(t, string(data), uuid2)

	// Verify JSON is properly formatted (has indentation)
	assert.Contains(t, string(data), "\n")
}
