package tmux

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CapturePaneContent captures the visible content from a tmux pane for the given session.
// Returns the pane content as a string, or an error if the session doesn't exist or tmux is not running.
//
// Parameters:
//   - sessionName: The name of the tmux session to capture from
//
// Returns:
//   - string: The captured pane content
//   - error: An error if the capture fails
//
// Edge cases handled:
//   - Session not found: Returns ErrSessionNotFound
//   - Tmux not running: Returns ErrTmuxNotRunning
//   - Permission denied: Returns ErrPermissionDenied
//   - Large content (>100KB): Handled correctly
func CapturePaneContent(sessionName string) (string, error) {
	if sessionName == "" {
		return "", fmt.Errorf("session name cannot be empty")
	}

	// Check if tmux is running
	if !IsTmuxRunning() {
		return "", ErrTmuxNotRunning
	}

	// Capture pane content using tmux capture-pane
	// -p: print to stdout
	// -t: target session
	cmd := exec.Command("tmux", "capture-pane", "-p", "-t", sessionName)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Parse error type
		errMsg := string(output)
		if strings.Contains(errMsg, "can't find pane") ||
			strings.Contains(errMsg, "can't find session") ||
			strings.Contains(errMsg, "no such session") ||
			strings.Contains(errMsg, "session not found") {
			return "", ErrSessionNotFound
		}
		if strings.Contains(errMsg, "permission denied") {
			return "", ErrPermissionDenied
		}
		return "", fmt.Errorf("failed to capture pane: %w", err)
	}

	return string(output), nil
}

// IsTmuxRunning checks if the tmux server is currently running.
func IsTmuxRunning() bool {
	cmd := exec.Command("tmux", "list-sessions")
	err := cmd.Run()
	// If tmux is not running, this will return an error
	// If tmux is running but there are no sessions, it still returns 0
	return err == nil || strings.Contains(err.Error(), "no server running")
}

// CapturePaneHistory captures the full scrollback history from a tmux pane.
// This is useful for getting more context than just the visible pane content.
//
// Parameters:
//   - sessionName: The name of the tmux session
//   - lines: Number of lines of history to capture (0 = all available)
//
// Returns:
//   - string: The captured pane history
//   - error: An error if the capture fails
func CapturePaneHistory(sessionName string, lines int) (string, error) {
	if sessionName == "" {
		return "", fmt.Errorf("session name cannot be empty")
	}

	if !IsTmuxRunning() {
		return "", ErrTmuxNotRunning
	}

	args := []string{"capture-pane", "-p", "-t", sessionName}

	// Add history limits if specified
	if lines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	} else {
		// Capture entire scrollback buffer
		args = append(args, "-S", "-")
	}

	cmd := exec.Command("tmux", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		errMsg := string(output)
		if strings.Contains(errMsg, "can't find pane") ||
			strings.Contains(errMsg, "can't find session") ||
			strings.Contains(errMsg, "no such session") ||
			strings.Contains(errMsg, "session not found") {
			return "", ErrSessionNotFound
		}
		if strings.Contains(errMsg, "permission denied") {
			return "", ErrPermissionDenied
		}
		return "", fmt.Errorf("failed to capture pane history: %w", err)
	}

	return string(output), nil
}

// GetSessionInfo returns basic information about a tmux session.
type SessionInfo struct {
	Name     string
	Windows  int
	Created  string
	Attached bool
}

// GetSessionInfo retrieves information about a specific tmux session.
func GetSessionInfo(sessionName string) (*SessionInfo, error) {
	if sessionName == "" {
		return nil, fmt.Errorf("session name cannot be empty")
	}

	if !IsTmuxRunning() {
		return nil, ErrTmuxNotRunning
	}

	// Get session info using tmux list-sessions
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}|#{session_windows}|#{session_created}|#{session_attached}")
	output, err := cmd.CombinedOutput()

	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	// Parse output to find our session
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) != 4 {
			continue
		}

		if parts[0] == sessionName {
			info := &SessionInfo{
				Name:     parts[0],
				Created:  parts[2],
				Attached: parts[3] == "1",
			}

			// Parse window count
			fmt.Sscanf(parts[1], "%d", &info.Windows)

			return info, nil
		}
	}

	return nil, ErrSessionNotFound
}

// SessionExists checks if a tmux session with the given name exists.
func SessionExists(sessionName string) (bool, error) {
	if sessionName == "" {
		return false, fmt.Errorf("session name cannot be empty")
	}

	if !IsTmuxRunning() {
		return false, nil
	}

	cmd := exec.Command("tmux", "has-session", "-t", sessionName)
	err := cmd.Run()
	return err == nil, nil
}

// CapturePaneLines is a wrapper around CapturePaneContent that returns lines as a slice.
// This is useful for CLI commands that need to process output line by line.
//
// Parameters:
//   - sessionName: The name of the tmux session
//   - lines: Number of lines to capture
//
// Returns:
//   - []string: Array of captured lines
//   - error: An error if the capture fails
func CapturePaneLines(sessionName string, lines int) ([]string, error) {
	if sessionName == "" {
		return nil, fmt.Errorf("session name cannot be empty")
	}

	if !IsTmuxRunning() {
		return nil, ErrTmuxNotRunning
	}

	// Use tmux capture-pane with -S to specify number of lines from end
	args := []string{"capture-pane", "-p", "-t", sessionName}
	if lines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	}

	cmd := exec.Command("tmux", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		errMsg := string(output)
		if strings.Contains(errMsg, "can't find pane") ||
			strings.Contains(errMsg, "can't find session") ||
			strings.Contains(errMsg, "no such session") ||
			strings.Contains(errMsg, "session not found") {
			return nil, ErrSessionNotFound
		}
		if strings.Contains(errMsg, "permission denied") {
			return nil, ErrPermissionDenied
		}
		return nil, fmt.Errorf("failed to capture pane: %w", err)
	}

	// Split into lines and trim trailing empty line
	text := strings.TrimRight(string(output), "\n")
	if text == "" {
		return []string{}, nil
	}
	return strings.Split(text, "\n"), nil
}

// CapturePaneHistoryLines is a wrapper around CapturePaneHistory that returns lines as a slice.
//
// Parameters:
//   - sessionName: The name of the tmux session
//   - lines: Number of lines of history to capture (0 = all available)
//
// Returns:
//   - []string: Array of captured lines
//   - error: An error if the capture fails
func CapturePaneHistoryLines(sessionName string, lines int) ([]string, error) {
	content, err := CapturePaneHistory(sessionName, lines)
	if err != nil {
		return nil, err
	}

	// Split into lines and trim trailing empty line
	text := strings.TrimRight(content, "\n")
	if text == "" {
		return []string{}, nil
	}
	return strings.Split(text, "\n"), nil
}

// Now returns the current time as a Unix timestamp in milliseconds.
// This is used for timestamping captured content.
func Now() int64 {
	return time.Now().UnixMilli()
}
