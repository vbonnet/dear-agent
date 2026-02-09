package helpers

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jubnzv/go-tmux"
	"github.com/stretchr/testify/require"
)

// SetupTestServer creates an isolated tmux server for testing.
//
// Creates a unique socket path in a temporary directory and initializes
// a tmux server using that socket. The server and all its sessions are
// automatically cleaned up via t.Cleanup() in LIFO order.
//
// Cleanup order (LIFO):
//  1. Kill all sessions
//  2. Kill server
//  3. Socket file cleaned by t.TempDir() cleanup
//
// Example:
//
//	func TestAGM(t *testing.T) {
//	    server := helpers.SetupTestServer(t)
//	    session := helpers.CreateSession(t, server, "test-session")
//	    // ... test code ...
//	}
func SetupTestServer(t *testing.T) *tmux.Server {
	t.Helper()

	// Create unique socket path in temp directory
	socketPath := filepath.Join(t.TempDir(), "tmux.sock")

	// Create isolated tmux server
	server := tmux.NewServer(socketPath, "", nil)

	// Register LIFO cleanup
	t.Cleanup(func() {
		// Step 1: Kill all sessions first
		sessions, _ := server.ListSessions()
		for _, session := range sessions {
			_ = session.Kill()
		}

		// Step 2: Kill server
		_ = exec.Command("tmux", "-S", socketPath, "kill-server").Run()

		// Step 3: Socket file cleaned by t.TempDir() cleanup
	})

	return server
}

// CapturePane captures tmux pane output as string.
//
// Uses tmux capture-pane command to retrieve pane content. The output
// is trimmed of leading/trailing whitespace. Fails the test if capture fails.
//
// Parameters:
//   - server: tmux server instance (from SetupTestServer)
//   - paneID: tmux pane identifier (e.g., "0", "1", "%0")
//
// Returns:
//   - Captured pane content as string (whitespace trimmed)
//
// Example:
//
//	output := helpers.CapturePane(t, server, session.Panes[0].ID)
//	helpers.CompareGolden(t, "testdata/golden/session-output.golden", output)
func CapturePane(t *testing.T, server *tmux.Server, paneID string) string {
	t.Helper()

	cmd := exec.Command("tmux", "-S", server.SocketPath,
		"capture-pane", "-p", "-J", "-t", paneID)

	output, err := cmd.Output()
	require.NoError(t, err, "Failed to capture pane %s", paneID)

	return strings.TrimSpace(string(output))
}

// CreateSession creates a test tmux session.
//
// Creates a new tmux session on the test server with the given name.
// The session is automatically cleaned up via SetupTestServer's t.Cleanup().
//
// Parameters:
//   - server: tmux server instance (from SetupTestServer)
//   - name: session name (must be unique per server)
//
// Returns:
//   - Session instance for further operations
//
// Example:
//
//	server := helpers.SetupTestServer(t)
//	session := helpers.CreateSession(t, server, "test-session")
//	// Session is automatically cleaned up when test ends
func CreateSession(t *testing.T, server *tmux.Server, name string) *tmux.Session {
	t.Helper()

	session, err := server.NewSession(name)
	require.NoError(t, err, "Failed to create session %s", name)

	return session
}
