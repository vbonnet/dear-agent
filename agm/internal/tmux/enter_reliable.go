package tmux

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

// sendEnterReliable sends Enter to a tmux pane using the raw hex byte 0x0d
// (carriage return) instead of "C-m", which tmux can confuse with pasted
// content after a paste-buffer operation.
//
// Protocol:
//  1. Wait 100ms for the paste-buffer to settle in the terminal.
//  2. Send Enter via "send-keys -H 0d" (raw byte, immune to paste coalescing).
//  3. Wait 200ms and capture the pane to check whether Enter registered.
//  4. If input is still stuck (isPasteStuck), re-send Enter once.
//
// This function does NOT acquire the tmux lock — callers inside a
// withTmuxLock block (SendCommand, SendPromptLiteral, etc.) call this
// directly. External callers should use the exported SendEnterReliable
// which handles socket/normalization but does acquire the lock.
func sendEnterReliable(socketPath, normalizedName string) error {
	time.Sleep(100 * time.Millisecond)

	cmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "-H", "0d")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send Enter (-H 0d): %w", err)
	}

	time.Sleep(200 * time.Millisecond)

	checkCmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedName, "-p", "-S", "-3")
	output, err := checkCmd.Output()
	if err != nil {
		return nil //nolint:nilerr // best-effort: capture failure is non-fatal
	}

	if !isPasteStuck(string(output)) {
		return nil
	}

	if os.Getenv("AGM_DEBUG") == "1" {
		slog.Debug("sendEnterReliable: Enter did not register, re-sending once")
	}

	retryCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "-H", "0d")
	if err := retryCmd.Run(); err != nil {
		return fmt.Errorf("failed to re-send Enter (-H 0d): %w", err)
	}

	return nil
}

// SendEnterReliable is the exported entry point for code outside the tmux
// package (CLI commands, sentinel recovery, etc.). It resolves the socket
// path, normalizes the session name, acquires the tmux lock, and delegates
// to sendEnterReliable.
func SendEnterReliable(sessionName string) error {
	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)

	return withTmuxLock(func() error {
		return sendEnterReliable(socketPath, normalizedName)
	})
}
