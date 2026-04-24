package sandbox

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	// activeSandbox is cached for the session
	activeSandbox     *Sandbox
	activeSandboxOnce sync.Once
	activeSandboxErr  error
)

// PathResolver resolves paths to sandbox-specific or default locations
type PathResolver struct {
	manager *Manager
}

// NewPathResolver creates a new path resolver
func NewPathResolver(manager *Manager) *PathResolver {
	return &PathResolver{manager: manager}
}

// GetSandboxPath returns the path to a resource within the active sandbox
// Falls back to default ~/.wayfinder/ path if no sandbox is active (backward compatible)
func (r *PathResolver) GetSandboxPath(subpath string) string {
	// Get active sandbox (cached)
	sandbox := r.getActiveSandbox()

	if sandbox != nil {
		// Sandbox mode: return sandbox-specific path
		return filepath.Join(r.manager.baseDir, sandbox.ID, subpath)
	}

	// Fallback mode: return default path (backward compatible)
	homeDir := os.Getenv("HOME")
	return filepath.Join(homeDir, ".wayfinder", subpath)
}

// getActiveSandbox returns the cached active sandbox
func (r *PathResolver) getActiveSandbox() *Sandbox {
	activeSandboxOnce.Do(func() {
		activeSandbox, activeSandboxErr = r.manager.GetActiveSandbox()
	})

	if activeSandboxErr != nil {
		// Error during detection, fallback to non-sandboxed mode
		return nil
	}

	return activeSandbox
}

// ResetCache clears the cached active sandbox (for testing)
func (r *PathResolver) ResetCache() {
	activeSandbox = nil
	activeSandboxErr = nil
	activeSandboxOnce = sync.Once{}
}

// GetSessionPath returns the path to session files
func (r *PathResolver) GetSessionPath() string {
	return r.GetSandboxPath("sessions")
}

// GetCostsDatabasePath returns the path to the costs database
func (r *PathResolver) GetCostsDatabasePath() string {
	return r.GetSandboxPath("costs/costs.db")
}

// GetTempPath returns the path to temporary files
func (r *PathResolver) GetTempPath() string {
	return r.GetSandboxPath("temp")
}
