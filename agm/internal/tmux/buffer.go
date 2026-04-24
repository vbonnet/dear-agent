package tmux

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CleanupBuffers removes all AGM-related named buffers from the tmux server.
// This prevents buffer accumulation from failed paste-buffer operations that
// can lead to memory pressure and eventual tmux server crash.
//
// Bug fix (2026-04-02): Under high load with 15+ concurrent sessions, orphaned
// "agm-cmd" buffers accumulated when paste-buffer failed or timed out. The -d flag
// on paste-buffer only deletes on success; failures left buffers indefinitely.
// This function provides explicit garbage collection.
//
// Returns the number of buffers cleaned up, or error if listing fails.
func CleanupBuffers() (int, error) {
	ctx := context.Background()
	socketPath := GetSocketPath()

	// List all named buffers
	output, err := RunWithTimeout(ctx, 5*time.Second, "tmux", "-S", socketPath, "list-buffers", "-F", "#{buffer_name}")
	if err != nil {
		// If tmux server is not running, nothing to clean
		if IsServerDeadError(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to list tmux buffers: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	cleaned := 0
	for _, name := range lines {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		// Clean up AGM buffers (agm-cmd is the primary one)
		if strings.HasPrefix(name, "agm-") {
			cmd, cancel := CommandWithTimeout(ctx, 2*time.Second, "tmux", "-S", socketPath, "delete-buffer", "-b", name)
			if err := cmd.Run(); err == nil {
				cleaned++
			}
			cancel()
		}
	}

	return cleaned, nil
}

// BufferCount returns the number of named buffers currently in the tmux server.
// Useful for monitoring buffer accumulation.
func BufferCount() (int, error) {
	ctx := context.Background()
	socketPath := GetSocketPath()

	output, err := RunWithTimeout(ctx, 5*time.Second, "tmux", "-S", socketPath, "list-buffers", "-F", "#{buffer_name}")
	if err != nil {
		if IsServerDeadError(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to list tmux buffers: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count, nil
}
