package tmux

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// LegacySocketPath is the old default socket path in /tmp (vulnerable to cleanup).
const LegacySocketPath = "/tmp/agm.sock"

// DefaultSocketPath returns the default Unix socket path for AGM tmux sessions.
// Uses ~/.agm/agm.sock to avoid /tmp cleanup causing tmux crashes.
func DefaultSocketPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return LegacySocketPath
	}
	return filepath.Join(home, ".agm", "agm.sock")
}

// GetSocketPath returns the AGM-specific tmux socket path
// This can be extended later to support custom paths via config
func GetSocketPath() string {
	// Check if AGM_TMUX_SOCKET environment variable is set
	if socketPath := os.Getenv("AGM_TMUX_SOCKET"); socketPath != "" {
		return socketPath
	}
	return DefaultSocketPath()
}

// GetReadSocketPaths returns all socket paths to check when reading/attaching
// Currently returns just the primary socket, but can be extended for dual-socket support
func GetReadSocketPaths() []string {
	return []string{GetSocketPath()}
}

// CleanStaleSocket removes the socket file if it exists but no tmux server is running
// This prevents "socket is in use" errors when a previous tmux server crashed
func CleanStaleSocket() error {
	socketPath := GetSocketPath()

	// Check if socket file exists
	info, err := os.Stat(socketPath)
	if os.IsNotExist(err) {
		return nil // No socket, nothing to clean
	}
	if err != nil {
		return fmt.Errorf("failed to stat socket: %w", err)
	}

	// Verify it's actually a socket
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("path exists but is not a socket: %s", socketPath)
	}

	// Try to connect to see if server is alive
	conn, err := net.DialTimeout("unix", socketPath, 1*time.Second) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		// Connection failed - socket is stale or server is not responding
		// Try to remove the stale socket
		if err := os.Remove(socketPath); err != nil {
			// Check if file was already removed (race condition)
			if !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove stale socket: %w", err)
			}
		}
		return nil
	}

	// Server is alive, close connection and keep socket
	conn.Close()
	return nil
}

// EnsureSocketDir ensures the parent directory of the socket exists with proper permissions
func EnsureSocketDir() error {
	socketPath := GetSocketPath()
	dir := filepath.Dir(socketPath)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}

	return nil
}

// WarnLegacySocket logs a warning if the old /tmp/agm.sock still exists.
// Call during startup to alert users about the migration.
func WarnLegacySocket() {
	if _, err := os.Stat(LegacySocketPath); err == nil {
		currentPath := GetSocketPath()
		if currentPath != LegacySocketPath {
			slog.Warn("legacy socket found at "+LegacySocketPath,
				"current", currentPath,
				"action", "consider running: rm "+LegacySocketPath)
		}
	}
}

// CheckSocketPermissions verifies the socket has secure permissions
// Returns nil if socket doesn't exist or has correct permissions
func CheckSocketPermissions() error {
	socketPath := GetSocketPath()

	info, err := os.Stat(socketPath)
	if os.IsNotExist(err) {
		return nil // No socket yet, will be created with correct permissions
	}
	if err != nil {
		return fmt.Errorf("failed to stat socket: %w", err)
	}

	// Check if socket is world-readable/writable (security issue)
	mode := info.Mode()
	if mode&0006 != 0 {
		return fmt.Errorf("socket has insecure permissions: %o (should be user-only)", mode.Perm())
	}

	return nil
}

// SocketInfo describes the current tmux socket's path, mode, and accessibility.
type SocketInfo struct {
	Path       string
	Exists     bool
	IsSocket   bool
	Mode       os.FileMode
	Size       int64
	Modified   time.Time
	IsStale    bool // True if socket exists but no server is responding
	Accessible bool // True if we can connect to the socket
}

// GetSocketInfo retrieves detailed information about the socket
func GetSocketInfo() (*SocketInfo, error) {
	socketPath := GetSocketPath()
	info := &SocketInfo{
		Path:   socketPath,
		Exists: false,
	}

	stat, err := os.Stat(socketPath)
	if os.IsNotExist(err) {
		return info, nil // Socket doesn't exist
	}
	if err != nil {
		return nil, fmt.Errorf("failed to stat socket: %w", err)
	}

	info.Exists = true
	info.IsSocket = stat.Mode()&os.ModeSocket != 0
	info.Mode = stat.Mode()
	info.Size = stat.Size()
	info.Modified = stat.ModTime()

	// Try to connect to check if server is alive
	conn, err := net.DialTimeout("unix", socketPath, 500*time.Millisecond) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		info.IsStale = true
		info.Accessible = false
	} else {
		conn.Close()
		info.IsStale = false
		info.Accessible = true
	}

	return info, nil
}

// RemoveSocket forcefully removes the socket file
// This should only be used after confirming no tmux server is running
func RemoveSocket() error {
	socketPath := GetSocketPath()

	if err := os.Remove(socketPath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already removed
		}
		return fmt.Errorf("failed to remove socket: %w", err)
	}

	return nil
}

// LockSocket creates a lock file to prevent concurrent socket operations
// Returns an unlock function that should be called with defer
func LockSocket() (unlock func() error, err error) {
	socketPath := GetSocketPath()
	lockPath := socketPath + ".lock"

	// Create lock file with O_EXCL to ensure atomicity
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			// Check if lock is stale (older than 5 seconds)
			stat, statErr := os.Stat(lockPath)
			if statErr == nil && time.Since(stat.ModTime()) > 5*time.Second {
				// Remove stale lock and try again
				os.Remove(lockPath)
				return LockSocket()
			}
			return nil, fmt.Errorf("socket lock is held by another process")
		}
		return nil, fmt.Errorf("failed to create lock file: %w", err)
	}

	// Write PID to lock file for debugging
	fmt.Fprintf(f, "%d\n", os.Getpid())
	f.Close()

	unlock = func() error {
		return os.Remove(lockPath)
	}

	return unlock, nil
}

// IsSocketInUse checks if the socket is currently being used by a tmux server
func IsSocketInUse() (bool, error) {
	socketPath := GetSocketPath()

	// Check if socket exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("failed to stat socket: %w", err)
	}

	// Try to connect
	conn, err := net.DialTimeout("unix", socketPath, 500*time.Millisecond) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return false, nil // Can't connect, so not in use
	}
	defer conn.Close()

	return true, nil
}

// WaitForSocket waits for the socket to become available
// Useful after starting a tmux server
func WaitForSocket(timeout time.Duration) error {
	socketPath := GetSocketPath()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check if socket exists and is accessible
		conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond) //nolint:noctx // TODO(context): plumb ctx through this layer
		if err == nil {
			conn.Close()
			return nil // Socket is ready
		}

		// Wait a bit before retrying
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for socket to become available")
}

// GetSocketOwner returns the UID of the socket owner
func GetSocketOwner() (int, error) {
	socketPath := GetSocketPath()

	info, err := os.Stat(socketPath)
	if err != nil {
		return -1, fmt.Errorf("failed to stat socket: %w", err)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return -1, fmt.Errorf("failed to get socket ownership info")
	}

	return int(stat.Uid), nil
}
