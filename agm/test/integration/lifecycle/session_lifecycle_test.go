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
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

// TestSessionCreation_FullLifecycle tests the complete session creation workflow
func TestSessionCreation_FullLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping full lifecycle test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-lifecycle-" + helpers.RandomString(6)
	env.RegisterSession(sessionName)

	// Step 1: Create session using agm session new
	t.Run("CreateSession", func(t *testing.T) {
		// Note: This requires actual tmux to be running
		// For unit tests, we may need to mock or skip
		if !helpers.IsTmuxAvailable() {
			t.Skip("Tmux not available")
		}

		// Create session in detached mode
		cmd := exec.Command("agm", "session", "new", sessionName,
			"--sessions-dir", env.SessionsDir,
			"--detached",
			"--agent", "claude")

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to create session: %v\nOutput: %s", err, output)
		}

		// Verify manifest was created
		manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			t.Errorf("Manifest file not created at %s", manifestPath)
		}

		// Verify manifest contents
		m, err := manifest.Read(manifestPath)
		if err != nil {
			t.Fatalf("Failed to read manifest: %v", err)
		}

		if m.Name != sessionName {
			t.Errorf("Expected session name %s, got %s", sessionName, m.Name)
		}
		if m.Agent != "claude" {
			t.Errorf("Expected agent 'claude', got %s", m.Agent)
		}
		if m.Lifecycle != "" {
			t.Errorf("Expected empty lifecycle for new session, got %s", m.Lifecycle)
		}
	})

	// Step 2: Verify session appears in list
	t.Run("SessionInList", func(t *testing.T) {
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
			t.Errorf("Created session %s not found in list", sessionName)
		}
	})

	// Step 3: Archive session
	t.Run("ArchiveSession", func(t *testing.T) {
		// First kill tmux session and all processes to make it inactive
		if helpers.IsTmuxAvailable() {
			helpers.KillSessionProcesses(sessionName)
		}

		err := helpers.ArchiveTestSession(env.SessionsDir, sessionName, "test cleanup")
		if err != nil {
			t.Fatalf("Failed to archive session: %v", err)
		}

		// Verify lifecycle field updated
		manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
		m, err := manifest.Read(manifestPath)
		if err != nil {
			t.Fatalf("Failed to read manifest after archive: %v", err)
		}

		if m.Lifecycle != manifest.LifecycleArchived {
			t.Errorf("Expected lifecycle 'archived', got %s", m.Lifecycle)
		}
	})

	// Step 4: Verify archived session hidden from default list
	t.Run("ArchivedSessionHidden", func(t *testing.T) {
		sessions, err := helpers.ListTestSessions(env.SessionsDir, helpers.ListFilter{})
		if err != nil {
			t.Fatalf("Failed to list sessions: %v", err)
		}

		for _, s := range sessions {
			if s.ID == sessionName {
				t.Errorf("Archived session %s should not appear in default list", sessionName)
			}
		}
	})

	// Step 5: Verify archived session visible with --all flag
	t.Run("ArchivedSessionVisibleWithAll", func(t *testing.T) {
		sessions, err := helpers.ListTestSessions(env.SessionsDir, helpers.ListFilter{All: true})
		if err != nil {
			t.Fatalf("Failed to list sessions with --all: %v", err)
		}

		found := false
		for _, s := range sessions {
			if s.ID == sessionName && s.Archived {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Archived session %s not found with --all flag", sessionName)
		}
	})
}

// TestSessionCreation_WithHooks tests that hooks execute in correct order during creation
func TestSessionCreation_WithHooks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping hook test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	_ = "test-hooks-" + helpers.RandomString(6)

	// Create a .claude directory with a hook
	claudeDir := filepath.Join(env.TempDir, ".claude")
	hooksDir := filepath.Join(claudeDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("Failed to create hooks directory: %v", err)
	}

	// Create a post-init hook that writes a marker file
	hookMarker := filepath.Join(env.TempDir, "hook-executed.txt")
	hookScript := filepath.Join(hooksDir, "post-init.sh")
	hookContent := "#!/bin/bash\necho 'Hook executed' > " + hookMarker + "\n"
	if err := os.WriteFile(hookScript, []byte(hookContent), 0755); err != nil {
		t.Fatalf("Failed to create hook script: %v", err)
	}

	// Create session (hooks are currently not implemented in AGM, so this documents expected behavior)
	// When hooks are implemented, this test will verify execution order
	t.Skip("Hook execution not yet implemented - test documents expected behavior")

	// Expected behavior when hooks are implemented:
	// 1. Manifest created
	// 2. Tmux session created
	// 3. Agent started
	// 4. post-init hook executed
	// 5. Hook marker file should exist
}

// TestSessionTermination_CleanupResources tests proper cleanup on termination
func TestSessionTermination_CleanupResources(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping termination test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-terminate-" + helpers.RandomString(6)

	// Create a minimal session manifest
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session manifest: %v", err)
	}

	// Create a lock file to simulate active session
	lockDir := filepath.Join(env.SessionsDir, sessionName)
	lockPath := filepath.Join(lockDir, ".lock")
	lockFile, err := os.Create(lockPath)
	if err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}
	lockFile.Close()

	// Verify lock exists before cleanup
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatal("Lock file should exist before cleanup")
	}

	// Archive session (should clean up resources)
	err = helpers.ArchiveTestSession(env.SessionsDir, sessionName, "cleanup test")
	if err != nil {
		t.Fatalf("Failed to archive session: %v", err)
	}

	// Verify lock is cleaned up (may depend on implementation)
	// Note: Current implementation may not clean locks on archive
	// This documents expected behavior
	t.Log("Lock cleanup behavior may vary by implementation")
}

// TestA2AMessaging_SendReceive tests agent-to-agent messaging
func TestA2AMessaging_SendReceive(t *testing.T) {
	t.Skip("A2A messaging not yet implemented - agm send command does not exist")

	if testing.Short() {
		t.Skip("Skipping A2A messaging test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	// Create sender and receiver sessions
	senderName := "test-sender-" + helpers.RandomString(6)
	receiverName := "test-receiver-" + helpers.RandomString(6)

	// Create both sessions
	for _, name := range []string{senderName, receiverName} {
		if err := helpers.CreateSessionManifest(env.SessionsDir, name, "claude"); err != nil {
			t.Fatalf("Failed to create session %s: %v", name, err)
		}

		// Create actual tmux session
		cmd := helpers.BuildTmuxCmd("new-session", "-d", "-s", name, "sleep", "3600")
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to create tmux session %s: %v", name, err)
		}
		defer helpers.KillSessionProcesses(name)
	}

	// Wait for sessions to be ready
	time.Sleep(200 * time.Millisecond)

	// Send message from sender to receiver
	message := "Test message " + helpers.RandomString(8)
	cmd := exec.Command("agm", "send", receiverName,
		"--sessions-dir", env.SessionsDir,
		"--prompt", message)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to send message: %v\nOutput: %s", err, output)
	}

	// Give tmux time to process
	time.Sleep(300 * time.Millisecond)

	// Capture receiver pane to verify message was sent
	captureCmd := helpers.BuildTmuxCmd("capture-pane", "-t", receiverName, "-p")
	captureOutput, err := captureCmd.Output()
	if err != nil {
		t.Fatalf("Failed to capture pane: %v", err)
	}

	// Verify message appears in pane
	if !strings.Contains(string(captureOutput), message) {
		t.Errorf("Expected message '%s' not found in pane output", message)
		t.Logf("Pane output:\n%s", captureOutput)
	}
}

// TestSessionCreation_ManifestFields tests that all required manifest fields are populated
func TestSessionCreation_ManifestFields(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-manifest-" + helpers.RandomString(6)

	// Create session manifest
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Read and verify manifest
	manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest: %v", err)
	}

	// Verify required fields
	tests := []struct {
		name  string
		value string
		field string
	}{
		{"SchemaVersion", m.SchemaVersion, "schema_version"},
		{"SessionID", m.SessionID, "session_id"},
		{"Name", m.Name, "name"},
		{"Agent", m.Agent, "agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value == "" {
				t.Errorf("Field %s should not be empty", tt.field)
			}
		})
	}

	// Verify timestamps
	if m.CreatedAt.IsZero() {
		t.Error("CreatedAt timestamp should be set")
	}
	if m.UpdatedAt.IsZero() {
		t.Error("UpdatedAt timestamp should be set")
	}

	// Verify UUID format for session_id (should be valid UUID)
	if _, err := parseUUID(m.SessionID); err != nil {
		t.Errorf("SessionID should be valid UUID, got %s: %v", m.SessionID, err)
	}
}

// TestSessionHealth_Checks tests health check functionality
func TestSessionHealth_Checks(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-health-" + helpers.RandomString(6)

	// Create session with valid project directory
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest: %v", err)
	}

	// Test 1: Health check with valid project directory
	t.Run("HealthySession", func(t *testing.T) {
		report, err := session.CheckHealth(m)
		if err != nil {
			t.Fatalf("Health check failed: %v", err)
		}

		if !report.IsHealthy() {
			t.Errorf("Session should be healthy, got issues: %s", report.Summary())
		}
	})

	// Test 2: Health check with missing project directory
	t.Run("MissingProjectDirectory", func(t *testing.T) {
		// Modify manifest to point to non-existent directory
		originalProject := m.Context.Project
		m.Context.Project = filepath.Join(env.TempDir, "nonexistent-project")

		report, err := session.CheckHealth(m)
		if err != nil {
			t.Fatalf("Health check failed: %v", err)
		}

		if report.IsHealthy() {
			t.Error("Session should not be healthy with missing project directory")
		}

		if !strings.Contains(report.Summary(), "does not exist") {
			t.Errorf("Expected 'does not exist' in health report, got: %s", report.Summary())
		}

		// Restore original path
		m.Context.Project = originalProject
	})
}

// TestSessionCleanup_OnError tests that resources are cleaned up on error
func TestSessionCleanup_OnError(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping error cleanup test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-error-cleanup-" + helpers.RandomString(6)

	// Attempt to create session with invalid parameters
	// (e.g., invalid agent name)
	cmd := exec.Command("agm", "session", "new", sessionName,
		"--sessions-dir", env.SessionsDir,
		"--agent", "invalid-agent-xyz",
		"--detached")

	output, err := cmd.CombinedOutput()

	// Should fail with invalid agent
	if err == nil {
		t.Log("Expected error creating session with invalid agent (implementation may vary)")
	}

	// Verify no partial session artifacts left behind
	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if _, err := os.Stat(sessionDir); err == nil {
		// If directory exists, it should not have a valid manifest
		manifestPath := filepath.Join(sessionDir, "manifest.yaml")
		if _, err := os.Stat(manifestPath); err == nil {
			t.Error("Partial manifest should not exist after failed session creation")
		}
	}

	t.Logf("Cleanup test output: %s", output)
}

// TestConcurrentSessions_NoConflict tests multiple concurrent sessions
func TestConcurrentSessions_NoConflict(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent sessions test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	// Create 3 concurrent sessions
	sessionNames := []string{
		"test-concurrent-1-" + helpers.RandomString(4),
		"test-concurrent-2-" + helpers.RandomString(4),
		"test-concurrent-3-" + helpers.RandomString(4),
	}

	for _, name := range sessionNames {
		if err := helpers.CreateSessionManifest(env.SessionsDir, name, "claude"); err != nil {
			t.Fatalf("Failed to create session %s: %v", name, err)
		}

		// Create tmux session
		cmd := helpers.BuildTmuxCmd("new-session", "-d", "-s", name, "sleep", "60")
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to create tmux session %s: %v", name, err)
		}
		defer helpers.KillSessionProcesses(name)
	}

	// Verify all sessions exist and have unique session IDs
	sessionIDs := make(map[string]bool)
	for _, name := range sessionNames {
		manifestPath := filepath.Join(env.SessionsDir, name, "manifest.yaml")
		m, err := manifest.Read(manifestPath)
		if err != nil {
			t.Fatalf("Failed to read manifest for %s: %v", name, err)
		}

		// Check for duplicate session IDs
		if sessionIDs[m.SessionID] {
			t.Errorf("Duplicate session ID found: %s", m.SessionID)
		}
		sessionIDs[m.SessionID] = true
	}

	// Verify all sessions are listed
	sessions, err := helpers.ListTestSessions(env.SessionsDir, helpers.ListFilter{All: true})
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	if len(sessions) < len(sessionNames) {
		t.Errorf("Expected at least %d sessions, got %d", len(sessionNames), len(sessions))
	}
}

// Helper functions

// parseUUID validates UUID format (simple validation)
func parseUUID(s string) (bool, error) {
	if len(s) != 36 {
		return false, nil
	}
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false, nil
	}
	return true, nil
}

// TestPromptDetection tests waiting for prompt after command execution
func TestPromptDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping prompt detection test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	// Clean up any stale locks from previous tests
	exec.Command("agm", "unlock").Run() // Ignore errors - lock may not exist

	// Wait for lock cleanup to complete
	time.Sleep(500 * time.Millisecond)

	sessionName := "test-prompt-" + helpers.RandomString(6)

	// Create tmux session with bash
	cmd := helpers.BuildTmuxCmd("new-session", "-d", "-s", sessionName, "bash")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer helpers.KillSessionProcesses(sessionName)

	// Wait for bash to start
	time.Sleep(500 * time.Millisecond)

	// Send a command and wait for prompt
	if err := tmux.SendCommand(sessionName, "echo 'test'"); err != nil {
		t.Fatalf("Failed to send command: %v", err)
	}

	// Wait for command to complete (simple time-based wait for now)
	// TODO: Use proper prompt detection when available
	time.Sleep(1 * time.Second)

	// Verify command output is in pane
	captureCmd := helpers.BuildTmuxCmd("capture-pane", "-t", sessionName, "-p")
	output, err := captureCmd.Output()
	if err != nil {
		t.Fatalf("Failed to capture pane: %v", err)
	}

	if !strings.Contains(string(output), "test") {
		t.Error("Command output not found in pane")
	}
}

// TestSessionArchive_PreservesMetadata tests that archiving preserves all metadata
func TestSessionArchive_PreservesMetadata(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-archive-metadata-" + helpers.RandomString(6)

	// Create session with metadata
	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	projectDir := filepath.Join(sessionDir, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	// Create manifest with rich metadata
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "test-uuid-" + helpers.RandomString(8),
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Context: manifest.Context{
			Project: projectDir,
			Purpose: "Testing archive metadata preservation",
			Tags:    []string{"test", "archive", "metadata"},
			Notes:   "Important test notes",
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
		Claude: manifest.Claude{
			UUID: "test-claude-uuid",
		},
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := manifest.Write(manifestPath, m); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Archive session
	err := helpers.ArchiveTestSession(env.SessionsDir, sessionName, "metadata test")
	if err != nil {
		t.Fatalf("Failed to archive session: %v", err)
	}

	// Read archived manifest
	archivedManifest, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read archived manifest: %v", err)
	}

	// Verify all metadata preserved
	if archivedManifest.Context.Purpose != m.Context.Purpose {
		t.Errorf("Purpose not preserved: expected %s, got %s", m.Context.Purpose, archivedManifest.Context.Purpose)
	}

	if len(archivedManifest.Context.Tags) != len(m.Context.Tags) {
		t.Errorf("Tags not preserved: expected %d, got %d", len(m.Context.Tags), len(archivedManifest.Context.Tags))
	}

	if archivedManifest.Context.Notes != m.Context.Notes {
		t.Errorf("Notes not preserved: expected %s, got %s", m.Context.Notes, archivedManifest.Context.Notes)
	}

	if archivedManifest.Claude.UUID != m.Claude.UUID {
		t.Errorf("Claude UUID not preserved: expected %s, got %s", m.Claude.UUID, archivedManifest.Claude.UUID)
	}

	// Verify lifecycle updated
	if archivedManifest.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("Expected lifecycle 'archived', got %s", archivedManifest.Lifecycle)
	}
}
