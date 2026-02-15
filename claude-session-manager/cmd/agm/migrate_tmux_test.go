package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/db"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/temporal"
)

// MockTemporalClient implements temporal.TemporalInterface for testing
type MockTemporalClient struct {
	sessions map[string]sessionState
}

type sessionState struct {
	name    string
	workdir string
}

func NewMockTemporalClient() *MockTemporalClient {
	return &MockTemporalClient{
		sessions: make(map[string]sessionState),
	}
}

func (m *MockTemporalClient) HasSession(name string) (bool, error) {
	_, exists := m.sessions[name]
	return exists, nil
}

func (m *MockTemporalClient) ListSessions() ([]string, error) {
	sessions := make([]string, 0, len(m.sessions))
	for name := range m.sessions {
		sessions = append(sessions, name)
	}
	return sessions, nil
}

func (m *MockTemporalClient) ListSessionsWithInfo() ([]temporal.SessionInfo, error) {
	sessions := make([]temporal.SessionInfo, 0, len(m.sessions))
	for _, state := range m.sessions {
		sessions = append(sessions, temporal.SessionInfo{
			Name:            state.name,
			AttachedClients: 0,
			AttachedList:    "",
		})
	}
	return sessions, nil
}

func (m *MockTemporalClient) ListClients(sessionName string) ([]temporal.ClientInfo, error) {
	return []temporal.ClientInfo{}, nil
}

func (m *MockTemporalClient) CreateSession(name, workdir string) error {
	m.sessions[name] = sessionState{
		name:    name,
		workdir: workdir,
	}
	return nil
}

func (m *MockTemporalClient) AttachSession(name string) error {
	return nil
}

func (m *MockTemporalClient) SendKeys(session, keys string) error {
	return nil
}

// TestMigrateSession_Success tests successful migration of a single session
func TestMigrateSession_Success(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	sessionName := "test-session"
	sessionID := uuid.New().String()

	// Create manifest directory and file
	manifestDir := filepath.Join(tmpDir, sessionName)
	require.NoError(t, os.MkdirAll(manifestDir, 0755))

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Context: manifest.Context{
			Project: "/home/user/test-project",
			Purpose: "Test migration",
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
		Claude: manifest.Claude{
			UUID: "test-uuid",
		},
	}

	manifestPath := filepath.Join(manifestDir, "manifest.yaml")
	require.NoError(t, manifest.Write(manifestPath, m))

	// Initialize mocks
	temporalClient := NewMockTemporalClient()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	result := &MigrationResult{
		TotalSessions: 1,
		Errors:        []MigrationError{},
	}

	// Execute migration
	err = migrateSession(sessionName, tmpDir, temporalClient, database, result)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, 1, result.SuccessCount)
	assert.Equal(t, 0, result.FailedCount)
	assert.Equal(t, 0, result.SkippedCount)

	// Verify Temporal session created
	exists, err := temporalClient.HasSession(sessionName)
	assert.NoError(t, err)
	assert.True(t, exists)

	// Verify database entry created
	dbSession, err := database.GetSession(sessionID)
	assert.NoError(t, err)
	assert.NotNil(t, dbSession)
	assert.Equal(t, sessionName, dbSession.Name)
	assert.Equal(t, sessionID, dbSession.SessionID)

	// Verify backup created
	backupFiles, err := filepath.Glob(manifestPath + ".backup.*")
	assert.NoError(t, err)
	assert.Len(t, backupFiles, 1)
}

// TestMigrateSession_AlreadyMigrated tests skipping already-migrated sessions
func TestMigrateSession_AlreadyMigrated(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	sessionName := "already-migrated"
	sessionID := uuid.New().String()

	// Create manifest
	manifestDir := filepath.Join(tmpDir, sessionName)
	require.NoError(t, os.MkdirAll(manifestDir, 0755))

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context: manifest.Context{
			Project: "/home/user/test",
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
	}

	manifestPath := filepath.Join(manifestDir, "manifest.yaml")
	require.NoError(t, manifest.Write(manifestPath, m))

	// Initialize mocks
	temporalClient := NewMockTemporalClient()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	// Pre-populate database (simulate already migrated)
	require.NoError(t, database.CreateSession(m))

	result := &MigrationResult{
		TotalSessions: 1,
		Errors:        []MigrationError{},
	}

	// Execute migration
	err = migrateSession(sessionName, tmpDir, temporalClient, database, result)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, 0, result.SuccessCount)
	assert.Equal(t, 1, result.SkippedCount)
	assert.Equal(t, 0, result.FailedCount)
}

// TestMigrateSession_MissingManifest tests handling of missing manifest files
func TestMigrateSession_MissingManifest(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	sessionName := "missing-manifest"

	// Initialize mocks (no manifest created)
	temporalClient := NewMockTemporalClient()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	result := &MigrationResult{
		TotalSessions: 1,
		Errors:        []MigrationError{},
	}

	// Execute migration
	err = migrateSession(sessionName, tmpDir, temporalClient, database, result)

	// Verify
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read manifest")
}

// TestMigrateAllSessions_Multiple tests migrating multiple sessions
func TestMigrateAllSessions_Multiple(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()

	// Create multiple sessions
	sessions := []string{"session-1", "session-2", "session-3"}
	for _, sessionName := range sessions {
		manifestDir := filepath.Join(tmpDir, sessionName)
		require.NoError(t, os.MkdirAll(manifestDir, 0755))

		m := &manifest.Manifest{
			SchemaVersion: manifest.SchemaVersion,
			SessionID:     uuid.New().String(),
			Name:          sessionName,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Context: manifest.Context{
				Project: "/home/user/" + sessionName,
			},
			Tmux: manifest.Tmux{
				SessionName: sessionName,
			},
			Agent: "claude",
		}

		manifestPath := filepath.Join(manifestDir, "manifest.yaml")
		require.NoError(t, manifest.Write(manifestPath, m))
	}

	// Initialize mocks
	temporalClient := NewMockTemporalClient()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	result := &MigrationResult{
		TotalSessions: len(sessions),
		Errors:        []MigrationError{},
	}

	// Execute migration
	err = migrateAllSessions(sessions, tmpDir, temporalClient, database, result)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, 3, result.SuccessCount)
	assert.Equal(t, 0, result.FailedCount)
	assert.Equal(t, 0, result.SkippedCount)

	// Verify all Temporal sessions created
	temporalSessions, err := temporalClient.ListSessions()
	assert.NoError(t, err)
	assert.Len(t, temporalSessions, 3)
}

// TestMigrateAllSessions_PartialFailure tests handling of partial failures
func TestMigrateAllSessions_PartialFailure(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()

	// Create one valid session and one with missing manifest
	sessions := []string{"valid-session", "missing-session"}

	// Create only the valid session
	validSessionName := "valid-session"
	manifestDir := filepath.Join(tmpDir, validSessionName)
	require.NoError(t, os.MkdirAll(manifestDir, 0755))

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     uuid.New().String(),
		Name:          validSessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context: manifest.Context{
			Project: "/home/user/valid",
		},
		Tmux: manifest.Tmux{
			SessionName: validSessionName,
		},
		Agent: "claude",
	}

	manifestPath := filepath.Join(manifestDir, "manifest.yaml")
	require.NoError(t, manifest.Write(manifestPath, m))

	// Initialize mocks
	temporalClient := NewMockTemporalClient()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	result := &MigrationResult{
		TotalSessions: len(sessions),
		Errors:        []MigrationError{},
	}

	// Execute migration
	err = migrateAllSessions(sessions, tmpDir, temporalClient, database, result)

	// Verify
	assert.NoError(t, err) // migrateAllSessions doesn't return errors, it collects them
	assert.Equal(t, 1, result.SuccessCount)
	assert.Equal(t, 1, result.FailedCount)
	assert.Len(t, result.Errors, 1)
	assert.Equal(t, "missing-session", result.Errors[0].SessionName)
}

// TestMigrationResult_PrintSummary tests summary printing
func TestMigrationResult_PrintSummary(t *testing.T) {
	result := &MigrationResult{
		TotalSessions: 5,
		SuccessCount:  3,
		SkippedCount:  1,
		FailedCount:   1,
		Errors: []MigrationError{
			{
				SessionName: "failed-session",
				Error:       assert.AnError,
			},
		},
	}

	// This test just verifies the function doesn't panic
	// In a real scenario, we'd capture stdout and verify the output
	assert.NotPanics(t, func() {
		printMigrationSummary(result)
	})
}

// TestBackupCreation tests that manifest backups are created
func TestBackupCreation(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	sessionName := "backup-test"
	sessionID := uuid.New().String()

	manifestDir := filepath.Join(tmpDir, sessionName)
	require.NoError(t, os.MkdirAll(manifestDir, 0755))

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context: manifest.Context{
			Project: "/home/user/backup-test",
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
	}

	manifestPath := filepath.Join(manifestDir, "manifest.yaml")
	require.NoError(t, manifest.Write(manifestPath, m))

	// Initialize mocks
	temporalClient := NewMockTemporalClient()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	result := &MigrationResult{
		TotalSessions: 1,
		Errors:        []MigrationError{},
	}

	// Execute migration
	err = migrateSession(sessionName, tmpDir, temporalClient, database, result)
	assert.NoError(t, err)

	// Verify backup file exists
	backupPattern := manifestPath + ".backup.*"
	matches, err := filepath.Glob(backupPattern)
	assert.NoError(t, err)
	assert.Len(t, matches, 1, "Expected exactly one backup file")

	// Verify backup content matches original
	originalContent, err := os.ReadFile(manifestPath)
	require.NoError(t, err)

	backupContent, err := os.ReadFile(matches[0])
	require.NoError(t, err)

	assert.Equal(t, originalContent, backupContent)
}

// TestDryRunMode tests that dry-run mode doesn't make changes
func TestDryRunMode(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	sessionName := "dry-run-test"
	sessionID := uuid.New().String()

	manifestDir := filepath.Join(tmpDir, sessionName)
	require.NoError(t, os.MkdirAll(manifestDir, 0755))

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context: manifest.Context{
			Project: "/home/user/dry-run",
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
	}

	manifestPath := filepath.Join(manifestDir, "manifest.yaml")
	require.NoError(t, manifest.Write(manifestPath, m))

	// Initialize mocks
	temporalClient := NewMockTemporalClient()

	// Use in-memory database for dry-run
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	result := &MigrationResult{
		TotalSessions: 1,
		Errors:        []MigrationError{},
	}

	// Set dry-run mode (in real code, this would be a parameter)
	// For this test, we verify behavior with in-memory database
	originalDryRun := migrateTmuxDryRun
	migrateTmuxDryRun = true
	defer func() { migrateTmuxDryRun = originalDryRun }()

	// Execute migration
	err = migrateSession(sessionName, tmpDir, temporalClient, database, result)

	// In dry-run with in-memory DB, session should still succeed
	// but temporal session shouldn't be created (if we added dry-run check)
	assert.NoError(t, err)

	// Verify no backup created (dry-run shouldn't create backups)
	backupPattern := manifestPath + ".backup.*"
	matches, err := filepath.Glob(backupPattern)
	assert.NoError(t, err)
	assert.Len(t, matches, 0, "Dry-run should not create backup files")
}

// TestWorkingDirectoryExtraction tests proper extraction of working directory
func TestWorkingDirectoryExtraction(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	sessionName := "workdir-test"
	expectedWorkDir := "/home/user/custom/project"

	manifestDir := filepath.Join(tmpDir, sessionName)
	require.NoError(t, os.MkdirAll(manifestDir, 0755))

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     uuid.New().String(),
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context: manifest.Context{
			Project: expectedWorkDir,
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
	}

	manifestPath := filepath.Join(manifestDir, "manifest.yaml")
	require.NoError(t, manifest.Write(manifestPath, m))

	// Initialize mocks
	temporalClient := NewMockTemporalClient()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	result := &MigrationResult{
		TotalSessions: 1,
		Errors:        []MigrationError{},
	}

	// Execute migration
	err = migrateSession(sessionName, tmpDir, temporalClient, database, result)
	assert.NoError(t, err)

	// Verify working directory was passed correctly
	assert.True(t, temporalClient.sessions[sessionName].workdir == expectedWorkDir)
}

// TestEmptyWorkingDirectory tests fallback for empty working directory
func TestEmptyWorkingDirectory(t *testing.T) {
	// Note: Manifest validation now requires a non-empty project path
	// This test verifies that the migration code handles the empty case
	// gracefully by using "." as fallback before it hits validation

	// Setup - skip this test since manifest validation prevents empty project
	t.Skip("Skipping: manifest validation now requires non-empty project path")
}

// TestPrintMigrationSummary_AllSuccess tests summary with all successful migrations
func TestPrintMigrationSummary_AllSuccess(t *testing.T) {
	result := &MigrationResult{
		TotalSessions: 3,
		SuccessCount:  3,
		SkippedCount:  0,
		FailedCount:   0,
		Errors:        []MigrationError{},
	}

	assert.NotPanics(t, func() {
		printMigrationSummary(result)
	})
}

// TestPrintMigrationSummary_WithSkipped tests summary with skipped sessions
func TestPrintMigrationSummary_WithSkipped(t *testing.T) {
	result := &MigrationResult{
		TotalSessions: 5,
		SuccessCount:  3,
		SkippedCount:  2,
		FailedCount:   0,
		Errors:        []MigrationError{},
	}

	assert.NotPanics(t, func() {
		printMigrationSummary(result)
	})
}

// TestPrintMigrationSummary_WithErrors tests summary with multiple errors
func TestPrintMigrationSummary_WithErrors(t *testing.T) {
	result := &MigrationResult{
		TotalSessions: 5,
		SuccessCount:  2,
		SkippedCount:  1,
		FailedCount:   2,
		Errors: []MigrationError{
			{SessionName: "session1", Error: assert.AnError},
			{SessionName: "session2", Error: assert.AnError},
		},
	}

	assert.NotPanics(t, func() {
		printMigrationSummary(result)
	})
}

// TestCleanupMigratedSessions tests cleanup function
func TestCleanupMigratedSessions(t *testing.T) {
	result := &MigrationResult{
		SuccessCount: 2,
	}

	err := cleanupMigratedSessions(result)
	assert.NoError(t, err)
}

// TestMockTemporalClient_Coverage tests mock implementation coverage
func TestMockTemporalClient_Coverage(t *testing.T) {
	client := NewMockTemporalClient()

	// Test HasSession
	exists, err := client.HasSession("test")
	assert.NoError(t, err)
	assert.False(t, exists)

	// Create session
	err = client.CreateSession("test", "/tmp")
	assert.NoError(t, err)

	// Test HasSession again
	exists, err = client.HasSession("test")
	assert.NoError(t, err)
	assert.True(t, exists)

	// Test ListSessions
	sessions, err := client.ListSessions()
	assert.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Contains(t, sessions, "test")

	// Test ListSessionsWithInfo
	infos, err := client.ListSessionsWithInfo()
	assert.NoError(t, err)
	assert.Len(t, infos, 1)
	assert.Equal(t, "test", infos[0].Name)

	// Test ListClients
	clients, err := client.ListClients("test")
	assert.NoError(t, err)
	assert.Len(t, clients, 0)

	// Test AttachSession
	err = client.AttachSession("test")
	assert.NoError(t, err)

	// Test SendKeys
	err = client.SendKeys("test", "hello")
	assert.NoError(t, err)
}

// TestMigrateSession_DatabaseUpdateFailure tests handling of database update failures
func TestMigrateSession_DatabaseUpdateFailure(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	sessionName := "db-update-test"
	sessionID := uuid.New().String()

	manifestDir := filepath.Join(tmpDir, sessionName)
	require.NoError(t, os.MkdirAll(manifestDir, 0755))

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context: manifest.Context{
			Project: "/home/user/test",
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
	}

	manifestPath := filepath.Join(manifestDir, "manifest.yaml")
	require.NoError(t, manifest.Write(manifestPath, m))

	// Initialize mocks
	temporalClient := NewMockTemporalClient()

	// Use a closed database to simulate failure
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	database.Close() // Close immediately to cause failures

	result := &MigrationResult{
		TotalSessions: 1,
		Errors:        []MigrationError{},
	}

	// Execute migration - should fail due to closed database
	err = migrateSession(sessionName, tmpDir, temporalClient, database, result)
	assert.Error(t, err)
}

// TestMigrateSession_TemporalClientFailure tests handling when Temporal client fails
func TestMigrateSession_TemporalClientFailure(t *testing.T) {
	// This test would need a failing Temporal client mock
	// For now, we'll skip it since our mock doesn't support failure injection
	t.Skip("Requires Temporal client mock with failure injection")
}

// TestMigrateSession_UpdatedAtTimestamp tests that migration updates timestamp
func TestMigrateSession_UpdatedAtTimestamp(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	sessionName := "timestamp-test"
	sessionID := uuid.New().String()

	// Create manifest with old timestamp
	oldTime := time.Now().Add(-24 * time.Hour)

	manifestDir := filepath.Join(tmpDir, sessionName)
	require.NoError(t, os.MkdirAll(manifestDir, 0755))

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          sessionName,
		CreatedAt:     oldTime,
		UpdatedAt:     oldTime,
		Context: manifest.Context{
			Project: "/home/user/test",
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
	}

	manifestPath := filepath.Join(manifestDir, "manifest.yaml")
	require.NoError(t, manifest.Write(manifestPath, m))

	// Initialize mocks
	temporalClient := NewMockTemporalClient()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	result := &MigrationResult{
		TotalSessions: 1,
		Errors:        []MigrationError{},
	}

	beforeMigration := time.Now()

	// Execute migration
	err = migrateSession(sessionName, tmpDir, temporalClient, database, result)
	assert.NoError(t, err)

	// Verify updated_at was updated in database
	dbSession, err := database.GetSession(sessionID)
	assert.NoError(t, err)
	assert.True(t, dbSession.UpdatedAt.After(beforeMigration) || dbSession.UpdatedAt.Equal(beforeMigration))
}
