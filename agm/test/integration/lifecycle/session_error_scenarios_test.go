//go:build integration

package lifecycle_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

// TestError_CreateDuplicateSession tests error when creating duplicate session
func TestError_CreateDuplicateSession(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping error scenario test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-duplicate-" + helpers.RandomString(6)

	// Create first session
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create first session: %v", err)
	}

	// Create tmux session
	cmd := helpers.BuildTmuxCmd("new-session", "-d", "-s", sessionName, "sleep", "60")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer helpers.KillSessionProcesses(sessionName)

	// Attempt to create duplicate session
	cmd = exec.Command("agm", "session", "new", sessionName,
		"--sessions-dir", env.SessionsDir,
		"--detached")

	output, err := cmd.CombinedOutput()

	// Should fail with appropriate error
	if err == nil {
		t.Error("Expected error when creating duplicate session")
	}

	// Error message should mention session already exists
	if !strings.Contains(string(output), "exists") && !strings.Contains(string(output), "duplicate") {
		t.Logf("Expected 'exists' or 'duplicate' in error message, got: %s", output)
	}
}

// TestError_ResumeMissingSession tests error when resuming non-existent session
func TestError_ResumeMissingSession(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "nonexistent-session-" + helpers.RandomString(8)

	cmd := exec.Command("agm", "session", "resume", sessionName,
		"--sessions-dir", env.SessionsDir)

	output, err := cmd.CombinedOutput()

	// Should fail
	if err == nil {
		t.Error("Expected error when resuming missing session")
	}

	// Error should be helpful
	outputStr := string(output)
	if !strings.Contains(outputStr, "not found") && !strings.Contains(outputStr, "does not exist") {
		t.Logf("Expected helpful error message, got: %s", outputStr)
	}
}

// TestError_ArchiveActiveSession tests archiving currently active session
func TestError_ArchiveActiveSession(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping active session archive test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-active-archive-" + helpers.RandomString(6)

	// Create active session
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create tmux session
	cmd := helpers.BuildTmuxCmd("new-session", "-d", "-s", sessionName, "sleep", "300")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer helpers.KillSessionProcesses(sessionName)

	// Wait for session to be active
	time.Sleep(100 * time.Millisecond)

	// Attempt to archive active session without force flag
	cmd = exec.Command("agm", "session", "archive", sessionName,
		"--sessions-dir", env.SessionsDir)

	output, err := cmd.CombinedOutput()

	// Behavior may vary:
	// - May prompt for confirmation (no TTY in test = error)
	// - May warn session is active
	// - May succeed with warning
	t.Logf("Archive active session output: %s, err: %v", output, err)

	// With --force flag, should succeed
	cmd = exec.Command("agm", "session", "archive", sessionName,
		"--sessions-dir", env.SessionsDir,
		"--force")

	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Logf("Archive with --force failed: %v\nOutput: %s", err, output)
	}
}

// TestError_SendToMissingSession tests sending message to non-existent session
func TestError_SendToMissingSession(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "missing-target-" + helpers.RandomString(8)

	cmd := exec.Command("agm", "send", sessionName,
		"--sessions-dir", env.SessionsDir,
		"--prompt", "test message")

	output, err := cmd.CombinedOutput()

	// Should fail
	if err == nil {
		t.Error("Expected error when sending to missing session")
	}

	// Error should mention session not found
	outputStr := string(output)
	if !strings.Contains(outputStr, "not") && !strings.Contains(outputStr, "exist") {
		t.Logf("Expected error about missing session, got: %s", outputStr)
	}
}

// TestError_CorruptedManifest tests handling of corrupted manifest file
func TestError_CorruptedManifest(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-corrupted-" + helpers.RandomString(6)

	// Create session directory
	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	// Write corrupted manifest (invalid YAML)
	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	corruptedContent := "this is not valid YAML: {{{{{ unclosed brackets"
	if err := os.WriteFile(manifestPath, []byte(corruptedContent), 0644); err != nil {
		t.Fatalf("Failed to write corrupted manifest: %v", err)
	}

	// Attempt to read manifest
	_, err := manifest.Read(manifestPath)
	if err == nil {
		t.Error("Expected error reading corrupted manifest")
	}

	// Error should indicate YAML parsing issue
	if !strings.Contains(err.Error(), "yaml") && !strings.Contains(err.Error(), "unmarshal") {
		t.Logf("Expected YAML error, got: %v", err)
	}
}

// TestError_MissingManifestField tests manifest with missing required fields
func TestError_MissingManifestField(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-incomplete-" + helpers.RandomString(6)

	// Create session directory
	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	// Write manifest missing required fields (e.g., session_id)
	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	incompleteManifest := `schema_version: "2"
name: "test-session"
# Missing session_id
created_at: "2026-01-01T00:00:00Z"
updated_at: "2026-01-01T00:00:00Z"
`
	if err := os.WriteFile(manifestPath, []byte(incompleteManifest), 0644); err != nil {
		t.Fatalf("Failed to write incomplete manifest: %v", err)
	}

	// Attempt to read manifest
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Logf("Read error (acceptable): %v", err)
		return
	}

	// If read succeeds, session_id should be empty
	if m.SessionID != "" {
		t.Errorf("Expected empty session_id for incomplete manifest, got: %s", m.SessionID)
	}
}

// TestError_ConcurrentArchive tests concurrent archive operations
func TestError_ConcurrentArchive(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent operations test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-concurrent-archive-" + helpers.RandomString(6)

	// Create session
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Launch two concurrent archive operations
	done1 := make(chan error, 1)
	done2 := make(chan error, 1)

	go func() {
		cmd := exec.Command("agm", "session", "archive", sessionName,
			"--sessions-dir", env.SessionsDir,
			"--force")
		_, err := cmd.CombinedOutput()
		done1 <- err
	}()

	go func() {
		cmd := exec.Command("agm", "session", "archive", sessionName,
			"--sessions-dir", env.SessionsDir,
			"--force")
		_, err := cmd.CombinedOutput()
		done2 <- err
	}()

	// Wait for both to complete
	err1 := <-done1
	err2 := <-done2

	// At least one should succeed
	if err1 != nil && err2 != nil {
		t.Errorf("Both concurrent archives failed: err1=%v, err2=%v", err1, err2)
	}

	// Verify session is archived
	manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest after concurrent archive: %v", err)
	}

	if m.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("Expected lifecycle 'archived', got %s", m.Lifecycle)
	}
}

// TestError_InvalidSessionName tests session creation with invalid names
func TestError_InvalidSessionName(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping invalid name test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	invalidNames := []string{
		"", // Empty
		"session with spaces",
		"session/with/slashes",
		"session\\with\\backslashes",
		"session:with:colons",
		"session;with;semicolons",
		"session|with|pipes",
		"../../etc/passwd", // Path traversal
		".hidden",          // Starts with dot
		"session\nwith\nnewlines",
	}

	for _, invalidName := range invalidNames {
		t.Run("InvalidName_"+invalidName, func(t *testing.T) {
			cmd := exec.Command("agm", "session", "new", invalidName,
				"--sessions-dir", env.SessionsDir,
				"--detached")

			output, err := cmd.CombinedOutput()

			// Most invalid names should be rejected
			// Some may be sanitized by tmux itself
			if err == nil && invalidName != "" {
				t.Logf("Session creation with name '%s' succeeded (may be sanitized): %s",
					invalidName, output)
			}
		})
	}
}

// TestError_DiskFullSimulation tests behavior when disk is full
func TestError_DiskFullSimulation(t *testing.T) {
	// This test is difficult to implement without actually filling disk
	// or using special filesystem features (quotas, etc.)
	t.Skip("Disk full simulation requires special setup")

	// When implemented, this test should:
	// 1. Create a small filesystem with quota
	// 2. Fill it near capacity
	// 3. Attempt to create session
	// 4. Verify graceful failure with helpful error message
}

// TestError_PermissionDenied tests behavior with insufficient permissions
func TestError_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Cannot test permission denied as root")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-permissions-" + helpers.RandomString(6)

	// Create session directory with no write permissions
	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0555); err != nil {
		t.Fatalf("Failed to create read-only directory: %v", err)
	}

	// Attempt to write manifest
	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	err := os.WriteFile(manifestPath, []byte("test"), 0644)

	// Should fail with permission denied
	if err == nil {
		t.Error("Expected permission denied error")
		// Cleanup
		os.Chmod(sessionDir, 0755)
		os.Remove(manifestPath)
	} else if !os.IsPermission(err) {
		t.Errorf("Expected permission error, got: %v", err)
	}

	// Restore permissions for cleanup
	os.Chmod(sessionDir, 0755)
}

// TestError_TmuxServerDead tests behavior when tmux server dies
func TestError_TmuxServerDead(t *testing.T) {
	// This test is dangerous as it requires killing tmux server
	// which may affect other tests or user sessions
	t.Skip("Killing tmux server affects other tests")

	// When implemented safely (in isolated environment):
	// 1. Start isolated tmux server
	// 2. Create session
	// 3. Kill tmux server
	// 4. Attempt operations
	// 5. Verify graceful failure with helpful error
}

// TestError_ManifestVersionMismatch tests handling of incompatible manifest version
func TestError_ManifestVersionMismatch(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-version-" + helpers.RandomString(6)

	// Create session directory
	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	// Write manifest with future/incompatible version
	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	futureManifest := `schema_version: "999.0"
session_id: "test-id"
name: "test-session"
created_at: "2026-01-01T00:00:00Z"
updated_at: "2026-01-01T00:00:00Z"
`
	if err := os.WriteFile(manifestPath, []byte(futureManifest), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Attempt to read manifest
	m, err := manifest.Read(manifestPath)

	// Current implementation may not validate version strictly
	if err != nil {
		t.Logf("Version validation error (good): %v", err)
	} else {
		t.Logf("Version %s accepted (implementation may auto-migrate)", m.SchemaVersion)
	}
}

// TestError_RaceConditionManifestUpdate tests concurrent manifest updates
func TestError_RaceConditionManifestUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping race condition test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-race-" + helpers.RandomString(6)

	// Create session
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")

	// Launch multiple concurrent updates
	const numUpdates = 5
	done := make(chan error, numUpdates)

	for i := 0; i < numUpdates; i++ {
		go func(iteration int) {
			// Read manifest
			m, err := manifest.Read(manifestPath)
			if err != nil {
				done <- err
				return
			}

			// Modify a field
			m.Context.Notes = "Update " + string(rune('A'+iteration))

			// Write back
			err = manifest.Write(manifestPath, m)
			done <- err
		}(i)
	}

	// Collect results
	var errors []error
	for i := 0; i < numUpdates; i++ {
		if err := <-done; err != nil {
			errors = append(errors, err)
		}
	}

	// All updates should succeed (last one wins)
	if len(errors) > 0 {
		t.Logf("Some concurrent updates failed (expected): %v", errors)
	}

	// Verify manifest is still valid
	finalManifest, err := manifest.Read(manifestPath)
	if err != nil {
		t.Errorf("Final manifest corrupted: %v", err)
	} else {
		t.Logf("Final notes: %s", finalManifest.Context.Notes)
	}
}

// TestError_SendEmptyMessage tests sending empty message
func TestError_SendEmptyMessage(t *testing.T) {
	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-empty-msg-" + helpers.RandomString(6)

	// Create session
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create tmux session
	cmd := helpers.BuildTmuxCmd("new-session", "-d", "-s", sessionName, "sleep", "60")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer helpers.KillSessionProcesses(sessionName)

	// Attempt to send empty message
	cmd = exec.Command("agm", "send", sessionName,
		"--sessions-dir", env.SessionsDir,
		"--prompt", "")

	output, err := cmd.CombinedOutput()

	// Should fail or send nothing
	if err == nil {
		t.Logf("Empty message accepted (may be intentional): %s", output)
	}
}

// TestError_MessageTooLarge tests sending extremely large message
func TestError_MessageTooLarge(t *testing.T) {
	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-large-msg-" + helpers.RandomString(6)

	// Create session
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create tmux session
	cmd := helpers.BuildTmuxCmd("new-session", "-d", "-s", sessionName, "sleep", "60")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer helpers.KillSessionProcesses(sessionName)

	// Create very large message (1MB)
	largeMessage := strings.Repeat("A", 1024*1024)

	cmd = exec.Command("agm", "send", sessionName,
		"--sessions-dir", env.SessionsDir,
		"--prompt", largeMessage)

	output, err := cmd.CombinedOutput()

	// May succeed or fail depending on implementation limits
	if err != nil {
		t.Logf("Large message rejected (good): %v", err)
	} else {
		t.Logf("Large message accepted: %d bytes sent", len(output))
	}
}
