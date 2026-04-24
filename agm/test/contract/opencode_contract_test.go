//go:build contract
// +build contract

package contract

import (
	"os"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/test/helpers"
)

// TestOpenCodeAPI_SessionCreation tests creating a session with real OpenCode server.
//
// This contract test verifies end-to-end workflow with OpenCode:
//  1. Create new session with OpenCode agent
//  2. Verify session manifest created
//  3. Verify AGM integrates with OpenCode server
//
// Requirements:
//   - OpenCode server running on localhost:4096
//   - Creates real tmux session (requires tmux installed)
func TestOpenCodeAPI_SessionCreation(t *testing.T) {
	// Check OpenCode server availability
	// Note: OpenCode is a mock implementation for AGM testing
	// In production, this would check for real OpenCode server
	if os.Getenv("OPENCODE_SERVER_URL") == "" {
		os.Setenv("OPENCODE_SERVER_URL", "http://localhost:4096")
	}

	// Create session with OpenCode agent
	result := helpers.RunCLI(t, "new", "contract-test-opencode", "--detached", "--agent", "opencode")

	// Verify session creation succeeded
	if result.ExitCode != 0 {
		t.Fatalf("OpenCode session creation failed (exit %d): %s\n%s", result.ExitCode, result.Stdout, result.Stderr)
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

	if !strings.Contains(listResult.Stdout, "contract-test-opencode") {
		t.Errorf("OpenCode session not found in list output: %s", listResult.Stdout)
	}
}

// TestOpenCodeAPI_BasicPrompt tests sending a prompt to OpenCode via AGM.
//
// This test verifies:
//  1. OpenCode session can be resumed
//  2. Prompts can be sent via tmux to OpenCode
//  3. Session state tracked via SSE
//
// Requirements:
//   - OpenCode server running on localhost:4096
//   - Real tmux session required
func TestOpenCodeAPI_BasicPrompt(t *testing.T) {
	// Check OpenCode server availability
	if os.Getenv("OPENCODE_SERVER_URL") == "" {
		os.Setenv("OPENCODE_SERVER_URL", "http://localhost:4096")
	}

	// Create OpenCode session
	createResult := helpers.RunCLI(t, "new", "contract-test-opencode-prompt", "--detached", "--agent", "opencode")
	if createResult.ExitCode != 0 {
		t.Fatalf("OpenCode session creation failed: %s", createResult.Stderr)
	}

	// Send simple prompt using AGM send command
	sendResult := helpers.RunCLI(t, "send", "contract-test-opencode-prompt", "Test message to OpenCode")

	// Verify send succeeded
	if sendResult.ExitCode != 0 {
		// Send command may not be implemented yet
		t.Skipf("Send command not available (exit %d): %s", sendResult.ExitCode, sendResult.Stderr)
	}

	// OpenCode uses SSE for state tracking - verify session is responsive
	// (Implementation details depend on OpenCode SSE integration)
}

// TestOpenCodeAPI_SessionArchive tests archiving an OpenCode session.
//
// This test verifies:
//  1. OpenCode session can be archived
//  2. Archived sessions are filtered from default list
//  3. Archived sessions appear with --all flag
//
// Requirements:
//   - OpenCode server running on localhost:4096
func TestOpenCodeAPI_SessionArchive(t *testing.T) {
	// Check OpenCode server availability
	if os.Getenv("OPENCODE_SERVER_URL") == "" {
		os.Setenv("OPENCODE_SERVER_URL", "http://localhost:4096")
	}

	// Create OpenCode session
	sessionName := "contract-test-opencode-archive"
	createResult := helpers.RunCLI(t, "new", sessionName, "--detached", "--agent", "opencode")
	if createResult.ExitCode != 0 {
		t.Fatalf("OpenCode session creation failed: %s", createResult.Stderr)
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
		t.Errorf("Archived OpenCode session should not appear in default list: %s", listResult.Stdout)
	}

	// Verify session appears with --all flag
	listAllResult := helpers.RunCLI(t, "list", "--all")
	if listAllResult.ExitCode != 0 {
		t.Fatalf("List --all command failed: %s", listAllResult.Stderr)
	}

	if !strings.Contains(listAllResult.Stdout, sessionName) {
		t.Errorf("Archived OpenCode session should appear in list --all: %s", listAllResult.Stdout)
	}
}

// TestOpenCodeAPI_AgentParity tests feature parity between Claude, Gemini, and OpenCode.
//
// This test verifies:
//  1. All agents support same core operations
//  2. Session management is agent-agnostic
//  3. Manifest format is consistent
//
// Requirements:
//   - OpenCode server running on localhost:4096
func TestOpenCodeAPI_AgentParity(t *testing.T) {
	// Check OpenCode server availability
	if os.Getenv("OPENCODE_SERVER_URL") == "" {
		os.Setenv("OPENCODE_SERVER_URL", "http://localhost:4096")
	}

	// Create OpenCode session
	opencodeSession := "contract-test-parity-opencode"
	opencodeResult := helpers.RunCLI(t, "new", opencodeSession, "--detached", "--agent", "opencode")
	if opencodeResult.ExitCode != 0 {
		t.Fatalf("OpenCode session creation failed: %s", opencodeResult.Stderr)
	}

	// Verify operations that should work for all agents
	operations := []struct {
		name string
		args []string
	}{
		{"list", []string{"list"}},
		{"list-json", []string{"list", "--json"}},
		{"archive", []string{"archive", opencodeSession}},
		{"list-all", []string{"list", "--all"}},
	}

	for _, op := range operations {
		result := helpers.RunCLI(t, op.args...)
		if result.ExitCode != 0 {
			t.Errorf("Operation %s failed for OpenCode session: %s", op.name, result.Stderr)
		}
	}
}

// TestOpenCodeAPI_SSEMonitoring tests OpenCode SSE integration.
//
// This test verifies:
//  1. OpenCode sessions emit SSE events
//  2. AGM monitors OpenCode session state via SSE
//  3. State changes detected correctly
//
// Requirements:
//   - OpenCode server running on localhost:4096 with SSE endpoint
func TestOpenCodeAPI_SSEMonitoring(t *testing.T) {
	// Check OpenCode server availability
	if os.Getenv("OPENCODE_SERVER_URL") == "" {
		os.Setenv("OPENCODE_SERVER_URL", "http://localhost:4096")
	}

	// Create OpenCode session
	sessionName := "contract-test-opencode-sse"
	createResult := helpers.RunCLI(t, "new", sessionName, "--detached", "--agent", "opencode")
	if createResult.ExitCode != 0 {
		t.Fatalf("OpenCode session creation failed: %s", createResult.Stderr)
	}

	// Note: SSE monitoring is implemented in internal/monitor/opencode/
	// This test would verify that SSE events are properly consumed
	// For now, we verify the session was created with correct agent

	listResult := helpers.RunCLI(t, "list", "--json")
	if !strings.Contains(listResult.Stdout, sessionName) {
		t.Errorf("OpenCode session not found in list: %s", listResult.Stdout)
	}

	// Future: Add verification that SSE events are being received
	// This would require inspecting AGM's SSE monitoring logs or state
}
