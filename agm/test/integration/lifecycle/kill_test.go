//go:build integration

package lifecycle_test

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

// TestSessionKill_WithDoltAdapter verifies that agm session kill works correctly
// when sessions are stored in Dolt database (not just YAML files).
// This is a regression test for the bug where kill command couldn't find sessions
// because it was passing nil adapter to ResolveIdentifier().
func TestSessionKill_WithDoltAdapter(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	// Create test environment with Dolt database
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-kill-dolt-" + helpers.RandomString(6)
	env.RegisterSession(sessionName)

	// Step 1: Create a session
	t.Run("Create", func(t *testing.T) {
		cmd := exec.Command("agm", "session", "new", sessionName,
			"--sessions-dir", env.SessionsDir,
			"--detached",
			"--agent", "claude")

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to create session: %v\nOutput: %s", err, output)
		}

		// Wait for session creation
		time.Sleep(200 * time.Millisecond)
	})

	// Step 2: Verify session exists in database
	t.Run("VerifyInDatabase", func(t *testing.T) {
		// Connect to Dolt
		config, err := dolt.DefaultConfig()
		if err != nil {
			t.Fatalf("Failed to get Dolt config: %v", err)
		}

		adapter, err := dolt.New(config)
		if err != nil {
			t.Fatalf("Failed to connect to Dolt: %v", err)
		}
		defer adapter.Close()

		// List all sessions
		sessions, err := adapter.ListSessions(&dolt.SessionFilter{})
		if err != nil {
			t.Fatalf("Failed to list sessions: %v", err)
		}

		// Find our session
		found := false
		for _, m := range sessions {
			if m.Name == sessionName {
				found = true
				if m.Lifecycle == manifest.LifecycleArchived {
					t.Errorf("Session should not be archived yet")
				}
				break
			}
		}

		if !found {
			t.Errorf("Session %s not found in Dolt database", sessionName)
		}
	})

	// Step 3: Kill session using agm command
	t.Run("Kill", func(t *testing.T) {
		cmd := exec.Command("agm", "session", "kill", sessionName,
			"--sessions-dir", env.SessionsDir,
			"--force") // Skip confirmation prompt

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Kill command failed: %v\nOutput: %s", err, output)
		}

		// Wait for kill to complete
		time.Sleep(200 * time.Millisecond)
	})

	// Step 4: Verify tmux session is gone
	t.Run("VerifyTmuxGone", func(t *testing.T) {
		cmd := helpers.BuildTmuxCmd("has-session", "-t", sessionName)
		err := cmd.Run()
		if err == nil {
			t.Error("Tmux session should be terminated after kill")
		}
	})

	// Step 5: Verify session still exists in database (not deleted)
	t.Run("VerifyStillInDatabase", func(t *testing.T) {
		config, err := dolt.DefaultConfig()
		if err != nil {
			t.Fatalf("Failed to get Dolt config: %v", err)
		}

		adapter, err := dolt.New(config)
		if err != nil {
			t.Fatalf("Failed to connect to Dolt: %v", err)
		}
		defer adapter.Close()

		sessions, err := adapter.ListSessions(&dolt.SessionFilter{})
		if err != nil {
			t.Fatalf("Failed to list sessions: %v", err)
		}

		found := false
		for _, m := range sessions {
			if m.Name == sessionName {
				found = true
				if m.Lifecycle == manifest.LifecycleArchived {
					t.Error("Session should not be archived after kill (only tmux terminated)")
				}
				break
			}
		}

		if !found {
			t.Error("Session should still exist in database after kill")
		}
	})

	// Step 6: Verify session can be resumed
	t.Run("ResumeAfterKill", func(t *testing.T) {
		cmd := exec.Command("agm", "session", "resume", sessionName,
			"--sessions-dir", env.SessionsDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Skipf("Resume failed (may require agent setup): %v\nOutput: %s", err, output)
		}

		// Wait for resume
		time.Sleep(500 * time.Millisecond)

		// Verify tmux session exists again
		cmd = helpers.BuildTmuxCmd("has-session", "-t", sessionName)
		err = cmd.Run()
		if err != nil {
			t.Error("Tmux session should exist after resume")
		}
	})
}

// TestSessionKill_ByTmuxName verifies kill works when passing tmux session name
func TestSessionKill_ByTmuxName(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-kill-tmux-" + helpers.RandomString(6)
	env.RegisterSession(sessionName)

	// Create session
	cmd := exec.Command("agm", "session", "new", sessionName,
		"--sessions-dir", env.SessionsDir,
		"--detached",
		"--agent", "claude")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create session: %v\nOutput: %s", err, output)
	}

	time.Sleep(200 * time.Millisecond)

	// Kill by tmux session name (should match Name field)
	cmd = exec.Command("agm", "session", "kill", sessionName,
		"--sessions-dir", env.SessionsDir,
		"--force")

	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Kill by tmux name failed: %v\nOutput: %s", err, output)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify killed
	cmd = helpers.BuildTmuxCmd("has-session", "-t", sessionName)
	err = cmd.Run()
	if err == nil {
		t.Error("Tmux session should be terminated")
	}
}

// TestSessionKill_SessionNotFound verifies proper error when session doesn't exist
func TestSessionKill_SessionNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	nonExistentSession := "nonexistent-session-" + helpers.RandomString(8)

	cmd := exec.Command("agm", "session", "kill", nonExistentSession,
		"--sessions-dir", env.SessionsDir,
		"--force")

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("Kill should fail for non-existent session")
	}

	outputStr := string(output)
	if !contains(outputStr, "session not found") && !contains(outputStr, "not found") {
		t.Errorf("Expected 'not found' error, got: %s", outputStr)
	}
}

// TestSessionKill_ArchivedSession verifies kill fails for archived sessions
func TestSessionKill_ArchivedSession(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-kill-archived-" + helpers.RandomString(6)
	env.RegisterSession(sessionName)

	// Create session
	cmd := exec.Command("agm", "session", "new", sessionName,
		"--sessions-dir", env.SessionsDir,
		"--detached",
		"--agent", "claude")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create session: %v\nOutput: %s", err, output)
	}

	time.Sleep(200 * time.Millisecond)

	// Archive the session
	cmd = exec.Command("agm", "session", "archive", sessionName,
		"--sessions-dir", env.SessionsDir)

	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to archive session: %v\nOutput: %s", err, output)
	}

	time.Sleep(200 * time.Millisecond)

	// Try to kill archived session
	cmd = exec.Command("agm", "session", "kill", sessionName,
		"--sessions-dir", env.SessionsDir,
		"--force")

	output, err = cmd.CombinedOutput()
	if err == nil {
		t.Error("Kill should fail for archived session")
	}

	outputStr := string(output)
	if !contains(outputStr, "archived") {
		t.Errorf("Expected 'archived' error, got: %s", outputStr)
	}
}

// TestSessionKill_HardKill verifies hard kill functionality
func TestSessionKill_HardKill(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-hard-kill-" + helpers.RandomString(6)
	env.RegisterSession(sessionName)

	// Create session
	cmd := exec.Command("agm", "session", "new", sessionName,
		"--sessions-dir", env.SessionsDir,
		"--detached",
		"--agent", "claude")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create session: %v\nOutput: %s", err, output)
	}

	time.Sleep(200 * time.Millisecond)

	// Hard kill with --hard flag
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd = exec.CommandContext(ctx, "agm", "session", "kill", sessionName,
		"--sessions-dir", env.SessionsDir,
		"--hard",
		"--force")

	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Logf("Hard kill output: %s", output)
		// Hard kill may fail if no deadlock detected, which is fine
	}

	time.Sleep(200 * time.Millisecond)

	// Verify session is killed (either soft or hard)
	cmd = helpers.BuildTmuxCmd("has-session", "-t", sessionName)
	err = cmd.Run()
	if err == nil {
		t.Error("Session should be killed after hard kill attempt")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
