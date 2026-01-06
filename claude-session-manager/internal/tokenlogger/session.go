package tokenlogger

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// getSessionDirLocked returns the CSM session directory path.
// Uses lazy initialization with caching to avoid repeated subprocess calls.
// Must be called with l.mu lock held.
func (l *TokenLogger) getSessionDirLocked() string {
	// Return cached value if already checked
	if l.cacheChecked {
		return l.sessionDir
	}

	// Mark as checked to avoid repeated subprocess calls
	l.cacheChecked = true

	// Get session UUID from CSM
	uuid := getSessionUUID()
	if uuid == "" {
		// No session or CSM not available
		return ""
	}

	// Build session directory path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Unable to get home directory - gracefully degrade
		return ""
	}

	l.sessionDir = filepath.Join(homeDir, "src", "sessions", uuid)
	return l.sessionDir
}

// getSessionUUID is a package variable that executes 'csm get-uuid' to retrieve the active session UUID.
// Returns empty string if no session active or CSM not installed.
// Can be overridden in tests.
var getSessionUUID = func() string {
	cmd := exec.Command("csm", "get-uuid")
	output, err := cmd.Output()
	if err != nil {
		// CSM not installed, no session, or command failed
		return ""
	}

	// Trim whitespace and return UUID
	return strings.TrimSpace(string(output))
}
