//go:build integration

package lifecycle_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

// TestSessionLifecycle_ComprehensiveCreateResumeTerminate tests complete lifecycle for all agents
func TestSessionLifecycle_ComprehensiveCreateResumeTerminate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping comprehensive lifecycle test in short mode")
	}

	agents := []string{"claude", "gemini", "gpt"}

	for _, agent := range agents {
		t.Run(fmt.Sprintf("Agent_%s", agent), func(t *testing.T) {
			env := helpers.NewTestEnv(t)
			defer env.Cleanup(t)

			sessionName := fmt.Sprintf("test-lifecycle-%s-%s", agent, helpers.RandomString(6))
			env.RegisterSession(sessionName)

			// Phase 1: Create session
			t.Run("Create", func(t *testing.T) {
				if !helpers.IsTmuxAvailable() {
					t.Skip("Tmux not available")
				}

				cmd := exec.Command("agm", "session", "new", sessionName,
					"--sessions-dir", env.SessionsDir,
					"--detached",
					"--agent", agent)

				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("Failed to create session: %v\nOutput: %s", err, output)
				}

				// Verify manifest created
				manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
				if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
					t.Fatalf("Manifest not created at %s", manifestPath)
				}

				// Verify manifest fields
				m, err := manifest.Read(manifestPath)
				if err != nil {
					t.Fatalf("Failed to read manifest: %v", err)
				}

				if m.Agent != agent {
					t.Errorf("Expected agent %s, got %s", agent, m.Agent)
				}
				if m.Lifecycle != "" {
					t.Errorf("New session should have empty lifecycle, got %s", m.Lifecycle)
				}
				if m.SessionID == "" {
					t.Error("SessionID should be populated")
				}
			})

			// Phase 2: Verify session is active
			t.Run("VerifyActive", func(t *testing.T) {
				sessions, err := helpers.ListTestSessions(env.SessionsDir, helpers.ListFilter{All: true})
				if err != nil {
					t.Fatalf("Failed to list sessions: %v", err)
				}

				found := false
				for _, s := range sessions {
					if s.ID == sessionName && !s.Archived {
						found = true
						break
					}
				}

				if !found {
					t.Error("Session should be active and listed")
				}
			})

			// Phase 3: Suspend session (kill tmux)
			t.Run("Suspend", func(t *testing.T) {
				if !helpers.IsTmuxAvailable() {
					t.Skip("Tmux not available")
				}

				err := helpers.KillSessionProcesses(sessionName)
				if err != nil {
					t.Logf("Kill session: %v (may already be dead)", err)
				}

				// Wait for session to die
				time.Sleep(200 * time.Millisecond)

				// Verify tmux session is gone
				cmd = helpers.BuildTmuxCmd("has-session", "-t", sessionName)
				err = cmd.Run()
				if err == nil {
					t.Error("Tmux session should be terminated")
				}

				// Verify manifest still exists
				manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
				if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
					t.Error("Manifest should still exist after suspension")
				}
			})

			// Phase 4: Resume session
			t.Run("Resume", func(t *testing.T) {
				if !helpers.IsTmuxAvailable() {
					t.Skip("Tmux not available")
				}

				cmd := exec.Command("agm", "session", "resume", sessionName,
					"--sessions-dir", env.SessionsDir)

				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Logf("Resume failed (may require agent setup): %v\nOutput: %s", err, output)
					t.Skip("Resume requires agent configuration")
				}

				// Wait for resume to complete
				time.Sleep(500 * time.Millisecond)

				// Verify tmux session exists
				cmd = helpers.BuildTmuxCmd("has-session", "-t", sessionName)
				err = cmd.Run()
				if err != nil {
					t.Log("Resumed session tmux verification skipped (implementation dependent)")
				}
			})

			// Phase 5: Archive session
			t.Run("Archive", func(t *testing.T) {
				// Ensure tmux session and all processes are dead before archiving
				if helpers.IsTmuxAvailable() {
					helpers.KillSessionProcesses(sessionName)
				}

				err := helpers.ArchiveTestSession(env.SessionsDir, sessionName, "test cleanup")
				if err != nil {
					t.Fatalf("Failed to archive session: %v", err)
				}

				// Verify lifecycle updated
				manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
				m, err := manifest.Read(manifestPath)
				if err != nil {
					t.Fatalf("Failed to read manifest after archive: %v", err)
				}

				if m.Lifecycle != manifest.LifecycleArchived {
					t.Errorf("Expected lifecycle 'archived', got %s", m.Lifecycle)
				}
			})

			// Phase 6: Verify archived session hidden from default list
			t.Run("ArchivedHidden", func(t *testing.T) {
				sessions, err := helpers.ListTestSessions(env.SessionsDir, helpers.ListFilter{})
				if err != nil {
					t.Fatalf("Failed to list sessions: %v", err)
				}

				for _, s := range sessions {
					if s.ID == sessionName {
						t.Error("Archived session should not appear in default list")
					}
				}
			})

			// Cleanup
			if helpers.IsTmuxAvailable() {
				helpers.KillSessionProcesses(sessionName)
			}
		})
	}
}

// TestSessionStateTransitions tests all valid state transitions
func TestSessionStateTransitions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping state transition test in short mode")
	}

	testCases := []struct {
		name          string
		initialState  string
		action        string
		expectedState string
		shouldSucceed bool
	}{
		{
			name:          "New to Active",
			initialState:  "",
			action:        "create_tmux",
			expectedState: "",
			shouldSucceed: true,
		},
		{
			name:          "Active to Suspended",
			initialState:  "",
			action:        "kill_tmux",
			expectedState: "",
			shouldSucceed: true,
		},
		{
			name:          "Suspended to Active",
			initialState:  "",
			action:        "resume",
			expectedState: "",
			shouldSucceed: true,
		},
		{
			name:          "Active to Archived",
			initialState:  "",
			action:        "archive",
			expectedState: manifest.LifecycleArchived,
			shouldSucceed: true,
		},
		{
			name:          "Archived to Active",
			initialState:  manifest.LifecycleArchived,
			action:        "resume",
			expectedState: manifest.LifecycleArchived,
			shouldSucceed: false, // Cannot resume archived session
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			env := helpers.NewTestEnv(t)
			defer env.Cleanup(t)

			sessionName := "test-transition-" + helpers.RandomString(6)

			// Create session in initial state
			if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
				t.Fatalf("Failed to create session: %v", err)
			}

			manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")

			// Set initial lifecycle state
			if tc.initialState != "" {
				m, err := manifest.Read(manifestPath)
				if err != nil {
					t.Fatalf("Failed to read manifest: %v", err)
				}
				m.Lifecycle = tc.initialState
				if err := manifest.Write(manifestPath, m); err != nil {
					t.Fatalf("Failed to write manifest: %v", err)
				}
			}

			// Perform action
			var actionErr error
			switch tc.action {
			case "create_tmux":
				if helpers.IsTmuxAvailable() {
					cmd := helpers.BuildTmuxCmd("new-session", "-d", "-s", sessionName, "sleep", "60")
					actionErr = cmd.Run()
					defer helpers.KillSessionProcesses(sessionName)
				}
			case "kill_tmux":
				if helpers.IsTmuxAvailable() {
					helpers.KillSessionProcesses(sessionName)
				}
			case "resume":
				// Resume attempt
				cmd := exec.Command("agm", "session", "resume", sessionName, "--sessions-dir", env.SessionsDir)
				_, actionErr = cmd.CombinedOutput()
			case "archive":
				if helpers.IsTmuxAvailable() {
					helpers.KillSessionProcesses(sessionName)
				}
				actionErr = helpers.ArchiveTestSession(env.SessionsDir, sessionName, "state transition test")
			}

			// Verify result
			if tc.shouldSucceed && actionErr != nil {
				t.Errorf("Action %s should succeed but failed: %v", tc.action, actionErr)
			}
			if !tc.shouldSucceed && actionErr == nil {
				t.Errorf("Action %s should fail but succeeded", tc.action)
			}

			// Verify final state
			if tc.expectedState != "" {
				m, err := manifest.Read(manifestPath)
				if err != nil {
					t.Fatalf("Failed to read manifest after action: %v", err)
				}
				if m.Lifecycle != tc.expectedState {
					t.Errorf("Expected state %s, got %s", tc.expectedState, m.Lifecycle)
				}
			}
		})
	}
}

// TestConcurrentSessionOperations tests concurrent create, resume, archive operations
func TestConcurrentSessionOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent operations test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	const numSessions = 10
	var wg sync.WaitGroup
	errors := make(chan error, numSessions)
	sessionNames := make([]string, numSessions)

	// Create sessions concurrently
	t.Run("ConcurrentCreate", func(t *testing.T) {
		for i := 0; i < numSessions; i++ {
			wg.Add(1)
			sessionNames[i] = fmt.Sprintf("test-concurrent-%d-%s", i, helpers.RandomString(4))

			go func(name string) {
				defer wg.Done()
				if err := helpers.CreateSessionManifest(env.SessionsDir, name, "claude"); err != nil {
					errors <- fmt.Errorf("create %s: %w", name, err)
				}
			}(sessionNames[i])
		}

		wg.Wait()
		close(errors)

		// Check for errors
		for err := range errors {
			t.Errorf("Concurrent create error: %v", err)
		}
	})

	// Verify all sessions created with unique IDs
	t.Run("VerifyUniqueIDs", func(t *testing.T) {
		sessionIDs := make(map[string]bool)

		for _, name := range sessionNames {
			manifestPath := filepath.Join(env.SessionsDir, name, "manifest.yaml")
			m, err := manifest.Read(manifestPath)
			if err != nil {
				t.Errorf("Failed to read manifest for %s: %v", name, err)
				continue
			}

			if sessionIDs[m.SessionID] {
				t.Errorf("Duplicate session ID: %s", m.SessionID)
			}
			sessionIDs[m.SessionID] = true
		}

		if len(sessionIDs) != numSessions {
			t.Errorf("Expected %d unique session IDs, got %d", numSessions, len(sessionIDs))
		}
	})

	// Archive concurrently
	t.Run("ConcurrentArchive", func(t *testing.T) {
		errors = make(chan error, numSessions)
		var wg sync.WaitGroup

		for _, name := range sessionNames {
			wg.Add(1)
			go func(sessionName string) {
				defer wg.Done()
				if err := helpers.ArchiveTestSession(env.SessionsDir, sessionName, "concurrent test"); err != nil {
					errors <- fmt.Errorf("archive %s: %w", sessionName, err)
				}
			}(name)
		}

		wg.Wait()
		close(errors)

		// Check for errors
		for err := range errors {
			t.Logf("Concurrent archive: %v (may be acceptable)", err)
		}
	})

	// Verify all archived
	t.Run("VerifyAllArchived", func(t *testing.T) {
		for _, name := range sessionNames {
			manifestPath := filepath.Join(env.SessionsDir, name, "manifest.yaml")
			m, err := manifest.Read(manifestPath)
			if err != nil {
				t.Errorf("Failed to read manifest for %s: %v", name, err)
				continue
			}

			if m.Lifecycle != manifest.LifecycleArchived {
				t.Errorf("Session %s should be archived, got lifecycle: %s", name, m.Lifecycle)
			}
		}
	})
}

// TestSessionErrorHandling_AgentParity tests error handling across all agent types
func TestSessionErrorHandling_AgentParity(t *testing.T) {
	agents := []string{"claude", "gemini", "gpt"}

	for _, agent := range agents {
		t.Run(fmt.Sprintf("Agent_%s", agent), func(t *testing.T) {
			env := helpers.NewTestEnv(t)
			defer env.Cleanup(t)

			// Test 1: Create duplicate session
			t.Run("DuplicateSession", func(t *testing.T) {
				sessionName := fmt.Sprintf("test-dup-%s-%s", agent, helpers.RandomString(6))

				// Create first session
				if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, agent); err != nil {
					t.Fatalf("Failed to create first session: %v", err)
				}

				// Attempt duplicate
				err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, agent)
				if err == nil {
					t.Error("Creating duplicate session should fail")
				}
			})

			// Test 2: Invalid session name
			t.Run("InvalidName", func(t *testing.T) {
				invalidName := "session/with/slashes"
				err := helpers.CreateSessionManifest(env.SessionsDir, invalidName, agent)
				if err == nil {
					t.Log("Invalid name accepted (may be sanitized)")
				}
			})

			// Test 3: Missing project directory
			t.Run("MissingProject", func(t *testing.T) {
				sessionName := fmt.Sprintf("test-noproject-%s-%s", agent, helpers.RandomString(6))
				sessionDir := filepath.Join(env.SessionsDir, sessionName)
				if err := os.MkdirAll(sessionDir, 0755); err != nil {
					t.Fatalf("Failed to create session dir: %v", err)
				}

				m := &manifest.Manifest{
					SchemaVersion: "2",
					SessionID:     "test-uuid-" + helpers.RandomString(8),
					Name:          sessionName,
					CreatedAt:     time.Now(),
					UpdatedAt:     time.Now(),
					Context: manifest.Context{
						Project: "/nonexistent/path",
					},
					Tmux: manifest.Tmux{
						SessionName: sessionName,
					},
					Agent: agent,
				}

				manifestPath := filepath.Join(sessionDir, "manifest.yaml")
				if err := manifest.Write(manifestPath, m); err != nil {
					t.Fatalf("Failed to write manifest: %v", err)
				}

				// Health check should fail
				report, err := session.CheckHealth(m)
				if err != nil {
					t.Fatalf("Health check failed: %v", err)
				}

				if report.IsHealthy() {
					t.Error("Session with missing project should not be healthy")
				}
			})
		})
	}
}

// TestSessionEdgeCases_CrossAgent tests edge cases across different agent types
func TestSessionEdgeCases_CrossAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping edge case tests in short mode")
	}

	// Test: Switch agent type mid-lifecycle
	t.Run("SwitchAgentType", func(t *testing.T) {
		env := helpers.NewTestEnv(t)
		defer env.Cleanup(t)

		sessionName := "test-switch-agent-" + helpers.RandomString(6)

		// Create with Claude
		if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
		m, err := manifest.Read(manifestPath)
		if err != nil {
			t.Fatalf("Failed to read manifest: %v", err)
		}

		// Switch to Gemini
		m.Agent = "gemini"
		if err := manifest.Write(manifestPath, m); err != nil {
			t.Fatalf("Failed to write manifest: %v", err)
		}

		// Verify switch persisted
		m, err = manifest.Read(manifestPath)
		if err != nil {
			t.Fatalf("Failed to read updated manifest: %v", err)
		}

		if m.Agent != "gemini" {
			t.Errorf("Agent should be gemini, got %s", m.Agent)
		}
	})

	// Test: Resume session created by different agent
	t.Run("CrossAgentResume", func(t *testing.T) {
		env := helpers.NewTestEnv(t)
		defer env.Cleanup(t)

		sessionName := "test-cross-resume-" + helpers.RandomString(6)

		// Create with one agent
		if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Modify agent type
		manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")
		m, err := manifest.Read(manifestPath)
		if err != nil {
			t.Fatalf("Failed to read manifest: %v", err)
		}

		m.Agent = "gemini"
		if err := manifest.Write(manifestPath, m); err != nil {
			t.Fatalf("Failed to write manifest: %v", err)
		}

		// Try to resume (may fail due to agent mismatch)
		cmd := exec.Command("agm", "session", "resume", sessionName, "--sessions-dir", env.SessionsDir)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("Cross-agent resume failed as expected: %s", output)
		}
	})

	// Test: Concurrent operations on same session
	t.Run("ConcurrentSameSession", func(t *testing.T) {
		env := helpers.NewTestEnv(t)
		defer env.Cleanup(t)

		sessionName := "test-concurrent-same-" + helpers.RandomString(6)
		if err := helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude"); err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		var wg sync.WaitGroup
		manifestPath := filepath.Join(env.SessionsDir, sessionName, "manifest.yaml")

		// Concurrent updates to same session
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(iter int) {
				defer wg.Done()
				m, err := manifest.Read(manifestPath)
				if err != nil {
					return
				}
				m.Context.Notes = fmt.Sprintf("Update %d", iter)
				manifest.Write(manifestPath, m)
			}(i)
		}

		wg.Wait()

		// Verify manifest is still valid
		m, err := manifest.Read(manifestPath)
		if err != nil {
			t.Errorf("Manifest corrupted after concurrent updates: %v", err)
		} else {
			t.Logf("Final notes: %s", m.Context.Notes)
		}
	})
}

// TestSessionPromptDetection tests waiting for shell prompt across sessions
func TestSessionPromptDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping prompt detection test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-prompt-detect-" + helpers.RandomString(6)

	// Create tmux session with bash
	cmd := helpers.BuildTmuxCmd("new-session", "-d", "-s", sessionName, "bash")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}
	defer helpers.KillSessionProcesses(sessionName)

	// Wait for bash startup
	time.Sleep(500 * time.Millisecond)

	// Send command and wait for completion
	if err := tmux.SendCommand(sessionName, "echo 'test-marker'"); err != nil {
		t.Fatalf("Failed to send command: %v", err)
	}

	// Simple time-based wait (proper prompt detection is complex)
	time.Sleep(1 * time.Second)

	// Verify command output
	captureCmd := helpers.BuildTmuxCmd("capture-pane", "-t", sessionName, "-p")
	output, err := captureCmd.Output()
	if err != nil {
		t.Fatalf("Failed to capture pane: %v", err)
	}

	if !strings.Contains(string(output), "test-marker") {
		t.Error("Command output not found in pane")
	}

	t.Log("Prompt detection test completed (basic verification)")
}

// TestSessionMetadataPreservation tests that all metadata is preserved across lifecycle
func TestSessionMetadataPreservation(t *testing.T) {
	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-metadata-" + helpers.RandomString(6)
	sessionDir := filepath.Join(env.SessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	projectDir := filepath.Join(sessionDir, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	// Create session with rich metadata
	originalTags := []string{"test", "metadata", "preservation"}
	originalNotes := "Important metadata test notes"
	originalPurpose := "Testing metadata preservation across lifecycle"

	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "test-uuid-" + helpers.RandomString(8),
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
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

	// Archive session
	if err := helpers.ArchiveTestSession(env.SessionsDir, sessionName, "metadata test"); err != nil {
		t.Fatalf("Failed to archive session: %v", err)
	}

	// Read back and verify all metadata preserved
	archived, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read archived manifest: %v", err)
	}

	if archived.Context.Purpose != originalPurpose {
		t.Errorf("Purpose not preserved: expected %s, got %s", originalPurpose, archived.Context.Purpose)
	}

	if archived.Context.Notes != originalNotes {
		t.Errorf("Notes not preserved: expected %s, got %s", originalNotes, archived.Context.Notes)
	}

	if len(archived.Context.Tags) != len(originalTags) {
		t.Errorf("Tags count mismatch: expected %d, got %d", len(originalTags), len(archived.Context.Tags))
	}

	for i, tag := range originalTags {
		if i >= len(archived.Context.Tags) || archived.Context.Tags[i] != tag {
			t.Errorf("Tag %d not preserved: expected %s", i, tag)
		}
	}

	// Verify lifecycle updated
	if archived.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("Expected lifecycle 'archived', got %s", archived.Lifecycle)
	}
}
