package helpers

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TmuxServer represents an isolated tmux server for testing.
type TmuxServer struct {
	SocketPath string
}

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
func SetupTestServer(t *testing.T) *TmuxServer {
	t.Helper()

	// Use /tmp for socket path to avoid macOS 104-char Unix socket limit.
	// Go's t.TempDir() paths can exceed this limit with long test names.
	socketDir, err := os.MkdirTemp("/tmp", "agm-test-") //nolint:usetesting // see comment above
	if err != nil {
		t.Fatalf("Failed to create socket dir: %v", err)
	}
	socketPath := filepath.Join(socketDir, "t.sock")

	// Create server instance
	server := &TmuxServer{
		SocketPath: socketPath,
	}

	// Register LIFO cleanup
	t.Cleanup(func() {
		// Kill server (this also kills all sessions)
		_ = exec.Command("tmux", "-S", socketPath, "kill-server").Run()
		os.RemoveAll(socketDir)
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
//   - paneID: tmux pane identifier (e.g., "session:0.0", "%0")
//
// Returns:
//   - Captured pane content as string (whitespace trimmed)
//
// Example:
//
//	output := helpers.CapturePane(t, server, "test-session:0.0")
//	helpers.CompareGolden(t, "testdata/golden/session-output.golden", output)
func CapturePane(t *testing.T, server *TmuxServer, paneID string) string {
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
//   - Session name for reference (use with paneID like "name:0.0")
//
// Example:
//
//	server := helpers.SetupTestServer(t)
//	session := helpers.CreateSession(t, server, "test-session")
//	output := helpers.CapturePane(t, server, session+":0.0")
func CreateSession(t *testing.T, server *TmuxServer, name string) string {
	t.Helper()

	cmd := exec.Command("tmux", "-S", server.SocketPath,
		"new-session", "-d", "-s", name)

	err := cmd.Run()
	require.NoError(t, err, "Failed to create session %s", name)

	return name
}
