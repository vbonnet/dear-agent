// Package tmux provides tmux session management.
package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/procguard"
	agmtmux "github.com/vbonnet/dear-agent/agm/internal/tmux"
)

const (
	commandTimeout   = 5 * time.Second
	commandWaitDelay = time.Second
)

// Client wraps tmux command execution for session monitoring.
// Handles socket detection and command execution.
type Client struct {
	// socketPaths contains all tmux socket paths to check.
	// Includes AGM socket (~/.agm/agm.sock), legacy (/tmp/agm.sock), and system default.
	socketPaths []string
	runCommand  tmuxCommandRunner
}

type tmuxCommandRunner func(args ...string) ([]byte, error)

func runBoundedTmuxCommand(args ...string) ([]byte, error) {
	cmd, cancel := boundedCommand(args...)
	defer cancel()
	return cmd.Output()
}

func (c *Client) run(args ...string) ([]byte, error) {
	if c.runCommand != nil {
		return c.runCommand(args...)
	}
	return runBoundedTmuxCommand(args...)
}

// boundedCommand applies AGM's subprocess safety contract to every tmux
// invocation in this client: deadline, isolated process group, group-wide
// cancellation, and bounded pipe-drain waiting.
func boundedCommand(args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	cmd := exec.CommandContext(ctx, "tmux", args...)
	cmd.SysProcAttr = procguard.ProcessGroupAttr()
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = commandWaitDelay
	return cmd, cancel
}

// NewClient creates a new tmux client.
// Automatically detects available tmux sockets (AGM + system default).
func NewClient() *Client {
	socketPaths := getReadSocketPaths()
	return &Client{
		socketPaths: socketPaths,
		runCommand:  runBoundedTmuxCommand,
	}
}

// NewClientWithSocket creates a client restricted to one explicitly configured
// tmux socket. Unlike NewClient, it never auto-discovers AGM, legacy, or system
// sockets. This is the safety boundary for callers that own an isolated socket.
func NewClientWithSocket(socketPath string) *Client {
	return newClientWithSocketAndRunner(socketPath, runBoundedTmuxCommand)
}

func newClientWithSocketAndRunner(socketPath string, runner tmuxCommandRunner) *Client {
	return &Client{socketPaths: []string{socketPath}, runCommand: runner}
}

// ConfiguredSocket reports the single explicit socket owned by this client.
// Auto-discovery clients and the system-default empty socket are not explicit.
func (c *Client) ConfiguredSocket() (string, bool) {
	if len(c.socketPaths) != 1 || c.socketPaths[0] == "" {
		return "", false
	}
	return c.socketPaths[0], true
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
	var probeErrors []error
	for _, socketPath := range c.socketPaths {
		args := []string{}
		if socketPath != "" {
			args = append(args, "-S", socketPath)
		}
		args = append(args, "has-session", "-t", exactTarget)

		_, err := c.run(args...)
		if err == nil {
			return socketPath, nil
		}
		probeErrors = append(probeErrors, fmt.Errorf("probe socket %q: %w", socketPath, err))
	}

	if probeErr := errors.Join(probeErrors...); probeErr != nil {
		return "", fmt.Errorf("session %s not found on any tmux socket: %w", sessionName, probeErr)
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

		output, err := c.run(args...)
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

	output, err := c.run(args...)
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

	output, err := c.run(args...)
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
// Enter is sent as raw hex 0x0d via -H flag to avoid paste coalescing.
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

	if keys == "Enter" || keys == "C-m" {
		args = append(args, "send-keys", "-t", sessionName, "-H", "0d")
	} else {
		args = append(args, "send-keys", "-t", sessionName, keys)
	}

	if _, err := c.run(args...); err != nil {
		return fmt.Errorf("failed to send keys to session %s: %w", sessionName, err)
	}

	return nil
}

// SendLiteral sends text without interpreting it as tmux key names.
func (c *Client) SendLiteral(sessionName, text string) error {
	socketPath, err := c.findSessionSocket(sessionName)
	if err != nil {
		return err
	}

	args := []string{}
	if socketPath != "" {
		args = append(args, "-S", socketPath)
	}
	args = append(args, "send-keys", "-t", sessionName, "-l", text)
	if _, err := c.run(args...); err != nil {
		return fmt.Errorf("failed to send literal text to session %s: %w", sessionName, err)
	}
	return nil
}

// GetPanePID returns the first pane PID for an exact session target.
func (c *Client) GetPanePID(sessionName string) (int, error) {
	socketPath, err := c.findSessionSocket(sessionName)
	if err != nil {
		return 0, err
	}

	args := []string{}
	if socketPath != "" {
		args = append(args, "-S", socketPath)
	}
	args = append(args, "list-panes", "-t", "="+sessionName, "-F", "#{pane_pid}")
	output, err := c.run(args...)
	if err != nil {
		return 0, fmt.Errorf("failed to get pane PID for session %s: %w", sessionName, err)
	}
	var pid int
	if _, err := fmt.Sscanf(strings.Split(strings.TrimSpace(string(output)), "\n")[0], "%d", &pid); err != nil {
		return 0, fmt.Errorf("failed to parse pane PID for session %s: %w", sessionName, err)
	}
	return pid, nil
}

// KillSession kills an exact session on the socket where it was discovered.
func (c *Client) KillSession(sessionName string) error {
	socketPath, err := c.findSessionSocket(sessionName)
	if err != nil {
		return err
	}

	args := []string{}
	if socketPath != "" {
		args = append(args, "-S", socketPath)
	}
	args = append(args, "kill-session", "-t", "="+sessionName)
	if _, err := c.run(args...); err != nil {
		return fmt.Errorf("failed to kill session %s: %w", sessionName, err)
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
