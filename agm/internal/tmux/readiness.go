package tmux

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// HarnessInputReady means the harness can safely receive input.
	HarnessInputReady = "YES"
	// HarnessInputBusy means the harness does not currently own an empty composer.
	HarnessInputBusy = "QUEUE"
	// HarnessInputProcessing means the exact foreground harness displays a
	// current, harness-native active-work indicator. It is positive liveness
	// evidence, but the composer is not ready to receive input.
	HarnessInputProcessing = "PROCESSING"
	// HarnessInputQueuedAGM means a queued-input marker and complete AGM message
	// header positively identify a stuck AGM paste in the current logical composer.
	HarnessInputQueuedAGM = "QUEUED_AGM"
	// HarnessInputPermission means a permission decision currently owns input.
	HarnessInputPermission = "PERMISSION"
	// HarnessInputOverlay means a harness overlay currently owns input.
	HarnessInputOverlay = "OVERLAY"
	// HarnessInputOnboarding means a documented first-run prompt currently owns input.
	HarnessInputOnboarding = "ONBOARDING"
	// HarnessInputReviewRequired means executable hooks require explicit
	// operator inspection before Codex startup may continue.
	HarnessInputReviewRequired = "REVIEW_REQUIRED"
	// HarnessInputNotFound means the exact tmux session does not exist.
	HarnessInputNotFound = "NOT_FOUND"
	// HarnessInputWrongHarness means the expected harness process is not alive.
	HarnessInputWrongHarness = "WRONG_HARNESS"
)

// HarnessInputReadiness is the harness-specific, fail-closed verdict shared by
// every surface before it sends input to a tmux pane.
type HarnessInputReadiness struct {
	Ready            bool
	State            string
	Content          string
	TargetPane       string
	TargetPanePID    int
	TargetPID        int
	HarnessStartTime string
	TargetSessionID  string
	StableSessionID  string
	Forced           bool
	// MayHaveStarted is set only when delivery crossed an irreversible prompt
	// submission boundary but tmux acknowledgement was lost.
	MayHaveStarted bool
	// PostSubmitProcessing is positive, exact-target evidence that the harness
	// displayed its native active-work indicator after Enter was accepted.
	PostSubmitProcessing bool
}

type harnessInputObservationRuntime struct {
	resolveActive func(context.Context, string) (activePaneTarget, bool, error)
	resolvePane   func(context.Context, string) (activePaneTarget, bool, error)
	liveness      func(context.Context, activePaneTarget, string) (PaneLiveness, error)
	capture       func(context.Context, string) (string, error)
}

func realHarnessInputObservationRuntime() harnessInputObservationRuntime {
	return harnessInputObservationRuntime{
		resolveActive: func(ctx context.Context, sessionName string) (activePaneTarget, bool, error) {
			return resolveActivePaneTarget(ctx, sessionName, GetSocketPath())
		},
		resolvePane: func(ctx context.Context, paneID string) (activePaneTarget, bool, error) {
			return resolvePaneTarget(ctx, paneID, GetSocketPath())
		},
		liveness: checkExpectedHarnessLivenessForPane,
		capture:  CapturePaneLogicalANSIOutputTargetContext,
	}
}

// InputDeliveryOptions controls narrowly scoped exceptions inside the tmux
// mutation boundary. AllowQueuedAGM accepts only a positively identified stuck
// AGM paste after the expected foreground harness and exact pane are proved;
// the implementation clears and re-proves that exact composer before replacing
// the queued input.
type InputDeliveryOptions struct {
	AllowQueuedAGM bool
	// RequireSubmissionConfirmation turns a lost post-Enter observation into a
	// submission-uncertain error instead of legacy best-effort success, then
	// requires the same exact target to remain positively ready or processing.
	RequireSubmissionConfirmation bool
	// RawBracketedPaste preserves embedded newlines as one native composer
	// paste instead of allowing tmux to translate them into submit keys.
	RawBracketedPaste bool
	// ExpectedStableSessionID binds strict delivery to the durable AGM session
	// identity stored on the tmux session at creation. Missing or mismatched
	// bindings fail before mutation; a same-named replacement cannot inherit
	// delivery authority from the manifest name alone.
	ExpectedStableSessionID string
}

// CheckExpectedHarnessInput proves that the exact session exists, an expected
// harness process is alive, and that harness currently owns the input composer.
// A stale prompt rendered by a dead or different process is never sufficient.
func CheckExpectedHarnessInput(ctx context.Context, sessionName, harness string) (HarnessInputReadiness, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateReadinessHarness(harness); err != nil {
		return HarnessInputReadiness{}, err
	}
	scanCtx, cancel := context.WithTimeout(ctx, livenessScanTimeout)
	defer cancel()
	return checkExpectedHarnessInput(scanCtx, sessionName, harness, realHarnessInputObservationRuntime())
}

func checkExpectedHarnessInput(ctx context.Context, sessionName, harness string, runtime harnessInputObservationRuntime) (HarnessInputReadiness, error) {
	pane, exists, err := runtime.resolveActive(ctx, sessionName)
	if err != nil {
		return HarnessInputReadiness{}, err
	}
	if !exists {
		return HarnessInputReadiness{State: HarnessInputNotFound}, nil
	}
	return checkExpectedHarnessInputTarget(ctx, pane, harness, 0, runtime)
}

// CheckExpectedHarnessInputForPane proves readiness for one exact pane and
// process identity previously returned by atomic delivery. It never follows a
// later active-pane change, and a missing or replaced pane cannot contribute
// completion evidence.
func CheckExpectedHarnessInputForPane(ctx context.Context, paneID string, harnessPID int, harness string) (HarnessInputReadiness, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateReadinessHarness(harness); err != nil {
		return HarnessInputReadiness{}, err
	}
	if !isPaneID(paneID) || harnessPID <= 0 {
		return HarnessInputReadiness{}, fmt.Errorf("invalid verified tmux pane identity %q/%d", paneID, harnessPID)
	}
	scanCtx, cancel := context.WithTimeout(ctx, livenessScanTimeout)
	defer cancel()
	return checkExpectedHarnessInputForPane(scanCtx, paneID, harnessPID, harness, realHarnessInputObservationRuntime())
}

func checkExpectedHarnessInputForPane(ctx context.Context, paneID string, harnessPID int, harness string, runtime harnessInputObservationRuntime) (HarnessInputReadiness, error) {
	pane, exists, err := runtime.resolvePane(ctx, paneID)
	if err != nil {
		return HarnessInputReadiness{}, err
	}
	if !exists {
		return HarnessInputReadiness{State: HarnessInputNotFound, TargetPane: paneID, TargetPID: harnessPID}, nil
	}
	return checkExpectedHarnessInputTarget(ctx, pane, harness, harnessPID, runtime)
}

func checkExpectedHarnessInputTarget(ctx context.Context, pane activePaneTarget, harness string, expectedHarnessPID int, runtime harnessInputObservationRuntime) (HarnessInputReadiness, error) {
	liveness, err := runtime.liveness(ctx, pane, harness)
	if err != nil {
		return HarnessInputReadiness{}, err
	}
	if !liveness.HarnessAlive {
		return HarnessInputReadiness{
			State: HarnessInputWrongHarness, TargetPane: pane.ID,
			TargetPanePID:   pane.RootPID,
			TargetSessionID: pane.SessionID, StableSessionID: pane.StableSessionID,
		}, nil
	}
	if expectedHarnessPID > 0 && liveness.HarnessPID != expectedHarnessPID {
		return HarnessInputReadiness{
			State: HarnessInputNotFound, TargetPane: pane.ID, TargetPID: expectedHarnessPID,
			TargetPanePID:   pane.RootPID,
			TargetSessionID: pane.SessionID, StableSessionID: pane.StableSessionID,
		}, nil
	}
	styledContent, err := runtime.capture(ctx, pane.ID)
	if err != nil {
		return HarnessInputReadiness{}, fmt.Errorf("capture expected %s pane: %w", harness, err)
	}
	ready, state, err := ClassifyHarnessInput(styledContent, harness)
	if err != nil {
		return HarnessInputReadiness{}, err
	}
	return HarnessInputReadiness{
		Ready: ready, State: state, Content: stripANSI(styledContent),
		TargetPane: pane.ID, TargetPanePID: pane.RootPID, TargetPID: liveness.HarnessPID,
		HarnessStartTime: liveness.HarnessStartTime,
		TargetSessionID:  pane.SessionID, StableSessionID: pane.StableSessionID,
	}, nil
}

// CheckExpectedHarnessInputAndSend serializes the readiness observation and
// exact-pane delivery under the same tmux mutation lock. A non-ready result
// never sends input unless options explicitly allow a positively identified
// stuck AGM paste; a ready result is returned only after delivery succeeds.
func CheckExpectedHarnessInputAndSend(ctx context.Context, sessionName, harness, command string, options InputDeliveryOptions) (HarnessInputReadiness, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if options.ExpectedStableSessionID != "" {
		if err := validateStableSessionID(options.ExpectedStableSessionID); err != nil {
			return HarnessInputReadiness{}, err
		}
	}
	if err := acquireTmuxSemaphore(ctx); err != nil {
		return HarnessInputReadiness{}, fmt.Errorf("tmux concurrency limit reached: %w", err)
	}
	defer releaseTmuxSemaphore()

	return checkExpectedHarnessInputAndSendAtBoundary(
		ctx, sessionName, harness, command, options,
		realHarnessInputDeliveryRuntime(), withTmuxLockContext,
	)
}

type tmuxMutationLockRunner func(context.Context, func() error) error

func realHarnessInputDeliveryRuntime() harnessInputDeliveryRuntime {
	return harnessInputDeliveryRuntime{
		check:        CheckExpectedHarnessInput,
		recheckExact: CheckExpectedHarnessInputForPane,
		sendKey:      sendReadinessKey,
		wait:         sleepWithContext,
		deliver: func(ctx context.Context, target HarnessInputReadiness, command, harness string, requireObservedSubmission, rawBracketedPaste bool) error {
			return sendCommandToExactTargetForHarnessLockedWithOptions(
				ctx, exactPasteTarget{
					PaneID: target.TargetPane, PanePID: target.TargetPanePID,
					SessionID: target.TargetSessionID, StableSessionID: target.StableSessionID,
					RequireNoAttachedClients:   requireObservedSubmission,
					SubmissionBaselineCaptured: target.Content != "",
					BaselineCommandAnchors: countExactCommandSubmissionAnchors(
						target.Content, harness, command,
					),
				}, command, harness, requireObservedSubmission, rawBracketedPaste,
			)
		},
	}
}

func checkExpectedHarnessInputAndSendAtBoundary(
	ctx context.Context,
	sessionName, harness, command string,
	options InputDeliveryOptions,
	runtime harnessInputDeliveryRuntime,
	withLock tmuxMutationLockRunner,
) (HarnessInputReadiness, error) {
	var readiness HarnessInputReadiness
	err := withLock(ctx, func() error {
		var deliveryErr error
		readiness, deliveryErr = checkExpectedHarnessInputAndSendLocked(ctx, sessionName, harness, command, options, runtime)
		return deliveryErr
	})
	if err != nil && readiness.Ready {
		// The callback can complete delivery and strict reproof before an unlock
		// failure replaces its nil result. Preserve that irreversible boundary:
		// retry is unsafe even though the mutation lock's release failed.
		readiness.Ready = false
		readiness.MayHaveStarted = true
		err = MarkPromptSubmissionUncertain(fmt.Errorf(
			"harness input was delivered but the tmux mutation boundary did not close cleanly: %w", err,
		))
	}
	return readiness, err
}

type harnessInputDeliveryRuntime struct {
	check        func(context.Context, string, string) (HarnessInputReadiness, error)
	recheckExact func(context.Context, string, int, string) (HarnessInputReadiness, error)
	sendKey      func(context.Context, string, string) error
	wait         func(context.Context, time.Duration) error
	deliver      func(context.Context, HarnessInputReadiness, string, string, bool, bool) error
}

// deliverToExactTarget keeps strict submission confirmation and exact runtime
// identity proof inside the atomic-delivery module. The caller retains the
// tmux mutation lock across both observations. Legacy callers intentionally
// stop after delivery and keep their existing best-effort behavior.
func (runtime harnessInputDeliveryRuntime) deliverToExactTarget(
	ctx context.Context,
	target HarnessInputReadiness,
	harness, command string,
	options InputDeliveryOptions,
) (HarnessInputReadiness, error) {
	if err := runtime.deliver(
		ctx, target, command, harness,
		options.RequireSubmissionConfirmation,
		options.RawBracketedPaste,
	); err != nil {
		return HarnessInputReadiness{}, err
	}
	if !options.RequireSubmissionConfirmation {
		return HarnessInputReadiness{}, nil
	}

	observed, err := runtime.recheckExact(ctx, target.TargetPane, target.TargetPID, harness)
	if err != nil {
		return HarnessInputReadiness{}, MarkPromptSubmissionUncertain(fmt.Errorf(
			"re-prove submitted harness target %q/%d after confirmed submission: %w",
			target.TargetPane, target.TargetPID, err,
		))
	}
	// deliver() established only a terminal rendering consistent with the exact
	// pasted command leaving its parked shape. A multiline human draft can render
	// the same prefix, so generic BUSY is not positive submission continuity.
	// Only native PROCESSING or a positively empty READY composer can confirm the
	// command at this separate exact-runtime boundary.
	positiveContinuity := (observed.Ready && observed.State == HarnessInputReady) ||
		(!observed.Ready && observed.State == HarnessInputProcessing)
	if observed.TargetPane != target.TargetPane ||
		observed.TargetPanePID != target.TargetPanePID ||
		observed.TargetPID != target.TargetPID ||
		observed.HarnessStartTime != target.HarnessStartTime ||
		observed.TargetSessionID != target.TargetSessionID ||
		!stableSessionBindingMatches(observed, options.ExpectedStableSessionID) ||
		!positiveContinuity {
		return HarnessInputReadiness{}, MarkPromptSubmissionUncertain(fmt.Errorf(
			"submitted harness target %q pane-pid %d harness-pid %d start %q could not be positively re-proven after confirmed submission: observed ready=%t state %q on %q pane-pid %d harness-pid %d start %q session %q stable %q",
			target.TargetPane, target.TargetPanePID, target.TargetPID, target.HarnessStartTime,
			observed.Ready, observed.State, observed.TargetPane, observed.TargetPanePID, observed.TargetPID,
			observed.HarnessStartTime, observed.TargetSessionID, observed.StableSessionID,
		))
	}
	return observed, nil
}

func checkExpectedHarnessInputAndSendLocked(
	ctx context.Context,
	sessionName, harness, command string,
	options InputDeliveryOptions,
	runtime harnessInputDeliveryRuntime,
) (HarnessInputReadiness, error) {
	readiness, err := runtime.check(ctx, sessionName, harness)
	if err != nil {
		return HarnessInputReadiness{}, err
	}
	allowed, forced := inputDeliveryAllowed(readiness, options)
	if !allowed {
		return readiness, nil
	}
	if err := validateReadyDeliveryTarget(readiness, options.ExpectedStableSessionID); err != nil {
		readiness.Ready = false
		return readiness, err
	}
	if forced {
		readiness.Ready = false
		readiness.Forced = true
		return replaceQueuedAGMInputLocked(ctx, readiness.TargetPane, readiness.TargetPID, command, queuedAGMRecoveryRuntime{
			sendKey: runtime.sendKey,
			wait:    runtime.wait,
			recheck: func() (HarnessInputReadiness, error) {
				return runtime.recheckExact(ctx, readiness.TargetPane, readiness.TargetPID, harness)
			},
			deliver: func(ctx context.Context, targetPane, command string) (HarnessInputReadiness, error) {
				return runtime.deliverToExactTarget(ctx, HarnessInputReadiness{
					TargetPane:       targetPane,
					TargetPanePID:    readiness.TargetPanePID,
					TargetPID:        readiness.TargetPID,
					HarnessStartTime: readiness.HarnessStartTime,
					TargetSessionID:  readiness.TargetSessionID,
					StableSessionID:  readiness.StableSessionID,
				}, harness, command, options)
			},
		})
	}

	// The first observation authorizes no mutation by itself. Re-resolve the
	// exact pane and foreground harness PID immediately before submission so a
	// harness restart between capture and send cannot inherit the command.
	rechecked, err := runtime.recheckExact(ctx, readiness.TargetPane, readiness.TargetPID, harness)
	if err != nil {
		return rechecked, err
	}
	if err := validateRecheckedDeliveryTarget(readiness, rechecked, options.ExpectedStableSessionID); err != nil {
		rechecked.Ready = false
		return rechecked, err
	}
	if !rechecked.Ready || rechecked.State != HarnessInputReady {
		rechecked.Ready = false
		return rechecked, nil
	}
	postSubmit, err := runtime.deliverToExactTarget(ctx, rechecked, harness, command, options)
	if err != nil {
		rechecked.Ready = false
		rechecked.MayHaveStarted = PromptSubmissionMayHaveOccurred(err)
		return rechecked, err
	}
	rechecked.PostSubmitProcessing = postSubmit.State == HarnessInputProcessing
	return rechecked, nil
}

func validateReadyDeliveryTarget(readiness HarnessInputReadiness, expectedStableSessionID string) error {
	if !isPaneID(readiness.TargetPane) {
		return fmt.Errorf("ready harness returned invalid tmux pane ID %q", readiness.TargetPane)
	}
	if readiness.TargetPID <= 0 {
		return fmt.Errorf("ready harness returned invalid tmux pane PID %d", readiness.TargetPID)
	}
	if expectedStableSessionID != "" && readiness.TargetPanePID <= 0 {
		return fmt.Errorf("ready harness returned invalid tmux pane process ID %d", readiness.TargetPanePID)
	}
	if expectedStableSessionID != "" && readiness.HarnessStartTime == "" {
		return errors.New("ready harness returned no process birth identity")
	}
	if !stableSessionBindingMatches(readiness, expectedStableSessionID) {
		return stableSessionBindingError(readiness, expectedStableSessionID)
	}
	return nil
}

func validateRecheckedDeliveryTarget(
	expected, observed HarnessInputReadiness,
	expectedStableSessionID string,
) error {
	if observed.TargetPane != expected.TargetPane ||
		observed.TargetPanePID != expected.TargetPanePID ||
		observed.TargetPID != expected.TargetPID ||
		observed.HarnessStartTime != expected.HarnessStartTime {
		return fmt.Errorf(
			"verified harness target changed from %q pane-pid %d harness-pid %d start %q to %q pane-pid %d harness-pid %d start %q before delivery",
			expected.TargetPane, expected.TargetPanePID, expected.TargetPID, expected.HarnessStartTime,
			observed.TargetPane, observed.TargetPanePID, observed.TargetPID, observed.HarnessStartTime,
		)
	}
	if observed.TargetSessionID != expected.TargetSessionID ||
		!stableSessionBindingMatches(observed, expectedStableSessionID) {
		return fmt.Errorf(
			"verified harness session binding changed from tmux session %q stable %q to %q stable %q before delivery",
			expected.TargetSessionID, expected.StableSessionID,
			observed.TargetSessionID, observed.StableSessionID,
		)
	}
	return nil
}

type queuedAGMRecoveryRuntime struct {
	sendKey func(context.Context, string, string) error
	wait    func(context.Context, time.Duration) error
	recheck func() (HarnessInputReadiness, error)
	deliver func(context.Context, string, string) (HarnessInputReadiness, error)
}

// replaceQueuedAGMInputLocked clears one positively identified AGM-owned
// composer and proves that the same exact pane now owns an empty composer before
// delivering its replacement. The caller must hold the tmux mutation lock.
func replaceQueuedAGMInputLocked(ctx context.Context, targetPane string, targetPID int, command string, runtime queuedAGMRecoveryRuntime) (HarnessInputReadiness, error) {
	clearSteps := []struct {
		key   string
		pause time.Duration
	}{
		{key: "C-c", pause: 200 * time.Millisecond},
		{key: "C-u", pause: 100 * time.Millisecond},
		{key: "C-a"},
		{key: "C-k", pause: 300 * time.Millisecond},
	}
	for _, step := range clearSteps {
		if err := runtime.sendKey(ctx, targetPane, step.key); err != nil {
			return HarnessInputReadiness{State: HarnessInputQueuedAGM, TargetPane: targetPane, TargetPID: targetPID, Forced: true}, fmt.Errorf("clear queued AGM input with %s: %w", step.key, err)
		}
		if step.pause > 0 {
			if err := runtime.wait(ctx, step.pause); err != nil {
				return HarnessInputReadiness{State: HarnessInputQueuedAGM, TargetPane: targetPane, TargetPID: targetPID, Forced: true}, fmt.Errorf("wait for queued AGM input clear: %w", err)
			}
		}
	}

	cleared, err := runtime.recheck()
	if err != nil {
		return HarnessInputReadiness{State: HarnessInputQueuedAGM, TargetPane: targetPane, TargetPID: targetPID, Forced: true}, fmt.Errorf("recheck cleared queued AGM input: %w", err)
	}
	cleared.Forced = true
	if cleared.TargetPane != targetPane || cleared.TargetPID != targetPID {
		cleared.Ready = false
		return cleared, fmt.Errorf(
			"queued AGM input moved from verified pane %q/%d to %q/%d while clearing",
			targetPane, targetPID, cleared.TargetPane, cleared.TargetPID,
		)
	}
	if !cleared.Ready || cleared.State != HarnessInputReady {
		cleared.Ready = false
		return cleared, fmt.Errorf("queued AGM input was not cleared on verified pane %q: state %s", targetPane, cleared.State)
	}
	postSubmit, err := runtime.deliver(ctx, targetPane, command)
	if err != nil {
		cleared.Ready = false
		cleared.MayHaveStarted = PromptSubmissionMayHaveOccurred(err)
		return cleared, fmt.Errorf("deliver replacement to cleared pane %q: %w", targetPane, err)
	}
	cleared.PostSubmitProcessing = postSubmit.State == HarnessInputProcessing
	return cleared, nil
}

func inputDeliveryAllowed(readiness HarnessInputReadiness, options InputDeliveryOptions) (allowed, forced bool) {
	if readiness.Ready && readiness.State == HarnessInputReady {
		return true, false
	}
	if options.AllowQueuedAGM && readiness.State == HarnessInputQueuedAGM {
		return true, true
	}
	return false, false
}

// ClassifyHarnessInput is the pure composer classifier. Readiness is scoped to
// the configured harness. Queue identity uses the complete joined logical
// composer; blockers and empty-composer readiness remain scoped to the pane
// tail that currently owns input.
func ClassifyHarnessInput(content, harness string) (bool, string, error) {
	if err := validateReadinessHarness(harness); err != nil {
		return false, "", err
	}
	styledTail := paneRawInputTail(content, 12)
	tail := stripANSI(styledTail)
	queuedInput, _ := classifyCurrentQueuedInput(content, harness)

	// Codex hooks can execute outside the sandbox after trust is granted. Treat
	// either complete review surface as a dedicated fail-closed state before
	// generic composer readiness or startup auto-advance can send any key. Use
	// the complete logical capture because a large pane can place the selector
	// above more than twelve trailing blank rows; the detector itself requires
	// current ownership and rejects retained review text above a newer composer.
	if harness == "codex-cli" && IsCodexHookReviewRequired(content) {
		return false, HarnessInputReviewRequired, nil
	}
	if harness == "codex-cli" && containsCodexUpdatePromptPattern(content) {
		return false, HarnessInputOnboarding, nil
	}

	// Pi's managed ready footer remains visible while its native confirmation
	// dialog owns input. Treat that dialog as authoritative before consulting
	// the footer so automated input can never become an authorization answer.
	if harness == "pi-cli" && hasPiManagedPermissionPrompt(tail) {
		return false, HarnessInputPermission, nil
	}
	// Claude's selected trust rows also use the ❯ glyph. Give the live dialog
	// precedence over composer detection so ANSI styling can never make either
	// selected option look like a ready input line.
	if harness == "claude-code" && TrustDialogOwnsInput(content) {
		return false, HarnessInputOnboarding, nil
	}

	var ready bool
	switch harness {
	case "claude-code":
		ready = hasTailOwnedClaudeComposer(styledTail)
	case "codex-cli":
		ready = isCodexInputComposerReady(styledTail)
	case "agy":
		ready = hasTailOwnedAgyComposer(tail)
	case "gemini-cli":
		ready = hasTailOwnedGeminiComposer(tail)
	case "opencode-cli":
		ready = hasTailOwnedOpenCodeComposer(tail)
	case "pi-cli":
		ready = containsPiReadyPattern(tail)
	}
	if ready && queuedInput == QueuedInputNone {
		return true, HarnessInputReady, nil
	}
	if hasInputOverlay(tail, harness) {
		return false, HarnessInputOverlay, nil
	}
	if hasOnboardingPrompt(tail, harness) {
		return false, HarnessInputOnboarding, nil
	}
	if hasTailOwnedPermissionPrompt(tail) {
		return false, HarnessInputPermission, nil
	}
	if queuedInput == QueuedInputAGM {
		return false, HarnessInputQueuedAGM, nil
	}
	// Active work is positive evidence only when the current tail carries a
	// harness-native status grammar. Generic occupied composers, human drafts,
	// queued input, and arbitrary prose containing words such as "Working" stay
	// fail-closed as BUSY.
	if queuedInput == QueuedInputNone && hasHarnessNativeProcessingTail(styledTail, harness) {
		return false, HarnessInputProcessing, nil
	}
	return false, HarnessInputBusy, nil
}

var (
	codexNativeProcessingPattern  = regexp.MustCompile(`(?i)^•\s+working\s+\([^)]*esc to interrupt[^)]*\)$`)
	claudeNativeProcessingPattern = regexp.MustCompile(`(?i)^[✶✢✻·]\s+\S.*esc to interrupt.*$`)
)

func hasHarnessNativeProcessingTail(content, harness string) bool {
	lines := strings.Split(stripANSI(content), "\n")
	switch harness {
	case "codex-cli":
		return nativeProcessingStatusOwnsTail(lines, codexNativeProcessingPattern, func(line string) bool {
			return codexFooterPattern.MatchString(strings.TrimSpace(line))
		})
	case "claude-code":
		return nativeProcessingStatusOwnsTail(lines, claudeNativeProcessingPattern, func(line string) bool {
			plain := strings.TrimSpace(line)
			return !isDecorativeChromeLine(plain) && isClaudeComposerFooterChrome(plain)
		})
	case "pi-cli":
		for _, line := range slices.Backward(lines) {
			if isDecorativeChromeLine(line) {
				continue
			}
			return PiManagedState(line) == "working"
		}
	}
	// AGY, Gemini, and OpenCode do not currently expose a sufficiently stable
	// native active-tail contract. Their non-ready output remains generic BUSY.
	return false
}

func nativeProcessingStatusOwnsTail(lines []string, status *regexp.Regexp, isNativeFooter func(string) bool) bool {
	statusIndex := -1
	for i, line := range lines {
		if status.MatchString(strings.TrimSpace(line)) {
			statusIndex = i
		}
	}
	if statusIndex < 0 {
		return false
	}
	foundNativeFooter := false
	for _, line := range lines[statusIndex+1:] {
		if isDecorativeChromeLine(line) {
			continue
		}
		if !isNativeFooter(line) {
			return false
		}
		foundNativeFooter = true
	}
	return foundNativeFooter
}

// classifyCurrentQueuedInput scopes queue evidence to the registered harness's
// latest composer. It downgrades an otherwise valid AGM header to human-owned
// input unless the marker is on the current occupied composer and idle chrome
// still owns the pane tail.
func classifyCurrentQueuedInput(content, harness string) (QueuedInputType, string) {
	region, anchorLine, ok := currentComposerInputRegion(content, harness)
	if !ok {
		return QueuedInputNone, ""
	}
	inputType, description := ClassifyQueuedInput(stripANSI(region))
	if inputType != QueuedInputAGM {
		if harness == "pi-cli" && inputType != QueuedInputNone && piQueuedMarkerIsHistorical(region) {
			return QueuedInputNone, ""
		}
		return inputType, description
	}
	if harness == "pi-cli" && piQueuedMarkerIsHistorical(region) {
		return QueuedInputNone, ""
	}
	if !hasQueuedInputMarker(stripANSI(anchorLine)) || !queuedComposerOwnsTail(region, content, harness) {
		return QueuedInputHuman, "session has human input in progress - not sending. Retry later"
	}
	return inputType, description
}

func currentComposerInputRegion(content, harness string) (region, anchorLine string, ok bool) {
	lines := strings.Split(content, "\n")
	anchor := -1
	for i, line := range lines {
		plain := strings.TrimSpace(stripANSI(line))
		if isCurrentComposerAnchor(plain, harness) {
			candidate := strings.Join(lines[i:], "\n")
			// Once a structural pasted-content marker owns the pane tail, all
			// following lines are payload. Prompt glyphs inside that payload must
			// not replace the composer anchor that introduced it.
			if hasQueuedInputMarker(plain) && queuedComposerOwnsTail(candidate, content, harness) {
				return candidate, lines[i], true
			}
			anchor = i
		}
	}
	if anchor < 0 {
		return "", "", false
	}
	return strings.Join(lines[anchor:], "\n"), lines[anchor], true
}

func isCurrentComposerAnchor(line, harness string) bool {
	switch harness {
	case "claude-code":
		return strings.HasPrefix(line, "❯")
	case "codex-cli":
		return isCodexComposerAnchor(line)
	case "agy":
		return line == ">" || strings.HasPrefix(line, "> ") && hasQueuedInputMarker(line)
	case "gemini-cli":
		return isGeminiComposerAnchor(line)
	case "opencode-cli":
		return isOpenCodeComposerAnchor(line)
	case "pi-cli":
		return hasQueuedInputMarker(line)
	default:
		return false
	}
}

func isCodexComposerAnchor(line string) bool {
	return isCodexEmptyCursor(line) || hasCodexCursorInputPrefix(line) || line == ">" || strings.HasPrefix(line, "> [Pasted Content")
}

func queuedComposerOwnsTail(region, content, harness string) bool {
	plainRegion := stripANSI(region)
	if hasActiveSpinner(plainRegion) || strings.Contains(strings.ToLower(plainRegion), "esc to interrupt") {
		return false
	}
	lines := strings.Split(strings.TrimSpace(plainRegion), "\n")
	if len(lines) == 0 {
		return false
	}
	switch harness {
	case "claude-code":
		// Share the composer footer parser with hasTailOwnedClaudeComposer so a
		// queued AGM paste on a modern cwd/login/effort footer is still
		// recognized as idle-composer-owned (ce-wn4qe); the legacy whitelist
		// rejected those lines and downgraded the paste to human input.
		return hasTerminalIdleFooter(lines, isClaudeComposerFooterChrome) && queuedPastePayloadOwnsTail(lines, isClaudeComposerFooterChrome)
	case "codex-cli":
		// Codex keeps its model footer visible while a turn is active, and a
		// queued paste replaces the empty cursor that would otherwise prove idle
		// ownership. Only the compact first-turn composer can therefore prove the
		// queue was pasted before any work began; post-turn queues fail closed.
		return codexInitialQueuedComposerOwnsTail(stripANSI(content))
	case "agy":
		return queuedPastePayloadOwnsTail(lines, isTerminalIdleChrome)
	case "gemini-cli":
		return hasTerminalIdleFooter(lines, isGeminiIdleFooter) && queuedPastePayloadOwnsTail(lines, isTerminalIdleChrome)
	case "opencode-cli":
		return queuedPastePayloadOwnsTail(lines, isTerminalIdleChrome)
	case "pi-cli":
		return hasTerminalIdleFooter(lines, isPiReadyFooter) && queuedPastePayloadOwnsTail(lines, isPiIdleComposerChrome)
	}
	return false
}

var queuedPastedTextLinePattern = regexp.MustCompile(`\[Pasted text(?: #\d+)? \+(\d+) lines?\]`)
var codexQueuedPasteCharPattern = regexp.MustCompile(`^[›»>]\s+\[Pasted Content (\d+) chars\]$`)

func queuedPastePayloadOwnsTail(lines []string, isChrome func(string) bool) bool {
	if len(lines) == 0 {
		return false
	}
	matches := queuedPastedTextLinePattern.FindStringSubmatch(lines[0])
	if matches == nil {
		return false
	}
	want, err := strconv.Atoi(matches[1])
	if err != nil {
		return false
	}
	payload := lines[1:]
	// Strip only the trailing footer/chrome that sits *below* the declared
	// payload — never into the payload itself. Otherwise a payload whose final
	// line happens to contain a footer token (e.g. "· /effort") would be
	// mis-stripped and the line count would fail (ce-wn4qe).
	for len(payload) > want && isChrome(payload[len(payload)-1]) {
		payload = payload[:len(payload)-1]
	}
	return len(payload) == want
}

func hasTerminalIdleFooter(lines []string, isFooter func(string) bool) bool {
	for _, line := range slices.Backward(lines) {
		if isDecorativeChromeLine(line) {
			continue
		}
		return isFooter(line)
	}
	return false
}

func isGeminiIdleFooter(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(stripANSI(line)))
	return strings.Contains(lower, "? for shortcuts") || strings.Contains(lower, "sandbox") && strings.Contains(lower, "gemini-")
}

func isPiIdleComposerChrome(line string) bool {
	plain := strings.TrimSpace(stripANSI(line))
	if isDecorativeChromeLine(plain) {
		return true
	}
	return PiManagedState(plain) != "" || strings.Contains(plain, " • pi-") || strings.Contains(plain, "%/")
}

func isPiReadyFooter(line string) bool {
	return PiManagedState(stripANSI(line)) == "ready"
}

func isDecorativeChromeLine(line string) bool {
	line = strings.TrimSpace(stripANSI(line))
	return line == "" || strings.Trim(line, "─━┄┈╌╍═│┃┆┊╎╏┌┐└┘├┤┬┴┼╭╮╰╯ ") == ""
}

func isGeminiComposerAnchor(line string) bool {
	line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "│"))
	return strings.HasPrefix(line, ">") && (strings.Contains(strings.ToLower(line), "type your message") || hasQueuedInputMarker(line))
}

func isOpenCodeComposerAnchor(line string) bool {
	line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "│"))
	switch line {
	case ">", ">>", "❯":
		return true
	}
	lower := strings.ToLower(line)
	if strings.HasPrefix(line, ">") && (strings.Contains(lower, "type your message") || strings.Contains(lower, "type here") || hasQueuedInputMarker(line)) {
		return true
	}
	return strings.HasPrefix(line, "❯") && (line == "❯" || hasQueuedInputMarker(line))
}

func piQueuedMarkerIsHistorical(region string) bool {
	lines := strings.Split(strings.TrimSpace(stripANSI(region)), "\n")
	return hasTerminalIdleFooter(lines, isPiReadyFooter) && !queuedPastePayloadOwnsTail(lines, isPiIdleComposerChrome)
}

func codexInitialQueuedComposerOwnsTail(content string) bool {
	// capture-pane -p appends one framing newline. Remove only that terminator:
	// trailing whitespace inside the queued payload contributes to Codex's
	// native pasted-character extent and must remain observable.
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	for i, line := range lines {
		if !strings.Contains(line, CodexPromptPatterns[0]) {
			continue
		}
		for j := i + 1; j < len(lines) && j <= i+4; j++ {
			if !strings.Contains(lines[j], CodexPromptPatterns[1]) {
				continue
			}
			for k, tailLine := range lines[j+1:] {
				candidate := strings.TrimSpace(tailLine)
				switch {
				case candidate == "":
				case strings.HasPrefix(candidate, "│") && strings.HasSuffix(candidate, "│"):
				case strings.HasPrefix(candidate, "╰") && strings.HasSuffix(candidate, "╯"):
				case (hasCodexCursorInputPrefix(candidate) || strings.HasPrefix(candidate, "> ")) && hasQueuedInputMarker(candidate):
					return codexQueuedPastePayloadOwnsTail(lines[j+1+k:])
				default:
					return false
				}
			}
		}
	}
	return false
}

func codexQueuedPastePayloadOwnsTail(lines []string) bool {
	if len(lines) < 2 {
		return false
	}
	matches := codexQueuedPasteCharPattern.FindStringSubmatch(strings.TrimSpace(lines[0]))
	if matches == nil {
		return false
	}
	want, err := strconv.Atoi(matches[1])
	if err != nil {
		return false
	}
	return utf8.RuneCountInString(strings.Join(lines[1:], "\n")) == want
}

// CheckExpectedHarnessLiveness scans the exact session's process tree and
// accepts only processes compatible with the configured harness. Node-backed
// harnesses must carry a harness-specific script/package identity in argv;
// an unrelated Node descendant is not liveness proof.
func CheckExpectedHarnessLiveness(ctx context.Context, sessionName, harness string) (PaneLiveness, error) {
	if err := validateReadinessHarness(harness); err != nil {
		return PaneLiveness{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	scanCtx, cancel := context.WithTimeout(ctx, livenessScanTimeout)
	defer cancel()
	pane, exists, err := resolveActivePaneTarget(scanCtx, sessionName, GetSocketPath())
	if err != nil {
		return PaneLiveness{}, err
	}
	if !exists {
		return PaneLiveness{SessionExists: false}, nil
	}
	return checkExpectedHarnessLivenessForPane(scanCtx, pane, harness)
}

func checkExpectedHarnessLivenessForPane(ctx context.Context, pane activePaneTarget, harness string) (PaneLiveness, error) {
	procs, err := readProcessTableWithArgs(ctx)
	if err != nil {
		return PaneLiveness{}, err
	}
	return classifyPaneLivenessProcesses([]int{pane.RootPID}, procs, expectedHarnessProcessMatcher(harness)), nil
}

// WaitForExpectedHarnessReady owns startup readiness for shared operations.
// It permits documented first-run transitions, but otherwise only observes.
func WaitForExpectedHarnessReady(ctx context.Context, sessionName, harness string, timeout time.Duration) error {
	if err := validateReadinessHarness(harness); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	observedHarness := false
	advanced := make(map[string]bool)
	for {
		readiness, err := CheckExpectedHarnessInput(waitCtx, sessionName, harness)
		if err != nil {
			if waitCtx.Err() != nil {
				return fmt.Errorf("wait for %s readiness in %q: %w", harness, sessionName, waitCtx.Err())
			}
			return fmt.Errorf("check %s readiness in %q: %w", harness, sessionName, err)
		}
		ready, err := handleHarnessStartupState(waitCtx, sessionName, harness, readiness, &observedHarness, advanced)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}

		select {
		case <-waitCtx.Done():
			return fmt.Errorf("timeout waiting for %s readiness in %q (last state %s): %w", harness, sessionName, readiness.State, waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func handleHarnessStartupState(
	ctx context.Context,
	sessionName, harness string,
	readiness HarnessInputReadiness,
	observedHarness *bool,
	advanced map[string]bool,
) (bool, error) {
	return handleHarnessStartupStateWithProbe(
		ctx, sessionName, harness, readiness, observedHarness, advanced, ProbeClaudeInputContext,
	)
}

func handleHarnessStartupStateWithProbe(
	ctx context.Context,
	sessionName, harness string,
	readiness HarnessInputReadiness,
	observedHarness *bool,
	advanced map[string]bool,
	probeClaude func(context.Context, string, bool) (ClaudeInputProbe, error),
) (bool, error) {
	switch readiness.State {
	case HarnessInputReady:
		*observedHarness = true
		return true, nil
	case HarnessInputNotFound:
		return false, fmt.Errorf("tmux session %q disappeared while waiting for %s readiness", sessionName, harness)
	case HarnessInputWrongHarness:
		if *observedHarness {
			return false, fmt.Errorf("expected %s process stopped in tmux session %q after startup", harness, sessionName)
		}
	case HarnessInputReviewRequired:
		*observedHarness = true
		return false, CodexHookReviewError()
	case HarnessInputOnboarding, HarnessInputOverlay:
		*observedHarness = true
		if !canAdvanceHarnessStartup(readiness.State, harness, readiness.Content) {
			return false, nil
		}
		transition := readiness.State + ":" + onboardingKind(readiness.Content, harness)
		if !advanced[transition] {
			if harness == "claude-code" {
				probe, err := probeClaude(ctx, sessionName, true)
				if err != nil {
					return false, fmt.Errorf("re-prove Claude startup trust selector in %q: %w", sessionName, err)
				}
				if probe.TrustAnswered {
					advanced[transition] = true
				}
				return false, nil
			}
			if err := advanceHarnessStartup(ctx, readiness.TargetPane, harness, readiness.Content); err != nil {
				return false, fmt.Errorf("advance %s startup in %q: %w", harness, sessionName, err)
			}
			advanced[transition] = true
		}
	default:
		*observedHarness = true
	}
	return false, nil
}

var stableSessionIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,100}$`)

func validateStableSessionID(stableSessionID string) error {
	if !stableSessionIDPattern.MatchString(stableSessionID) {
		return fmt.Errorf("invalid stable AGM session ID %q", stableSessionID)
	}
	return nil
}

func stableSessionBindingMatches(readiness HarnessInputReadiness, expected string) bool {
	return expected == "" ||
		readiness.TargetSessionID != "" && readiness.StableSessionID == expected
}

func stableSessionBindingError(readiness HarnessInputReadiness, expected string) error {
	if expected == "" {
		return nil
	}
	if readiness.StableSessionID == "" {
		return fmt.Errorf(
			"tmux session %q has no stable AGM session binding; cold-resume or recreate it before strict delivery for %q",
			readiness.TargetSessionID, expected,
		)
	}
	return fmt.Errorf(
		"tmux session %q is bound to stable AGM session %q, not %q",
		readiness.TargetSessionID, readiness.StableSessionID, expected,
	)
}

func validateReadinessHarness(harness string) error {
	switch harness {
	case "claude-code", "codex-cli", "agy", "gemini-cli", "opencode-cli", "pi-cli":
		return nil
	default:
		return fmt.Errorf("unsupported harness readiness check %q", harness)
	}
}

func expectedHarnessProcessMatcher(harness string) func(ProcEntry) bool {
	return func(process ProcEntry) bool {
		if !processOwnsForegroundTerminal(process) {
			return false
		}
		base := filepath.Base(strings.TrimSpace(process.Comm))
		return processBaseMatchesHarness(base, harness) ||
			nodeProcessMatchesHarness(process.Args, harness)
	}
}

// processOwnsForegroundTerminal rejects a matching harness that is merely a
// stopped or background descendant. Only a process in the terminal's current
// foreground process group can own the composer that will receive input.
func processOwnsForegroundTerminal(process ProcEntry) bool {
	return process.PGID > 0 && process.TPGID == process.PGID &&
		!strings.Contains(strings.ToUpper(process.State), "T")
}

func processBaseMatchesHarness(base, harness string) bool {
	switch harness {
	case "claude-code":
		return base == "claude" || isClaudeProcess(base)
	case "codex-cli":
		return base == "codex"
	case "agy":
		return base == "agy"
	case "gemini-cli":
		return base == "gemini"
	case "opencode-cli":
		return base == "opencode"
	case "pi-cli":
		return base == "pi"
	default:
		return false
	}
}

func nodeProcessMatchesHarness(args, harness string) bool {
	if harness == "pi-cli" {
		return isPiProcessCommand(args)
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return false
	}
	executable := 0
	if filepath.Base(strings.Trim(fields[0], "'\"")) == "env" {
		executable++
		for executable < len(fields) && strings.Contains(fields[executable], "=") {
			executable++
		}
	}
	if executable >= len(fields) || filepath.Base(strings.Trim(fields[executable], "'\"")) != "node" {
		return false
	}
	script, mustResolve, ok := nodeScriptArgument(args, fields, executable)
	if !ok {
		return false
	}
	if mustResolve {
		resolved, err := filepath.EvalSymlinks(script)
		if err != nil {
			return false
		}
		script = resolved
	}
	script = strings.ToLower(filepath.ToSlash(script))
	patterns := map[string][]string{
		"claude-code":  {"@anthropic-ai/claude-code", "/claude-code/", "/bin/claude", "claude.js"},
		"codex-cli":    {"@openai/codex", "/codex/bin/", "/bin/codex", "codex.js"},
		"gemini-cli":   {"@google/gemini-cli", "/gemini-cli/", "/bin/gemini", "gemini.js"},
		"opencode-cli": {"opencode-ai", "/opencode/bin/", "/bin/opencode", "opencode.js"},
	}
	for _, pattern := range patterns[harness] {
		if strings.Contains(script, pattern) {
			return true
		}
	}
	return false
}

func paneRawInputTail(content string, maxLines int) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

func hasTailOwnedClaudeComposer(content string) bool {
	lines := strings.Split(content, "\n")
	promptIndex := -1
	for i, line := range lines {
		plainLine := strings.TrimSpace(stripANSI(line))
		if plainLine == "❯" || strings.HasPrefix(plainLine, "❯") && HasGhostTextInANSI(line) {
			promptIndex = i
		}
	}
	if promptIndex < 0 {
		return false
	}
	// The composer owns the tail only when every line below the ❯ input line is
	// the composer box's border or its idle status footer — never active
	// output. Any unrecognised/dynamic line (a spinner, tool progress like
	// "Running tests") fails closed, which is what keeps a mid-turn ❯ from being
	// read as ready.
	for _, line := range lines[promptIndex+1:] {
		if !isClaudeComposerFooterChrome(line) {
			return false
		}
	}
	return true
}

// claudeStatusCwdPattern matches the status-footer cwd anchor "user@host:/path".
var claudeStatusCwdPattern = regexp.MustCompile(`^\S+@\S+:`)

// isClaudeComposerFooterChrome reports whether a line rendered below the ❯ input
// line is part of the composer's idle box/status footer rather than active
// output. Claude Code's status footer (cwd@host, mode, auth, effort, hints)
// grew several lines across releases; recognise those variants so a ready
// composer is not mistaken for a running turn (ce-wn4qe), while a spinner or any
// dynamic tool output below the composer still fails closed.
func isClaudeComposerFooterChrome(line string) bool {
	plain := strings.TrimSpace(stripANSI(line))
	if plain == "" {
		return true
	}
	// Box borders / decorative rules.
	if strings.Trim(plain, "─━┄┈╌╍═│┃┆┊╎╏┌┐└┘├┤┬┴┼╭╮╰╯ ") == "" {
		return true
	}
	lower := strings.ToLower(plain)
	// A running turn is never idle chrome.
	if strings.Contains(lower, "esc to interrupt") || hasActiveSpinner(plain) {
		return false
	}
	// Known status-footer / hint tokens across Claude Code releases. These are
	// deliberately specific (mode lines use their "… on" form, auth uses the full
	// "not logged in" / "run /login") so ordinary model output that merely
	// contains a word like "/model" is not mistaken for footer chrome.
	for _, marker := range []string{
		"? for shortcuts", "for shortcuts", "shift+tab", "← for agents",
		"bypass permissions on", "accept edits on", "plan mode on", "auto-accept edits",
		"run /login", "not logged in",
		"· /effort", "context left", "% context",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	// Status-line cwd anchor, e.g. "vbonnet@mac:/private/tmp/wd".
	return claudeStatusCwdPattern.MatchString(plain)
}

func hasTailOwnedGeminiComposer(content string) bool {
	return hasTailOwnedTextComposer(content, func(line string) bool {
		line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "│"))
		return strings.HasPrefix(line, ">") && strings.Contains(strings.ToLower(line), "type your message")
	})
}

func hasTailOwnedAgyComposer(content string) bool {
	return hasTailOwnedTextComposer(content, func(line string) bool {
		return strings.TrimSpace(line) == ">"
	})
}

func hasTailOwnedOpenCodeComposer(content string) bool {
	return hasTailOwnedTextComposer(content, func(line string) bool {
		line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "│"))
		switch line {
		case ">", ">>", "❯":
			return true
		}
		lower := strings.ToLower(line)
		return strings.HasPrefix(line, ">") &&
			(strings.Contains(lower, "type your message") || strings.Contains(lower, "type here"))
	})
}

func hasTailOwnedTextComposer(content string, isComposer func(string) bool) bool {
	lines := strings.Split(content, "\n")
	composerIndex := -1
	for i, line := range lines {
		if isComposer(line) {
			composerIndex = i
		}
	}
	if composerIndex < 0 {
		return false
	}
	for _, line := range lines[composerIndex+1:] {
		if !isTerminalIdleChrome(line) {
			return false
		}
	}
	return true
}

func isTerminalIdleChrome(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return true
	}
	if strings.Trim(line, "─━┄┈╌╍═│┃┆┊╎╏┌┐└┘├┤┬┴┼╭╮╰╯ ") == "" {
		return true
	}
	lower := strings.ToLower(line)
	for _, marker := range []string{"? for shortcuts", "shift+tab to", "accept edits"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return strings.Contains(lower, "sandbox") && strings.Contains(lower, "gemini-")
}

func isCodexInputComposerReady(content string) bool {
	return IsCodexComposerReady(content)
}

var permissionChoicePattern = regexp.MustCompile(`(?i)^\s*(?:[❯>•]\s*(?:\d+[.)]\s*)?|\d+[.)]\s+)(yes|no|allow(?:\s+once|\s+always)?|deny|approve|reject|cancel|don't allow)\b`)

func hasTailOwnedPermissionPrompt(content string) bool {
	lines := strings.Split(content, "\n")
	anchor := -1
	for i, line := range lines {
		lower := strings.ToLower(line)
		for _, marker := range []string{
			"do you want to proceed?",
			"allow this command",
			"allow this action",
			"approve this action",
		} {
			if strings.Contains(lower, marker) {
				anchor = i
				break
			}
		}
	}
	if anchor >= 0 && permissionChoicesOwnTail(lines[anchor+1:]) {
		return true
	}
	return unanchoredPermissionChoicesOwnTail(lines)
}

func hasPiManagedPermissionPrompt(content string) bool {
	return strings.Contains(strings.ToLower(content), "agm permission required") ||
		hasTailOwnedPermissionPrompt(content)
}

func permissionChoicesOwnTail(lines []string) bool {
	hasChoice := false
	for _, line := range lines {
		if _, ok := permissionChoiceKind(line); ok {
			hasChoice = true
			continue
		}
		if isPermissionPromptChrome(line) {
			continue
		}
		return false
	}
	return hasChoice
}

func unanchoredPermissionChoicesOwnTail(lines []string) bool {
	var allow, deny, structured bool
	for _, line := range slices.Backward(lines) {
		if isPermissionPromptChrome(line) {
			continue
		}
		kind, ok := permissionChoiceKind(line)
		if !ok {
			break
		}
		structured = true
		switch kind {
		case "yes", "allow", "approve":
			allow = true
		case "no", "deny", "reject", "cancel", "don't allow":
			deny = true
		}
	}
	return structured && allow && deny
}

func permissionChoiceKind(line string) (string, bool) {
	match := permissionChoicePattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(match) != 2 {
		return "", false
	}
	kind := strings.ToLower(match[1])
	if strings.HasPrefix(kind, "allow") {
		kind = "allow"
	}
	return kind, true
}

func isPermissionPromptChrome(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return true
	}
	if strings.Trim(line, "─━┄┈╌╍═│┃┆┊╎╏┌┐└┘├┤┬┴┼╭╮╰╯ ") == "" {
		return true
	}
	lower := strings.ToLower(line)
	return strings.Contains(lower, "enter to confirm") ||
		strings.Contains(lower, "esc to cancel") ||
		strings.Contains(lower, "use arrow keys")
}

func hasInputOverlay(content, harness string) bool {
	if strings.Contains(content, "Background tasks") && strings.Contains(content, "to close") {
		return true
	}
	return harness == "agy" && ContainsAgySurveyPrompt(content)
}

func hasOnboardingPrompt(content, harness string) bool {
	switch harness {
	case "claude-code":
		// Classify either selected trust option as onboarding while the dialog
		// owns the tail. Startup advancement separately requires the affirmative
		// selection, so a selected "No" blocks input without authorizing Enter.
		return TrustDialogOwnsInput(content)
	case "codex-cli":
		return containsCodexTrustPromptPattern(content) || containsCodexModelUpgradePromptPattern(content) || containsCodexUpdatePromptPattern(content)
	case "agy":
		return containsAgyTrustPromptPattern(content)
	case "gemini-cli":
		return containsGeminiTrustPromptPattern(content)
	default:
		return false
	}
}

func containsGeminiTrustPromptPattern(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "do you trust the files in this folder") ||
		strings.Contains(lower, "do you trust this folder")
}

func onboardingKind(content, harness string) string {
	if harness == "codex-cli" && containsCodexUpdatePromptPattern(content) {
		return "update"
	}
	if harness == "codex-cli" && containsCodexModelUpgradePromptPattern(content) {
		return "model-upgrade"
	}
	if harness == "agy" && ContainsAgySurveyPrompt(content) {
		return "survey"
	}
	return "trust"
}

func canAdvanceHarnessStartup(state, harness, content string) bool {
	if state == HarnessInputOnboarding {
		return harness != "claude-code" || TrustSelectorOwnsInput(content)
	}
	return state == HarnessInputOverlay && harness == "agy" && ContainsAgySurveyPrompt(content)
}

func advanceHarnessStartup(ctx context.Context, targetPane, harness, content string) error {
	if !isPaneID(targetPane) {
		return fmt.Errorf("missing verified pane for %s startup transition", harness)
	}
	for _, key := range harnessStartupAdvanceKeys(harness, content) {
		if err := sendReadinessKey(ctx, targetPane, key); err != nil {
			return err
		}
	}
	return nil
}

func harnessStartupAdvanceKeys(harness, content string) []string {
	if harness == "codex-cli" && (containsCodexModelUpgradePromptPattern(content) || containsCodexUpdatePromptPattern(content)) {
		return []string{"Down", "Enter"}
	}
	if harness == "agy" && ContainsAgySurveyPrompt(content) {
		return []string{"0"}
	}
	if harness == "gemini-cli" && containsGeminiTrustPromptPattern(content) {
		return []string{"1", "Enter"}
	}
	return []string{"Enter"}
}

func sendReadinessKey(ctx context.Context, targetPane, key string) error {
	_, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", GetSocketPath(), "send-keys", "-t", targetPane, key)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
