package tmux

import (
	"errors"
	"fmt"
	"strings"
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
	if probeDialable(socketPath) {
		return nil
	}

	// Socket connection failed — try tmux command as fallback
	// (socket might be temporarily busy but server still alive)
	if probeTmuxCommand(socketPath) {
		return nil
	}

	// Both checks failed — server is unreachable.
	return &ServerDeadError{
		Reason: fmt.Sprintf("socket %s is unreachable by both connect and list-sessions", socketPath),
		Recovery: fmt.Sprintf("  1. Check for an orphaned server: agm admin doctor\n"+
			"  2. If none, remove the stale socket: rm -f %s\n"+
			"  3. Or let AGM recreate it: agm session new <name>", socketPath),
	}
}

// ServerAliveOrRecover checks if the tmux server is alive. If dead, attempts
// to clean up the stale socket so the next operation can start a fresh server.
// Returns nil if server is alive or was successfully cleaned up for restart.
// Returns error if cleanup fails.
func ServerAliveOrRecover() error {
	if err := ServerAlive(); err == nil {
		return nil
	}

	// Server is unreachable — attempt to clean the socket if it is truly stale.
	if err := CleanStaleSocket(); err != nil {
		// An orphaned server (live, but its socket is unreachable) must be
		// propagated verbatim: the generic advice below is `rm -f` the socket,
		// which is precisely the action that created the orphan in the first
		// place and would strand its sessions for good. See ce-7ep9.
		var bound *LiveServerBoundError
		if errors.As(err, &bound) {
			return err
		}
		return &ServerDeadError{
			Reason:   "server crashed and socket cleanup failed",
			Recovery: fmt.Sprintf("  rm -f %s\n  agm session new <name>", GetSocketPath()),
		}
	}

	return nil
}
