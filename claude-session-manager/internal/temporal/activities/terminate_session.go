package activities

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// TerminateSessionInput contains parameters for session termination
type TerminateSessionInput struct {
	SessionID      string        // Session to terminate
	SessionName    string        // Session name for logging
	PID            int           // Process ID to terminate
	GracePeriod    time.Duration // How long to wait for graceful shutdown
	ForceKill      bool          // Whether to force kill if graceful fails
	CleanupFiles   bool          // Whether to cleanup temporary files
	ArchiveSession bool          // Whether to archive session data
}

// TerminateSessionOutput contains the result of termination
type TerminateSessionOutput struct {
	SessionID       string    // Session that was terminated
	ProcessKilled   bool      // Whether process was killed
	FilesRemoved    int       // Number of temporary files removed
	SessionArchived bool      // Whether session was archived
	TerminatedAt    time.Time // When termination completed
	GracefulExit    bool      // Whether process exited gracefully
}

// TerminateSessionActivity gracefully terminates an agent process and cleans up resources
// This activity handles:
// 1. Graceful process termination (SIGTERM)
// 2. Force kill if needed (SIGKILL)
// 3. Cleanup of temporary files
// 4. Archiving session data
func TerminateSessionActivity(ctx context.Context, input TerminateSessionInput) (*TerminateSessionOutput, error) {
	// Validate input
	if input.SessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}
	if input.PID <= 0 {
		return nil, fmt.Errorf("invalid PID: %d", input.PID)
	}

	// Set defaults
	if input.GracePeriod == 0 {
		input.GracePeriod = 10 * time.Second // Default 10s grace period
	}

	output := &TerminateSessionOutput{
		SessionID:     input.SessionID,
		ProcessKilled: false,
		FilesRemoved:  0,
		TerminatedAt:  time.Now(),
		GracefulExit:  false,
	}

	// Step 1: Try graceful shutdown (SIGTERM)
	process, err := os.FindProcess(input.PID)
	if err != nil {
		// Process doesn't exist (already terminated)
		output.ProcessKilled = true
		output.GracefulExit = true
	} else {
		// Send SIGTERM for graceful shutdown
		if err := process.Signal(syscall.SIGTERM); err != nil {
			// Process may have already exited
			if err.Error() != "os: process already finished" {
				return nil, fmt.Errorf("failed to send SIGTERM: %w", err)
			}
			output.ProcessKilled = true
			output.GracefulExit = true
		} else {
			// Wait for graceful shutdown
			gracefulDone := make(chan bool, 1)
			go func() {
				// Wait for process to exit
				_, _ = process.Wait()
				gracefulDone <- true
			}()

			select {
			case <-gracefulDone:
				// Process exited gracefully
				output.ProcessKilled = true
				output.GracefulExit = true
			case <-time.After(input.GracePeriod):
				// Grace period expired
				if input.ForceKill {
					// Force kill with SIGKILL
					if err := process.Signal(syscall.SIGKILL); err != nil {
						return nil, fmt.Errorf("failed to send SIGKILL: %w", err)
					}
					// Give it a moment to die
					time.Sleep(500 * time.Millisecond)
					output.ProcessKilled = true
					output.GracefulExit = false
				} else {
					return nil, fmt.Errorf("process did not exit gracefully within %v", input.GracePeriod)
				}
			case <-ctx.Done():
				// Context cancelled
				return nil, ctx.Err()
			}
		}
	}

	// Step 2: Cleanup temporary files if requested
	if input.CleanupFiles {
		filesRemoved, err := cleanupSessionFiles(input.SessionID)
		if err != nil {
			// Log error but don't fail termination
			fmt.Fprintf(os.Stderr, "Warning: failed to cleanup session files: %v\n", err)
		}
		output.FilesRemoved = filesRemoved
	}

	// Step 3: Archive session data if requested
	if input.ArchiveSession {
		if err := archiveSessionData(input.SessionID); err != nil {
			// Log error but don't fail termination
			fmt.Fprintf(os.Stderr, "Warning: failed to archive session data: %v\n", err)
		} else {
			output.SessionArchived = true
		}
	}

	output.TerminatedAt = time.Now()
	return output, nil
}

// cleanupSessionFiles removes temporary files for a session
// Returns the number of files removed
func cleanupSessionFiles(sessionID string) (int, error) {
	sessionDir, err := GetSessionDataDir(sessionID)
	if err != nil {
		return 0, err
	}

	filesRemoved := 0

	// List of temporary file patterns to clean up
	tempPatterns := []string{
		"*.tmp",
		"*.pid",
		"*.lock",
		"temp_*",
	}

	for _, pattern := range tempPatterns {
		matches, err := filepath.Glob(filepath.Join(sessionDir, pattern))
		if err != nil {
			continue
		}

		for _, file := range matches {
			if err := os.Remove(file); err != nil {
				// Log but continue
				fmt.Fprintf(os.Stderr, "Warning: failed to remove %s: %v\n", file, err)
			} else {
				filesRemoved++
			}
		}
	}

	return filesRemoved, nil
}

// archiveSessionData archives session data to the archive directory
// This preserves checkpoint, logs, and other session data for later reference
func archiveSessionData(sessionID string) error {
	sessionDir, err := GetSessionDataDir(sessionID)
	if err != nil {
		return err
	}

	// Get archive directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	archiveDir := filepath.Join(homeDir, ".agm", "archive", sessionID)
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return fmt.Errorf("failed to create archive directory: %w", err)
	}

	// Copy important files to archive
	filesToArchive := []string{
		"checkpoint.json",
		"manifest.yaml",
		"session.log",
	}

	for _, filename := range filesToArchive {
		srcPath := filepath.Join(sessionDir, filename)
		dstPath := filepath.Join(archiveDir, filename)

		// Skip if source doesn't exist
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			continue
		}

		// Copy file
		if err := copyFile(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to archive %s: %w", filename, err)
		}
	}

	// Create archive metadata
	metadata := fmt.Sprintf("Session %s archived at %s\n", sessionID, time.Now().Format(time.RFC3339))
	metadataPath := filepath.Join(archiveDir, "archive_info.txt")
	if err := os.WriteFile(metadataPath, []byte(metadata), 0644); err != nil {
		return fmt.Errorf("failed to write archive metadata: %w", err)
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, 0644)
}

// KillProcessActivity forcefully kills a process (helper activity)
func KillProcessActivity(ctx context.Context, pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid PID: %d", pid)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	if err := process.Signal(syscall.SIGKILL); err != nil {
		return fmt.Errorf("failed to kill process: %w", err)
	}

	return nil
}

// CheckProcessActivity checks if a process is running
func CheckProcessActivity(ctx context.Context, pid int) (bool, error) {
	if pid <= 0 {
		return false, fmt.Errorf("invalid PID: %d", pid)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		// Process doesn't exist
		return false, nil
	}

	// Try sending signal 0 (doesn't actually send a signal, just checks if process exists)
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// Process doesn't exist
		return false, nil
	}

	return true, nil
}

// CleanupSessionActivity removes all session data (for testing or complete cleanup)
func CleanupSessionActivity(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}

	sessionDir, err := GetSessionDataDir(sessionID)
	if err != nil {
		return err
	}

	// Remove entire session directory
	if err := os.RemoveAll(sessionDir); err != nil {
		if os.IsNotExist(err) {
			// Already removed
			return nil
		}
		return fmt.Errorf("failed to remove session directory: %w", err)
	}

	return nil
}
