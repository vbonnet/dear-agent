package tmux

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/procguard"
)

// ErrPasteNotSubmitted is returned when the submit-Enter could not be confirmed
// to have taken effect — the pasted prompt is still sitting in the composer
// after every retry. Callers (SendCommand → SendMessage) MUST surface this as a
// delivery FAILURE rather than reporting success, so a silently-parked task
// (ce-mjk9) fails loud instead of hanging forever.
var ErrPasteNotSubmitted = errors.New("prompt pasted but submission not confirmed after retries")

// PromptSubmissionUncertainError means tmux was asked to submit the composer
// but its command acknowledgement was lost. The harness may already be
// processing the prompt, so transactional callers must not compensate or
// report a retryable failure unless they can positively prove it stayed
// unsubmitted.
type PromptSubmissionUncertainError struct {
	err error
}

func (e *PromptSubmissionUncertainError) Error() string {
	return fmt.Sprintf("prompt submission acknowledgement is uncertain: %v", e.err)
}

func (e *PromptSubmissionUncertainError) Unwrap() error {
	return e.err
}

// MarkPromptSubmissionUncertain preserves the original error while exposing
// the irreversible submission boundary to callers.
func MarkPromptSubmissionUncertain(err error) error {
	if err == nil {
		return nil
	}
	return &PromptSubmissionUncertainError{err: err}
}

// PromptSubmissionMayHaveOccurred reports whether a delivery failure happened
// after tmux was asked to submit the composer.
func PromptSubmissionMayHaveOccurred(err error) bool {
	var uncertain *PromptSubmissionUncertainError
	return errors.As(err, &uncertain)
}

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
//     may well have succeeded); once this happens after an accepted Enter, later
//     retry errors retain that submission uncertainty across the entire loop;
//   - only a confirmed-stuck final state fails definitively, unless an earlier
//     accepted Enter could not be observed, in which case the failure remains
//     submission-uncertain.
func verifyingEnter(sendEnter func() error, capture func() (string, error), cfg enterVerifyConfig) error {
	time.Sleep(cfg.initialSettle)

	lastStuck := false
	submissionMayHaveOccurred := false
	for i := 0; i < len(cfg.backoffs); i++ {
		if err := sendEnter(); err != nil {
			sendErr := fmt.Errorf("failed to send Enter (-H 0d): %w", err)
			if submissionMayHaveOccurred {
				return MarkPromptSubmissionUncertain(sendErr)
			}
			return sendErr
		}
		time.Sleep(cfg.backoffs[i])

		content, err := capture()
		if err != nil {
			// Cannot verify whether this accepted Enter started the prompt. Keep
			// that uncertainty sticky: a later retry cannot undo work that may
			// already have crossed the submission boundary.
			submissionMayHaveOccurred = true
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
		if submissionMayHaveOccurred {
			return MarkPromptSubmissionUncertain(ErrPasteNotSubmitted)
		}
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
	return sendEnterReliableContext(context.Background(), socketPath, normalizedName)
}

func sendEnterReliableContext(parent context.Context, socketPath, target string) error {
	if parent == nil {
		parent = context.Background()
	}
	// sendEnter runs `tmux send-keys` under the repo subprocess-safety contract:
	// timeout context, isolated process group, and a bounded WaitDelay with a
	// Cancel that SIGKILLs the whole group — a hung tmux can never wedge the loop.
	sendEnter := func() error {
		timeout := getAdaptiveTimeout()
		ctx, cancel := context.WithTimeout(parent, timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "send-keys", "-t", target, "-H", "0d")
		cmd.SysProcAttr = procguard.ProcessGroupAttr()
		cmd.Cancel = func() error {
			if cmd.Process == nil {
				return nil
			}
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		cmd.WaitDelay = time.Second
		return runPromptEnterCommand(ctx, cmd, timeout)
	}
	// capture reuses the exported, policy-compliant pane capture (isolated process
	// group + bounded WaitDelay per CapturePanePolicy).
	capture := func() (string, error) {
		return CapturePaneLogicalANSIOutputContext(parent, target, 5)
	}
	return verifyingEnter(sendEnter, capture, defaultEnterVerifyConfig())
}

// runPromptEnterCommand distinguishes definite failures from a lost reply
// after the tmux client process started. Failure to start and an ordinary
// non-zero exit are explicit rejection before submission. A timeout,
// cancellation, signal, or other indeterminate wait failure may occur after
// the tmux server accepted the Enter and therefore crosses the irreversible
// prompt-submission boundary.
func runPromptEnterCommand(ctx context.Context, cmd *exec.Cmd, timeout time.Duration) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	waitErr := cmd.Wait()
	if waitErr == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		cause := errors.Join(waitErr, ctxErr)
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			cause = &TimeoutError{
				Problem:  fmt.Sprintf("tmux send-keys (Enter) timed out after %v (server may be hung)", timeout),
				Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  agm session list         # Verify recovery",
				Duration: timeout,
			}
		}
		return MarkPromptSubmissionUncertain(cause)
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) && exitErr.ExitCode() >= 0 {
		return waitErr
	}
	return MarkPromptSubmissionUncertain(waitErr)
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
