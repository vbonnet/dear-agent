package tmux

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

// ErrPasteNotSubmitted is returned when the submit-Enter could not be confirmed
// to have taken effect — the pasted prompt is still sitting in the composer
// after every retry. Callers (SendCommand → SendMessage) MUST surface this as a
// delivery FAILURE rather than reporting success, so a silently-parked task
// (ce-mjk9) fails loud instead of hanging forever.
var ErrPasteNotSubmitted = errors.New("prompt pasted but submission not confirmed after retries")

// enterVerifyConfig controls the submit-verify backoff loop.
type enterVerifyConfig struct {
	initialSettle time.Duration   // wait before the first Enter (let the paste land)
	backoffs      []time.Duration // wait after each Enter before re-checking
}

// defaultEnterVerifyConfig: ~4.25s of cumulative post-Enter waiting across 5
// attempts. Sized to outlast codex's large-paste "[Pasted Content]" chip
// conversion under a LOADED host (load 20-28), which is exactly when the old
// fixed 100ms+one-retry lost the race and parked the task (ce-mjk9).
func defaultEnterVerifyConfig() enterVerifyConfig {
	return enterVerifyConfig{
		initialSettle: 100 * time.Millisecond,
		backoffs: []time.Duration{
			150 * time.Millisecond,
			300 * time.Millisecond,
			600 * time.Millisecond,
			1200 * time.Millisecond,
			2000 * time.Millisecond,
		},
	}
}

// verifyingEnter delivers Enter and polls the pane until the paste is confirmed
// SUBMITTED (composer no longer stuck), retrying the raw Enter with backoff.
//
// It is deliberately decoupled from tmux: `sendEnter` delivers one raw Enter and
// `capture` returns recent pane content — so the loop logic is unit-testable by
// simulating a slow-paste (capture stays stuck for the first N polls, then
// clears).
//
// Contract:
//   - returns nil the moment `capture` shows the pane is no longer stuck;
//   - returns ErrPasteNotSubmitted if the pane is still POSITIVELY stuck after
//     every backoff (fail loud);
//   - a capture that itself errors is treated as "cannot verify" — best-effort,
//     never a hard failure on its own (infra flakiness must not fail a send that
//     may well have succeeded); only a confirmed-stuck final state fails.
func verifyingEnter(sendEnter func() error, capture func() (string, error), cfg enterVerifyConfig) error {
	time.Sleep(cfg.initialSettle)

	lastStuck := false
	for i := 0; i < len(cfg.backoffs); i++ {
		if err := sendEnter(); err != nil {
			return fmt.Errorf("failed to send Enter (-H 0d): %w", err)
		}
		time.Sleep(cfg.backoffs[i])

		content, err := capture()
		if err != nil {
			// Cannot verify this round — keep retrying, but remember we couldn't
			// confirm a stuck state so we don't fail loud purely on capture infra.
			lastStuck = false
			continue
		}
		if !isPasteStuck(content) {
			return nil // submission confirmed
		}
		lastStuck = true
		if os.Getenv("AGM_DEBUG") == "1" {
			slog.Debug("verifyingEnter: prompt still parked, re-sending Enter",
				"attempt", i+1, "of", len(cfg.backoffs))
		}
	}

	if lastStuck {
		return ErrPasteNotSubmitted
	}
	// Never positively confirmed stuck (all captures failed) — best-effort success.
	return nil
}

// sendEnterReliable sends Enter to a tmux pane using the raw hex byte 0x0d
// (carriage return) instead of "C-m", then VERIFIES the paste was actually
// submitted, retrying with backoff and failing loud if it never is (ce-mjk9).
//
// Wraps the real tmux send-keys / capture-pane commands around verifyingEnter.
// Does NOT acquire the tmux lock — callers inside a withTmuxLock block
// (SendCommand, etc.) call this directly; SendEnterReliable is the exported,
// lock-acquiring entry point.
func sendEnterReliable(socketPath, normalizedName string) error {
	sendEnter := func() error {
		return exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "-H", "0d").Run()
	}
	capture := func() (string, error) {
		out, err := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedName, "-p", "-S", "-5").Output()
		return string(out), err
	}
	return verifyingEnter(sendEnter, capture, defaultEnterVerifyConfig())
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
