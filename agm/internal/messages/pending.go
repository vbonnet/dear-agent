package messages

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WritePendingFile writes a .msg file to ~/.agm/pending/{sessionName}/ so that
// the pretool-message-check hook can deliver it on the next tool call.
//
// File naming: {unix-nanoseconds}-{messageID-prefix}.msg
// File content: the formatted message text (UTF-8)
//
// This function is best-effort: errors are returned but callers may choose to
// log and continue (the SQLite queue + daemon path remains the primary delivery).
func WritePendingFile(sessionName, messageID, formattedMessage string) error {
	if sessionName == "" {
		return fmt.Errorf("session name is required")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	return WritePendingFileToDir(filepath.Join(homeDir, ".agm", "pending"), sessionName, messageID, formattedMessage)
}

// WritePendingFileToDir writes a .msg file to baseDir/{sessionName}/.
// Extracted for testability (avoids dependency on $HOME).
func WritePendingFileToDir(baseDir, sessionName, messageID, formattedMessage string) error {
	if sessionName == "" {
		return fmt.Errorf("session name is required")
	}
	if formattedMessage == "" {
		return fmt.Errorf("message content is required")
	}

	pendingDir := filepath.Join(baseDir, sessionName)
	if err := os.MkdirAll(pendingDir, 0700); err != nil {
		return fmt.Errorf("failed to create pending directory: %w", err)
	}

	// Build a filename that sorts chronologically and avoids collisions.
	// Format: {unix-nanos}-{messageID-prefix}.msg
	prefix := messageID
	if len(prefix) > 20 {
		prefix = prefix[:20]
	}
	filename := fmt.Sprintf("%d-%s.msg", time.Now().UnixNano(), prefix)
	filePath := filepath.Join(pendingDir, filename)

	if err := os.WriteFile(filePath, []byte(formattedMessage), 0600); err != nil {
		return fmt.Errorf("failed to write pending file: %w", err)
	}

	return nil
}
