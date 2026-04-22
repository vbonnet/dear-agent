package tmux

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

// ServerDeadError indicates the tmux server has crashed or is unreachable.
// This is a distinct error type so callers can detect server death and attempt recovery
// rather than retrying individual operations against a dead server.
//
// Bug fix (2026-04-02): Previously, tmux server death manifested as a cascade of
// individual operation failures (timeout, connection refused, etc.) with no unified
// detection. Callers would retry operations against a dead server, wasting resources.
type ServerDeadError struct {
	Reason   string
	Recovery string
}

func (e *ServerDeadError) Error() string {
	msg := fmt.Sprintf("tmux server is dead: %s", e.Reason)
	if e.Recovery != "" {
		msg += fmt.Sprintf("\n\nRecovery:\n%s", e.Recovery)
	}
	return msg
}

// IsServerDeadError checks if an error indicates the tmux server has crashed.
// Matches against known error patterns from tmux when the server is unreachable.
func IsServerDeadError(err error) bool {
	if err == nil {
		return false
	}
	var sde *ServerDeadError
	if errors.As(err, &sde) {
		return true
	}

	errStr := err.Error()
	deadPatterns := []string{
		"no server running",
		"error connecting to",
		"server not found",
		"connection refused",
		"no such file or directory",
		"broken pipe",
	}
	for _, pattern := range deadPatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}
	return false
}

// ServerAlive performs a lightweight probe to check if the tmux server is responsive.
// Returns nil if the server is alive, ServerDeadError if it's unreachable.
//
// This uses two checks:
//  1. Socket file existence and connectivity (fast, no tmux process spawn)
//  2. Fallback: tmux list-sessions command (spawns process, more reliable)
func ServerAlive() error {
	socketPath := GetSocketPath()

	// Fast path: try to connect to socket directly (avoids spawning tmux process)
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		// Socket connection failed — try tmux command as fallback
		// (socket might be temporarily busy but server still alive)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "list-sessions")
		if cmdErr := cmd.Run(); cmdErr != nil {
			// Both checks failed — server is dead
			return &ServerDeadError{
				Reason: fmt.Sprintf("socket unreachable (%v) and list-sessions failed (%v)", err, cmdErr),
				Recovery: fmt.Sprintf("  1. Remove stale socket: rm -f %s\n"+
					"  2. Restart tmux: tmux -S %s new-session -d\n"+
					"  3. Or let AGM recreate: agm session new <name>", socketPath, socketPath),
			}
		}
		// tmux command succeeded even though socket connect failed — server is alive
		return nil
	}
	conn.Close()
	return nil
}

// ServerAliveOrRecover checks if the tmux server is alive. If dead, attempts
// to clean up the stale socket so the next operation can start a fresh server.
// Returns nil if server is alive or was successfully cleaned up for restart.
// Returns error if cleanup fails.
func ServerAliveOrRecover() error {
	if err := ServerAlive(); err == nil {
		return nil
	}

	// Server is dead — attempt to clean stale socket
	if err := CleanStaleSocket(); err != nil {
		return &ServerDeadError{
			Reason:   "server crashed and socket cleanup failed",
			Recovery: fmt.Sprintf("  rm -f %s\n  agm session new <name>", GetSocketPath()),
		}
	}

	return nil
}
