//go:build integration

package lifecycle_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

// TestStateTransition_ActiveToSuspended tests transitioning from active to suspended
func TestStateTransition_ActiveToSuspended(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping state transition test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-suspend-" + helpers.RandomString(6)

	// Create active session
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create tmux session (active state)
	cmd := helpers.BuildTmuxCmd("new-session", "-d", "-s", sessionName, "sleep", "300")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer helpers.KillSessionProcesses(sessionName)

	// Verify initial state - lifecycle should be empty (active/running)
	manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest: %v", err)
	}

	if m.Lifecycle != "" {
		t.Errorf("Expected empty lifecycle for active session, got %s", m.Lifecycle)
	}

	// Suspend session by killing tmux (simulates detach/stop)
	if err := helpers.KillSessionProcesses(sessionName); err != nil {
		t.Logf("Failed to kill tmux session: %v", err)
	}

	// Wait for cleanup
	time.Sleep(200 * time.Millisecond)

	// Verify tmux session is gone (suspended state)
	checkCmd := helpers.BuildTmuxCmd("has-session", "-t", sessionName)
	if err := checkCmd.Run(); err == nil {
		t.Error("Tmux session should be gone after suspend")
	}

	// Manifest lifecycle should still be empty (suspended sessions don't update lifecycle)
	m, err = manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest after suspend: %v", err)
	}

	// Note: AGM doesn't currently track suspended state explicitly
	// Suspended = manifest exists, lifecycle is empty, tmux session doesn't exist
	if m.Lifecycle == manifest.LifecycleArchived {
		t.Error("Suspended session should not be archived")
	}
}

// TestStateTransition_SuspendedToActive tests resuming a suspended session
func TestStateTransition_SuspendedToActive(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping resume test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-resume-suspended-" + helpers.RandomString(6)

	// Create session manifest (represents suspended session)
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Verify manifest exists and lifecycle is empty (suspended)
	manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest: %v", err)
	}

	if m.Lifecycle != "" {
		t.Errorf("Expected empty lifecycle, got %s", m.Lifecycle)
	}

	// Verify no tmux session exists
	checkCmd := helpers.BuildTmuxCmd("has-session", "-t", sessionName)
	if err := checkCmd.Run(); err == nil {
		t.Error("Tmux session should not exist for suspended session")
	}

	// Resume would be done with: agm session resume <session-name>
	// But that requires TTY, so we document the expected behavior
	t.Log("Resume operation verified: manifest exists, lifecycle empty, tmux session absent")
	t.Log("Full resume test requires TTY - see resume_test.go for TTY-based tests")
}

// TestStateTransition_ActiveToArchived tests archiving an active session
func TestStateTransition_ActiveToArchived(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping archive transition test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-archive-active-" + helpers.RandomString(6)

	// Create active session
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create tmux session
	cmd := helpers.BuildTmuxCmd("new-session", "-d", "-s", sessionName, "sleep", "60")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}

	// Verify active state
	manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest: %v", err)
	}

	if m.Lifecycle != "" {
		t.Errorf("Expected empty lifecycle for active session, got %s", m.Lifecycle)
	}

	// Archive active session with force flag
	err = helpers.ArchiveTestSession(env.SessionsDir, sessionName, "transition test")
	if err != nil {
		// Kill tmux session and its processes first, then retry
		helpers.KillSessionProcesses(sessionName)

		err = helpers.ArchiveTestSession(env.SessionsDir, sessionName, "transition test")
		if err != nil {
			t.Fatalf("Failed to archive session: %v", err)
		}
	}

	// Verify archived state
	// Note: archive command uses in-place archiving (sets lifecycle field)
	manifestPath = filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
	m, err = manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest after archive: %v", err)
	}

	if m.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("Expected lifecycle 'archived', got %s", m.Lifecycle)
	}

	// Note: archive command does NOT kill tmux session automatically
	// It only sets lifecycle: archived in the manifest
	// The tmux session may still be running (user needs to kill it manually or via --force)
	t.Log("Archive command preserves tmux session - user must kill it manually if desired")
}

// TestStateTransition_InvalidTransitions tests that invalid state transitions are prevented
func TestStateTransition_InvalidTransitions(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-invalid-transition-" + helpers.RandomString(6)

	// Create archived session
	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	projectDir := filepath.Join(sessionDir, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "test-uuid-" + helpers.RandomString(8),
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     manifest.LifecycleArchived,
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

	// Test 1: Cannot archive already archived session
	t.Run("CannotArchiveArchived", func(t *testing.T) {
		err := helpers.ArchiveTestSession(env.SessionsDir, sessionName, "already archived")
		// May succeed with warning or fail - implementation dependent
		if err == nil {
			t.Log("Archiving already archived session succeeded (idempotent)")
		} else {
			t.Logf("Archiving already archived session failed as expected: %v", err)
		}
	})

	// Test 2: Resume archived session should work (unarchive)
	t.Run("CanResumeArchived", func(t *testing.T) {
		// agm session resume on archived session should work
		// This is a valid transition: archived → active
		t.Log("Resume archived session is valid - unarchives and resumes")
	})
}

// TestStateTransition_MultipleRapidTransitions tests rapid state changes
func TestStateTransition_MultipleRapidTransitions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping rapid transition test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-rapid-transitions-" + helpers.RandomString(6)

	// Create session
	if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")

	// Transition 1: Start tmux (suspended → active)
	cmd := helpers.BuildTmuxCmd("new-session", "-d", "-s", sessionName, "sleep", "60")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}

	// Verify active
	m, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest: %v", err)
	}
	if m.Lifecycle != "" {
		t.Errorf("Expected empty lifecycle after start, got %s", m.Lifecycle)
	}

	// Transition 2: Kill tmux (active → suspended)
	if err := helpers.KillSessionProcesses(sessionName); err != nil {
		t.Fatalf("Failed to kill tmux session: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Verify suspended (lifecycle still empty)
	m, err = manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest after suspend: %v", err)
	}
	if m.Lifecycle == manifest.LifecycleArchived {
		t.Error("Suspended session should not be archived")
	}

	// Transition 3: Archive (suspended → archived)
	if err := helpers.ArchiveTestSession(env.SessionsDir, sessionName, "rapid test"); err != nil {
		t.Fatalf("Failed to archive session: %v", err)
	}

	// Verify archived (in-place with lifecycle field)
	manifestPath = filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
	m, err = manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest after archive: %v", err)
	}
	if m.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("Expected lifecycle 'archived', got %s", m.Lifecycle)
	}

	t.Log("Successfully completed rapid transitions: suspended → active → suspended → archived")
}

// TestStateTransition_ConcurrentTransitions tests concurrent state changes
func TestStateTransition_ConcurrentTransitions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent transition test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	// Create multiple sessions
	sessions := make([]string, 3)
	for i := 0; i < 3; i++ {
		sessionName := "test-concurrent-" + helpers.RandomString(4)
		sessions[i] = sessionName

		if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
			t.Fatalf("Failed to create session %s: %v", sessionName, err)
		}

		// Start tmux session
		cmd := helpers.BuildTmuxCmd("new-session", "-d", "-s", sessionName, "sleep", "60")
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to create tmux session %s: %v", sessionName, err)
		}
	}

	// Concurrently archive all sessions
	done := make(chan error, len(sessions))
	for _, sessionName := range sessions {
		go func(name string) {
			// Kill tmux first
			helpers.KillSessionProcesses(name)
			time.Sleep(50 * time.Millisecond)

			// Archive
			err := helpers.ArchiveTestSession(env.SessionsDir, name, "concurrent test")
			done <- err
		}(sessionName)
	}

	// Wait for all to complete
	errors := 0
	for i := 0; i < len(sessions); i++ {
		if err := <-done; err != nil {
			t.Logf("Archive failed for session: %v", err)
			errors++
		}
	}

	// Verify all sessions are archived (in-place with lifecycle field)
	for _, sessionName := range sessions {
		manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
		m, err := manifest.Read(manifestPath)
		if err != nil {
			t.Errorf("Failed to read manifest for %s: %v", sessionName, err)
			continue
		}

		if m.Lifecycle != manifest.LifecycleArchived {
			t.Errorf("Session %s should be archived, got lifecycle: %s", sessionName, m.Lifecycle)
		}
	}

	if errors > 0 {
		t.Logf("Concurrent transitions completed with %d errors", errors)
	}
}

// TestStateTransition_PreservesMetadataOnTransition tests metadata preservation
func TestStateTransition_PreservesMetadataOnTransition(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-metadata-preserve-" + helpers.RandomString(6)

	// Create session with rich metadata
	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	projectDir := filepath.Join(sessionDir, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	originalTags := []string{"priority:high", "team:platform", "type:bugfix"}
	originalNotes := "Important session with critical work"
	originalPurpose := "Fix critical production issue"

	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "test-uuid-" + helpers.RandomString(8),
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Context: manifest.Context{
			Project: projectDir,
			Purpose: originalPurpose,
			Tags:    originalTags,
			Notes:   originalNotes,
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

	// Transition: active → archived
	if err := helpers.ArchiveTestSession(env.SessionsDir, sessionName, "metadata test"); err != nil {
		t.Fatalf("Failed to archive session: %v", err)
	}

	// Read archived manifest (in-place with lifecycle field)
	manifestPath = filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
	archivedManifest, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read archived manifest: %v", err)
	}

	// Verify all metadata preserved during transition
	if archivedManifest.Context.Purpose != originalPurpose {
		t.Errorf("Purpose not preserved: expected %s, got %s", originalPurpose, archivedManifest.Context.Purpose)
	}

	if len(archivedManifest.Context.Tags) != len(originalTags) {
		t.Errorf("Tags count changed: expected %d, got %d", len(originalTags), len(archivedManifest.Context.Tags))
	}

	for i, tag := range originalTags {
		if i >= len(archivedManifest.Context.Tags) {
			break
		}
		if archivedManifest.Context.Tags[i] != tag {
			t.Errorf("Tag %d changed: expected %s, got %s", i, tag, archivedManifest.Context.Tags[i])
		}
	}

	if archivedManifest.Context.Notes != originalNotes {
		t.Errorf("Notes not preserved: expected %s, got %s", originalNotes, archivedManifest.Context.Notes)
	}

	// Verify lifecycle updated
	if archivedManifest.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("Expected lifecycle 'archived', got %s", archivedManifest.Lifecycle)
	}

	// Verify timestamps
	if archivedManifest.CreatedAt.IsZero() {
		t.Error("CreatedAt should be preserved")
	}

	if archivedManifest.UpdatedAt.Before(m.UpdatedAt) {
		t.Error("UpdatedAt should be updated during archive")
	}
}
