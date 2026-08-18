package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/logging"
)

var logger = logging.DefaultLogger()

// WaitForPaneClose waits for a tmux pane to close
// Polls list-panes until it fails (indicating pane closed)
func WaitForPaneClose(sessionName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	socketPath := GetSocketPath()
	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedName := NormalizeTmuxSessionName(sessionName)
	checkCount := 0

	logger.Info("Monitoring pane closure", "session", sessionName)

	for time.Now().Before(deadline) {
		checkCount++

		// Check if pane still exists
		// Note: list-panes can target sessions, but doesn't support = prefix in tmux 3.4
		cmd := exec.Command("tmux", "-S", socketPath, "list-panes", "-t", normalizedName, "-F", "#{pane_id}")
		output, err := cmd.CombinedOutput()

		if err != nil {
			// Exit code != 0 means pane doesn't exist anymore
			logger.Info("Pane closed", "checks", checkCount, "duration_seconds", time.Since(deadline.Add(-timeout)).Seconds())
			return nil //nolint:nilerr // intentional: caller signals via separate bool/optional
		}

		// Log first few checks and periodically thereafter for debugging
		if checkCount <= 3 || checkCount%10 == 0 {
			logger.Info("Pane still active", "check", checkCount, "panes", strings.TrimSpace(string(output)))
		}

		// Pane still exists, wait a bit
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for pane to close after %d checks (waited %v)", checkCount, timeout)
}

// SendKeysToPane sends keys to a specific pane using atomic paste-buffer.
// Loads text into a tmux buffer and pastes atomically, then sends Enter.
//
// Bug fix (2026-04-07): Switched from send-keys -l to load-buffer + paste-buffer.
// send-keys -l delivers text character-by-character, creating a race condition
// where Enter (C-m) can arrive before the terminal finishes processing the text.
// paste-buffer is atomic — the entire text appears in the input at once.
func SendKeysToPane(sessionName string, keys string) error {
	ctx := context.Background()
	socketPath := GetSocketPath()
	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedName := NormalizeTmuxSessionName(sessionName)

	logger.Info("Sending keys to session", "session", sessionName, "keys", keys)

	// Lock tmux server for buffer operations (prevent interleaved pastes).
	return withTmuxLock(func() error {
		// Ensure buffer is cleaned up on any error path.
		bufferLoaded := false
		defer func() {
			if bufferLoaded {
				deleteBuffer()
			}
		}()

		// Step 1: Load text into tmux paste buffer via stdin.
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

		if _, err := stdin.Write([]byte(keys)); err != nil {
			stdin.Close()
			cmdLoad.Wait()
			return fmt.Errorf("failed to write to load-buffer stdin: %w", err)
		}
		stdin.Close()

		if err := cmdLoad.Wait(); err != nil {
			return fmt.Errorf("failed to load keys into tmux buffer: %w", err)
		}
		bufferLoaded = true
		logger.Info("Keys loaded into buffer", "keys", keys)

		// Step 2: Paste buffer to session (atomic, -d deletes buffer after paste).
		cmdPaste, cancel2 := CommandWithTimeout(ctx, timeout, "tmux", "-S", socketPath, "paste-buffer", "-b", "agm-cmd", "-t", normalizedName, "-d")
		defer cancel2()
		if err := cmdPaste.Run(); err != nil {
			return fmt.Errorf("failed to paste buffer to tmux session: %w", err)
		}
		bufferLoaded = false

		// Step 3: Send Enter reliably using hex 0x0d instead of C-m.
		if err := sendEnterReliable(socketPath, normalizedName); err != nil {
			return err
		}

		logger.Info("Enter key sent")
		return nil
	})
}

// GetPanePID returns the PID of the process running in the first pane of the session.
// Returns 0 if the pane doesn't exist or the PID cannot be determined.
func GetPanePID(sessionName string) (int, error) {
	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)

	cmd := exec.Command("tmux", "-S", socketPath, "list-panes", "-t", normalizedName, "-F", "#{pane_pid}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("pane not found: %w", err)
	}

	pidStr := strings.TrimSpace(string(output))
	// Take the first line (first pane)
	if idx := strings.Index(pidStr, "\n"); idx != -1 {
		pidStr = pidStr[:idx]
	}

	var pid int
	if _, err := fmt.Sscanf(pidStr, "%d", &pid); err != nil {
		return 0, fmt.Errorf("failed to parse PID %q: %w", pidStr, err)
	}
	return pid, nil
}

// KillSession kills a tmux session by name. It preserves the historical
// best-effort behavior for cleanup callers that do not own a transaction.
func KillSession(sessionName string) {
	_ = KillSessionChecked(sessionName) //nolint:errcheck // compatibility wrapper is intentionally best-effort
}

// KillSessionWithError kills a tmux session and reports failure. Lifecycle
// rollback paths use this variant so cleanup failures cannot be discarded.
func KillSessionWithError(sessionName string) error {
	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)
	cmd := exec.Command("tmux", "-S", socketPath, "kill-session", "-t", FormatSessionTarget(normalizedName))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kill tmux session %q: %w", sessionName, err)
	}
	return nil
}

// KillSessionChecked kills a tmux session and reports backend failures. A
// confirmed missing target is treated as idempotent success; socket, timeout,
// permission, and other command failures remain observable.
func KillSessionChecked(sessionName string) error {
	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)
	output, err := RunWithTimeout(context.Background(), globalTimeout, "tmux", "-S", socketPath, "kill-session", "-t", FormatSessionTarget(normalizedName))
	if err == nil || isMissingSessionOutput(output) {
		return nil
	}
	return tmuxCommandError("kill session", normalizedName, output, err)
}

// KillSessionIdentityChecked kills only the tmux session whose server-local ID
// and stored token match, or whose name is the token-derived provisional name
// retained before the token write. If the client loses ID output, the exact
// random provisional name remains sufficient ownership proof. The full-ID
// comparison and conditional kill execute in one tmux command queue so a
// replacement cannot be selected by a stale ID. Missing and mismatched targets
// mean the owned creation is already gone and are treated as idempotent success.
func KillSessionIdentityChecked(identity SessionIdentity) error {
	if !identity.Cleanable() {
		return fmt.Errorf("invalid tmux session identity %q", identity.ID)
	}
	if !identity.Valid() {
		return KillSessionChecked(identity.CreationName)
	}
	condition := fmt.Sprintf("#{==:#{@agm_session_identity},%s}", identity.Token)
	if identity.CreationName != "" {
		condition = fmt.Sprintf("#{||:%s,#{==:#{session_name},%s}}", condition, identity.CreationName)
	}
	killCommand := fmt.Sprintf("kill-session -t %s", identity.ID)
	output, err := RunWithTimeout(context.Background(), globalTimeout, "tmux", "-S", GetSocketPath(),
		"if-shell", "-F", "-t", identity.ID, condition, killCommand, "")
	if err == nil || isMissingSessionOutput(output) {
		return nil
	}
	return tmuxCommandError("kill session identity", identity.ID, output, err)
}

// IsPaneActive checks if a pane is currently active
func IsPaneActive(sessionName string) (bool, error) {
	socketPath := GetSocketPath()
	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedName := NormalizeTmuxSessionName(sessionName)

	cmd := exec.Command("tmux", "-S", socketPath, "list-panes", "-t", normalizedName)
	err := cmd.Run()

	if err != nil {
		// Non-zero exit = pane doesn't exist
		return false, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	return true, nil
}
