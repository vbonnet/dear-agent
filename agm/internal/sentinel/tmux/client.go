// Package tmux provides tmux session management.
package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	agmtmux "github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// Client wraps tmux command execution for session monitoring.
// Handles socket detection and command execution.
type Client struct {
	// socketPaths contains all tmux socket paths to check.
	// Includes AGM socket (~/.agm/agm.sock), legacy (/tmp/agm.sock), and system default.
	socketPaths []string
}

// NewClient creates a new tmux client.
// Automatically detects available tmux sockets (AGM + system default).
func NewClient() *Client {
	socketPaths := getReadSocketPaths()
	return &Client{
		socketPaths: socketPaths,
	}
}

// getReadSocketPaths returns all tmux socket paths to check for sessions.
// Returns AGM socket (~/.agm/agm.sock) if it exists, plus legacy and system default.
func getReadSocketPaths() []string {
	var paths []string

	// AGM socket (primary — ~/.agm/agm.sock)
	agmSocket := agmtmux.DefaultSocketPath()
	if _, err := os.Stat(agmSocket); err == nil {
		paths = append(paths, agmSocket)
	}

	// Legacy socket (/tmp/agm.sock) — check for sessions from before migration
	if agmtmux.LegacySocketPath != agmSocket {
		if _, err := os.Stat(agmtmux.LegacySocketPath); err == nil {
			paths = append(paths, agmtmux.LegacySocketPath)
		}
	}

	// System default socket. tmux's actual default location is
	// $TMUX_TMPDIR/tmux-{uid}/default, falling back to /tmp — NOT
	// os.TempDir(), which honors $TMPDIR (e.g. /var/folders/... on macOS)
	// and won't match where tmux actually puts the socket.
	tmuxTmp := os.Getenv("TMUX_TMPDIR")
	if tmuxTmp == "" {
		tmuxTmp = "/tmp"
	}
	systemSocket := filepath.Join(tmuxTmp, fmt.Sprintf("tmux-%d", os.Getuid()), "default")
	if _, err := os.Stat(systemSocket); err == nil {
		paths = append(paths, systemSocket)
	}

	// Fallback: if no sockets found, use empty string (tmux will use default)
	if len(paths) == 0 {
		paths = append(paths, "")
	}

	return paths
}

// findSessionSocket returns the socket path for a specific session.
// Returns empty string and error if session not found on any socket.
// Uses exact-match (= prefix) to prevent tmux prefix matching where
// targeting "foo" could accidentally match "foo-bar".
func (c *Client) findSessionSocket(sessionName string) (string, error) {
	// Use = prefix for exact session name matching (prevents prefix matching).
	// Without this, "tmux has-session -t test" matches "test-something".
	exactTarget := "=" + sessionName
	for _, socketPath := range c.socketPaths {
		args := []string{}
		if socketPath != "" {
			args = append(args, "-S", socketPath)
		}
		args = append(args, "has-session", "-t", exactTarget)

		cmd := exec.Command("tmux", args...)
		if err := cmd.Run(); err == nil {
			return socketPath, nil
		}
	}

	return "", fmt.Errorf("session %s not found on any tmux socket", sessionName)
}

// ListSessions returns all tmux sessions across all sockets.
// Deduplicates sessions that may appear on multiple sockets.
func (c *Client) ListSessions() ([]string, error) {
	sessionSet := make(map[string]bool)

	for _, socketPath := range c.socketPaths {
		args := []string{}
		if socketPath != "" {
			args = append(args, "-S", socketPath)
		}
		args = append(args, "list-sessions", "-F", "#{session_name}")

		cmd := exec.Command("tmux", args...)
		output, err := cmd.Output()
		if err != nil {
			// Socket may not have any sessions, continue to next
			continue
		}

		sessions := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, session := range sessions {
			if session != "" {
				sessionSet[session] = true
			}
		}
	}

	// Return an empty (non-nil) slice when no sessions match, so callers
	// can range without nil-checking. Pre-allocates by map size to avoid
	// reallocating during the loop.
	sessions := make([]string, 0, len(sessionSet))
	for session := range sessionSet {
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// GetPaneContent captures the content of a tmux pane.
// Captures up to 500 lines of scrollback history.
// Returns error if session not found or capture fails.
func (c *Client) GetPaneContent(sessionName string) (string, error) {
	socketPath, err := c.findSessionSocket(sessionName)
	if err != nil {
		return "", err
	}

	args := []string{}
	if socketPath != "" {
		args = append(args, "-S", socketPath)
	}
	args = append(args, "capture-pane", "-t", sessionName, "-p", "-S", "-500")

	cmd := exec.Command("tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to capture pane for session %s: %w", sessionName, err)
	}

	return string(output), nil
}

// GetCursorPosition returns the cursor position (x, y) for a session.
// Returns error if session not found or position cannot be retrieved.
func (c *Client) GetCursorPosition(sessionName string) (int, int, error) {
	socketPath, err := c.findSessionSocket(sessionName)
	if err != nil {
		return 0, 0, err
	}

	args := []string{}
	if socketPath != "" {
		args = append(args, "-S", socketPath)
	}
	args = append(args, "display-message", "-t", sessionName, "-p", "#{cursor_x},#{cursor_y}")

	cmd := exec.Command("tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get cursor position for session %s: %w", sessionName, err)
	}

	var x, y int
	_, err = fmt.Sscanf(strings.TrimSpace(string(output)), "%d,%d", &x, &y)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse cursor position: %w", err)
	}

	return x, y, nil
}

// SendKeys sends keystrokes to a tmux pane.
// For special keys, use tmux key names (e.g., "Escape", "C-c").
// Returns error if session not found or send fails.
func (c *Client) SendKeys(sessionName, keys string) error {
	socketPath, err := c.findSessionSocket(sessionName)
	if err != nil {
		return err
	}

	args := []string{}
	if socketPath != "" {
		args = append(args, "-S", socketPath)
	}
	args = append(args, "send-keys", "-t", sessionName, keys)

	cmd := exec.Command("tmux", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send keys to session %s: %w", sessionName, err)
	}

	return nil
}

// HasSession checks if a tmux session exists.
func (c *Client) HasSession(sessionName string) bool {
	_, err := c.findSessionSocket(sessionName)
	return err == nil
}

// GetPaneInfo gets comprehensive pane information for a session.
// Returns PaneInfo with content, cursor position, and timestamp.
func (c *Client) GetPaneInfo(sessionName string) (*PaneInfo, error) {
	// Get pane content
	content, err := c.GetPaneContent(sessionName)
	if err != nil {
		return nil, err
	}

	// Get cursor position
	cursorX, cursorY, err := c.GetCursorPosition(sessionName)
	if err != nil {
		return nil, err
	}

	// Create PaneInfo
	paneInfo := &PaneInfo{
		SessionName: sessionName,
		Content:     content,
		CursorX:     cursorX,
		CursorY:     cursorY,
		CapturedAt:  time.Now(),
	}

	// Extract last command (if detectable)
	paneInfo.LastCommand = paneInfo.ExtractLastCommand()

	return paneInfo, nil
}
