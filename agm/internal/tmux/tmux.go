package tmux

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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
		timeoutError := &TimeoutError{}
		if errors.As(err, &timeoutError) {
			return false, err
		}
		// Exit code 1 means session doesn't exist
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check tmux session: %w", err)
	}
	return true, nil
}

// HasSessionStrict checks whether a session exists without collapsing every
// tmux exit failure into a missing-session verdict. It is intended for
// destructive-operation postconditions, where an inaccessible socket must not
// be reported as proof that the target is gone.
func HasSessionStrict(name string) (bool, error) {
	ctx := context.Background()
	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(name)
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "has-session", "-t", FormatSessionTarget(normalizedName))
	if err == nil {
		return true, nil
	}
	if isMissingSessionOutput(output) {
		return false, nil
	}
	return false, tmuxCommandError("check session", normalizedName, output, err)
}

// SessionIdentity identifies one specific tmux session creation. ID is local
// to a tmux server generation and may be reused after a server restart; Token
// is stored on the session at creation so the pair remains creation-specific.
type SessionIdentity struct {
	ID    string
	Token string
}

// Valid reports whether both parts of a creation-specific identity are valid.
func (identity SessionIdentity) Valid() bool {
	if !IsSessionID(identity.ID) || len(identity.Token) != 32 {
		return false
	}
	decoded, err := hex.DecodeString(identity.Token)
	return err == nil && len(decoded) == 16
}

func newSessionIdentity() (SessionIdentity, error) {
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return SessionIdentity{}, fmt.Errorf("generate tmux session identity: %w", err)
	}
	return SessionIdentity{Token: hex.EncodeToString(tokenBytes)}, nil
}

func sanitizeNewSessionName(name string) string {
	sanitizedName := SanitizeSessionName(name)
	if sanitizedName != name {
		fmt.Fprintf(os.Stderr, "Warning: Sanitized session name from %q to %q (tmux requires alphanumeric, dash, underscore)\n", name, sanitizedName)
	}
	return sanitizedName
}

// HasSessionIdentityStrict checks a creation-specific tmux identity without
// allowing a same-named replacement or a new server's reused ID to satisfy it.
func HasSessionIdentityStrict(identity SessionIdentity) (bool, error) {
	if !identity.Valid() {
		return false, fmt.Errorf("invalid tmux session identity %q", identity.ID)
	}
	output, err := RunWithTimeout(context.Background(), globalTimeout, "tmux", "-S", GetSocketPath(), "display-message", "-p", "-t", identity.ID, "#{@agm_session_identity}")
	if err == nil {
		return strings.TrimSpace(string(output)) == identity.Token, nil
	}
	if isMissingSessionOutput(output) {
		return false, nil
	}
	return false, tmuxCommandError("check session identity", identity.ID, output, err)
}

func isMissingSessionOutput(output []byte) bool {
	return strings.Contains(strings.ToLower(string(output)), "can't find session")
}

func tmuxCommandError(operation, sessionName string, output []byte, err error) error {
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return fmt.Errorf("tmux %s %q: %w", operation, sessionName, err)
	}
	return fmt.Errorf("tmux %s %q: %w: %s", operation, sessionName, err, detail)
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

// NewSession creates a new tmux session with optimized settings.
func NewSession(name string, workDir string) error {
	_, err := NewSessionWithIdentity(name, workDir)
	return err
}

// NewSessionWithIdentity creates a new tmux session and returns a server-local
// ID paired with a random option stored on that specific session creation.
// Transactional callers must retain both so later compensation cannot kill a
// replacement that reused either the name or ID after a server restart.
//
// If initialization fails after tmux created the session, the returned
// identity remains non-empty so the caller can compensate the exact resource.
func NewSessionWithIdentity(name string, workDir string) (SessionIdentity, error) {
	ctx := context.Background()
	socketPath := GetSocketPath()
	identity, err := newSessionIdentity()
	if err != nil {
		return SessionIdentity{}, err
	}

	// Sanitize session name (tmux only allows alphanumeric, dash, underscore).
	sanitizedName := sanitizeNewSessionName(name)

	// Clean stale socket if exists
	if err := CleanStaleSocket(); err != nil {
		return SessionIdentity{}, fmt.Errorf("failed to clean stale socket: %w", err)
	}

	// Lock tmux server for session creation + settings (prevent parallel mutations)
	err = withTmuxLock(func() error {
		// Create session with detached mode (use sanitized name)
		cmd, cancel := CommandWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath,
			"new-session", "-d", "-P", "-F", "#{session_id}", "-s", sanitizedName, "-c", workDir,
			";", "set-option", "-t", sanitizedName, "@agm_session_identity", identity.Token)
		defer cancel()
		output, err := cmd.Output()
		if err != nil {
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
		identity.ID = strings.TrimSpace(string(output))
		if !identity.Valid() {
			return fmt.Errorf("tmux created session %q but returned invalid session identity %q", sanitizedName, identity.ID)
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
		// Propagate autonomous mode so sessions spawned by an autonomous
		// supervisor inherit the flag durably (pane-level os.Getenv check).
		if v := os.Getenv("AGM_AUTONOMOUS"); v != "" {
			buildEnv["AGM_AUTONOMOUS"] = v
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
	if err != nil {
		return identity, err
	}

	// Verify the pane actually landed in workDir and repair it if not.
	// tmux silently ignores `new-session -c` when the server's own cwd has
	// been deleted, leaving the pane in a dead directory where harness CLIs
	// die instantly (ce-5zbg). Must run outside withTmuxLock: the repair path
	// sends a `cd` via SendCommand, which takes the lock itself.
	if err := EnsureSessionWorkDir(sanitizedName, workDir); err != nil {
		return identity, err
	}
	return identity, nil
}

// IsSessionID reports whether target is a tmux server-local session identity.
func IsSessionID(target string) bool {
	if len(target) < 2 || target[0] != '$' {
		return false
	}
	_, err := strconv.ParseUint(target[1:], 10, 64)
	return err == nil
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

		// Step 3: Send Enter reliably using hex 0x0d instead of C-m.
		// sendEnterReliable waits 100ms, sends -H 0d, then auto-retries
		// once if the Enter didn't register (replaces the old 50ms + C-m +
		// retryEnterAfterPaste sequence).
		if err := sendEnterReliable(socketPath, normalizedName); err != nil {
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
		timeoutError := &TimeoutError{}
		if errors.As(err, &timeoutError) {
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
		timeoutError := &TimeoutError{}
		if errors.As(err, &timeoutError) {
			return nil, err
		}
		// If no tmux server running, return empty list
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
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
		timeoutError := &TimeoutError{}
		if errors.As(err, &timeoutError) {
			return nil, err
		}
		// If no tmux server running, return empty list
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
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
		timeoutError := &TimeoutError{}
		if errors.As(err, &timeoutError) {
			return nil, err
		}
		// If session not found or no clients, return empty list
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
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
		timeoutError := &TimeoutError{}
		if errors.As(err, &timeoutError) {
			return "", err
		}
		return "", fmt.Errorf("failed to get current tmux session name: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// GetPaneCommands returns command hints for each pane in the tmux session.
// It includes both the current foreground command and the pane's start command:
// Node-based CLIs such as Codex can report "node" as the current command while
// retaining "codex ..." as the start command.
func GetPaneCommands(sessionName string) ([]string, error) {
	return GetPaneCommandsContext(context.Background(), sessionName)
}

// GetPaneCommandsContext is the command-scoped variant of GetPaneCommands.
func GetPaneCommandsContext(ctx context.Context, sessionName string) ([]string, error) {
	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "list-panes", "-t", normalizedName,
		"-F", "#{pane_current_command}\t#{pane_start_command}")
	if err != nil {
		timeoutError := &TimeoutError{}
		if errors.As(err, &timeoutError) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to list tmux pane commands: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var commands []string
	for _, line := range lines {
		for field := range strings.SplitSeq(line, "\t") {
			if cmd := strings.TrimSpace(field); cmd != "" {
				commands = append(commands, cmd)
			}
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
	return IsProcessRunningContext(context.Background(), sessionName, processName)
}

// IsProcessRunningContext is the command-scoped variant of IsProcessRunning.
func IsProcessRunningContext(ctx context.Context, sessionName, processName string) (bool, error) {
	commands, err := GetPaneCommandsContext(ctx, sessionName)
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

	// Fallback: Claude can run behind a shell after crash/resume, or behind
	// the AGM wrapper. In this state, pane_current_command shows the wrapper
	// instead of the Claude process. Detect by capturing recent pane output
	// and looking for the Claude prompt character.
	for _, cmd := range commands {
		if IsShellCommand(cmd) || cmd == "agm" {
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
// or as "claude" if invoked directly. macOS also reports the process with
// underscores instead of dots (e.g., "2_1_195") due to how the OS names it.
// When tmux formats a tab-separated line and pane_start_command is empty, it
// appends a trailing "_" as a null placeholder (e.g., "2_1_195_"), so we also
// strip a single trailing underscore before checking.
func isClaudeProcess(command string) bool {
	if command == "claude" {
		return true
	}
	// Strip a single trailing underscore that tmux appends as a null placeholder
	// for an empty pane_start_command field in tab-separated format output.
	command = strings.TrimSuffix(command, "_")
	// Normalise: macOS reports "2_1_195" but the canonical form is "2.1.195"
	normalised := strings.ReplaceAll(command, "_", ".")
	// Claude Code reports as semver (e.g., "2.1.50", "3.0.0") — N.N.N all digits
	parts := strings.Split(normalised, ".")
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
	return WaitForProcessReadyContext(context.Background(), sessionName, processName, timeout)
}

// WaitForProcessReadyContext is the command-scoped variant of
// WaitForProcessReady. Caller cancellation takes precedence over the timeout.
func WaitForProcessReadyContext(parent context.Context, sessionName, processName string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	pollInterval := 100 * time.Millisecond

	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("timeout waiting for %s to start (waited %v)", processName, timeout)
			}
			return err
		}
		running, err := isProcessReadyContext(ctx, sessionName, processName)
		if err != nil {
			// Ignore transient errors (e.g., brief tmux unavailability)
			if err := sleepWithContext(ctx, pollInterval); err != nil {
				continue
			}
			continue
		}
		if running {
			return nil // Process is ready!
		}
		if err := sleepWithContext(ctx, pollInterval); err != nil {
			continue
		}
	}
}

func isProcessReadyContext(ctx context.Context, sessionName, processName string) (bool, error) {
	isForeground := func(sessionName, processName string) (bool, error) {
		return IsProcessRunningContext(ctx, sessionName, processName)
	}
	return isProcessReadyWithRuntime(ctx, sessionName, processName, GetSocketPath(), isForeground, IsProcessInPaneTreeContext)
}

func isProcessReadyWithRuntime(
	ctx context.Context,
	sessionName, processName, socketPath string,
	isForeground func(string, string) (bool, error),
	isInPaneTree func(context.Context, string, string, string) (bool, error),
) (bool, error) {
	running, err := isForeground(sessionName, processName)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, ctxErr
	}
	if running {
		return true, nil
	}
	if processName != "codex" {
		return false, err
	}

	// Codex is commonly launched through a Node wrapper. tmux then reports
	// "node" as the foreground command even though a healthy Codex harness is
	// running. Use the shared full-descendant scan so both the native binary
	// and its Node wrapper satisfy Codex readiness.
	for _, candidate := range []string{"codex", "node"} {
		running, treeErr := isInPaneTree(ctx, sessionName, socketPath, candidate)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		if treeErr != nil {
			return false, treeErr
		}
		if running {
			return true, nil
		}
	}
	return false, err
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
		timeoutError := &TimeoutError{}
		if errors.As(err, &timeoutError) {
			return "", err
		}
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}
