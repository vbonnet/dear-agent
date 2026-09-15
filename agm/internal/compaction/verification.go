package compaction

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// VerificationTarget binds every completion observation to the exact harness
// pane that received the compaction command.
type VerificationTarget struct {
	SessionName      string
	Harness          string
	PaneID           string
	PanePID          int
	TargetPID        int
	HarnessStartTime string
	TargetSessionID  string
	StableSessionID  string
	// InitialProcessingObserved is seeded only by an exact post-submit native
	// PROCESSING observation from the atomic delivery transaction. It allows a
	// very short compaction to complete before the first verifier poll without
	// treating a post-submit READY frame as causality proof.
	InitialProcessingObserved bool
}

func (target VerificationTarget) validate() error {
	if target.SessionName == "" || target.Harness == "" || target.PaneID == "" ||
		target.PanePID <= 0 || target.TargetPID <= 0 || target.HarnessStartTime == "" || target.TargetSessionID == "" || target.StableSessionID == "" {
		return fmt.Errorf("incomplete verification target: %#v", target)
	}
	return nil
}

// StateObserver returns one typed observation for the fixed delivery target.
// Implementations must honor the supplied context.
type StateObserver func(context.Context, VerificationTarget) (*session.DetectionResult, error)

// CompletionProof identifies the evidence protocol that established completion.
type CompletionProof string

const (
	// ProofBusyThenStableReady requires a post-delivery live active transition
	// followed by two consecutive live-ready observations.
	ProofBusyThenStableReady CompletionProof = "busy_then_stable_ready"
)

// Verification is returned only after completion has been positively observed.
type Verification struct {
	Proof   CompletionProof
	Elapsed time.Duration
}

// UnverifiedReason classifies why positive completion evidence was not obtained.
type UnverifiedReason string

// Unverified reasons identify the first fail-closed boundary that prevented a
// positive completion proof.
const (
	UnverifiedTimeout           UnverifiedReason = "timeout"
	UnverifiedEvidenceLost      UnverifiedReason = "evidence_lost"
	UnverifiedSessionLost       UnverifiedReason = "session_lost"
	UnverifiedBlocked           UnverifiedReason = "blocked"
	UnverifiedObservationFailed UnverifiedReason = "observation_failed"
	UnverifiedCausalityLost     UnverifiedReason = "causality_lost"
)

// ErrCompletionUnverified is the stable identity shared by all unverified
// completion outcomes.
var ErrCompletionUnverified = errors.New("compaction completion unverified")

// UnverifiedError carries a stable reason, the last observation, and any
// underlying observation failure without turning ambiguity into success.
type UnverifiedError struct {
	Reason UnverifiedReason
	Last   session.DetectionResult
	Cause  error
}

func (e *UnverifiedError) Error() string {
	message := fmt.Sprintf("%s: %s", ErrCompletionUnverified, e.Reason)
	if e.Last.State != "" || e.Last.Evidence != "" {
		message += fmt.Sprintf(" (state: %s, evidence: %s)", e.Last.State, e.Last.Evidence)
	}
	if e.Cause != nil {
		message += ": " + e.Cause.Error()
	}
	return message
}

func (e *UnverifiedError) Unwrap() error {
	return e.Cause
}

// Is reports the shared completion sentinel and any wrapped observation cause.
func (e *UnverifiedError) Is(target error) bool {
	return target == ErrCompletionUnverified || errors.Is(e.Cause, target)
}

type verifierClock interface {
	Now() time.Time
	Wait(context.Context, time.Duration) error
}

type systemVerifierClock struct{}

func (systemVerifierClock) Now() time.Time {
	return time.Now()
}

func (systemVerifierClock) Wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Verifier owns the positive-evidence completion state machine.
type Verifier struct {
	observe      StateObserver
	clock        verifierClock
	pollInterval time.Duration
}

// NewVerifier constructs a verifier with production time and the requested
// polling cadence.
func NewVerifier(observe StateObserver, pollInterval time.Duration) *Verifier {
	return newVerifierWithClock(observe, systemVerifierClock{}, pollInterval)
}

func newVerifierWithClock(observe StateObserver, clock verifierClock, pollInterval time.Duration) *Verifier {
	return &Verifier{observe: observe, clock: clock, pollInterval: pollInterval}
}

type verificationRun struct {
	observer           StateObserver
	clock              verifierClock
	pollInterval       time.Duration
	target             VerificationTarget
	startedAt          time.Time
	deadline           time.Time
	transitionObserved bool
	readyStreak        int
	last               session.DetectionResult
}

func newVerificationRun(verifier *Verifier, target VerificationTarget, timeout time.Duration) (*verificationRun, error) {
	if verifier == nil || verifier.observe == nil || verifier.clock == nil {
		return nil, &UnverifiedError{Reason: UnverifiedObservationFailed, Cause: errors.New("verifier is not configured")}
	}
	if err := target.validate(); err != nil {
		return nil, &UnverifiedError{Reason: UnverifiedObservationFailed, Cause: err}
	}
	if timeout <= 0 {
		return nil, &UnverifiedError{Reason: UnverifiedTimeout}
	}
	pollInterval := verifier.pollInterval
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	startedAt := verifier.clock.Now()
	return &verificationRun{
		observer:           verifier.observe,
		clock:              verifier.clock,
		pollInterval:       pollInterval,
		target:             target,
		startedAt:          startedAt,
		deadline:           startedAt.Add(timeout),
		transitionObserved: target.InitialProcessingObserved,
	}, nil
}

// Verify requires a live WORKING or COMPACTING transition after delivery and
// then two consecutive live READY observations. Initial readiness, ambiguous
// state, observer failure, disappearance, blocked state, and timeout cannot
// prove completion.
func (v *Verifier) Verify(ctx context.Context, target VerificationTarget, timeout time.Duration) (Verification, error) {
	if err := ctx.Err(); err != nil {
		return Verification{}, err
	}
	run, err := newVerificationRun(v, target, timeout)
	if err != nil {
		return Verification{}, err
	}

	for {
		observation, observedAt, err := run.observe(ctx)
		if err != nil {
			return Verification{}, err
		}
		verification, complete, err := run.evaluate(ctx, observation, observedAt)
		if err != nil || complete {
			return verification, err
		}
		if err := run.wait(ctx); err != nil {
			return Verification{}, err
		}
	}
}

func (run *verificationRun) observe(ctx context.Context) (*session.DetectionResult, time.Time, error) {
	if err := ctx.Err(); err != nil {
		return nil, time.Time{}, err
	}
	if !run.clock.Now().Before(run.deadline) {
		return nil, time.Time{}, run.unverified(UnverifiedTimeout, nil)
	}
	remaining := run.deadline.Sub(run.clock.Now())
	if remaining <= 0 {
		return nil, time.Time{}, run.unverified(UnverifiedTimeout, nil)
	}
	observeCtx, cancelObserve := context.WithTimeout(ctx, remaining)
	observation, err := run.observer(observeCtx, run.target)
	observeCtxErr := observeCtx.Err()
	cancelObserve()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, time.Time{}, ctxErr
	}
	observedAt := run.clock.Now()
	if !observedAt.Before(run.deadline) || errors.Is(observeCtxErr, context.DeadlineExceeded) {
		if observation != nil {
			run.last = *observation
		}
		return nil, time.Time{}, run.unverified(UnverifiedTimeout, nil)
	}
	if err != nil {
		return nil, time.Time{}, run.unverified(UnverifiedObservationFailed, err)
	}
	if observation == nil {
		return nil, time.Time{}, run.unverified(UnverifiedObservationFailed, errors.New("observer returned no result"))
	}
	run.last = *observation
	return observation, observedAt, nil
}

func (run *verificationRun) evaluate(
	ctx context.Context,
	observation *session.DetectionResult,
	observedAt time.Time,
) (Verification, bool, error) {
	if observation.Evidence == session.EvidenceAbsent || observation.State == manifest.StateOffline {
		return Verification{}, false, run.unverified(UnverifiedSessionLost, nil)
	}
	if observation.Evidence != session.EvidenceLive {
		return Verification{}, false, run.unverified(UnverifiedEvidenceLost, nil)
	}

	switch observation.State {
	case manifest.StateWorking, manifest.StateCompacting:
		if run.readyStreak > 0 {
			return Verification{}, false, run.unverified(UnverifiedCausalityLost, nil)
		}
		run.transitionObserved = true
	case manifest.StateReady:
		if !run.transitionObserved {
			return Verification{}, false, run.unverified(UnverifiedCausalityLost, nil)
		}
		run.readyStreak++
		if run.readyStreak == 2 {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Verification{}, false, ctxErr
			}
			return Verification{
				Proof:   ProofBusyThenStableReady,
				Elapsed: observedAt.Sub(run.startedAt),
			}, true, nil
		}
	case manifest.StateUserPrompt, manifest.StatePermissionPrompt,
		manifest.StateWaitingAgent, manifest.StateLooping,
		manifest.StateBackgroundTasks:
		return Verification{}, false, run.unverified(UnverifiedBlocked, nil)
	default:
		return Verification{}, false, run.unverified(UnverifiedEvidenceLost, nil)
	}
	return Verification{}, false, nil
}

func (run *verificationRun) wait(ctx context.Context) error {
	remaining := run.deadline.Sub(run.clock.Now())
	if remaining <= 0 {
		return run.unverified(UnverifiedTimeout, nil)
	}
	if err := run.clock.Wait(ctx, min(run.pollInterval, remaining)); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return run.unverified(UnverifiedObservationFailed, err)
	}
	return nil
}

func (run *verificationRun) unverified(reason UnverifiedReason, cause error) error {
	return &UnverifiedError{Reason: reason, Last: run.last, Cause: cause}
}
