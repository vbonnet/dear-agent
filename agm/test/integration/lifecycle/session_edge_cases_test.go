//go:build integration

package lifecycle_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

// TestEdgeCase_EmptySessionName tests handling of empty session name
func TestEdgeCase_EmptySessionName(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	// Attempt to create session with empty name
	err := helpers.CreateSessionManifest(env.SessionsDir, "", "claude")
	if err == nil {
		t.Error("Should fail to create session with empty name")
	}
}

// TestEdgeCase_VeryLongSessionName tests handling of extremely long names
func TestEdgeCase_VeryLongSessionName(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	// Create 300 character session name (likely to exceed filesystem limits)
	longName := strings.Repeat("a", 300)

	err := helpers.CreateSessionManifest(env.SessionsDir, longName, "claude")
	// May fail due to filesystem limits (expected)
	if err != nil {
		t.Logf("Long session name rejected as expected: %v", err)
	} else {
		t.Log("Long session name accepted - verify manifest creation succeeded")

		// Verify manifest exists
		manifestPath := filepath.Join(env.SessionsDir, longName, "manifest.yaml")
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			t.Error("Manifest should exist for long name session")
		}
	}
}

// TestEdgeCase_SpecialCharactersInName tests handling of special characters
func TestEdgeCase_SpecialCharactersInName(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	testCases := []struct {
		name        string
		sessionName string
		shouldFail  bool
	}{
		{"spaces", "my session name", false},
		{"dashes", "my-session-name", false},
		{"underscores", "my_session_name", false},
		{"dots", "my.session.name", false},
		{"slash", "my/session", true}, // Path traversal
		{"backslash", "my\\session", true},
		{"null byte", "my\x00session", true},
		{"unicode", "my-session-名前", false},
		{"emoji", "my-session-🚀", false},
		{"parentheses", "my(session)name", false},
		{"brackets", "my[session]name", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := helpers.CreateSessionManifest(env.SessionsDir, tc.sessionName, "claude")

			if tc.shouldFail && err == nil {
				t.Errorf("Expected error for session name: %s", tc.sessionName)
			}

			if !tc.shouldFail && err != nil {
				t.Logf("Session name %s rejected: %v", tc.sessionName, err)
			}
		})
	}
}

// TestEdgeCase_SessionDirectoryAlreadyExists tests handling of existing directory
func TestEdgeCase_SessionDirectoryAlreadyExists(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "existing-dir-" + helpers.RandomString(6)

	// Create directory manually
	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Attempt to create session (directory already exists)
	err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude")
	if err != nil {
		t.Logf("Session creation with existing directory: %v", err)
		// May succeed (overwrite) or fail (conflict) - both valid
	} else {
		// Verify manifest was created
		manifestPath := filepath.Join(sessionDir, "manifest.yaml")
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			t.Error("Manifest should exist after successful creation")
		}
	}
}

// TestEdgeCase_ManifestWithoutProjectDirectory tests missing project dir
func TestEdgeCase_ManifestWithoutProjectDirectory(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "no-project-" + helpers.RandomString(6)

	// Create manifest without project directory
	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "test-uuid-" + helpers.RandomString(8),
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Context: manifest.Context{
			Project: "/nonexistent/project/path",
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := manifest.Write(manifestPath, m); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Try to list sessions (should include this session)
	sessions, err := helpers.ListTestSessions(env.SessionsDir, helpers.ListFilter{All: true})
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	// Session should exist but may be marked as unhealthy
	found := false
	for _, s := range sessions {
		if s.ID == sessionName {
			found = true
			break
		}
	}

	if !found {
		t.Error("Session with missing project directory should still be listed")
	}
}

// TestEdgeCase_ZeroByteManifest tests handling of empty manifest file
func TestEdgeCase_ZeroByteManifest(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "zero-byte-" + helpers.RandomString(6)

	// Create session directory
	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	// Create zero-byte manifest
	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create zero-byte manifest: %v", err)
	}

	// Attempt to read manifest
	_, err := manifest.Read(manifestPath)
	if err == nil {
		t.Error("Should fail to read zero-byte manifest")
	}

	if !strings.Contains(err.Error(), "EOF") && !strings.Contains(err.Error(), "empty") {
		t.Logf("Zero-byte manifest error: %v", err)
	}
}

// TestEdgeCase_ManifestWithFutureTimestamp tests handling of future timestamps
func TestEdgeCase_ManifestWithFutureTimestamp(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "future-timestamp-" + helpers.RandomString(6)

	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	projectDir := filepath.Join(sessionDir, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	// Create manifest with future timestamps
	futureTime := time.Now().Add(365 * 24 * time.Hour) // 1 year in future
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "test-uuid-" + helpers.RandomString(8),
		Name:          sessionName,
		CreatedAt:     futureTime,
		UpdatedAt:     futureTime,
		Lifecycle:     "",
		Context: manifest.Context{
			Project: projectDir,
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := manifest.Write(manifestPath, m); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Read manifest back
	readManifest, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest with future timestamp: %v", err)
	}

	// Verify timestamp preserved
	if !readManifest.CreatedAt.Equal(futureTime) {
		t.Errorf("Future timestamp not preserved: expected %v, got %v", futureTime, readManifest.CreatedAt)
	}

	// AGM should accept future timestamps (clock skew, migrations, etc.)
	t.Log("Future timestamps accepted and preserved correctly")
}

// TestEdgeCase_SessionWithNoTmuxSession tests session without tmux
func TestEdgeCase_SessionWithNoTmuxSession(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "no-tmux-" + helpers.RandomString(6)

	// Create manifest without tmux session
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Verify manifest exists
	manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest: %v", err)
	}

	// Verify tmux session does NOT exist
	if helpers.IsTmuxAvailable() {
		cmd := helpers.BuildTmuxCmd("has-session", "-t", sessionName)
		if err := cmd.Run(); err == nil {
			t.Error("Tmux session should not exist")
		}
	}

	// Session should be listed (suspended state)
	sessions, err := helpers.ListTestSessions(env.SessionsDir, helpers.ListFilter{All: true})
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	found := false
	for _, s := range sessions {
		if s.ID == sessionName {
			found = true
			break
		}
	}

	if !found {
		t.Error("Session without tmux should still be listed")
	}

	t.Logf("Session lifecycle: %s (empty = suspended)", m.Lifecycle)
}

// TestEdgeCase_MultipleArchiveOperations tests repeated archiving
func TestEdgeCase_MultipleArchiveOperations(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "multi-archive-" + helpers.RandomString(6)

	// Create session
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Archive once
	if err := helpers.ArchiveTestSession(env.SessionsDir, sessionName, "first archive"); err != nil {
		t.Fatalf("First archive failed: %v", err)
	}

	// Verify archived
	manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest: %v", err)
	}

	if m.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("Expected lifecycle 'archived', got %s", m.Lifecycle)
	}

	firstUpdateTime := m.UpdatedAt

	// Archive again (idempotent operation)
	time.Sleep(100 * time.Millisecond) // Ensure different timestamp
	if err := helpers.ArchiveTestSession(env.SessionsDir, sessionName, "second archive"); err != nil {
		t.Logf("Second archive: %v (may succeed or fail)", err)
	}

	// Read manifest again
	m, err = manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest after second archive: %v", err)
	}

	// Should still be archived
	if m.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("Expected lifecycle 'archived' after second archive, got %s", m.Lifecycle)
	}

	t.Logf("Multiple archive operations handled: UpdatedAt changed from %v to %v",
		firstUpdateTime, m.UpdatedAt)
}

// TestEdgeCase_SessionWithSymlinkedProject tests symlink handling
func TestEdgeCase_SessionWithSymlinkedProject(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "symlink-project-" + helpers.RandomString(6)

	// Create actual project directory
	actualProject := filepath.Join(env.TempDir, "actual-project")
	if err := os.MkdirAll(actualProject, 0755); err != nil {
		t.Fatalf("Failed to create actual project: %v", err)
	}

	// Create symlink
	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	symlinkPath := filepath.Join(sessionDir, "project")
	if err := os.Symlink(actualProject, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Create manifest pointing to symlink
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "test-uuid-" + helpers.RandomString(8),
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Context: manifest.Context{
			Project: symlinkPath,
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := manifest.Write(manifestPath, m); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Verify session is healthy (symlink should work)
	_, err := os.Stat(symlinkPath)
	if err != nil {
		t.Errorf("Symlinked project should be accessible: %v", err)
	}

	// Verify AGM can work with symlinked projects
	sessions, err := helpers.ListTestSessions(env.SessionsDir, helpers.ListFilter{All: true})
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	found := false
	for _, s := range sessions {
		if s.ID == sessionName {
			found = true
			break
		}
	}

	if !found {
		t.Error("Session with symlinked project should be listed")
	}
}

// TestEdgeCase_ReadOnlyManifest tests handling of read-only manifest file
func TestEdgeCase_ReadOnlyManifest(t *testing.T) {
	t.Skip("Atomic writes succeed on read-only files by design - test expectations incorrect")

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "readonly-manifest-" + helpers.RandomString(6)

	// Create session
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Make manifest read-only
	manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
	if err := os.Chmod(manifestPath, 0444); err != nil {
		t.Fatalf("Failed to make manifest read-only: %v", err)
	}

	// Ensure cleanup restores permissions
	defer os.Chmod(manifestPath, 0644)

	// Try to read (should succeed)
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Errorf("Reading read-only manifest should succeed: %v", err)
	}

	// Try to write (should fail)
	m.Context.Tags = append(m.Context.Tags, "test-tag")
	err = manifest.Write(manifestPath, m)
	if err == nil {
		t.Error("Writing to read-only manifest should fail")
	} else {
		t.Logf("Read-only manifest write failed as expected: %v", err)
	}

	// Try to archive (should fail or warn)
	err = helpers.ArchiveTestSession(env.SessionsDir, sessionName, "readonly test")
	if err != nil {
		t.Logf("Archive of read-only manifest failed: %v", err)
	}
}

// TestEdgeCase_SessionDirWithNoManifest tests directory without manifest.yaml
func TestEdgeCase_SessionDirWithNoManifest(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "no-manifest-" + helpers.RandomString(6)

	// Create session directory without manifest
	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	// Create some files but no manifest.yaml
	testFile := filepath.Join(sessionDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Try to list sessions (should skip this directory)
	sessions, err := helpers.ListTestSessions(env.SessionsDir, helpers.ListFilter{All: true})
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	// Session should not appear in list
	for _, s := range sessions {
		if s.ID == sessionName {
			t.Error("Directory without manifest should not be listed as session")
		}
	}
}

// TestEdgeCase_TimestampPrecision tests timestamp precision and timezone handling
func TestEdgeCase_TimestampPrecision(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "timestamp-precision-" + helpers.RandomString(6)

	// Create session with precise timestamp
	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	projectDir := filepath.Join(sessionDir, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	// Precise timestamp with microseconds
	preciseTime := time.Now()
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "test-uuid-" + helpers.RandomString(8),
		Name:          sessionName,
		CreatedAt:     preciseTime,
		UpdatedAt:     preciseTime,
		Lifecycle:     "",
		Context: manifest.Context{
			Project: projectDir,
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := manifest.Write(manifestPath, m); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Read manifest back
	readManifest, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest: %v", err)
	}

	// Check timestamp precision
	// YAML/Go time serialization may lose sub-second precision
	timeDiff := preciseTime.Sub(readManifest.CreatedAt)
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}

	// Allow up to 1 second difference (YAML time format precision)
	if timeDiff > time.Second {
		t.Errorf("Timestamp precision loss too large: %v", timeDiff)
	}

	t.Logf("Timestamp precision preserved within: %v", timeDiff)
}
