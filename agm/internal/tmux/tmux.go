package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
)

// HasSession checks if tmux session exists
func HasSession(name string) (bool, error) {
	ctx := context.Background()
	socketPath := GetSocketPath()
	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedName := NormalizeTmuxSessionName(name)
	_, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "has-session", "-t", FormatSessionTarget(normalizedName))
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

// NormalizeTmuxSessionName converts AGM session names to tmux-compatible format.
// This matches tmux's actual normalization behavior when creating sessions.
//
// tmux converts certain characters to dashes:
//   - Dots (.) → dashes (-)
//   - Colons (:) → dashes (-)
//   - Spaces ( ) → dashes (-)
//
// This function is used for lookups to match session names that may have been
// created with dots/colons/spaces but are stored in tmux with dashes.
//
// Examples:
//   - "gemini-task-1.4" → "gemini-task-1-4"
//   - "foo.bar.baz" → "foo-bar-baz"
//   - "test:session" → "test-session"
//   - "my session" → "my-session"
//   - "multi.char:name" → "multi-char-name"
//   - "normal-name" → "normal-name" (unchanged)
//
// Background: BUG-001 (2026-02-19 merge incident)
// Root cause: AGM stored "gemini-task-1.4" but tmux normalized it to "gemini-task-1-4"
// Impact: 40% message delivery failure during multi-session coordination
// Fix: Apply normalization before all tmux lookups
func NormalizeTmuxSessionName(name string) string {
	// tmux converts dots to dashes
	name = strings.ReplaceAll(name, ".", "-")

	// tmux converts colons to dashes
	name = strings.ReplaceAll(name, ":", "-")

	// tmux converts spaces to dashes
	name = strings.ReplaceAll(name, " ", "-")

	// Future: Add other normalizations as discovered
	return name
}

// FormatSessionTarget formats a session name for exact matching in tmux session-level commands.
//
// The = prefix forces exact session name matching, preventing tmux's default prefix matching behavior.
//
// IMPORTANT: This ONLY works for session-level commands:
//   - has-session, kill-session, list-sessions, list-clients, etc.
//
// For pane-level commands (send-keys, capture-pane), the = prefix does NOT work in tmux 3.4.
// Those commands should use plain session names and rely on session validation via HasSession.
//
// Example of prefix matching bug:
//   - Sessions: "astrocyte" (doesn't exist), "astrocyte-improvements" (exists)
//   - Command: tmux has-session -t astrocyte
//   - Result: Matches "astrocyte-improvements" via prefix (WRONG!)
//   - Fix: tmux has-session -t =astrocyte (exact match, returns error)
func FormatSessionTarget(sessionName string) string {
	return "=" + sessionName
}

// SanitizeSessionName ensures session name contains only characters valid for tmux.
// Tmux session names can only contain: alphanumeric, dash (-), underscore (_).
// Other characters are dropped, spaces become dashes.
//
// Examples:
//   - "my.session" → "mysession" (period dropped)
//   - "my session" → "my-session" (space becomes dash)
//   - "my@session!" → "mysession" (special chars dropped)
func SanitizeSessionName(name string) string {
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			result.WriteRune(r)
		} else if r == ' ' {
			result.WriteRune('-')
		}
		// All other characters (including '.') are dropped
	}
	sanitized := result.String()

	// Ensure we don't return empty string
	if sanitized == "" {
		return "session"
	}

	return sanitized
}

// supervisorRolePrefixes are session name prefixes that indicate supervisor roles.
// Supervisor sessions get auto-respawn so they recover from crashes without manual intervention.
var supervisorRolePrefixes = []string{
	"orchestrator",
	"meta-orchestrator",
	"overseer",
}

// IsSupervisorSession returns true if the session name indicates a supervisor role
// (orchestrator, meta-orchestrator, or overseer). Detection is based on the session
// name containing one of the supervisor role prefixes as a word boundary match.
func IsSupervisorSession(name string) bool {
	lower := strings.ToLower(name)
	for _, prefix := range supervisorRolePrefixes {
		if strings.Contains(lower, prefix) {
			return true
		}
	}
	return false
}

// EnableAutoRespawn configures a tmux session to automatically restart its pane
// process when it exits. This sets remain-on-exit (keeps dead panes visible) and
// a pane-died hook that calls respawn-pane to restart the process.
//
// Used for supervisor sessions (orchestrator, meta-orchestrator, overseer) to ensure
// crash recovery without manual intervention.
func EnableAutoRespawn(sessionName string) error {
	ctx := context.Background()
	socketPath := GetSocketPath()

	type respawnSetting struct {
		args        []string
		description string
	}

	settings := []respawnSetting{
		{
			args:        []string{"set-option", "-t", sessionName, "remain-on-exit", "on"},
			description: "Keep pane visible after process exits (auto-respawn prerequisite)",
		},
		{
			args:        []string{"set-hook", "-t", sessionName, "pane-died", "respawn-pane"},
			description: "Auto-respawn pane process on crash",
		},
	}

	for _, setting := range settings {
		cmdArgs := append([]string{"-S", socketPath}, setting.args...)
		cmd, cancel := CommandWithTimeout(ctx, globalTimeout, "tmux", cmdArgs...)
		if err := cmd.Run(); err != nil {
			cancel()
			return fmt.Errorf("failed to apply auto-respawn setting '%s': %w", setting.description, err)
		}
		cancel()
	}

	return nil
}

// NewSession creates a new tmux session with optimized settings
func NewSession(name string, workDir string) error {
	ctx := context.Background()
	socketPath := GetSocketPath()

	// Sanitize session name (tmux only allows alphanumeric, dash, underscore)
	sanitizedName := SanitizeSessionName(name)
	if sanitizedName != name {
		// Log warning but continue with sanitized name
		fmt.Fprintf(os.Stderr, "Warning: Sanitized session name from %q to %q (tmux requires alphanumeric, dash, underscore)\n", name, sanitizedName)
	}

	// Clean stale socket if exists
	if err := CleanStaleSocket(); err != nil {
		return fmt.Errorf("failed to clean stale socket: %w", err)
	}

	// Lock tmux server for session creation + settings (prevent parallel mutations)
	return withTmuxLock(func() error {
		// Create session with detached mode (use sanitized name)
		cmd, cancel := CommandWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "new-session", "-d", "-s", sanitizedName, "-c", workDir)
		defer cancel()
		if err := cmd.Run(); err != nil {
			// Check for timeout
			if ctx.Err() == context.DeadlineExceeded {
				return &TimeoutError{
					Problem:  fmt.Sprintf("tmux command timed out after %v (server may be hung)", globalTimeout),
					Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  agm session list         # Verify recovery",
					Duration: globalTimeout,
				}
			}
			return fmt.Errorf("failed to create tmux session: %w", err)
		}

		// Inject tmux settings for better UX
		// These settings improve multi-device usage, copy/paste, and mouse support
		// IMPORTANT: Run as actual tmux commands, NOT via send-keys (which sends to bash shell)
		type tmuxSetting struct {
			args        []string
			description string
		}
		settings := []tmuxSetting{
			// Server-global crash prevention: without these, tmux server exits when
			// all clients detach (e.g., SSH drops), killing every session.
			// Root cause of mass session crashes identified 2026-04-02.
			{[]string{"set", "-g", "exit-empty", "off"}, "Prevent server exit when all sessions detach (crash fix)"},
			{[]string{"set", "-g", "destroy-unattached", "off"}, "Keep sessions alive when clients disconnect (crash fix)"},
			// Per-session UX settings
			{[]string{"set-window-option", "-t", name, "aggressive-resize", "on"}, "Fix multi-device layout conflicts"},
			{[]string{"set-option", "-t", name, "window-size", "latest"}, "Force window to fit current screen"},
			{[]string{"set", "-t", name, "mouse", "on"}, "Enable mouse scrolling"},
			{[]string{"set", "-s", "set-clipboard", "on"}, "Enable OSC 52 for Cmd-C over SSH"},
			{[]string{"set", "-s", "escape-time", "10"}, "Reduce Escape key delay for copy-mode (default 500ms causes lag)"},
		}

		// Set build environment variables so worker sessions share Go caches
		// and limit parallelism to avoid CPU contention across concurrent workers.
		homeDir, _ := os.UserHomeDir()
		goCache := filepath.Join(homeDir, ".cache", "go-build")
		goModCache := filepath.Join(homeDir, "go", "pkg", "mod")
		// Limit worker parallelism: use half the available cores (min 1),
		// capped at 4 to avoid CPU contention across concurrent workers.
		maxProcs := strconv.Itoa(min(max(runtime.NumCPU()/2, 1), 4))
		buildEnv := map[string]string{
			"GOCACHE":    goCache,
			"GOMODCACHE": goModCache,
			"GOMAXPROCS": maxProcs,
			"GOWORK":     "off",
		}
		for k, v := range buildEnv {
			cmdArgs := []string{"-S", socketPath, "set-environment", "-t", sanitizedName, k, v}
			envCmd, envCancel := CommandWithTimeout(ctx, globalTimeout, "tmux", cmdArgs...)
			if err := envCmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to set env %s=%s: %v\n", k, v, err)
			}
			envCancel()
		}

		for _, setting := range settings {
			// Build full command args: ["tmux", "-S", socketPath, ...setting.args]
			cmdArgs := append([]string{"-S", socketPath}, setting.args...)
			cmd, cancel := CommandWithTimeout(ctx, globalTimeout, "tmux", cmdArgs...)
			if err := cmd.Run(); err != nil {
				// Log warning but don't fail - these are UX improvements, not critical
				fmt.Fprintf(os.Stderr, "Warning: Failed to apply tmux setting '%s': %v\n", setting.description, err)
			}
			cancel()
		}

		// Enable auto-respawn for supervisor sessions so they recover from crashes
		if IsSupervisorSession(name) {
			if err := EnableAutoRespawn(sanitizedName); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to enable auto-respawn for supervisor session %q: %v\n", sanitizedName, err)
			}
		}

		return nil
	})
}

// AttachSession attaches to tmux session or switches if already inside tmux
// Returns nil if session exists and was successfully switched/attached
// In non-interactive environments (no TTY), it skips attach and returns nil
// IMPORTANT: This function uses syscall.Exec to replace the process, so it does NOT return
func AttachSession(name string) error {
	ctx := context.Background()
	socketPath := GetSocketPath()

	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedName := NormalizeTmuxSessionName(name)

	// Check if we're already inside a tmux session
	if os.Getenv("TMUX") != "" {
		// Already in tmux - DO NOT switch unless user is interactive
		// This prevents unexpected window switching when running from within tmux
		// (e.g., running tests, background commands, etc.)

		// Check if stdin is a TTY (interactive terminal)
		isTTY := term.IsTerminal(int(os.Stdin.Fd()))
		if !isTTY {
			// Not interactive (tests, scripts, etc.) - skip switch to avoid disruption
			return nil
		}

		// Interactive session - use switch-client to switch to target session
		cmd, cancel := CommandWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "switch-client", "-t", normalizedName)
		defer cancel()
		if err := cmd.Run(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return &TimeoutError{
					Problem:  fmt.Sprintf("tmux command timed out after %v (server may be hung)", globalTimeout),
					Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  agm session list         # Verify recovery",
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
		"tmux",           // argv[0] - program name
		"-S", socketPath, // Use isolated socket
		"attach-session",     // Command
		"-t", normalizedName, // Target session (normalized)
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

// deleteBuffer attempts to delete the named buffer "agm-cmd" from the tmux server.
// This is a best-effort cleanup — errors are logged but not returned, since the
// buffer may not exist (already deleted by -d flag on successful paste-buffer).
//
// Bug fix (2026-04-02): Previously, if paste-buffer failed or timed out, the agm-cmd
// buffer was never cleaned up. Under high load with many concurrent sessions, orphaned
// buffers accumulated and contributed to tmux server memory pressure and eventual crash.
func deleteBuffer() {
	ctx := context.Background()
	socketPath := GetSocketPath()
	cmd, cancel := CommandWithTimeout(ctx, 2*time.Second, "tmux", "-S", socketPath, "delete-buffer", "-b", "agm-cmd")
	defer cancel()
	_ = cmd.Run() // Best-effort: buffer may already be deleted by -d flag
}

// SendCommand sends a command to tmux pane
func SendCommand(sessionName string, command string) error {
	ctx := context.Background()
	socketPath := GetSocketPath()

	// Acquire concurrency semaphore to prevent resource exhaustion
	if err := acquireTmuxSemaphore(ctx); err != nil {
		return fmt.Errorf("tmux concurrency limit reached: %w", err)
	}
	defer releaseTmuxSemaphore()

	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedName := NormalizeTmuxSessionName(sessionName)

	// Lock tmux server for buffer operations (prevent interleaved pastes)
	return withTmuxLock(func() error {
		// Ensure buffer is cleaned up on any error path.
		// The -d flag on paste-buffer only deletes on success; if paste fails
		// or times out, the buffer persists indefinitely. This defer guarantees cleanup.
		bufferLoaded := false
		defer func() {
			if bufferLoaded {
				deleteBuffer()
			}
		}()

		// Step 1: Load command text into tmux paste buffer via stdin
		// This avoids command-line length limits and special character escaping issues
		timeout := getAdaptiveTimeout()
		cmdLoad, cancel1 := CommandWithTimeout(ctx, timeout, "tmux", "-S", socketPath, "load-buffer", "-b", "agm-cmd", "-")
		defer cancel1()

		stdin, err := cmdLoad.StdinPipe()
		if err != nil {
			return fmt.Errorf("failed to create stdin pipe for load-buffer: %w", err)
		}

		if err := cmdLoad.Start(); err != nil {
			return fmt.Errorf("failed to start load-buffer: %w", err)
		}

		// Write command to buffer via stdin
		if _, err := stdin.Write([]byte(command)); err != nil {
			stdin.Close()
			cmdLoad.Wait()
			return fmt.Errorf("failed to write to load-buffer stdin: %w", err)
		}
		stdin.Close()

		if err := cmdLoad.Wait(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return &TimeoutError{
					Problem:  fmt.Sprintf("tmux load-buffer timed out after %v (server may be hung)", timeout),
					Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  agm session list         # Verify recovery",
					Duration: timeout,
				}
			}
			return fmt.Errorf("failed to load command into tmux buffer: %w", err)
		}
		bufferLoaded = true

		// Step 2: Paste buffer to session (atomic operation, -d deletes buffer after paste)
		// Note: paste-buffer targets panes, not sessions, so we don't use FormatSessionTarget (=prefix)
		cmdPaste, cancel2 := CommandWithTimeout(ctx, timeout, "tmux", "-S", socketPath, "paste-buffer", "-b", "agm-cmd", "-t", normalizedName, "-d")
		defer cancel2()
		if err := cmdPaste.Run(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return &TimeoutError{
					Problem:  fmt.Sprintf("tmux paste-buffer timed out after %v (server may be hung)", timeout),
					Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  agm session list         # Verify recovery",
					Duration: timeout,
				}
			}
			return fmt.Errorf("failed to paste buffer to tmux session: %w", err)
		}
		bufferLoaded = false // paste-buffer -d already deleted it

		// Step 3: Send Enter key to submit the command
		// Delay prevents tmux from coalescing pasted text with ENTER keystroke.
		// Do not remove.
		time.Sleep(50 * time.Millisecond)

		// Note: send-keys targets panes, not sessions, so we don't use FormatSessionTarget (=prefix)
		cmdEnter, cancel3 := CommandWithTimeout(ctx, timeout, "tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "C-m")
		defer cancel3()
		if err := cmdEnter.Run(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return &TimeoutError{
					Problem:  fmt.Sprintf("tmux send-keys timed out after %v (server may be hung)", timeout),
					Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  agm session list         # Verify recovery",
					Duration: timeout,
				}
			}
			return fmt.Errorf("failed to send Enter key to tmux: %w", err)
		}

		// Step 4: Auto-detect and retry Enter if paste left text unsubmitted.
		// Bug fix (2026-04-10): After paste-buffer, Enter (C-m) sometimes doesn't
		// register. Detect via capture-pane and re-send Enter up to 2 times.
		if err := retryEnterAfterPaste(socketPath, normalizedName, 2); err != nil {
			return err
		}

		return nil
	})
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

// ClientInfo holds information about a tmux client
type ClientInfo struct {
	SessionName string
	TTY         string
	PID         int
}

// ListClients returns all clients attached to a specific session
func ListClients(sessionName string) ([]ClientInfo, error) {
	ctx := context.Background()
	socketPath := GetSocketPath()
	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedName := NormalizeTmuxSessionName(sessionName)
	// Format: session_name:client_tty:client_pid
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "list-clients", "-t", FormatSessionTarget(normalizedName), "-F", "#{session_name}:#{client_tty}:#{client_pid}")
	if err != nil {
		// Check for timeout error
		if _, ok := err.(*TimeoutError); ok {
			return nil, err
		}
		// If session not found or no clients, return empty list
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []ClientInfo{}, nil
		}
		return nil, fmt.Errorf("failed to list tmux clients: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	clients := make([]ClientInfo, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Parse "session_name:tty:pid" format
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		var pid int
		fmt.Sscanf(parts[2], "%d", &pid)

		clients = append(clients, ClientInfo{
			SessionName: parts[0],
			TTY:         parts[1],
			PID:         pid,
		})
	}
	return clients, nil
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

// GetPaneCommands returns the foreground command for each pane in the tmux session.
func GetPaneCommands(sessionName string) ([]string, error) {
	ctx := context.Background()
	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "list-panes", "-t", FormatSessionTarget(normalizedName),
		"-F", "#{pane_current_command}")
	if err != nil {
		if _, ok := err.(*TimeoutError); ok {
			return nil, err
		}
		return nil, fmt.Errorf("failed to list tmux pane commands: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var commands []string
	for _, line := range lines {
		if cmd := strings.TrimSpace(line); cmd != "" {
			commands = append(commands, cmd)
		}
	}
	return commands, nil
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
	commands, err := GetPaneCommands(sessionName)
	if err != nil {
		return false, err
	}

	for _, cmd := range commands {
		if cmd == processName {
			return true, nil
		}
	}

	return false, nil
}

// IsClaudeRunning checks if Claude Code is running in any pane of the tmux session.
// Claude Code reports as its version string (e.g., "2.1.50") rather than "claude"
// in tmux's pane_current_command, so this function checks for both patterns.
func IsClaudeRunning(sessionName string) (bool, error) {
	commands, err := GetPaneCommands(sessionName)
	if err != nil {
		return false, err
	}

	for _, cmd := range commands {
		if isClaudeProcess(cmd) {
			return true, nil
		}
	}

	// Fallback: Claude runs as child of bash after crash/resume.
	// In this state, pane_current_command shows "bash" (or zsh/sh) instead of
	// the Claude process. Detect by capturing recent pane output and looking
	// for the Claude prompt character.
	for _, cmd := range commands {
		if cmd == "bash" || cmd == "zsh" || cmd == "sh" {
			ctx := context.Background()
			socketPath := GetSocketPath()
			normalizedName := NormalizeTmuxSessionName(sessionName)
			// Note: capture-pane targets panes, not sessions, so we don't use FormatSessionTarget (=prefix)
			output, captureErr := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath,
				"capture-pane", "-t", normalizedName, "-p", "-S", "-5")
			if captureErr == nil && strings.Contains(string(output), "❯") {
				return true, nil
			}
		}
	}

	return false, nil
}

// isClaudeProcess checks if a pane command name indicates Claude Code.
// Claude Code shows up as its semver version (e.g., "2.1.50") in tmux,
// or as "claude" if invoked directly.
func isClaudeProcess(command string) bool {
	if command == "claude" {
		return true
	}
	// Claude Code reports as semver (e.g., "2.1.50", "3.0.0")
	// Check for N.N.N pattern where N is digits
	parts := strings.Split(command, ".")
	if len(parts) == 3 {
		allDigits := true
		for _, p := range parts {
			if p == "" {
				allDigits = false
				break
			}
			for _, c := range p {
				if c < '0' || c > '9' {
					allDigits = false
					break
				}
			}
			if !allDigits {
				break
			}
		}
		if allDigits {
			return true
		}
	}
	return false
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
	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedName := NormalizeTmuxSessionName(sessionName)
	// Note: display-message targets panes, not sessions, so we don't use FormatSessionTarget (=prefix)
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "display-message", "-t", normalizedName, "-p", "#{pane_current_path}")
	if err != nil {
		// Check for timeout error
		if _, ok := err.(*TimeoutError); ok {
			return "", err
		}
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}
