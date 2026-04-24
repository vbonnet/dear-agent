// Package cli provides cli functionality.
package cli

import "sync"

var (
	projectDirectory string
	projectDirMutex  sync.RWMutex
)

// SetProjectDirectory stores the resolved project directory for commands to use.
// This directory is where commands should look for .agm files and session data.
// IMPORTANT: This does NOT change the process working directory (no os.Chdir).
func SetProjectDirectory(dir string) {
	projectDirMutex.Lock()
	defer projectDirMutex.Unlock()
	projectDirectory = dir
}

// GetProjectDirectory returns the resolved project directory.
// Commands should use this when building file paths.
// Returns "." if not set (for backward compatibility).
func GetProjectDirectory() string {
	projectDirMutex.RLock()
	defer projectDirMutex.RUnlock()
	if projectDirectory == "" {
		return "."
	}
	return projectDirectory
}
