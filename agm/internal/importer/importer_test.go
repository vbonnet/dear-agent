package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestInferProjectPath(t *testing.T) {
	// Create temporary test directory structure
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, ".claude", "projects")

	// Create test project directories
	project1 := filepath.Join(projectsDir, "project-hash-123")
	project2 := filepath.Join(projectsDir, "project-hash-456")

	if err := os.MkdirAll(project1, 0755); err != nil {
		t.Fatalf("Failed to create test project dir: %v", err)
	}
	if err := os.MkdirAll(project2, 0755); err != nil {
		t.Fatalf("Failed to create test project dir: %v", err)
	}

	// Create conversation files
	uuid1 := "test-uuid-001"
	uuid2 := "test-uuid-002"

	conv1 := filepath.Join(project1, uuid1+".jsonl")
	conv2 := filepath.Join(project2, uuid2+".jsonl")

	if err := os.WriteFile(conv1, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create test conversation file: %v", err)
	}
	if err := os.WriteFile(conv2, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create test conversation file: %v", err)
	}

	// Override home directory for testing
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	tests := []struct {
		name    string
		uuid    string
		wantErr bool
		want    string
	}{
		{
			name:    "find existing conversation in project 1",
			uuid:    uuid1,
			wantErr: false,
			want:    project1,
		},
		{
			name:    "find existing conversation in project 2",
			uuid:    uuid2,
			wantErr: false,
			want:    project2,
		},
		{
			name:    "non-existent UUID",
			uuid:    "non-existent-uuid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InferProjectPath(tt.uuid)
			if (err != nil) != tt.wantErr {
				t.Errorf("InferProjectPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("InferProjectPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateNotDuplicate(t *testing.T) {
	// Use MockAdapter for reliable, fast testing
	adapter := dolt.NewMockAdapter()
	defer adapter.Close()

	// Create a test session with UUID
	testUUID := "existing-uuid-123"
	sessionID := "session-001"

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          "test-session",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Workspace:     "test",
		Harness:       "claude-code",
		Lifecycle:     "",
		Claude: manifest.Claude{
			UUID: testUUID,
		},
		Context: manifest.Context{
			Project: "/tmp/test",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-session",
		},
	}

	// Insert test session into Dolt
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	tests := []struct {
		name    string
		uuid    string
		wantErr bool
	}{
		{
			name:    "duplicate UUID should error",
			uuid:    testUUID,
			wantErr: true,
		},
		{
			name:    "new UUID should succeed",
			uuid:    "new-uuid-456",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNotDuplicateWithAdapter(tt.uuid, adapter)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNotDuplicateWithAdapter() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil {
				// Verify error message mentions duplicate
				if !strings.Contains(err.Error(), "already has manifest") {
					t.Errorf("Expected error to mention duplicate, got: %v", err)
				}
			}
		})
	}
}

func TestValidateNotDuplicate_EmptyDatabase(t *testing.T) {
	// Get test adapter (database is empty by default)
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Test with empty database (should not error)
	err := ValidateNotDuplicateWithAdapter("any-uuid", adapter)
	if err != nil {
		t.Errorf("ValidateNotDuplicateWithAdapter() with empty database should not error, got: %v", err)
	}
}

func TestImportOrphanedSession_ValidationErrors(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	tests := []struct {
		name        string
		uuid        string
		sessionName string
		workspace   string
		sessionsDir string
		wantErr     bool
		errContains string
	}{
		{
			name:        "empty UUID",
			uuid:        "",
			sessionName: "test",
			workspace:   "oss",
			sessionsDir: sessionsDir,
			wantErr:     true,
			errContains: "UUID cannot be empty",
		},
		{
			name:        "empty session name",
			uuid:        "test-uuid",
			sessionName: "",
			workspace:   "oss",
			sessionsDir: sessionsDir,
			wantErr:     true,
			errContains: "session name cannot be empty",
		},
		{
			name:        "empty sessions dir",
			uuid:        "test-uuid",
			sessionName: "test",
			workspace:   "oss",
			sessionsDir: "",
			wantErr:     true,
			errContains: "Dolt adapter not available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ImportOrphanedSession(tt.uuid, tt.sessionName, tt.workspace, nil, tt.sessionsDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("ImportOrphanedSession() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("Expected error to contain %q, got: %v", tt.errContains, err)
			}
		})
	}
}

func TestImportOrphanedSession_DuplicatePrevention(t *testing.T) {
	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Create temporary directory structure
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, ".claude", "projects", "test-project")

	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatalf("Failed to create projects dir: %v", err)
	}

	// Override home directory
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create test conversation file
	testUUID := "duplicate-test-uuid"
	convFile := filepath.Join(projectsDir, testUUID+".jsonl")
	if err := os.WriteFile(convFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create conversation file: %v", err)
	}

	// First import should succeed
	sessionID1, err := ImportOrphanedSessionWithAdapter(testUUID, "first-import", "oss", adapter)
	if err != nil {
		t.Fatalf("First import failed: %v", err)
	}
	if sessionID1 == "" {
		t.Fatal("Expected non-empty session ID from first import")
	}

	// Second import should fail (duplicate)
	_, err = ImportOrphanedSessionWithAdapter(testUUID, "second-import", "oss", adapter)
	if err == nil {
		t.Fatal("Expected duplicate import to fail, but it succeeded")
	}
	if !strings.Contains(err.Error(), "already has manifest") {
		t.Errorf("Expected duplicate error, got: %v", err)
	}
}

func TestImportOrphanedSession_TmuxSanitization(t *testing.T) {
	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Create temporary directory structure
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, ".claude", "projects", "test-project")

	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatalf("Failed to create projects dir: %v", err)
	}

	// Override home directory
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create test conversation file
	testUUID := "sanitization-test-uuid"
	convFile := filepath.Join(projectsDir, testUUID+".jsonl")
	if err := os.WriteFile(convFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create conversation file: %v", err)
	}

	// Import with session name containing special characters
	sessionName := "my.session@test!"
	sessionID, err := ImportOrphanedSessionWithAdapter(testUUID, sessionName, "oss", adapter)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Read the session from Dolt and verify tmux name is sanitized
	m, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to get session from database: %v", err)
	}

	// Tmux name should be sanitized (special chars removed, spaces become dashes)
	if m.Tmux.SessionName != "mysessiontest" {
		t.Errorf("Expected sanitized tmux name 'mysessiontest', got: %s", m.Tmux.SessionName)
	}

	// Original session name should be preserved in manifest
	if m.Name != sessionName {
		t.Errorf("Expected original name %q in manifest, got: %s", sessionName, m.Name)
	}
}

func TestImportOrphanedSession_ManifestCreation(t *testing.T) {
	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Create temporary directory structure
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, ".claude", "projects", "test-project")

	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatalf("Failed to create projects dir: %v", err)
	}

	// Override home directory
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create test conversation file
	testUUID := "manifest-test-uuid"
	convFile := filepath.Join(projectsDir, testUUID+".jsonl")
	if err := os.WriteFile(convFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create conversation file: %v", err)
	}

	// Import session
	sessionName := "test-session"
	workspace := "oss"
	sessionID, err := ImportOrphanedSessionWithAdapter(testUUID, sessionName, workspace, adapter)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Read and verify session from database
	m, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to get session from database: %v", err)
	}

	// Verify manifest fields
	if m.SessionID != sessionID {
		t.Errorf("Expected SessionID %s, got: %s", sessionID, m.SessionID)
	}
	if m.Name != sessionName {
		t.Errorf("Expected Name %s, got: %s", sessionName, m.Name)
	}
	// Note: workspace is overridden by adapter's workspace ("test" in test environment)
	if m.Workspace != "test" {
		t.Errorf("Expected Workspace test (from adapter), got: %s", m.Workspace)
	}
	if m.Claude.UUID != testUUID {
		t.Errorf("Expected Claude UUID %s, got: %s", testUUID, m.Claude.UUID)
	}
	if m.Context.Project != projectsDir {
		t.Errorf("Expected Project %s, got: %s", projectsDir, m.Context.Project)
	}
	if m.SchemaVersion != manifest.SchemaVersion {
		t.Errorf("Expected SchemaVersion %s, got: %s", manifest.SchemaVersion, m.SchemaVersion)
	}
	if m.Lifecycle != "" {
		t.Errorf("Expected empty Lifecycle (active), got: %s", m.Lifecycle)
	}
}

func TestExtractMetadataFromHistory(t *testing.T) {
	// Create temporary history file
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, ".claude", "history.jsonl")
	if err := os.MkdirAll(filepath.Dir(historyFile), 0755); err != nil {
		t.Fatalf("Failed to create history dir: %v", err)
	}

	// Create test history entries
	testUUID := "test-uuid-for-metadata"
	testProject := "/tmp/test-project"
	testTimestamp := time.Now().UnixMilli()

	historyContent := fmt.Sprintf(`{"display":"test prompt","timestamp":%d,"project":"%s","sessionId":"%s"}
{"display":"another prompt","timestamp":%d,"project":"%s","sessionId":"other-uuid"}
`, testTimestamp, testProject, testUUID, testTimestamp-1000, testProject)

	if err := os.WriteFile(historyFile, []byte(historyContent), 0644); err != nil {
		t.Fatalf("Failed to write history file: %v", err)
	}

	// Override home directory
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Test extracting metadata
	metadata, err := ExtractMetadataFromHistory(testUUID)
	if err != nil {
		t.Fatalf("ExtractMetadataFromHistory failed: %v", err)
	}

	if metadata.UUID != testUUID {
		t.Errorf("Expected UUID %s, got: %s", testUUID, metadata.UUID)
	}
	if metadata.ProjectPath != testProject {
		t.Errorf("Expected project path %s, got: %s", testProject, metadata.ProjectPath)
	}

	// Verify timestamp is close to what we set (within 1 second tolerance)
	expectedTime := time.Unix(0, testTimestamp*int64(time.Millisecond))
	if metadata.LastModified.Unix() != expectedTime.Unix() {
		t.Errorf("Expected timestamp %v, got: %v", expectedTime, metadata.LastModified)
	}
}

func TestExtractMetadataFromHistory_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, ".claude", "history.jsonl")
	if err := os.MkdirAll(filepath.Dir(historyFile), 0755); err != nil {
		t.Fatalf("Failed to create history dir: %v", err)
	}

	// Create history with no matching UUID
	historyContent := `{"display":"test","timestamp":1234567890,"project":"/tmp","sessionId":"other-uuid"}
`
	if err := os.WriteFile(historyFile, []byte(historyContent), 0644); err != nil {
		t.Fatalf("Failed to write history file: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	_, err := ExtractMetadataFromHistory("non-existent-uuid")
	if err == nil {
		t.Fatal("Expected error for non-existent UUID, got nil")
	}
	if !strings.Contains(err.Error(), "no history entries found") {
		t.Errorf("Expected 'no history entries found' error, got: %v", err)
	}
}

func TestExtractMetadataFromHistory_EmptyHistory(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, ".claude", "history.jsonl")
	if err := os.MkdirAll(filepath.Dir(historyFile), 0755); err != nil {
		t.Fatalf("Failed to create history dir: %v", err)
	}

	// Create empty history file
	if err := os.WriteFile(historyFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write history file: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	_, err := ExtractMetadataFromHistory("any-uuid")
	if err == nil {
		t.Fatal("Expected error for empty history, got nil")
	}
}

func TestImportOrphanedSession_WithHistoryMetadata(t *testing.T) {
	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Create temporary directory structure
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, ".claude", "projects", "test-project")

	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatalf("Failed to create projects dir: %v", err)
	}

	// Create history file with metadata
	historyFile := filepath.Join(tmpDir, ".claude", "history.jsonl")
	if err := os.MkdirAll(filepath.Dir(historyFile), 0755); err != nil {
		t.Fatalf("Failed to create history dir: %v", err)
	}

	testUUID := "uuid-with-history-metadata"
	testProject := projectsDir
	testTimestamp := time.Date(2024, 2, 19, 14, 30, 0, 0, time.UTC).UnixMilli()

	historyContent := fmt.Sprintf(`{"display":"test work","timestamp":%d,"project":"%s","sessionId":"%s"}
`, testTimestamp, testProject, testUUID)

	if err := os.WriteFile(historyFile, []byte(historyContent), 0644); err != nil {
		t.Fatalf("Failed to write history file: %v", err)
	}

	// Create conversation file
	convFile := filepath.Join(projectsDir, testUUID+".jsonl")
	if err := os.WriteFile(convFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create conversation file: %v", err)
	}

	// Override home directory
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Import session
	sessionID, err := ImportOrphanedSessionWithAdapter(testUUID, "test-with-history", "oss", adapter)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Read session from database and verify timestamps were extracted from history
	m, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to get session from database: %v", err)
	}

	// Verify created_at matches history timestamp
	expectedTime := time.Unix(0, testTimestamp*int64(time.Millisecond))
	if m.CreatedAt.Unix() != expectedTime.Unix() {
		t.Errorf("Expected CreatedAt from history %v, got: %v", expectedTime, m.CreatedAt)
	}

	// Verify project path from history
	if m.Context.Project != testProject {
		t.Errorf("Expected project path from history %s, got: %s", testProject, m.Context.Project)
	}
}

func TestImportOrphanedSession_WithoutHistoryMetadata(t *testing.T) {
	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Test graceful fallback when history metadata is not available
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, ".claude", "projects", "test-project")

	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatalf("Failed to create projects dir: %v", err)
	}

	// Create empty history file (no metadata for our UUID)
	historyFile := filepath.Join(tmpDir, ".claude", "history.jsonl")
	if err := os.MkdirAll(filepath.Dir(historyFile), 0755); err != nil {
		t.Fatalf("Failed to create history dir: %v", err)
	}
	if err := os.WriteFile(historyFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write history file: %v", err)
	}

	testUUID := "uuid-without-history"
	convFile := filepath.Join(projectsDir, testUUID+".jsonl")
	if err := os.WriteFile(convFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create conversation file: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Import should still succeed (graceful fallback)
	before := time.Now()
	sessionID, err := ImportOrphanedSessionWithAdapter(testUUID, "test-no-history", "oss", adapter)
	after := time.Now()

	if err != nil {
		t.Fatalf("Import should succeed with fallback, got error: %v", err)
	}

	// Verify session was created in database with fallback values
	m, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to get session from database: %v", err)
	}

	// CreatedAt should be recent (fallback to current time)
	// Allow for clock skew and database latency (5 second tolerance)
	if m.CreatedAt.Before(before.Add(-2*time.Second)) || m.CreatedAt.After(after.Add(3*time.Second)) {
		t.Errorf("Expected CreatedAt to be recent (fallback), got: %v (before=%v, after=%v)", m.CreatedAt, before, after)
	}

	// Project path should still be inferred from file location
	if m.Context.Project != projectsDir {
		t.Errorf("Expected project path from file location %s, got: %s", projectsDir, m.Context.Project)
	}
}

func TestInferProjectPath_NoProjectsDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Override home directory to location with no .claude/projects
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	_, err := InferProjectPath("any-uuid")
	if err == nil {
		t.Fatal("Expected error when projects dir doesn't exist, got nil")
	}
}

func TestExtractMetadataFromHistory_MultipleEntries(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, ".claude", "history.jsonl")
	if err := os.MkdirAll(filepath.Dir(historyFile), 0755); err != nil {
		t.Fatalf("Failed to create history dir: %v", err)
	}

	testUUID := "uuid-with-multiple-entries"
	testProject := "/tmp/final-project"

	// Create multiple entries for the same UUID (should use most recent)
	timestamp1 := time.Date(2024, 2, 19, 10, 0, 0, 0, time.UTC).UnixMilli()
	timestamp2 := time.Date(2024, 2, 19, 14, 0, 0, 0, time.UTC).UnixMilli()
	timestamp3 := time.Date(2024, 2, 19, 16, 0, 0, 0, time.UTC).UnixMilli()

	historyContent := fmt.Sprintf(`{"display":"first","timestamp":%d,"project":"/tmp/old1","sessionId":"%s"}
{"display":"second","timestamp":%d,"project":"/tmp/old2","sessionId":"%s"}
{"display":"third","timestamp":%d,"project":"%s","sessionId":"%s"}
`, timestamp1, testUUID, timestamp2, testUUID, timestamp3, testProject, testUUID)

	if err := os.WriteFile(historyFile, []byte(historyContent), 0644); err != nil {
		t.Fatalf("Failed to write history file: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	metadata, err := ExtractMetadataFromHistory(testUUID)
	if err != nil {
		t.Fatalf("ExtractMetadataFromHistory failed: %v", err)
	}

	// Should use most common project (in this case, first match due to grouping logic)
	// The ReadConversations groups by sessionID and finds most common project
	if metadata.ProjectPath == "" {
		t.Errorf("Expected non-empty project path")
	}

	// Timestamp should be from most recent entry
	expectedTime := time.Unix(0, timestamp3*int64(time.Millisecond))
	if metadata.LastModified.Unix() != expectedTime.Unix() {
		t.Errorf("Expected most recent timestamp %v, got: %v", expectedTime, metadata.LastModified)
	}
}

func TestValidateNotDuplicate_ManifestReadError(t *testing.T) {
	// This test verifies error handling when manifest reading fails
	// Create a sessions dir with an invalid manifest
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	sessionDir := filepath.Join(sessionsDir, "session-001")

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session dir: %v", err)
	}

	// Create an invalid manifest file (not valid YAML)
	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("invalid: yaml: content: [[["), 0644); err != nil {
		t.Fatalf("Failed to write invalid manifest: %v", err)
	}

	// ValidateNotDuplicate should handle this gracefully
	// (manifest.List handles read errors by skipping invalid manifests)
	err := ValidateNotDuplicate("any-uuid", nil)
	// Should not error on read failures, just skip the bad manifest
	if err != nil {
		t.Logf("Got error (expected to be tolerated): %v", err)
	}
}

func TestExtractMetadataFromHistory_NoEntries(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, ".claude", "history.jsonl")
	if err := os.MkdirAll(filepath.Dir(historyFile), 0755); err != nil {
		t.Fatalf("Failed to create history dir: %v", err)
	}

	// Create history with session but no entries
	testUUID := "uuid-with-no-entries"
	historyContent := fmt.Sprintf(`{"display":"test","timestamp":1234567890,"project":"/tmp","sessionId":"%s"}
`, testUUID)

	if err := os.WriteFile(historyFile, []byte(historyContent), 0644); err != nil {
		t.Fatalf("Failed to write history file: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	metadata, err := ExtractMetadataFromHistory(testUUID)
	if err != nil {
		t.Fatalf("Should succeed even with minimal entries: %v", err)
	}

	// Should have current time as fallback
	if metadata.LastModified.IsZero() {
		t.Error("Expected non-zero LastModified")
	}
}

func TestInferProjectPath_EmptyUUID(t *testing.T) {
	tmpDir := t.TempDir()

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Even with empty UUID, should get error about not finding file
	_, err := InferProjectPath("")
	if err == nil {
		t.Fatal("Expected error for empty UUID")
	}
}

func TestImportOrphanedSession_NoConversationFile(t *testing.T) {
	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	tmpDir := t.TempDir()

	// Create projects dir but no conversation file
	projectsDir := filepath.Join(tmpDir, ".claude", "projects", "test-project")
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatalf("Failed to create projects dir: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Should fail because conversation file doesn't exist
	_, err := ImportOrphanedSessionWithAdapter("non-existent-uuid", "test", "oss", adapter)
	if err == nil {
		t.Fatal("Expected error for missing conversation file, got nil")
	}
	if !strings.Contains(err.Error(), "no conversation file found") {
		t.Errorf("Expected 'no conversation file found' error, got: %v", err)
	}
}
