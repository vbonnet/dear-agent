package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/db"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// TestDualWriter_CreateSession tests creating a session in both SQLite and YAML
func TestDualWriter_CreateSession(t *testing.T) {
	// Setup: Create in-memory SQLite + temp YAML directory
	database, cleanup := setupTestDB(t)
	defer cleanup()

	yamlDir := t.TempDir()
	dw := NewDualWriter(database, yamlDir)

	// Create test manifest
	m := createTestManifest("session-1", "Test Session")

	// Test: Create session
	err := dw.CreateSession(m)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Verify: SQLite has the session
	retrieved, err := dw.db.GetSession("session-1")
	if err != nil {
		t.Fatalf("Failed to get session from SQLite: %v", err)
	}
	if retrieved.Name != "Test Session" {
		t.Errorf("Expected name 'Test Session', got '%s'", retrieved.Name)
	}

	// Verify: YAML file was created
	yamlPath := filepath.Join(yamlDir, "session-1", "manifest.yaml")
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		t.Errorf("YAML file was not created at %s", yamlPath)
	}

	// Verify: YAML content matches
	yamlManifest, err := dw.ReadYAML(yamlPath)
	if err != nil {
		t.Fatalf("Failed to read YAML: %v", err)
	}
	if yamlManifest.SessionID != "session-1" {
		t.Errorf("Expected session_id 'session-1', got '%s'", yamlManifest.SessionID)
	}
}

// TestDualWriter_UpdateSession tests updating a session in both SQLite and YAML
func TestDualWriter_UpdateSession(t *testing.T) {
	// Setup
	database, cleanup := setupTestDB(t)
	defer cleanup()

	yamlDir := t.TempDir()
	dw := NewDualWriter(database, yamlDir)

	// Create initial session
	m := createTestManifest("session-1", "Original Name")
	if err := dw.CreateSession(m); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Update session
	m.Name = "Updated Name"
	m.Context.Purpose = "Updated Purpose"

	err := dw.UpdateSession(m)
	if err != nil {
		t.Fatalf("UpdateSession failed: %v", err)
	}

	// Verify: SQLite has updated data
	retrieved, err := dw.db.GetSession("session-1")
	if err != nil {
		t.Fatalf("Failed to get session from SQLite: %v", err)
	}
	if retrieved.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", retrieved.Name)
	}
	if retrieved.Context.Purpose != "Updated Purpose" {
		t.Errorf("Expected purpose 'Updated Purpose', got '%s'", retrieved.Context.Purpose)
	}

	// Verify: YAML has updated data
	yamlPath := filepath.Join(yamlDir, "session-1", "manifest.yaml")
	yamlManifest, err := dw.ReadYAML(yamlPath)
	if err != nil {
		t.Fatalf("Failed to read YAML: %v", err)
	}
	if yamlManifest.Name != "Updated Name" {
		t.Errorf("YAML: Expected name 'Updated Name', got '%s'", yamlManifest.Name)
	}
}

// TestDualWriter_DeleteSession tests deleting a session from both SQLite and YAML
func TestDualWriter_DeleteSession(t *testing.T) {
	// Setup
	database, cleanup := setupTestDB(t)
	defer cleanup()

	yamlDir := t.TempDir()
	dw := NewDualWriter(database, yamlDir)

	// Create session
	m := createTestManifest("session-1", "Test Session")
	if err := dw.CreateSession(m); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Verify session exists
	yamlPath := filepath.Join(yamlDir, "session-1", "manifest.yaml")
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		t.Fatalf("YAML file was not created")
	}

	// Delete session
	err := dw.DeleteSession("session-1")
	if err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	// Verify: SQLite no longer has the session
	_, err = dw.db.GetSession("session-1")
	if err == nil {
		t.Error("Expected error when getting deleted session from SQLite, got nil")
	}

	// Verify: YAML directory is deleted
	yamlSessionDir := filepath.Join(yamlDir, "session-1")
	if _, err := os.Stat(yamlSessionDir); !os.IsNotExist(err) {
		t.Error("YAML directory still exists after delete")
	}
}

// TestDualWriter_GetSession tests reading always from SQLite
func TestDualWriter_GetSession(t *testing.T) {
	// Setup
	database, cleanup := setupTestDB(t)
	defer cleanup()

	yamlDir := t.TempDir()
	dw := NewDualWriter(database, yamlDir)

	// Create session
	m := createTestManifest("session-1", "SQLite Version")
	if err := dw.db.CreateSession(m); err != nil {
		t.Fatalf("Failed to create session in SQLite: %v", err)
	}

	// Create different YAML file manually (simulating out-of-sync cache)
	yamlManifest := createTestManifest("session-1", "YAML Version - WRONG")
	yamlPath := filepath.Join(yamlDir, "session-1", "manifest.yaml")
	if err := dw.WriteYAML(yamlManifest, yamlPath); err != nil {
		t.Fatalf("Failed to write YAML: %v", err)
	}

	// GetSession should return SQLite version, not YAML version
	retrieved, err := dw.GetSession("session-1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	// Verify: Got SQLite version, not YAML version
	if retrieved.Name != "SQLite Version" {
		t.Errorf("Expected 'SQLite Version' (from SQLite), got '%s' (should ignore YAML)", retrieved.Name)
	}
}

// TestDualWriter_ListSessions tests listing always from SQLite
func TestDualWriter_ListSessions(t *testing.T) {
	// Setup
	database, cleanup := setupTestDB(t)
	defer cleanup()

	yamlDir := t.TempDir()
	dw := NewDualWriter(database, yamlDir)

	// Create sessions in SQLite
	m1 := createTestManifest("session-1", "Session 1")
	m2 := createTestManifest("session-2", "Session 2")

	if err := dw.db.CreateSession(m1); err != nil {
		t.Fatalf("Failed to create session 1: %v", err)
	}
	if err := dw.db.CreateSession(m2); err != nil {
		t.Fatalf("Failed to create session 2: %v", err)
	}

	// Create extra YAML file that's NOT in SQLite (simulating stale cache)
	staleManifest := createTestManifest("session-stale", "Stale YAML - Should be ignored")
	stalePath := filepath.Join(yamlDir, "session-stale", "manifest.yaml")
	if err := dw.WriteYAML(staleManifest, stalePath); err != nil {
		t.Fatalf("Failed to write stale YAML: %v", err)
	}

	// ListSessions should return only SQLite sessions
	sessions, err := dw.ListSessions(nil)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	// Verify: Only 2 sessions (from SQLite), stale YAML is ignored
	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions (from SQLite only), got %d", len(sessions))
	}

	// Verify session IDs
	sessionIDs := make(map[string]bool)
	for _, s := range sessions {
		sessionIDs[s.SessionID] = true
	}

	if !sessionIDs["session-1"] || !sessionIDs["session-2"] {
		t.Error("Expected session-1 and session-2 from SQLite")
	}
	if sessionIDs["session-stale"] {
		t.Error("Should not include session-stale (stale YAML, not in SQLite)")
	}
}

// TestDualWriter_SQLiteFailurePreventsYAML tests that SQLite failures prevent YAML writes
func TestDualWriter_SQLiteFailurePreventsYAML(t *testing.T) {
	// Setup
	database, cleanup := setupTestDB(t)
	defer cleanup()

	yamlDir := t.TempDir()
	dw := NewDualWriter(database, yamlDir)

	// Create a session
	m := createTestManifest("session-1", "Test Session")
	if err := dw.CreateSession(m); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Try to create duplicate (SQLite will fail on unique constraint)
	duplicate := createTestManifest("session-1", "Duplicate - Should Fail")

	err := dw.CreateSession(duplicate)
	if err == nil {
		t.Error("Expected error when creating duplicate session, got nil")
	}

	// Verify: Original session is still in SQLite
	retrieved, err := dw.db.GetSession("session-1")
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}
	if retrieved.Name != "Test Session" {
		t.Errorf("Session was modified when it shouldn't have been")
	}

	// Verify: YAML file still has original data (or doesn't exist for duplicate)
	yamlPath := filepath.Join(yamlDir, "session-1", "manifest.yaml")
	yamlManifest, err := dw.ReadYAML(yamlPath)
	if err != nil {
		t.Fatalf("Failed to read YAML: %v", err)
	}
	if yamlManifest.Name != "Test Session" {
		t.Errorf("YAML should have original data, not duplicate data")
	}
}

// TestDualWriter_YAMLFailureDoesNotAffectSQLite tests that YAML failures don't affect SQLite
func TestDualWriter_YAMLFailureDoesNotAffectSQLite(t *testing.T) {
	// Setup
	database, cleanup := setupTestDB(t)
	defer cleanup()

	// Create read-only YAML directory to force write failure
	yamlDir := t.TempDir()
	readOnlyDir := filepath.Join(yamlDir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0500); err != nil { // r-x------ (no write)
		t.Fatalf("Failed to create read-only directory: %v", err)
	}
	defer os.Chmod(readOnlyDir, 0700) // Cleanup: make writable again

	dw := NewDualWriter(database, readOnlyDir)

	// Create session - SQLite should succeed, YAML will fail
	m := createTestManifest("session-1", "Test Session")

	// This should NOT return an error, even though YAML write will fail
	// because SQLite is the source of truth
	err := dw.CreateSession(m)
	if err != nil {
		t.Fatalf("CreateSession should succeed even if YAML fails (SQLite is source of truth): %v", err)
	}

	// Verify: SQLite has the session (source of truth)
	retrieved, err := dw.db.GetSession("session-1")
	if err != nil {
		t.Fatalf("Failed to get session from SQLite: %v", err)
	}
	if retrieved.Name != "Test Session" {
		t.Errorf("Expected name 'Test Session', got '%s'", retrieved.Name)
	}

	// Note: YAML write failed, but that's okay - SQLite succeeded
}

// TestDualWriter_SyncYAMLFromSQLite tests rebuilding YAML cache from SQLite
func TestDualWriter_SyncYAMLFromSQLite(t *testing.T) {
	// Setup
	database, cleanup := setupTestDB(t)
	defer cleanup()

	yamlDir := t.TempDir()
	dw := NewDualWriter(database, yamlDir)

	// Create sessions in SQLite only (no YAML)
	m1 := createTestManifest("session-1", "Session 1")
	m2 := createTestManifest("session-2", "Session 2")
	m3 := createTestManifest("session-3", "Session 3")

	if err := dw.db.CreateSession(m1); err != nil {
		t.Fatalf("Failed to create session 1: %v", err)
	}
	if err := dw.db.CreateSession(m2); err != nil {
		t.Fatalf("Failed to create session 2: %v", err)
	}
	if err := dw.db.CreateSession(m3); err != nil {
		t.Fatalf("Failed to create session 3: %v", err)
	}

	// Sync YAML from SQLite
	syncCount, err := dw.SyncYAMLFromSQLite()
	if err != nil {
		t.Fatalf("SyncYAMLFromSQLite failed: %v", err)
	}

	// Verify: All 3 sessions were synced
	if syncCount != 3 {
		t.Errorf("Expected 3 sessions synced, got %d", syncCount)
	}

	// Verify: YAML files exist and have correct data
	for i := 1; i <= 3; i++ {
		sessionID := fmt.Sprintf("session-%d", i)
		yamlPath := filepath.Join(yamlDir, sessionID, "manifest.yaml")

		if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
			t.Errorf("YAML file for %s was not created", sessionID)
			continue
		}

		yamlManifest, err := dw.ReadYAML(yamlPath)
		if err != nil {
			t.Errorf("Failed to read YAML for %s: %v", sessionID, err)
			continue
		}

		expectedName := fmt.Sprintf("Session %d", i)
		if yamlManifest.Name != expectedName {
			t.Errorf("Expected name '%s', got '%s'", expectedName, yamlManifest.Name)
		}
	}
}

// TestDualWriter_SyncYAMLFromSQLite_OverwritesStale tests that sync overwrites stale YAML
func TestDualWriter_SyncYAMLFromSQLite_OverwritesStale(t *testing.T) {
	// Setup
	database, cleanup := setupTestDB(t)
	defer cleanup()

	yamlDir := t.TempDir()
	dw := NewDualWriter(database, yamlDir)

	// Create session in SQLite
	m := createTestManifest("session-1", "SQLite Version - Correct")
	if err := dw.db.CreateSession(m); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create stale YAML file manually
	staleManifest := createTestManifest("session-1", "YAML Version - Stale")
	yamlPath := filepath.Join(yamlDir, "session-1", "manifest.yaml")
	if err := dw.WriteYAML(staleManifest, yamlPath); err != nil {
		t.Fatalf("Failed to write stale YAML: %v", err)
	}

	// Verify stale YAML exists
	yamlManifest, err := dw.ReadYAML(yamlPath)
	if err != nil {
		t.Fatalf("Failed to read YAML: %v", err)
	}
	if yamlManifest.Name != "YAML Version - Stale" {
		t.Fatalf("Stale YAML not set up correctly")
	}

	// Sync YAML from SQLite (should overwrite stale data)
	syncCount, err := dw.SyncYAMLFromSQLite()
	if err != nil {
		t.Fatalf("SyncYAMLFromSQLite failed: %v", err)
	}

	if syncCount != 1 {
		t.Errorf("Expected 1 session synced, got %d", syncCount)
	}

	// Verify: YAML now has SQLite data (stale data was overwritten)
	yamlManifest, err = dw.ReadYAML(yamlPath)
	if err != nil {
		t.Fatalf("Failed to read synced YAML: %v", err)
	}
	if yamlManifest.Name != "SQLite Version - Correct" {
		t.Errorf("Expected YAML to be overwritten with SQLite data, got '%s'", yamlManifest.Name)
	}
}

// TestDualWriter_EmptySessionID tests error handling for empty session IDs
func TestDualWriter_EmptySessionID(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	yamlDir := t.TempDir()
	dw := NewDualWriter(database, yamlDir)

	// Test GetSession with empty ID
	_, err := dw.GetSession("")
	if err == nil {
		t.Error("Expected error for empty session_id in GetSession")
	}

	// Test DeleteSession with empty ID
	err = dw.DeleteSession("")
	if err == nil {
		t.Error("Expected error for empty session_id in DeleteSession")
	}
}

// TestDualWriter_NilManifest tests error handling for nil manifests
func TestDualWriter_NilManifest(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	yamlDir := t.TempDir()
	dw := NewDualWriter(database, yamlDir)

	// Test CreateSession with nil manifest
	err := dw.CreateSession(nil)
	if err == nil {
		t.Error("Expected error for nil manifest in CreateSession")
	}

	// Test UpdateSession with nil manifest
	err = dw.UpdateSession(nil)
	if err == nil {
		t.Error("Expected error for nil manifest in UpdateSession")
	}

	// Test WriteYAML with nil manifest
	err = dw.WriteYAML(nil, "/tmp/test.yaml")
	if err == nil {
		t.Error("Expected error for nil manifest in WriteYAML")
	}
}

// Helper functions

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) (*db.DB, func()) {
	t.Helper()

	// Use in-memory SQLite database
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}

	cleanup := func() {
		if err := database.Close(); err != nil {
			t.Errorf("Failed to close database: %v", err)
		}
	}

	return database, cleanup
}

// createTestManifest creates a test manifest with the given ID and name
func createTestManifest(sessionID, name string) *manifest.Manifest {
	return &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     sessionID,
		Name:          name,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "", // active
		Agent:         "claude",
		Context: manifest.Context{
			Project: "/home/user/test",
			Purpose: "Testing",
			Tags:    []string{"test"},
			Notes:   "Test notes",
		},
		Claude: manifest.Claude{
			UUID: "test-uuid-" + sessionID,
		},
		Tmux: manifest.Tmux{
			SessionName: "test-tmux-" + sessionID,
		},
		EngramMetadata: nil,
	}
}
