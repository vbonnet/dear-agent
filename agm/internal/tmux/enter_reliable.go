package tmux

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
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
	initialSettle             time.Duration   // wait before the first Enter (let the paste land)
	backoffs                  []time.Duration // wait after each Enter before re-checking
	requireObservedSubmission bool            // all capture failures remain submission-uncertain
	classifySubmission        func(string) submissionObservation
}

type submissionObservation uint8

const (
	submissionObserved submissionObservation = iota
	submissionStillParked
	submissionAmbiguous
)

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
	return verifyingEnterContext(context.Background(), sendEnter, capture, cfg)
}

type enterVerificationState struct {
	lastStuck                 bool
	submissionMayHaveOccurred bool
	lastCaptureErr            error
}

func verifyingEnterContext(ctx context.Context, sendEnter func() error, capture func() (string, error), cfg enterVerifyConfig) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := waitForEnterVerification(ctx, cfg.initialSettle); err != nil {
		return err
	}

	state := enterVerificationState{}
	for i, backoff := range cfg.backoffs {
		submitted, err := verifyEnterAttempt(ctx, sendEnter, capture, cfg, backoff, i, &state)
		if err != nil {
			return err
		}
		if submitted {
			return nil
		}
	}
	return finishEnterVerification(state, cfg.requireObservedSubmission)
}

func verifyEnterAttempt(
	ctx context.Context,
	sendEnter func() error,
	capture func() (string, error),
	cfg enterVerifyConfig,
	backoff time.Duration,
	attempt int,
	state *enterVerificationState,
) (bool, error) {
	if err := sendEnter(); err != nil {
		sendErr := fmt.Errorf("failed to send Enter (-H 0d): %w", err)
		if state.submissionMayHaveOccurred {
			return false, MarkPromptSubmissionUncertain(sendErr)
		}
		return false, sendErr
	}
	if err := waitForEnterVerification(ctx, backoff); err != nil {
		return false, MarkPromptSubmissionUncertain(err)
	}

	content, err := capture()
	if err != nil {
		// Cannot verify whether this accepted Enter started the prompt. Keep
		// that uncertainty sticky: a later retry cannot undo work that may
		// already have crossed the submission boundary.
		state.submissionMayHaveOccurred = true
		state.lastCaptureErr = err
		state.lastStuck = false
		if cfg.requireObservedSubmission {
			return false, MarkPromptSubmissionUncertain(fmt.Errorf("could not observe prompt submission after accepted Enter: %w", err))
		}
		return false, nil
	}

	observation := submissionObserved
	if isPasteStuck(content) {
		observation = submissionStillParked
	}
	if cfg.classifySubmission != nil {
		observation = cfg.classifySubmission(content)
	}
	if observation == submissionAmbiguous && cfg.requireObservedSubmission {
		return false, MarkPromptSubmissionUncertain(errors.New("post-Enter composer state is ambiguous"))
	}
	if observation == submissionObserved {
		return true, nil
	}

	state.lastStuck = true
	if os.Getenv("AGM_DEBUG") == "1" {
		slog.Debug("verifyingEnter: prompt still parked, re-sending Enter",
			"attempt", attempt+1, "of", len(cfg.backoffs))
	}
	return false, nil
}

func finishEnterVerification(state enterVerificationState, requireObservedSubmission bool) error {
	if state.lastStuck {
		if state.submissionMayHaveOccurred {
			return MarkPromptSubmissionUncertain(ErrPasteNotSubmitted)
		}
		return ErrPasteNotSubmitted
	}
	// Never positively confirmed stuck because every post-Enter observation
	// failed. Legacy callers retain best-effort behavior, while transactional
	// callers require this lost acknowledgement to surface as uncertainty.
	if state.submissionMayHaveOccurred && requireObservedSubmission {
		return MarkPromptSubmissionUncertain(fmt.Errorf("could not observe prompt submission after accepted Enter: %w", state.lastCaptureErr))
	}
	return nil
}

func waitForEnterVerification(ctx context.Context, duration time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
	return sendEnterReliableContextWithConfirmation(parent, socketPath, target, false)
}

func sendEnterReliableContextWithConfirmation(parent context.Context, socketPath, target string, requireObservedSubmission bool) error {
	return sendEnterReliableContextWithProbe(parent, socketPath, exactPasteTarget{PaneID: target}, requireObservedSubmission, 5, nil)
}

func sendEnterReliableContextWithProbe(
	parent context.Context,
	socketPath string,
	target exactPasteTarget,
	requireObservedSubmission bool,
	captureLines int,
	classifySubmission func(string) submissionObservation,
) error {
	if parent == nil {
		parent = context.Background()
	}
	if captureLines < 5 {
		captureLines = 5
	}
	// sendEnter runs `tmux send-keys` under the repo subprocess-safety contract:
	// timeout context, isolated process group, and a bounded WaitDelay with a
	// Cancel that SIGKILLs the whole group — a hung tmux can never wedge the loop.
	sendEnter := func() error {
		if target.strict() {
			return sendEnterToExactTarget(parent, socketPath, target)
		}
		timeout := getAdaptiveTimeout()
		ctx, cancel := context.WithTimeout(parent, timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "send-keys", "-t", target.PaneID, "-H", "0d")
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
		if target.strict() {
			// Strict confirmation must retain the entire generated command. A
			// bounded tail can truncate a long parked prompt's outer composer and
			// falsely interpret its absence as submission. The exact-pane all-
			// history capture is still process-isolated and timeout-bounded.
			return CapturePaneLogicalANSIOutputTargetContext(parent, target.PaneID)
		}
		return CapturePaneLogicalANSIOutputContext(parent, target.PaneID, captureLines)
	}
	config := defaultEnterVerifyConfig()
	config.requireObservedSubmission = requireObservedSubmission
	config.classifySubmission = classifySubmission
	return verifyingEnterContext(parent, sendEnter, capture, config)
}

func sendEnterToExactTarget(ctx context.Context, socketPath string, target exactPasteTarget) error {
	identity, err := newSessionIdentity()
	if err != nil {
		return fmt.Errorf("generate tmux Enter acknowledgement: %w", err)
	}
	ack := "AGM_ENTER_OK_" + identity.Token
	refused := "AGM_ENTER_REFUSED_" + identity.Token
	condition := exactPasteTargetCondition(target, false)
	enterCommand := fmt.Sprintf(
		"send-keys -t %s -H 0d ; display-message -p %s",
		strconv.Quote(target.PaneID), strconv.Quote(ack),
	)
	refuseCommand := fmt.Sprintf("display-message -p %s", strconv.Quote(refused))
	output, runErr := RunWithTimeout(ctx, getAdaptiveTimeout(), "tmux", "-S", socketPath,
		"if-shell", "-F", "-t", target.PaneID, condition, enterCommand, refuseCommand)
	if runErr != nil {
		return MarkPromptSubmissionUncertain(fmt.Errorf("atomic exact-target Enter acknowledgement lost: %w", runErr))
	}
	switch strings.TrimSpace(string(output)) {
	case ack:
		return nil
	case refused:
		detail := "target identity changed"
		if target.RequireNoAttachedClients {
			detail = "target identity changed or a tmux client attached; detach all clients before strict compaction"
		}
		return fmt.Errorf(
			"verified tmux target %q/%d/%s/%s refused Enter because %s; no replacement or attached draft was submitted",
			target.PaneID, target.PanePID, target.SessionID, target.StableSessionID, detail,
		)
	default:
		return MarkPromptSubmissionUncertain(fmt.Errorf(
			"atomic exact-target Enter returned unrecognized acknowledgement %q",
			strings.TrimSpace(string(output)),
		))
	}
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
