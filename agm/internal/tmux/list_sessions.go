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
		timeoutError := &TimeoutError{}
		if errors.As(err, &timeoutError) {
			return nil, err
		}
		// tmux exits non-zero both when it observed an empty server and when
		// it could not reach one at all, so the exit status alone cannot tell
		// them apart. Classifying on *exec.ExitError — as ListSessionsWithInfo
		// does — turns a permission-denied or otherwise unreachable socket
		// into a successful observation of zero sessions, which is exactly the
		// misclassification this strict form exists to prevent. Decide on the
		// message tmux actually printed instead.
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) && isEmptyServerOutput(string(output)) {
			return []SessionInfo{}, nil
		}
		return nil, fmt.Errorf("failed to list tmux sessions: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return parseSessionInfoLines(string(output)), nil
}

// isEmptyServerOutput reports whether tmux's own message describes a server
// that was reached and holds no sessions, or no server at all — both of which
// are genuine observations that nothing is running. Anything else (permission
// denied, a socket that exists but cannot be spoken to, an unexpected
// diagnostic) is a failure to observe and must not read as "no sessions".
func isEmptyServerOutput(output string) bool {
	msg := strings.ToLower(strings.TrimSpace(output))
	if msg == "" {
		return false
	}
	switch {
	case strings.Contains(msg, "no server running"):
		return true
	case strings.Contains(msg, "no sessions"):
		return true
	case strings.Contains(msg, "error connecting") && strings.Contains(msg, "no such file or directory"):
		return true
	default:
		return false
	}
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
