//go:build integration

package lifecycle_test

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

// TestCleanup_NoSessionsLeftBehind verifies that test cleanup removes all test sessions
// This is a regression test for the bug where tests left sessions on the main AGM socket,
// causing random tmux window switching.
func TestCleanup_NoSessionsLeftBehind(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cleanup regression test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	// Create a test environment
	env := helpers.NewTestEnv(t)

	// List sessions before cleanup
	sessionsBefore, err := helpers.ListTmuxSessions(env.TmuxPrefix)
	if err != nil {
		t.Fatalf("Failed to list sessions before test: %v", err)
	}

	// Create some test sessions
	sessionName1 := env.UniqueSessionName("cleanup-test")
	sessionName2 := env.UniqueSessionName("cleanup-test")
	sessionName3 := env.UniqueSessionName("cleanup-test")

	// Create tmux sessions
	for _, name := range []string{sessionName1, sessionName2, sessionName3} {
		cmd := helpers.BuildTmuxCmd("new-session", "-d", "-s", name, "sleep", "60")
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to create test session %s: %v", name, err)
		}
	}

	// Verify sessions exist
	sessions, err := helpers.ListTmuxSessions(env.TmuxPrefix)
	if err != nil {
		t.Fatalf("Failed to list sessions after creation: %v", err)
	}

	expectedCount := len(sessionsBefore) + 3
	if len(sessions) != expectedCount {
		t.Errorf("Expected %d sessions with prefix %s, got %d", expectedCount, env.TmuxPrefix, len(sessions))
	}

	// Now cleanup
	if err := env.Cleanup(t); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify all test sessions are gone
	sessionsAfter, err := helpers.ListTmuxSessions(env.TmuxPrefix)
	if err != nil {
		t.Fatalf("Failed to list sessions after cleanup: %v", err)
	}

	if len(sessionsAfter) != 0 {
		t.Errorf("Expected 0 sessions after cleanup, got %d: %v", len(sessionsAfter), sessionsAfter)
		t.Error("Test sessions were not cleaned up! This can cause random tmux window switching.")
	}
}

// TestCleanup_MultipleConcurrentTests simulates multiple tests running concurrently
// and verifies that cleanup for each test only affects its own sessions.
func TestCleanup_MultipleConcurrentTests(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent cleanup test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	// Create two separate test environments (simulating concurrent tests)
	env1 := helpers.NewTestEnv(t)
	env2 := helpers.NewTestEnv(t)

	// Create sessions for env1
	session1 := env1.UniqueSessionName("concurrent")
	cmd1 := helpers.BuildTmuxCmd("new-session", "-d", "-s", session1, "sleep", "60")
	if err := cmd1.Run(); err != nil {
		t.Fatalf("Failed to create session for env1: %v", err)
	}

	// Create sessions for env2
	session2 := env2.UniqueSessionName("concurrent")
	cmd2 := helpers.BuildTmuxCmd("new-session", "-d", "-s", session2, "sleep", "60")
	if err := cmd2.Run(); err != nil {
		t.Fatalf("Failed to create session for env2: %v", err)
	}

	// Verify both sessions exist
	sessions, err := helpers.ListTmuxSessions(env1.TmuxPrefix)
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	foundSession1 := false
	foundSession2 := false
	for _, s := range sessions {
		if s == session1 {
			foundSession1 = true
		}
		if s == session2 {
			foundSession2 = true
		}
	}

	if !foundSession1 || !foundSession2 {
		t.Errorf("Not all test sessions found. session1=%v, session2=%v", foundSession1, foundSession2)
	}

	// Cleanup both environments
	if err := env1.Cleanup(t); err != nil {
		t.Fatalf("env1.Cleanup failed: %v", err)
	}
	if err := env2.Cleanup(t); err != nil {
		t.Fatalf("env2.Cleanup failed: %v", err)
	}

	// Verify all test sessions are cleaned up
	sessionsAfter, err := helpers.ListTmuxSessions(env1.TmuxPrefix)
	if err != nil {
		t.Fatalf("Failed to list sessions after cleanup: %v", err)
	}

	if len(sessionsAfter) != 0 {
		t.Errorf("Expected 0 sessions after cleanup, got %d: %v", len(sessionsAfter), sessionsAfter)
	}
}
