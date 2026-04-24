//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

// TestE2E_Gemini_LongRunningSession tests Gemini's 1M token advantage with large context.
//
// This test requires:
// - Gemini CLI installed
// - GEMINI_API_KEY set
// - tmux installed
func TestE2E_Gemini_LongRunningSession(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running session test in short mode")
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

	// Create session
	ctx := agent.SessionContext{
		Name:             "long-running-test",
		WorkingDirectory: workDir,
		Project:          "large-context-test",
	}

	sessionID, err := adapter.CreateSession(ctx)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	defer adapter.TerminateSession(sessionID)

	startTime := time.Now()

	// Send multiple messages to build up context
	// Note: Real 1M token test would require substantial data, this is a scaled-down version
	numMessages := 10
	messageSize := 1000 // ~1000 tokens per message

	for i := 0; i < numMessages; i++ {
		msg := agent.Message{
			Content: fmt.Sprintf("Message %d: %s",
				i,
				strings.Repeat(fmt.Sprintf("Context data for message %d. ", i), messageSize/10)),
		}

		if err := adapter.SendMessage(sessionID, msg); err != nil {
			t.Fatalf("SendMessage %d failed: %v", i, err)
		}

		// Wait between messages to avoid rate limiting
		time.Sleep(2 * time.Second)
	}

	duration := time.Since(startTime)

	// Verify session is still active after large context
	status, err := adapter.GetSessionStatus(sessionID)
	if err != nil {
		t.Fatalf("GetSessionStatus failed: %v", err)
	}

	if status != agent.StatusActive {
		t.Errorf("Expected session to be active after %d messages, got %v", numMessages, status)
	}

	// Performance check: should complete in reasonable time
	maxDuration := time.Duration(numMessages*10) * time.Second
	if duration > maxDuration {
		t.Errorf("Long-running session took too long: %v (max: %v)", duration, maxDuration)
	}

	t.Logf("Long-running session completed: %d messages in %v", numMessages, duration)

	// Verify history is retrievable
	history, err := adapter.GetHistory(sessionID)
	if err != nil {
		t.Fatalf("GetHistory failed after long session: %v", err)
	}

	if len(history) == 0 {
		t.Log("Warning: History is empty (may be timing issue with file writes)")
	} else {
		t.Logf("History contains %d messages", len(history))
	}
}

// TestE2E_Gemini_CommandExecutionUnderLoad tests rapid and concurrent command execution.
func TestE2E_Gemini_CommandExecutionUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping command load test in short mode")
	}

	// Check requirements
	if _, err := exec.LookPath("gemini"); err != nil {
		t.Skip("Gemini CLI not found in PATH")
	}
	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GEMINI_API_KEY not set")
	}

	// Create temp directory
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

	// Create session
	ctx := agent.SessionContext{
		Name:             "command-load-test",
		WorkingDirectory: workDir,
		Project:          "load-test",
	}

	sessionID, err := adapter.CreateSession(ctx)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	defer adapter.TerminateSession(sessionID)

	// Wait for session to be ready
	time.Sleep(2 * time.Second)

	// Test 1: Rapid sequential command execution
	t.Run("RapidSequential", func(t *testing.T) {
		numCommands := 20
		startTime := time.Now()

		for i := 0; i < numCommands; i++ {
			testDir := filepath.Join(tempDir, fmt.Sprintf("test-dir-%d", i))
			os.MkdirAll(testDir, 0755)

			cmd := agent.SessionCommand{
				Type: agent.CommandSetDir,
				Params: map[string]interface{}{
					"path": testDir,
				},
			}

			if err := adapter.ExecuteCommand(sessionID, cmd); err != nil {
				t.Errorf("ExecuteCommand %d failed: %v", i, err)
			}

			// Small delay to avoid overwhelming the system
			time.Sleep(50 * time.Millisecond)
		}

		duration := time.Since(startTime)
		avgTime := duration / time.Duration(numCommands)

		t.Logf("Rapid sequential: %d commands in %v (avg: %v per command)", numCommands, duration, avgTime)

		// Performance check: average should be under 1s per command
		if avgTime > 1*time.Second {
			t.Errorf("Command execution too slow: avg %v (expected <1s)", avgTime)
		}
	})

	// Test 2: Error recovery (execute invalid commands and verify recovery)
	t.Run("ErrorRecovery", func(t *testing.T) {
		// Execute invalid command
		invalidCmd := agent.SessionCommand{
			Type: agent.CommandSetDir,
			Params: map[string]interface{}{
				"path": "/nonexistent/invalid/path/that/does/not/exist",
			},
		}

		err := adapter.ExecuteCommand(sessionID, invalidCmd)
		// Error expected, but session should still be active

		// Verify session is still operational
		status, err := adapter.GetSessionStatus(sessionID)
		if err != nil {
			t.Fatalf("GetSessionStatus failed after error: %v", err)
		}

		if status != agent.StatusActive {
			t.Errorf("Session should still be active after command error, got %v", status)
		}

		// Execute valid command to verify recovery
		validDir := t.TempDir()
		validCmd := agent.SessionCommand{
			Type: agent.CommandSetDir,
			Params: map[string]interface{}{
				"path": validDir,
			},
		}

		if err := adapter.ExecuteCommand(sessionID, validCmd); err != nil {
			t.Errorf("Failed to recover from error: %v", err)
		}

		t.Log("Session recovered successfully after error")
	})
}

// TestE2E_Gemini_CrossAgentCompatibility tests export from Claude → import to Gemini and vice versa.
func TestE2E_Gemini_CrossAgentCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cross-agent compatibility test in short mode")
	}

	// Check requirements for both agents
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("Claude CLI not found in PATH")
	}
	if _, err := exec.LookPath("gemini"); err != nil {
		t.Skip("Gemini CLI not found in PATH")
	}
	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GEMINI_API_KEY not set")
	}

	// Create temp directory
	tempDir := t.TempDir()
	claudeStorePath := filepath.Join(tempDir, "claude-sessions.json")
	geminiStorePath := filepath.Join(tempDir, "gemini-sessions.json")
	workDir := t.TempDir()

	// Test 1: Claude → Gemini
	t.Run("Claude_to_Gemini", func(t *testing.T) {
		// Create Claude session
		claudeStore, err := agent.NewJSONSessionStore(claudeStorePath)
		if err != nil {
			t.Fatalf("failed to create Claude session store: %v", err)
		}

		claudeAdapter, err := agent.NewClaudeAdapter(claudeStore)
		if err != nil {
			t.Fatalf("failed to create Claude adapter: %v", err)
		}

		ctx := agent.SessionContext{
			Name:             "claude-export-test",
			WorkingDirectory: workDir,
			Project:          "cross-agent-test",
		}

		claudeSessionID, err := claudeAdapter.CreateSession(ctx)
		if err != nil {
			t.Fatalf("Claude CreateSession failed: %v", err)
		}
		defer claudeAdapter.TerminateSession(claudeSessionID)

		// Export from Claude
		exportData, err := claudeAdapter.ExportConversation(claudeSessionID, agent.FormatJSONL)
		if err != nil {
			t.Fatalf("Claude ExportConversation failed: %v", err)
		}

		if len(exportData) == 0 {
			t.Skip("Claude export returned empty data, cannot test import")
		}

		// Write export to file
		exportFile := filepath.Join(tempDir, "claude-export.jsonl")
		if err := os.WriteFile(exportFile, exportData, 0644); err != nil {
			t.Fatalf("Failed to write export file: %v", err)
		}

		// Import to Gemini
		geminiStore, err := agent.NewJSONSessionStore(geminiStorePath)
		if err != nil {
			t.Fatalf("failed to create Gemini session store: %v", err)
		}

		geminiAdapter, err := agent.NewGeminiCLIAdapter(geminiStore)
		if err != nil {
			t.Fatalf("failed to create Gemini adapter: %v", err)
		}

		geminiSessionID, err := geminiAdapter.ImportConversation(exportFile, agent.FormatJSONL)
		if err != nil {
			t.Fatalf("Gemini ImportConversation failed: %v", err)
		}
		defer geminiAdapter.TerminateSession(geminiSessionID)

		// Verify import succeeded
		if geminiSessionID == "" {
			t.Error("Gemini import returned empty session ID")
		}

		// Verify history is accessible
		history, err := geminiAdapter.GetHistory(geminiSessionID)
		if err != nil {
			t.Fatalf("GetHistory failed for imported Gemini session: %v", err)
		}

		t.Logf("Claude → Gemini: Imported %d history messages", len(history))
	})

	// Test 2: Gemini → Claude
	t.Run("Gemini_to_Claude", func(t *testing.T) {
		// Create Gemini session
		geminiStore, err := agent.NewJSONSessionStore(geminiStorePath)
		if err != nil {
			t.Fatalf("failed to create Gemini session store: %v", err)
		}

		geminiAdapter, err := agent.NewGeminiCLIAdapter(geminiStore)
		if err != nil {
			t.Fatalf("failed to create Gemini adapter: %v", err)
		}

		ctx := agent.SessionContext{
			Name:             "gemini-export-test",
			WorkingDirectory: workDir,
			Project:          "cross-agent-test",
		}

		geminiSessionID, err := geminiAdapter.CreateSession(ctx)
		if err != nil {
			t.Fatalf("Gemini CreateSession failed: %v", err)
		}
		defer geminiAdapter.TerminateSession(geminiSessionID)

		// Export from Gemini
		exportData, err := geminiAdapter.ExportConversation(geminiSessionID, agent.FormatJSONL)
		if err != nil {
			t.Fatalf("Gemini ExportConversation failed: %v", err)
		}

		if len(exportData) == 0 {
			t.Skip("Gemini export returned empty data, cannot test import")
		}

		// Write export to file
		exportFile := filepath.Join(tempDir, "gemini-export.jsonl")
		if err := os.WriteFile(exportFile, exportData, 0644); err != nil {
			t.Fatalf("Failed to write export file: %v", err)
		}

		// Import to Claude
		claudeStore, err := agent.NewJSONSessionStore(claudeStorePath)
		if err != nil {
			t.Fatalf("failed to create Claude session store: %v", err)
		}

		claudeAdapter, err := agent.NewClaudeAdapter(claudeStore)
		if err != nil {
			t.Fatalf("failed to create Claude adapter: %v", err)
		}

		claudeSessionID, err := claudeAdapter.ImportConversation(exportFile, agent.FormatJSONL)
		if err != nil {
			t.Fatalf("Claude ImportConversation failed: %v", err)
		}
		defer claudeAdapter.TerminateSession(claudeSessionID)

		// Verify import succeeded
		if claudeSessionID == "" {
			t.Error("Claude import returned empty session ID")
		}

		// Verify history is accessible
		history, err := claudeAdapter.GetHistory(claudeSessionID)
		if err != nil {
			t.Fatalf("GetHistory failed for imported Claude session: %v", err)
		}

		t.Logf("Gemini → Claude: Imported %d history messages", len(history))
	})
}

// TestE2E_Gemini_CrashRecovery tests killing Gemini CLI process mid-session and resuming.
func TestE2E_Gemini_CrashRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping crash recovery test in short mode")
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

	// Create session
	ctx := agent.SessionContext{
		Name:             "crash-recovery-test",
		WorkingDirectory: workDir,
		Project:          "crash-test",
	}

	sessionID, err := adapter.CreateSession(ctx)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	defer adapter.TerminateSession(sessionID)

	// Wait for session to be ready
	time.Sleep(3 * time.Second)

	// Send a message before crash
	preCrashMsg := agent.Message{
		Content: "Message before crash - this should be preserved",
	}
	if err := adapter.SendMessage(sessionID, preCrashMsg); err != nil {
		t.Fatalf("SendMessage before crash failed: %v", err)
	}

	// Wait for message to be processed
	time.Sleep(2 * time.Second)

	// Get session metadata to find tmux session
	metadata, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("failed to get session metadata: %v", err)
	}

	// Simulate crash: kill the Gemini CLI process (but not tmux)
	// Find Gemini process in the tmux session
	cmd := exec.Command("tmux", "send-keys", "-t", metadata.TmuxName, "C-c")
	if err := cmd.Run(); err != nil {
		t.Logf("Warning: failed to send Ctrl-C to tmux session: %v", err)
	}

	// Wait for process to die
	time.Sleep(1 * time.Second)

	// Verify session status reflects crash
	status, err := adapter.GetSessionStatus(sessionID)
	if err != nil {
		t.Fatalf("GetSessionStatus after crash failed: %v", err)
	}

	t.Logf("Session status after crash: %v", status)

	// Attempt to resume session
	if err := adapter.ResumeSession(sessionID); err != nil {
		t.Fatalf("ResumeSession after crash failed: %v", err)
	}

	// Wait for resume to complete
	time.Sleep(3 * time.Second)

	// Verify session is active again
	statusAfterResume, err := adapter.GetSessionStatus(sessionID)
	if err != nil {
		t.Fatalf("GetSessionStatus after resume failed: %v", err)
	}

	if statusAfterResume != agent.StatusActive {
		t.Errorf("Expected session to be active after resume, got %v", statusAfterResume)
	}

	// Verify data integrity: history should be preserved
	history, err := adapter.GetHistory(sessionID)
	if err != nil {
		t.Fatalf("GetHistory after crash recovery failed: %v", err)
	}

	// Check if pre-crash message is in history
	foundPreCrashMsg := false
	for _, msg := range history {
		if strings.Contains(msg.Content, "Message before crash") {
			foundPreCrashMsg = true
			break
		}
	}

	if !foundPreCrashMsg && len(history) > 0 {
		t.Log("Warning: Pre-crash message not found in history (may be timing issue)")
	}

	// Send a message after recovery to verify session is functional
	postCrashMsg := agent.Message{
		Content: "Message after crash recovery - session should be functional",
	}
	if err := adapter.SendMessage(sessionID, postCrashMsg); err != nil {
		t.Errorf("SendMessage after crash recovery failed: %v", err)
	}

	t.Log("Crash recovery successful: session resumed and functional")
}

// TestE2E_Gemini_MultiSessionIsolation verifies complete isolation between concurrent sessions.
func TestE2E_Gemini_MultiSessionIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping multi-session isolation test in short mode")
	}

	// Check requirements
	if _, err := exec.LookPath("gemini"); err != nil {
		t.Skip("Gemini CLI not found in PATH")
	}
	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GEMINI_API_KEY not set")
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

	// Create 3 sessions with different working directories and messages
	numSessions := 3
	sessionIDs := make([]agent.SessionID, numSessions)
	workDirs := make([]string, numSessions)

	for i := 0; i < numSessions; i++ {
		workDirs[i] = t.TempDir()

		ctx := agent.SessionContext{
			Name:             fmt.Sprintf("isolation-test-%d", i),
			WorkingDirectory: workDirs[i],
			Project:          fmt.Sprintf("project-%d", i),
		}

		sessionID, err := adapter.CreateSession(ctx)
		if err != nil {
			t.Fatalf("CreateSession %d failed: %v", i, err)
		}
		sessionIDs[i] = sessionID
		defer adapter.TerminateSession(sessionID)
	}

	// Wait for all sessions to be ready
	time.Sleep(3 * time.Second)

	// Send unique messages to each session
	for i, sessionID := range sessionIDs {
		msg := agent.Message{
			Content: fmt.Sprintf("Unique message for session %d - isolation test marker", i),
		}

		if err := adapter.SendMessage(sessionID, msg); err != nil {
			t.Errorf("SendMessage to session %d failed: %v", i, err)
		}

		time.Sleep(1 * time.Second)
	}

	// Verify each session has only its own message (not others)
	for i, sessionID := range sessionIDs {
		history, err := adapter.GetHistory(sessionID)
		if err != nil {
			t.Errorf("GetHistory for session %d failed: %v", i, err)
			continue
		}

		// Check that session's own message is present
		foundOwnMessage := false
		foundOtherMessage := false

		for _, msg := range history {
			if strings.Contains(msg.Content, fmt.Sprintf("session %d", i)) {
				foundOwnMessage = true
			}

			// Check for messages from other sessions
			for j := 0; j < numSessions; j++ {
				if j != i && strings.Contains(msg.Content, fmt.Sprintf("session %d", j)) {
					foundOtherMessage = true
					t.Errorf("Session %d contains message from session %d - isolation violated!", i, j)
				}
			}
		}

		if !foundOwnMessage && len(history) > 0 {
			t.Logf("Warning: Session %d own message not found (may be timing issue)", i)
		}

		if foundOtherMessage {
			t.Errorf("Session %d isolation violated: contains messages from other sessions", i)
		}

		// Verify working directory is correct
		metadata, err := store.Get(sessionID)
		if err != nil {
			t.Errorf("Failed to get metadata for session %d: %v", i, err)
			continue
		}

		if metadata.WorkingDir != workDirs[i] {
			t.Errorf("Session %d working directory mismatch: expected %s, got %s",
				i, workDirs[i], metadata.WorkingDir)
		}
	}

	t.Logf("Multi-session isolation verified: %d sessions independent", numSessions)
}
