package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
)

// HasSession checks if tmux session exists
func HasSession(name string) (bool, error) {
	ctx := context.Background()
	socketPath := GetSocketPath()
	_, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "has-session", "-t", name)
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

// NewSession creates a new tmux session with optimized settings
func NewSession(name string, workDir string) error {
	ctx := context.Background()
	socketPath := GetSocketPath()

	// Clean stale socket if exists
	if err := CleanStaleSocket(); err != nil {
		return fmt.Errorf("failed to clean stale socket: %w", err)
	}

	// Create session with detached mode
	cmd, cancel := CommandWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "new-session", "-d", "-s", name, "-c", workDir)
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

	// Inject tmux settings for better UX
	// These settings improve multi-device usage, copy/paste, and mouse support
	settings := []string{
		"set-window-option -g aggressive-resize on", // Fix multi-device layout conflicts
		"set-option -g window-size latest",           // Force window to fit current screen
		"set -g mouse on",                            // Enable mouse scrolling
		"set -s set-clipboard on",                    // Enable OSC 52 for Cmd-C over SSH
	}

	for _, setting := range settings {
		cmd, cancel := CommandWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "send-keys", "-t", name, setting, "C-m")
		if err := cmd.Run(); err != nil {
			// Log warning but don't fail - these are UX improvements, not critical
			fmt.Fprintf(os.Stderr, "Warning: Failed to apply tmux setting '%s': %v\n", setting, err)
		}
		cancel()
		// Small delay to ensure commands are processed
		time.Sleep(50 * time.Millisecond)
	}

	return nil
}

// AttachSession attaches to tmux session or switches if already inside tmux
// Returns nil if session exists and was successfully switched/attached
// In non-interactive environments (no TTY), it skips attach and returns nil
// IMPORTANT: This function uses syscall.Exec to replace the process, so it does NOT return
func AttachSession(name string) error {
	ctx := context.Background()
	socketPath := GetSocketPath()

	// Check if we're already inside a tmux session
	if os.Getenv("TMUX") != "" {
		// Already in tmux - use switch-client instead of attach
		cmd, cancel := CommandWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "switch-client", "-t", name)
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

	// Have a real TTY - use attach-session with zero-overhead exec
	// CRITICAL: This replaces the current process, so ensure all cleanup is done BEFORE calling this

	// Find tmux binary path
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found in PATH: %w", err)
	}

	// Build arguments for tmux attach
	// DO NOT use -d (detached) flag - we want to attach interactively
	args := []string{
		"tmux",                 // argv[0] - program name
		"-S", socketPath,       // Use isolated socket
		"attach-session",       // Command
		"-t", name,             // Target session
	}

	// Get current environment
	env := os.Environ()

	// Replace current process with tmux
	// This is the LAST statement - process is replaced, NO RETURN!
	// syscall.Exec does NOT return on success
	err = syscall.Exec(tmuxPath, args, env)
	if err != nil {
		// Only reached if exec fails
		return fmt.Errorf("failed to exec tmux attach: %w", err)
	}

	// Unreachable code - exec replaces the process
	return nil
}

// SendCommand sends a command to tmux pane
func SendCommand(sessionName string, command string) error {
	ctx := context.Background()
	socketPath := GetSocketPath()

	// Send command text using -l (literal) flag
	cmdText, cancel1 := CommandWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "send-keys", "-t", sessionName, "-l", command)
	defer cancel1()
	if err := cmdText.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &TimeoutError{
				Problem:  fmt.Sprintf("tmux command timed out after %v (server may be hung)", globalTimeout),
				Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  csm list         # Verify recovery",
				Duration: globalTimeout,
			}
		}
		return fmt.Errorf("failed to send command text to tmux: %w", err)
	}

	// Send Enter key separately (required - sending C-m in same command creates newline in prompt)
	cmdEnter, cancel2 := CommandWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "send-keys", "-t", sessionName, "C-m")
	defer cancel2()
	if err := cmdEnter.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
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
	// Note: -V doesn't need socket path as it doesn't connect to server
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
	socketPath := GetSocketPath()
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "list-sessions", "-F", "#{session_name}")
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

// SessionInfo holds information about a tmux session
type SessionInfo struct {
	Name            string
	AttachedClients int
	AttachedList    string
}

// ListSessionsWithInfo returns all active tmux sessions with attachment information
func ListSessionsWithInfo() ([]SessionInfo, error) {
	ctx := context.Background()
	socketPath := GetSocketPath()
	// Format: session_name:attached_count:attached_list
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "list-sessions", "-F", "#{session_name}:#{session_attached}:#{session_attached_list}")
	if err != nil {
		// Check for timeout error
		if _, ok := err.(*TimeoutError); ok {
			return nil, err
		}
		// If no tmux server running, return empty list
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []SessionInfo{}, nil
		}
		return nil, fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	sessions := make([]SessionInfo, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Parse "name:count:attached_list" format
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
	socketPath := GetSocketPath()
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "display-message", "-p", "#S")
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
	socketPath := GetSocketPath()
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "list-panes", "-t", sessionName,
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

// GetCurrentWorkingDirectory returns the current working directory of the
// first pane in the tmux session.
//
// Returns the absolute path to the working directory, or an error if the
// command fails or the session doesn't exist.
func GetCurrentWorkingDirectory(sessionName string) (string, error) {
	ctx := context.Background()
	socketPath := GetSocketPath()
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "display-message", "-t", sessionName, "-p", "#{pane_current_path}")
	if err != nil {
		// Check for timeout error
		if _, ok := err.(*TimeoutError); ok {
			return "", err
		}
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}
