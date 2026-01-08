package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/debug"
	"golang.org/x/term"
)

// HasSession checks if tmux session exists
func HasSession(name string) (bool, error) {
	ctx := context.Background()
	_, err := RunWithTimeout(ctx, globalTimeout, "tmux", "has-session", "-t", name)
	if err != nil {
		// Check for timeout error
		if _, ok := err.(*TimeoutError); ok {
			return false, err
		}
		// Exit code 1 means session doesn't exist
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("failed to check tmux session: %w", err)
	}
	return true, nil
}

// NewSession creates a new tmux session
func NewSession(name string, workDir string) error {
	ctx := context.Background()
	cmd, cancel := CommandWithTimeout(ctx, globalTimeout, "tmux", "new-session", "-d", "-s", name, "-c", workDir)
	defer cancel()
	if err := cmd.Run(); err != nil {
		// Check for timeout
		if ctx.Err() == context.DeadlineExceeded {
			return &TimeoutError{
				Problem:  fmt.Sprintf("tmux command timed out after %v (server may be hung)", globalTimeout),
				Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  csm list         # Verify recovery",
				Duration: globalTimeout,
			}
		}
		return fmt.Errorf("failed to create tmux session: %w", err)
	}
	return nil
}

// AttachSession attaches to tmux session or switches if already inside tmux
// Returns nil if session exists and was successfully switched/attached
// In non-interactive environments (no TTY), it skips attach and returns nil
func AttachSession(name string) error {
	ctx := context.Background()

	// Check if we're already inside a tmux session
	if os.Getenv("TMUX") != "" {
		// Already in tmux - use switch-client instead of attach
		cmd, cancel := CommandWithTimeout(ctx, globalTimeout, "tmux", "switch-client", "-t", name)
		defer cancel()
		if err := cmd.Run(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return &TimeoutError{
					Problem:  fmt.Sprintf("tmux command timed out after %v (server may be hung)", globalTimeout),
					Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  csm list         # Verify recovery",
					Duration: globalTimeout,
				}
			}
			return fmt.Errorf("failed to switch to tmux session: %w", err)
		}
		return nil
	}

	// Not in tmux - check if we have a TTY available
	// If stdin is not a terminal (e.g., running from Claude Code), skip attach
	// The session is still ready, just can't interactively attach

	// First check: can we stat stdin?
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		// Error checking stdin - assume no TTY and skip attach
		return nil
	}

	// Second check: is it a character device?
	// Note: This alone is insufficient - /dev/null is also a char device
	if (fileInfo.Mode() & os.ModeCharDevice) == 0 {
		// Not a character device - definitely not a TTY
		return nil
	}

	// Third check: use syscall isatty to actually verify it's a terminal
	// This is the proper way to check if stdin is a real terminal
	isTTY := term.IsTerminal(int(os.Stdin.Fd()))
	if !isTTY {
		// stdin is a char device but not a terminal (e.g., /dev/null)
		// Silently skip attach - session is ready, just can't attach interactively
		return nil
	}

	// Have a real TTY - use attach-session
	// Note: attach-session is an interactive command that runs until user detaches
	// DO NOT use a timeout here - the session should run indefinitely
	cmd := exec.Command("tmux", "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to attach to tmux session: %w", err)
	}
	return nil
}

// SendCommand sends a command to tmux pane
func SendCommand(sessionName string, command string) error {
	// Send the command text first
	ctx1 := context.Background()
	cmd, cancel := CommandWithTimeout(ctx1, globalTimeout, "tmux", "send-keys", "-t", sessionName, command)
	if err := cmd.Run(); err != nil {
		cancel()
		if ctx1.Err() == context.DeadlineExceeded {
			return &TimeoutError{
				Problem:  fmt.Sprintf("tmux command timed out after %v (server may be hung)", globalTimeout),
				Recovery:  "  pkill -9 tmux    # Kill hung tmux server\n  csm list         # Verify recovery",
				Duration: globalTimeout,
			}
		}
		return fmt.Errorf("failed to send command to tmux: %w", err)
	}
	cancel()

	// Small delay to ensure tmux processes the text before we send Enter
	// See: https://github.com/tmux/tmux/issues/1778
	time.Sleep(100 * time.Millisecond)

	// Send Enter key separately (C-m doesn't work when combined with text in same command)
	// Need a fresh context since we canceled the first one
	ctx2 := context.Background()
	cmd2, cancel2 := CommandWithTimeout(ctx2, globalTimeout, "tmux", "send-keys", "-t", sessionName, "C-m")
	defer cancel2()
	if err := cmd2.Run(); err != nil {
		if ctx2.Err() == context.DeadlineExceeded {
			return &TimeoutError{
				Problem:  fmt.Sprintf("tmux command timed out after %v (server may be hung)", globalTimeout),
				Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  csm list         # Verify recovery",
				Duration: globalTimeout,
			}
		}
		return fmt.Errorf("failed to send Enter key to tmux: %w", err)
	}
	return nil
}

// Version returns tmux version
func Version() (string, error) {
	ctx := context.Background()
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-V")
	if err != nil {
		// Check for timeout error
		if _, ok := err.(*TimeoutError); ok {
			return "", err
		}
		return "", fmt.Errorf("failed to get tmux version: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// ListSessions returns all active tmux session names
func ListSessions() ([]string, error) {
	ctx := context.Background()
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "list-sessions", "-F", "#{session_name}")
	if err != nil {
		// Check for timeout error
		if _, ok := err.(*TimeoutError); ok {
			return nil, err
		}
		// If no tmux server running, return empty list
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	sessions := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}

// GetCurrentSessionName returns the name of the current tmux session
// Returns error if not running inside tmux or if command fails
func GetCurrentSessionName() (string, error) {
	// Check if we're in a tmux session
	if os.Getenv("TMUX") == "" {
		return "", fmt.Errorf("not running inside a tmux session")
	}

	ctx := context.Background()
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "display-message", "-p", "#S")
	if err != nil {
		// Check for timeout error
		if _, ok := err.(*TimeoutError); ok {
			return "", err
		}
		return "", fmt.Errorf("failed to get current tmux session name: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// IsProcessRunning checks if a specific process is running as the foreground
// process in any pane of the tmux session. Used to detect if Claude is already
// active before sending resume commands, preventing text injection.
//
// Limitations:
// - Only detects foreground processes (suspended processes appear as shell)
// - Requires tmux 2.6+ for #{pane_current_command} format string support
// - Process name matching is case-sensitive and exact
//
// Returns (true, nil) if process found in any pane
// Returns (false, nil) if process not found
// Returns (false, error) if tmux command fails
func IsProcessRunning(sessionName, processName string) (bool, error) {
	ctx := context.Background()
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "list-panes", "-t", sessionName,
		"-F", "#{pane_current_command}")
	if err != nil {
		// Check for timeout error
		if _, ok := err.(*TimeoutError); ok {
			return false, err
		}
		return false, fmt.Errorf("failed to check tmux pane process: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == processName {
			return true, nil
		}
	}

	return false, nil
}

// WaitForProcessReady polls until the specified process is running in the
// tmux session, or returns error on timeout. This improves UX by ensuring
// Claude is fully started before attaching to the tmux session.
//
// Parameters:
//   - sessionName: tmux session to check
//   - processName: process to wait for (e.g., "claude")
//   - timeout: maximum time to wait
//
// Returns nil when process is ready, error on timeout or check failure.
func WaitForProcessReady(sessionName, processName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 100 * time.Millisecond

	for time.Now().Before(deadline) {
		running, err := IsProcessRunning(sessionName, processName)
		if err != nil {
			// Ignore transient errors (e.g., brief tmux unavailability)
			time.Sleep(pollInterval)
			continue
		}
		if running {
			return nil // Process is ready!
		}
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timeout waiting for %s to start (waited %v)", processName, timeout)
}

// WaitForInputReady polls the tmux pane until the input prompt appears,
// indicating that the program is ready to accept commands. This is needed
// because even after the process is running, the input handler may not be
// fully initialized yet.
//
// Parameters:
//   - sessionName: tmux session to check
//   - promptPattern: text pattern to look for (e.g., "> " for Claude)
//   - timeout: maximum time to wait
//
// Returns nil when prompt is ready, error on timeout.
func WaitForInputReady(sessionName, promptPattern string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 100 * time.Millisecond
	pollCount := 0

	debug.Log("Starting input ready polling (interval: %v, timeout: %v)", pollInterval, timeout)

	for time.Now().Before(deadline) {
		ctx := context.Background()
		pollCount++

		// Capture last 200 lines including scrollback to catch prompt even if SessionStart hooks scrolled it up
		output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "capture-pane", "-t", sessionName, "-p", "-S", "-200")
		if err != nil {
			// Ignore transient errors
			if pollCount%10 == 0 { // Log every 10th poll (~1 second)
				debug.Log("Poll #%d: capture-pane error: %v", pollCount, err)
			}
			time.Sleep(pollInterval)
			continue
		}

		// Log captured output periodically
		if pollCount%50 == 0 { // Log every 50th poll (~5 seconds)
			lines := strings.Split(string(output), "\n")
			lastLines := lines
			if len(lines) > 10 {
				lastLines = lines[len(lines)-10:]
			}
			debug.Log("Poll #%d: Last %d lines of captured output:", pollCount, len(lastLines))
			for i, line := range lastLines {
				if len(line) > 100 {
					line = line[:100] + "..."
				}
				debug.Log("  [%d] %s", i, line)
			}
		}

		// Check if prompt pattern appears in output
		if strings.Contains(string(output), promptPattern) {
			debug.Log("Poll #%d: Found prompt pattern '%s' in output!", pollCount, promptPattern)
			debug.Log("Input is ready after %d polls", pollCount)
			return nil // Input is ready!
		}

		time.Sleep(pollInterval)
	}

	debug.Log("Timeout after %d polls waiting for prompt pattern '%s'", pollCount, promptPattern)
	return fmt.Errorf("timeout waiting for input prompt (waited %v)", timeout)
}

// GetCurrentWorkingDirectory returns the current working directory of the
// first pane in the tmux session.
//
// Returns the absolute path to the working directory, or an error if the
// command fails or the session doesn't exist.
func GetCurrentWorkingDirectory(sessionName string) (string, error) {
	ctx := context.Background()
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "display-message", "-t", sessionName, "-p", "#{pane_current_path}")
	if err != nil {
		// Check for timeout error
		if _, ok := err.(*TimeoutError); ok {
			return "", err
		}
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}
