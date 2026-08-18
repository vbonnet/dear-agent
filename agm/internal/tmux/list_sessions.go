package tmux

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ListSessionsWithInfoStrict lists sessions without collapsing a tmux exec
// failure into an empty result. ListSessionsWithInfo maps every ExitError to
// "[]SessionInfo{}, nil" so a missing server reads as "no sessions" — but a
// permission-denied or unreachable socket produces the same ExitError, and a
// caller that treats the empty list as an observation then concludes every live
// session is stopped. Callers that must distinguish "observed, nothing running"
// from "could not observe" use this form (ce-0zng9).
func ListSessionsWithInfoStrict() ([]SessionInfo, error) {
	ctx := context.Background()
	socketPath := GetSocketPath()
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "list-sessions", "-F", "#{session_name}:#{session_attached}:#{session_attached_list}")
	if err != nil {
		// A running server with zero sessions exits 1 ("no sessions"); that
		// is a successful observation of an empty list, not a failure to
		// observe. Mirror ListSessionsWithInfo's classification: only a
		// non-ExitError (socket/permission/timeout) is a real failure.
		timeoutError := &TimeoutError{}
		if errors.As(err, &timeoutError) {
			return nil, err
		}
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			return []SessionInfo{}, nil
		}
		return nil, fmt.Errorf("failed to list tmux sessions: %w", err)
	}
	return parseSessionInfoLines(string(output)), nil
}

// parseSessionInfoLines parses the shared "name:count:attached_list" format
// emitted by both list-sessions callers.
func parseSessionInfoLines(output string) []SessionInfo {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	sessions := make([]SessionInfo, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 2 {
			continue
		}
		var attachedCount int
		fmt.Sscanf(parts[1], "%d", &attachedCount)

		attachedList := ""
		if len(parts) >= 3 {
			attachedList = parts[2]
		}

		sessions = append(sessions, SessionInfo{
			Name:            parts[0],
			AttachedClients: attachedCount,
			AttachedList:    attachedList,
		})
	}
	return sessions
}
