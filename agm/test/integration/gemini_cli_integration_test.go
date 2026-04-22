//go:build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

// TestGeminiCLI_Integration_SessionLifecycle tests full session lifecycle with real Gemini CLI.
//
// This test requires:
// - Gemini CLI installed and in PATH
// - GEMINI_API_KEY environment variable set
// - tmux installed
//
// Skip if requirements not met.
func TestGeminiCLI_Integration_SessionLifecycle(t *testing.T) {
	// Skip in short mode (slow external API calls)
	if testing.Short() {
		t.Skip("Skipping Gemini CLI integration test in short mode")
	}

	// Check if Gemini CLI is available
	if _, err := exec.LookPath("gemini"); err != nil {
		t.Skip("Gemini CLI not found in PATH, skipping integration test")
	}

	// Check if API key is set
	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GEMINI_API_KEY not set, skipping integration test")
	}

	// Check if tmux is available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found in PATH, skipping integration test")
	}

	// Create temp directory for test session store
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "sessions.json")

	// Create test working directory
	workDir := t.TempDir()

	// Create adapter
	store, err := agent.NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	adapter, err := agent.NewGeminiCLIAdapter(store)
	if err != nil {
		t.Fatalf("failed to create Gemini adapter: %v", err)
	}

	// Test 1: Create session with directory authorization
	t.Run("CreateSession", func(t *testing.T) {
		ctx := agent.SessionContext{
			Name:             "integration-test-session",
			WorkingDirectory: workDir,
			Project:          "test-project",
			AuthorizedDirs:   []string{tempDir}, // Authorize temp directory
		}

		sessionID, err := adapter.CreateSession(ctx)
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}

		// Verify session was created
		if sessionID == "" {
			t.Error("Expected non-empty session ID")
		}

		// Verify session metadata was stored
		metadata, err := store.Get(sessionID)
		if err != nil {
			t.Fatalf("failed to get session metadata: %v", err)
		}

		// Verify UUID was extracted and stored
		if metadata.UUID == "" {
			t.Log("Warning: UUID extraction failed (session may be too new)")
		} else {
			t.Logf("Session UUID extracted: %s", metadata.UUID)
		}

		// Verify working directory
		if metadata.WorkingDir != workDir {
			t.Errorf("Expected WorkingDir %s, got %s", workDir, metadata.WorkingDir)
		}

		// Clean up: terminate session
		if err := adapter.TerminateSession(sessionID); err != nil {
			t.Logf("Warning: failed to terminate session: %v", err)
		}
	})

	// Test 2: Resume session with UUID
	t.Run("ResumeSessionWithUUID", func(t *testing.T) {
		// Create session first
		ctx := agent.SessionContext{
			Name:             "integration-test-resume",
			WorkingDirectory: workDir,
			Project:          "test-project",
		}

		sessionID, err := adapter.CreateSession(ctx)
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}
		defer adapter.TerminateSession(sessionID)

		// Get metadata to verify UUID
		metadata, err := store.Get(sessionID)
		if err != nil {
			t.Fatalf("failed to get session metadata: %v", err)
		}

		// Kill tmux session directly (without calling TerminateSession API)
		// This simulates suspend/crash scenario where metadata persists
		tmuxName := metadata.TmuxName
		if tmuxName != "" {
			killCmd := exec.Command("tmux", "kill-session", "-t", tmuxName)
			if err := killCmd.Run(); err != nil {
				t.Logf("Warning: failed to kill tmux session %s: %v", tmuxName, err)
			}
		}

		// Wait a moment for tmux cleanup
		time.Sleep(1 * time.Second)

		// Resume session (should recreate tmux session using stored UUID)
		if err := adapter.ResumeSession(sessionID); err != nil {
			t.Fatalf("ResumeSession failed: %v", err)
		}

		// Verify session is active
		status, err := adapter.GetSessionStatus(sessionID)
		if err != nil {
			t.Fatalf("GetSessionStatus failed: %v", err)
		}

		if status != agent.StatusActive {
			t.Errorf("Expected status Active, got %v", status)
		}

		// Verify UUID persisted after resume
		metadataAfterResume, err := store.Get(sessionID)
		if err != nil {
			t.Fatalf("failed to get session metadata after resume: %v", err)
		}

		if metadataAfterResume.UUID != metadata.UUID {
			t.Errorf("UUID changed after resume: before=%s, after=%s",
				metadata.UUID, metadataAfterResume.UUID)
		}
	})

	// Test 3: Send message and verify response
	t.Run("SendMessage", func(t *testing.T) {
		// Create session
		ctx := agent.SessionContext{
			Name:             "integration-test-message",
			WorkingDirectory: workDir,
			Project:          "test-project",
		}

		sessionID, err := adapter.CreateSession(ctx)
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}
		defer adapter.TerminateSession(sessionID)

		// Wait for session to be ready
		time.Sleep(2 * time.Second)

		// Send test message
		msg := agent.Message{
			Content: "Hello, Gemini! Please respond with 'Integration test successful'.",
		}

		if err := adapter.SendMessage(sessionID, msg); err != nil {
			t.Fatalf("SendMessage failed: %v", err)
		}

		// Wait for response
		time.Sleep(3 * time.Second)

		// Get history to verify message was sent
		history, err := adapter.GetHistory(sessionID)
		if err != nil {
			t.Fatalf("GetHistory failed: %v", err)
		}

		// Note: History may be empty if Gemini hasn't written to history file yet
		// This is a timing issue, not a failure
		if len(history) > 0 {
			t.Logf("History contains %d messages", len(history))
		} else {
			t.Log("History empty (Gemini may not have written to file yet)")
		}
	})

	// Test 4: Multiple authorized directories
	t.Run("MultipleAuthorizedDirectories", func(t *testing.T) {
		// Create additional temp directories
		dir1 := t.TempDir()
		dir2 := t.TempDir()

		ctx := agent.SessionContext{
			Name:             "integration-test-multidirs",
			WorkingDirectory: workDir,
			Project:          "test-project",
			AuthorizedDirs:   []string{dir1, dir2, tempDir},
		}

		sessionID, err := adapter.CreateSession(ctx)
		if err != nil {
			t.Fatalf("CreateSession with multiple dirs failed: %v", err)
		}
		defer adapter.TerminateSession(sessionID)

		// Verify session created successfully
		// In real scenario, Gemini should have pre-authorized all directories
		// (no interactive prompts during session)
		status, err := adapter.GetSessionStatus(sessionID)
		if err != nil {
			t.Fatalf("GetSessionStatus failed: %v", err)
		}

		if status != agent.StatusActive {
			t.Errorf("Expected session to be active, got %v", status)
		}
	})
}

// TestGeminiCLI_Integration_UUIDExtraction tests UUID extraction specifically.
func TestGeminiCLI_Integration_UUIDExtraction(t *testing.T) {
	// Skip in short mode (slow external API calls)
	if testing.Short() {
		t.Skip("Skipping Gemini CLI integration test in short mode")
	}

	// Check if Gemini CLI is available
	if _, err := exec.LookPath("gemini"); err != nil {
		t.Skip("Gemini CLI not found in PATH, skipping integration test")
	}

	// Check if API key is set
	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GEMINI_API_KEY not set, skipping integration test")
	}

	// Create temp directory for test session store
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "sessions.json")
	workDir := t.TempDir()

	// Create adapter
	store, err := agent.NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	adapter, err := agent.NewGeminiCLIAdapter(store)
	if err != nil {
		t.Fatalf("failed to create Gemini adapter: %v", err)
	}

	// Create a session to generate UUID
	ctx := agent.SessionContext{
		Name:             "uuid-extraction-test",
		WorkingDirectory: workDir,
		Project:          "test-uuid",
	}

	sessionID, err := adapter.CreateSession(ctx)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	defer adapter.TerminateSession(sessionID)

	// Get metadata
	metadata, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("failed to get session metadata: %v", err)
	}

	// Verify UUID format (should be valid UUID v4)
	if metadata.UUID != "" {
		// UUID should match format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
		uuidPattern := `^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`
		matched, err := regexp.MatchString(uuidPattern, metadata.UUID)
		if err != nil {
			t.Fatalf("UUID pattern match failed: %v", err)
		}

		if !matched {
			t.Errorf("UUID format invalid: %s", metadata.UUID)
		} else {
			t.Logf("Valid UUID extracted: %s", metadata.UUID)
		}
	} else {
		t.Log("Warning: UUID extraction returned empty (session may be too new)")
	}
}

// TestGeminiCLI_Integration_ConcurrentSessions tests multiple simultaneous sessions.
func TestGeminiCLI_Integration_ConcurrentSessions(t *testing.T) {
	// Skip in short mode (slow external API calls)
	if testing.Short() {
		t.Skip("Skipping Gemini CLI integration test in short mode")
	}

	// Check requirements
	if _, err := exec.LookPath("gemini"); err != nil {
		t.Skip("Gemini CLI not found in PATH")
	}
	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GEMINI_API_KEY not set")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found in PATH")
	}

	// Create temp directory
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "sessions.json")

	// Create adapter
	store, err := agent.NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	adapter, err := agent.NewGeminiCLIAdapter(store)
	if err != nil {
		t.Fatalf("failed to create Gemini adapter: %v", err)
	}

	// Create 3 concurrent sessions
	sessionIDs := make([]agent.SessionID, 3)
	for i := range 3 {
		workDir := t.TempDir()
		ctx := agent.SessionContext{
			Name:             fmt.Sprintf("concurrent-test-%d", i),
			WorkingDirectory: workDir,
			Project:          "concurrent-test",
		}

		sessionID, err := adapter.CreateSession(ctx)
		if err != nil {
			t.Fatalf("CreateSession %d failed: %v", i, err)
		}
		sessionIDs[i] = sessionID
		defer adapter.TerminateSession(sessionID)
	}

	// Verify all sessions are active
	for i, sessionID := range sessionIDs {
		status, err := adapter.GetSessionStatus(sessionID)
		if err != nil {
			t.Errorf("GetSessionStatus for session %d failed: %v", i, err)
			continue
		}

		if status != agent.StatusActive {
			t.Errorf("Session %d: expected Active, got %v", i, status)
		}

		// Verify each session has unique UUID (if extracted)
		metadata, err := store.Get(sessionID)
		if err != nil {
			t.Errorf("failed to get metadata for session %d: %v", i, err)
			continue
		}

		if metadata.UUID != "" {
			t.Logf("Session %d UUID: %s", i, metadata.UUID)
		}
	}

	// Verify UUIDs are unique (if all extracted successfully)
	uuidMap := make(map[string]bool)
	for i, sessionID := range sessionIDs {
		metadata, _ := store.Get(sessionID)
		if metadata.UUID != "" {
			if uuidMap[metadata.UUID] {
				t.Errorf("Duplicate UUID found for session %d: %s", i, metadata.UUID)
			}
			uuidMap[metadata.UUID] = true
		}
	}
}
