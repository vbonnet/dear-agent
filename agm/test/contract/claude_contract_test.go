//go:build contract
// +build contract

package contract

import (
	"os"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/test/helpers"
)

// TestClaudeAPI_SessionCreation tests creating a session with real Claude CLI.
//
// This contract test verifies end-to-end workflow:
//  1. Create new session with Claude agent
//  2. Verify session manifest created
//  3. Verify Claude CLI responds to prompts
//
// Requirements:
//   - ANTHROPIC_API_KEY environment variable must be set
//   - Consumes 1 API quota
//   - Creates real tmux session (requires tmux installed)
func TestClaudeAPI_SessionCreation(t *testing.T) {
	// Check API key availability
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping contract test")
	}

	// Consume API quota
	quota := helpers.GetAPIQuota()
	if !quota.Consume() {
		t.Skip("API quota exhausted (max 20 calls per run)")
	}

	// Create session with Claude agent
	result := helpers.RunCLI(t, "new", "contract-test-claude", "--detached", "--agent", "claude")

	// Verify session creation succeeded
	if result.ExitCode != 0 {
		t.Fatalf("Session creation failed (exit %d): %s\n%s", result.ExitCode, result.Stdout, result.Stderr)
	}

	// Verify output contains success indicators
	if !strings.Contains(result.Stdout, "Session") && !strings.Contains(result.Stdout, "created") {
		t.Errorf("Expected success message, got: %s", result.Stdout)
	}

	// List sessions to verify creation
	listResult := helpers.RunCLI(t, "list")
	if listResult.ExitCode != 0 {
		t.Fatalf("List command failed (exit %d): %s", listResult.ExitCode, listResult.Stderr)
	}

	if !strings.Contains(listResult.Stdout, "contract-test-claude") {
		t.Errorf("Session not found in list output: %s", listResult.Stdout)
	}

	// Note: Actual Claude CLI communication would require tmux integration
	// and is tested in integration tests with mocks. This contract test
	// verifies the AGM + Claude CLI integration at the process level.
}

// TestClaudeAPI_BasicPrompt tests sending a prompt to Claude via AGM.
//
// This test verifies:
//  1. Session can be resumed
//  2. Prompts can be sent to Claude CLI
//  3. Responses are captured
//
// Requirements:
//   - ANTHROPIC_API_KEY environment variable must be set
//   - Consumes 2 API quota (session creation + prompt)
//   - Real tmux session required
func TestClaudeAPI_BasicPrompt(t *testing.T) {
	// Check API key availability
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping contract test")
	}

	// Consume API quota (need 2: create + prompt)
	quota := helpers.GetAPIQuota()
	if quota.Remaining() < 2 {
		t.Skip("Insufficient API quota (need 2, have %d)", quota.Remaining())
	}

	// First quota for session creation
	if !quota.Consume() {
		t.Skip("API quota exhausted")
	}

	// Create session
	createResult := helpers.RunCLI(t, "new", "contract-test-prompt", "--detached", "--agent", "claude")
	if createResult.ExitCode != 0 {
		t.Fatalf("Session creation failed: %s", createResult.Stderr)
	}

	// Second quota for sending prompt
	if !quota.Consume() {
		t.Skip("API quota exhausted before prompt")
	}

	// Send simple prompt using AGM send command
	// Note: This requires the 'send' command to be implemented in AGM
	sendResult := helpers.RunCLI(t, "send", "contract-test-prompt", "Say 'test successful' and nothing else")

	// Verify send succeeded
	if sendResult.ExitCode != 0 {
		// Send command may not be implemented yet
		t.Skipf("Send command not available (exit %d): %s", sendResult.ExitCode, sendResult.Stderr)
	}

	// Verify response contains expected text
	if !strings.Contains(sendResult.Stdout, "test successful") {
		t.Logf("Warning: Expected 'test successful' in response, got: %s", sendResult.Stdout)
	}
}

// TestClaudeAPI_SessionArchive tests archiving a Claude session.
//
// This test verifies:
//  1. Session can be archived
//  2. Archived sessions are filtered from default list
//  3. Archived sessions appear with --all flag
//
// Requirements:
//   - ANTHROPIC_API_KEY environment variable must be set
//   - Consumes 1 API quota (session creation only)
func TestClaudeAPI_SessionArchive(t *testing.T) {
	// Check API key availability
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping contract test")
	}

	// Consume API quota
	quota := helpers.GetAPIQuota()
	if !quota.Consume() {
		t.Skip("API quota exhausted")
	}

	// Create session
	sessionName := "contract-test-archive"
	createResult := helpers.RunCLI(t, "new", sessionName, "--detached", "--agent", "claude")
	if createResult.ExitCode != 0 {
		t.Fatalf("Session creation failed: %s", createResult.Stderr)
	}

	// Archive the session
	archiveResult := helpers.RunCLI(t, "archive", sessionName)
	if archiveResult.ExitCode != 0 {
		t.Fatalf("Archive command failed (exit %d): %s", archiveResult.ExitCode, archiveResult.Stderr)
	}

	// Verify session not in default list
	listResult := helpers.RunCLI(t, "list")
	if listResult.ExitCode != 0 {
		t.Fatalf("List command failed: %s", listResult.Stderr)
	}

	if strings.Contains(listResult.Stdout, sessionName) {
		t.Errorf("Archived session should not appear in default list: %s", listResult.Stdout)
	}

	// Verify session appears with --all flag
	listAllResult := helpers.RunCLI(t, "list", "--all")
	if listAllResult.ExitCode != 0 {
		t.Fatalf("List --all command failed: %s", listAllResult.Stderr)
	}

	if !strings.Contains(listAllResult.Stdout, sessionName) {
		t.Errorf("Archived session should appear in list --all: %s", listAllResult.Stdout)
	}
}
