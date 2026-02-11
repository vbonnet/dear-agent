package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/config"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// setupArchiveTest creates a temporary test environment with a sessions directory
func setupArchiveTest(t *testing.T) (tmpDir string, sessionsDir string, cleanup func()) {
	t.Helper()

	tmpDir = t.TempDir()
	sessionsDir = filepath.Join(tmpDir, "sessions")

	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatalf("Failed to create sessions directory: %v", err)
	}

	// Save and restore global cfg
	oldCfg := cfg
	cfg = &config.Config{
		SessionsDir: sessionsDir,
	}

	cleanup = func() {
		cfg = oldCfg
	}

	return tmpDir, sessionsDir, cleanup
}

// createArchiveTestSession creates a test session with manifest
// testingTB is an interface that both *testing.T and *testing.B implement
type testingTB interface {
	Helper()
	Fatalf(format string, args ...interface{})
}

func createArchiveTestSession(t testingTB, sessionsDir, sessionID, name, tmuxName, lifecycle string) string {
	t.Helper()

	sessionDir := filepath.Join(sessionsDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     sessionID,
		Name:          name,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     lifecycle,
		Context: manifest.Context{
			Project: "/tmp/test-project",
		},
		Tmux: manifest.Tmux{
			SessionName: tmuxName,
		},
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := manifest.Write(manifestPath, m); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	return manifestPath
}

// TestArchiveSession_Success tests successful archive of an active session
func TestArchiveSession_Success(t *testing.T) {
	_, sessionsDir, cleanup := setupArchiveTest(t)
	defer cleanup()

	// Create a test session
	sessionID := "test-session-123"
	createArchiveTestSession(t, sessionsDir, sessionID, "my-session", "claude-my-session", "")

	// Set force flag to skip confirmation and tmux check
	oldForce := forceArchive
	forceArchive = true
	defer func() { forceArchive = oldForce }()

	// Run archive command
	err := archiveSession(nil, []string{"my-session"})
	if err != nil {
		t.Fatalf("archiveSession failed: %v", err)
	}

	// Verify session remains in original directory (in-place archive)
	originalDir := filepath.Join(sessionsDir, sessionID)
	if _, err := os.Stat(originalDir); os.IsNotExist(err) {
		t.Errorf("Session directory should remain in place: %s", originalDir)
	}

	// Verify manifest has archived lifecycle
	manifestPath := filepath.Join(originalDir, "manifest.yaml")
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read archived manifest: %v", err)
	}
	if m.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("Expected lifecycle 'archived', got '%s'", m.Lifecycle)
	}
}

// TestArchiveSession_WithForceFlag tests archive with force flag
func TestArchiveSession_WithForceFlag(t *testing.T) {
	_, sessionsDir, cleanup := setupArchiveTest(t)
	defer cleanup()

	sessionID := "force-test-session"
	createArchiveTestSession(t, sessionsDir, sessionID, "force-session", "claude-force", "")

	// Set force flag
	oldForce := forceArchive
	forceArchive = true
	defer func() { forceArchive = oldForce }()

	// Run archive - should succeed without prompting
	err := archiveSession(nil, []string{"force-session"})
	if err != nil {
		t.Fatalf("archiveSession with force flag failed: %v", err)
	}

	// Verify session remains in original location (in-place archive)
	originalDir := filepath.Join(sessionsDir, sessionID)
	if _, err := os.Stat(originalDir); os.IsNotExist(err) {
		t.Errorf("Session directory should remain in place: %s", originalDir)
	}

	// Verify manifest has archived lifecycle
	manifestPath := filepath.Join(originalDir, "manifest.yaml")
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read archived manifest: %v", err)
	}
	if m.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("Expected lifecycle 'archived', got '%s'", m.Lifecycle)
	}
}

// TestArchiveSession_SessionNotFound tests error when session doesn't exist
func TestArchiveSession_SessionNotFound(t *testing.T) {
	_, _, cleanup := setupArchiveTest(t)
	defer cleanup()

	oldForce := forceArchive
	forceArchive = true
	defer func() { forceArchive = oldForce }()

	// Try to archive non-existent session
	err := archiveSession(nil, []string{"nonexistent-session"})
	if err == nil {
		t.Fatal("Expected error for non-existent session, got nil")
	}
}

// TestArchiveSession_AlreadyArchived tests archiving an already archived session (idempotent)
func TestArchiveSession_AlreadyArchived(t *testing.T) {
	_, sessionsDir, cleanup := setupArchiveTest(t)
	defer cleanup()

	sessionID := "already-archived-session"
	createArchiveTestSession(t, sessionsDir, sessionID, "archived-session",
		"claude-archived", manifest.LifecycleArchived)

	oldForce := forceArchive
	forceArchive = true
	defer func() { forceArchive = oldForce }()

	// Run archive on already archived session using session ID (not name)
	// This should succeed and show a warning that it's already archived
	err := archiveSession(nil, []string{sessionID})
	if err != nil {
		t.Fatalf("archiveSession on already archived session failed: %v", err)
	}

	// Verify session still exists in original location (not moved again)
	originalPath := filepath.Join(sessionsDir, sessionID, "manifest.yaml")
	if _, err := os.Stat(originalPath); os.IsNotExist(err) {
		t.Errorf("Already archived session was moved again")
	}
}

// TestArchiveSession_InvalidIdentifier tests error handling for invalid identifiers
func TestArchiveSession_InvalidIdentifier(t *testing.T) {
	_, _, cleanup := setupArchiveTest(t)
	defer cleanup()

	oldForce := forceArchive
	forceArchive = true
	defer func() { forceArchive = oldForce }()

	testCases := []struct {
		name       string
		identifier string
	}{
		{"path traversal", "../../../etc/passwd"},
		{"with forward slash", "session/with/slashes"},
		{"with backslash", "session\\with\\backslash"},
		{"with dots", "session..name"},
		{"hidden file", ".hidden-session"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := archiveSession(nil, []string{tc.identifier})
			if err == nil {
				t.Errorf("Expected error for invalid identifier '%s', got nil", tc.identifier)
			}
		})
	}
}

// TestArchiveSession_ManifestReadError tests error handling when manifest cannot be read
func TestArchiveSession_ManifestReadError(t *testing.T) {
	_, sessionsDir, cleanup := setupArchiveTest(t)
	defer cleanup()

	// Create session directory with unreadable manifest
	sessionID := "unreadable-session"
	sessionDir := filepath.Join(sessionsDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	// Create manifest with invalid permissions
	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("invalid yaml content: ["), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	oldForce := forceArchive
	forceArchive = true
	defer func() { forceArchive = oldForce }()

	// Try to archive - should fail to read manifest
	err := archiveSession(nil, []string{sessionID})
	if err == nil {
		t.Fatal("Expected error when reading invalid manifest, got nil")
	}
}

// TestArchiveSession_ManifestWriteError tests error handling when manifest cannot be written
func TestArchiveSession_ManifestWriteError(t *testing.T) {
	_, sessionsDir, cleanup := setupArchiveTest(t)
	defer cleanup()

	sessionID := "readonly-session"
	createArchiveTestSession(t, sessionsDir, sessionID, "readonly", "claude-readonly", "")

	// Make session directory read-only to prevent manifest write
	sessionDir := filepath.Join(sessionsDir, sessionID)
	if err := os.Chmod(sessionDir, 0555); err != nil {
		t.Fatalf("Failed to chmod directory: %v", err)
	}
	// Restore permissions after test
	defer os.Chmod(sessionDir, 0755)

	oldForce := forceArchive
	forceArchive = true
	defer func() { forceArchive = oldForce }()

	// Try to archive - should fail to write manifest
	err := archiveSession(nil, []string{"readonly"})
	if err == nil {
		t.Fatal("Expected error when writing to read-only directory, got nil")
	}
}

// TestArchiveSession_ByTmuxName tests resolving session by tmux name
func TestArchiveSession_ByTmuxName(t *testing.T) {
	_, sessionsDir, cleanup := setupArchiveTest(t)
	defer cleanup()

	sessionID := "tmux-name-test"
	createArchiveTestSession(t, sessionsDir, sessionID, "my-project", "claude-myproject", "")

	oldForce := forceArchive
	forceArchive = true
	defer func() { forceArchive = oldForce }()

	// Archive by tmux name instead of manifest name
	err := archiveSession(nil, []string{"claude-myproject"})
	if err != nil {
		t.Fatalf("archiveSession by tmux name failed: %v", err)
	}

	// Verify session remains in original location (in-place archive)
	originalDir := filepath.Join(sessionsDir, sessionID)
	if _, err := os.Stat(originalDir); os.IsNotExist(err) {
		t.Errorf("Session directory should remain in place: %s", originalDir)
	}

	// Verify manifest has archived lifecycle
	manifestPath := filepath.Join(originalDir, "manifest.yaml")
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read archived manifest: %v", err)
	}
	if m.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("Expected lifecycle 'archived', got '%s'", m.Lifecycle)
	}
}

// TestArchiveSession_BySessionID tests resolving session by session ID
func TestArchiveSession_BySessionID(t *testing.T) {
	_, sessionsDir, cleanup := setupArchiveTest(t)
	defer cleanup()

	sessionID := "session-id-12345"
	createArchiveTestSession(t, sessionsDir, sessionID, "my-session", "claude-session", "")

	oldForce := forceArchive
	forceArchive = true
	defer func() { forceArchive = oldForce }()

	// Archive by session ID
	err := archiveSession(nil, []string{sessionID})
	if err != nil {
		t.Fatalf("archiveSession by session ID failed: %v", err)
	}

	// Verify session remains in original location (in-place archive)
	originalDir := filepath.Join(sessionsDir, sessionID)
	if _, err := os.Stat(originalDir); os.IsNotExist(err) {
		t.Errorf("Session directory should remain in place: %s", originalDir)
	}

	// Verify manifest has archived lifecycle
	manifestPath := filepath.Join(originalDir, "manifest.yaml")
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read archived manifest: %v", err)
	}
	if m.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("Expected lifecycle 'archived', got '%s'", m.Lifecycle)
	}
}

// TestArchiveSession_UpdatedAtTimestamp tests that UpdatedAt is set correctly
func TestArchiveSession_UpdatedAtTimestamp(t *testing.T) {
	_, sessionsDir, cleanup := setupArchiveTest(t)
	defer cleanup()

	sessionID := "timestamp-test"
	createArchiveTestSession(t, sessionsDir, sessionID, "timestamp", "claude-timestamp", "")

	// Record time before archive
	beforeArchive := time.Now()

	oldForce := forceArchive
	forceArchive = true
	defer func() { forceArchive = oldForce }()

	// Archive session
	err := archiveSession(nil, []string{"timestamp"})
	if err != nil {
		t.Fatalf("archiveSession failed: %v", err)
	}

	// Read archived manifest from original location (in-place archive)
	originalDir := filepath.Join(sessionsDir, sessionID)
	manifestPath := filepath.Join(originalDir, "manifest.yaml")
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read archived manifest: %v", err)
	}

	// Verify UpdatedAt was updated (should be after beforeArchive)
	if m.UpdatedAt.Before(beforeArchive) {
		t.Errorf("UpdatedAt timestamp not updated: got %v, expected after %v",
			m.UpdatedAt, beforeArchive)
	}
}

// TestArchiveSession_PreservesManifestFields tests that all manifest fields are preserved
func TestArchiveSession_PreservesManifestFields(t *testing.T) {
	_, sessionsDir, cleanup := setupArchiveTest(t)
	defer cleanup()

	sessionID := "preserve-test"
	sessionDir := filepath.Join(sessionsDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	// Create manifest with all fields populated
	originalManifest := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     sessionID,
		Name:          "test-session",
		CreatedAt:     time.Now().Add(-24 * time.Hour),
		UpdatedAt:     time.Now().Add(-1 * time.Hour),
		Lifecycle:     "",
		Context: manifest.Context{
			Project: "/tmp/test-project",
			Purpose: "Testing archive functionality",
			Tags:    []string{"test", "archive"},
			Notes:   "Important test session",
		},
		Claude: manifest.Claude{
			UUID: "test-uuid-1234",
		},
		Tmux: manifest.Tmux{
			SessionName: "claude-preserve",
		},
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := manifest.Write(manifestPath, originalManifest); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	oldForce := forceArchive
	forceArchive = true
	defer func() { forceArchive = oldForce }()

	// Archive session
	err := archiveSession(nil, []string{"test-session"})
	if err != nil {
		t.Fatalf("archiveSession failed: %v", err)
	}

	// Read archived manifest from original location (in-place archive)
	originalDir := filepath.Join(sessionsDir, sessionID)
	archivedManifestPath := filepath.Join(originalDir, "manifest.yaml")
	m, err := manifest.Read(archivedManifestPath)
	if err != nil {
		t.Fatalf("Failed to read archived manifest: %v", err)
	}

	// Verify all fields are preserved (except Lifecycle and UpdatedAt)
	if m.SessionID != originalManifest.SessionID {
		t.Errorf("SessionID not preserved: got %s, want %s", m.SessionID, originalManifest.SessionID)
	}
	if m.Name != originalManifest.Name {
		t.Errorf("Name not preserved: got %s, want %s", m.Name, originalManifest.Name)
	}
	if m.Context.Purpose != originalManifest.Context.Purpose {
		t.Errorf("Purpose not preserved: got %s, want %s", m.Context.Purpose, originalManifest.Context.Purpose)
	}
	if len(m.Context.Tags) != len(originalManifest.Context.Tags) {
		t.Errorf("Tags not preserved: got %v, want %v", m.Context.Tags, originalManifest.Context.Tags)
	}
	if m.Context.Notes != originalManifest.Context.Notes {
		t.Errorf("Notes not preserved: got %s, want %s", m.Context.Notes, originalManifest.Context.Notes)
	}
	if m.Claude.UUID != originalManifest.Claude.UUID {
		t.Errorf("Claude UUID not preserved: got %s, want %s", m.Claude.UUID, originalManifest.Claude.UUID)
	}

	// Verify Lifecycle was updated
	if m.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("Lifecycle not set to archived: got %s, want %s",
			m.Lifecycle, manifest.LifecycleArchived)
	}
}

// TestArchiveSession_EmptySessionsDir tests archive with empty sessions directory
func TestArchiveSession_EmptySessionsDir(t *testing.T) {
	_, _, cleanup := setupArchiveTest(t)
	defer cleanup()

	oldForce := forceArchive
	forceArchive = true
	defer func() { forceArchive = oldForce }()

	// Try to archive when sessions directory is empty
	err := archiveSession(nil, []string{"nonexistent"})
	if err == nil {
		t.Fatal("Expected error for session in empty directory, got nil")
	}
}

// TestParseDuration tests the duration parsing helper function
func TestParseDuration(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{
			name:     "days format",
			input:    "30d",
			expected: 30 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "single day",
			input:    "1d",
			expected: 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "weeks format",
			input:    "2w",
			expected: 2 * 7 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "single week",
			input:    "1w",
			expected: 7 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "hours format",
			input:    "48h",
			expected: 48 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "minutes format",
			input:    "30m",
			expected: 30 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "invalid format",
			input:    "invalid",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "invalid days",
			input:    "xd",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "invalid weeks",
			input:    "yw",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "empty string",
			input:    "",
			expected: 0,
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseDuration(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("Expected error for input '%s', got nil", tc.input)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for input '%s': %v", tc.input, err)
				}
				if result != tc.expected {
					t.Errorf("For input '%s', expected %v, got %v", tc.input, tc.expected, result)
				}
			}
		})
	}
}

// Benchmark tests

func BenchmarkArchiveSession(b *testing.B) {
	tmpDir := b.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	os.MkdirAll(sessionsDir, 0755)

	oldCfg := cfg
	cfg = &config.Config{
		SessionsDir: sessionsDir,
	}
	defer func() { cfg = oldCfg }()

	oldForce := forceArchive
	forceArchive = true
	defer func() { forceArchive = oldForce }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		sessionID := fmt.Sprintf("bench-session-%d", i)

		// Create session inline for benchmark
		sessionDir := filepath.Join(sessionsDir, sessionID)
		os.MkdirAll(sessionDir, 0755)
		m := &manifest.Manifest{
			SchemaVersion: "2",
			SessionID:     sessionID,
			Name:          fmt.Sprintf("bench-%d", i),
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Lifecycle:     "",
			Context: manifest.Context{
				Project: "/tmp/test-project",
			},
			Tmux: manifest.Tmux{
				SessionName: fmt.Sprintf("claude-bench-%d", i),
			},
		}
		manifestPath := filepath.Join(sessionDir, "manifest.yaml")
		manifest.Write(manifestPath, m)

		b.StartTimer()
		_ = archiveSession(nil, []string{sessionID})
	}
}
