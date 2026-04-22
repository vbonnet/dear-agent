package hooks

import (
	"fmt"
	"os"
	"path/filepath"
)

// CheckExitGate verifies the worker has completed exit protocol.
// Returns nil if exit protocol completed, error otherwise.
func CheckExitGate(sessionName string) error {
	homeDir, _ := os.UserHomeDir()
	markerPath := filepath.Join(homeDir, ".agm", "exit-markers", sessionName+".exit")
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		return fmt.Errorf("session %s has not completed exit protocol — run /agm:agm-exit first", sessionName)
	}
	return nil
}

// WriteExitMarker marks a session as having completed exit protocol.
func WriteExitMarker(sessionName string) error {
	homeDir, _ := os.UserHomeDir()
	dir := filepath.Join(homeDir, ".agm", "exit-markers")
	os.MkdirAll(dir, 0755)
	marker := filepath.Join(dir, sessionName+".exit")
	return os.WriteFile(marker, []byte("exited\n"), 0600)
}
