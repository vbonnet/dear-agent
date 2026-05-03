package hippocampus

import (
	"fmt"
	"os"
	"path/filepath"
)

// archiveSession saves full session history for reference.
//
// Phase 5 V1: Simple file write.
// Phase 5 V2+: Compressed archives, metadata tracking.
func (h *Hippocampus) archiveSession(sessionID string, history string) (string, error) {
	// Create archive path
	archiveDir := filepath.Join(h.archiveDir, "../archive/sessions")
	archivePath := filepath.Join(archiveDir, fmt.Sprintf("%s-full.jsonl", sessionID))

	// Ensure directory exists
	if err := os.MkdirAll(archiveDir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create archive directory: %w", err)
	}

	// Write full history
	if err := os.WriteFile(archivePath, []byte(history), 0o600); err != nil {
		return "", fmt.Errorf("failed to write archive: %w", err)
	}

	return archivePath, nil
}
