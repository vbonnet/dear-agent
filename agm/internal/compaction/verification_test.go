package compaction

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

type fakeVerifierClock struct {
	now time.Time
}

func (clock *fakeVerifierClock) Now() time.Time {
	return clock.now
}

func (clock *fakeVerifierClock) Wait(ctx context.Context, duration time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	clock.now = clock.now.Add(duration)
	return nil
}

type observationStep struct {
	result session.DetectionResult
	err    error
}

type scriptedStateObserver struct {
	steps []observationStep
	calls int
}

func (observer *scriptedStateObserver) observe(ctx context.Context, _ VerificationTarget) (*session.DetectionResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if observer.calls >= len(observer.steps) {
		return nil, fmt.Errorf("unexpected observation %d", observer.calls+1)
	}
	step := observer.steps[observer.calls]
	observer.calls++
	if step.err != nil {
		return nil, step.err
	}
	result := step.result
	return &result, nil
}

var testVerificationTarget = VerificationTarget{
	SessionName:      "target",
	Harness:          "codex-cli",
	PaneID:           "%7",
	PanePID:          77,
	TargetPID:        701,
	HarnessStartTime: "Thu Aug 27 07:00:00 2026",
	TargetSessionID:  "$7",
	StableSessionID:  "stable-target",
}

func observed(state string, evidence session.ObservationEvidence) observationStep {
	return observationStep{result: session.DetectionResult{State: state, Evidence: evidence}}
}

func TestVerifierRequiresActiveThenStableReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		steps []observationStep
	}{
		{
			name: "working transition",
			steps: []observationStep{
				observed(manifest.StateWorking, session.EvidenceLive),
				observed(manifest.StateReady, session.EvidenceLive),
				observed(manifest.StateReady, session.EvidenceLive),
			},
		},
		{
			name: "compacting transition",
			steps: []observationStep{
				observed(manifest.StateCompacting, session.EvidenceLive),
				observed(manifest.StateReady, session.EvidenceLive),
				observed(manifest.StateReady, session.EvidenceLive),
			},
		},
		{
			name: "repeated active observations before readiness",
			steps: []observationStep{
				observed(manifest.StateWorking, session.EvidenceLive),
				observed(manifest.StateWorking, session.EvidenceLive),
				observed(manifest.StateReady, session.EvidenceLive),
				observed(manifest.StateReady, session.EvidenceLive),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			clock := &fakeVerifierClock{now: time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)}
			observer := &scriptedStateObserver{steps: test.steps}
			verifier := newVerifierWithClock(observer.observe, clock, time.Second)

			result, err := verifier.Verify(t.Context(), testVerificationTarget, time.Minute)
			if err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
			if result.Proof != ProofBusyThenStableReady {
				t.Fatalf("Verify() proof = %q, want %q", result.Proof, ProofBusyThenStableReady)
			}
			if observer.calls != len(test.steps) {
				t.Fatalf("Verify() observations = %d, want %d", observer.calls, len(test.steps))
			}
		})
	}
}

func TestVerifierAcceptsExactPostSubmitProcessingSeed(t *testing.T) {
	t.Parallel()

	target := testVerificationTarget
	target.InitialProcessingObserved = true
	observer := &scriptedStateObserver{steps: []observationStep{
		observed(manifest.StateReady, session.EvidenceLive),
		observed(manifest.StateReady, session.EvidenceLive),
	}}
	verifier := newVerifierWithClock(
		observer.observe,
		&fakeVerifierClock{now: time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)},
		time.Second,
	)

	result, err := verifier.Verify(t.Context(), target, time.Minute)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if result.Proof != ProofBusyThenStableReady || observer.calls != 2 {
		t.Fatalf("Verify() = %#v with %d observations, want seeded stable-ready proof", result, observer.calls)
	}
}

func TestVerifierFailsClosedWithoutPositiveCompletionEvidence(t *testing.T) {
	t.Parallel()

	observerFailure := errors.New("detector failed")
	tests := []struct {
		name       string
		steps      []observationStep
		timeout    time.Duration
		wantReason UnverifiedReason
		wantCause  error
	}{
		{
			name: "initial ready loses causal attribution",
			steps: []observationStep{
				observed(manifest.StateReady, session.EvidenceLive),
			},
			timeout:    time.Minute,
			wantReason: UnverifiedCausalityLost,
		},
		{
			name: "unreadable evidence",
			steps: []observationStep{
				observed(manifest.StateDone, session.EvidenceUnreadable),
			},
			timeout:    time.Minute,
			wantReason: UnverifiedEvidenceLost,
		},
		{
			name: "terminal evidence",
			steps: []observationStep{
				observed(manifest.StateDone, session.EvidenceTerminal),
			},
			timeout:    time.Minute,
			wantReason: UnverifiedEvidenceLost,
		},
		{
			name: "unknown after partial ready",
			steps: []observationStep{
				observed(manifest.StateWorking, session.EvidenceLive),
				observed(manifest.StateReady, session.EvidenceLive),
				observed(manifest.StateReady, session.EvidenceUnknown),
			},
			timeout:    time.Minute,
			wantReason: UnverifiedEvidenceLost,
		},
		{
			name: "session disappears",
			steps: []observationStep{
				observed(manifest.StateWorking, session.EvidenceLive),
				observed(manifest.StateOffline, session.EvidenceAbsent),
			},
			timeout:    time.Minute,
			wantReason: UnverifiedSessionLost,
		},
		{
			name: "blocked on user prompt",
			steps: []observationStep{
				observed(manifest.StateWorking, session.EvidenceLive),
				observed(manifest.StateUserPrompt, session.EvidenceLive),
			},
			timeout:    time.Minute,
			wantReason: UnverifiedBlocked,
		},
		{
			name: "observer failure",
			steps: []observationStep{
				{err: observerFailure},
			},
			timeout:    time.Minute,
			wantReason: UnverifiedObservationFailed,
			wantCause:  observerFailure,
		},
		{
			name:       "owned zero timeout",
			timeout:    0,
			wantReason: UnverifiedTimeout,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			clock := &fakeVerifierClock{now: time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)}
			observer := &scriptedStateObserver{steps: test.steps}
			verifier := newVerifierWithClock(observer.observe, clock, time.Second)

			_, err := verifier.Verify(t.Context(), testVerificationTarget, test.timeout)
			if err == nil {
				t.Fatal("Verify() error = nil, want unverified completion")
			}
			if !errors.Is(err, ErrCompletionUnverified) {
				t.Fatalf("Verify() error = %v, want ErrCompletionUnverified", err)
			}
			var unverified *UnverifiedError
			if !errors.As(err, &unverified) {
				t.Fatalf("Verify() error type = %T, want *UnverifiedError", err)
			}
			if unverified.Reason != test.wantReason {
				t.Fatalf("Verify() reason = %q, want %q", unverified.Reason, test.wantReason)
			}
			if test.wantCause != nil && !errors.Is(err, test.wantCause) {
				t.Fatalf("Verify() error = %v, want preserved cause %v", err, test.wantCause)
			}
		})
	}
}

func TestVerifierRejectsAmbiguousPostDeliveryCycles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		steps     []observationStep
		wantCalls int
	}{
		{
			name: "ready before transition cannot wait for unrelated work",
			steps: []observationStep{
				observed(manifest.StateReady, session.EvidenceLive),
				observed(manifest.StateWorking, session.EvidenceLive),
				observed(manifest.StateReady, session.EvidenceLive),
				observed(manifest.StateReady, session.EvidenceLive),
			},
			wantCalls: 1,
		},
		{
			name: "new work after partial ready breaks attribution",
			steps: []observationStep{
				observed(manifest.StateWorking, session.EvidenceLive),
				observed(manifest.StateReady, session.EvidenceLive),
				observed(manifest.StateWorking, session.EvidenceLive),
				observed(manifest.StateReady, session.EvidenceLive),
				observed(manifest.StateReady, session.EvidenceLive),
			},
			wantCalls: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			observer := &scriptedStateObserver{steps: test.steps}
			verifier := newVerifierWithClock(
				observer.observe,
				&fakeVerifierClock{now: time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)},
				time.Second,
			)

			_, err := verifier.Verify(t.Context(), testVerificationTarget, time.Minute)
			var unverified *UnverifiedError
			if !errors.As(err, &unverified) || unverified.Reason != UnverifiedCausalityLost {
				t.Fatalf("Verify() error = %v, want causality-lost UnverifiedError", err)
			}
			if observer.calls != test.wantCalls {
				t.Fatalf("Verify() observations = %d, want %d", observer.calls, test.wantCalls)
			}
		})
	}
}

func TestVerifierReturnsCallerCancellationUnchanged(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	observer := &scriptedStateObserver{steps: []observationStep{
		observed(manifest.StateWorking, session.EvidenceLive),
	}}
	verifier := newVerifierWithClock(observer.observe, &fakeVerifierClock{now: time.Now()}, time.Second)

	_, err := verifier.Verify(ctx, testVerificationTarget, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Verify() error = %v, want context.Canceled", err)
	}
	if observer.calls != 0 {
		t.Fatalf("Verify() observations = %d, want 0 after pre-cancellation", observer.calls)
	}
}

func TestVerifierRejectsProofObservedAfterOwnedDeadline(t *testing.T) {
	t.Parallel()

	clock := &fakeVerifierClock{now: time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)}
	steps := []session.DetectionResult{
		{State: manifest.StateWorking, Evidence: session.EvidenceLive},
		{State: manifest.StateReady, Evidence: session.EvidenceLive},
		{State: manifest.StateReady, Evidence: session.EvidenceLive},
	}
	calls := 0
	observer := StateObserver(func(context.Context, VerificationTarget) (*session.DetectionResult, error) {
		result := steps[calls]
		calls++
		if calls == len(steps) {
			clock.now = clock.now.Add(time.Minute)
		}
		return &result, nil
	})
	verifier := newVerifierWithClock(observer, clock, time.Second)

	_, err := verifier.Verify(t.Context(), testVerificationTarget, time.Minute)
	var unverified *UnverifiedError
	if !errors.As(err, &unverified) || unverified.Reason != UnverifiedTimeout {
		t.Fatalf("Verify() error = %v, want timeout UnverifiedError", err)
	}
}

type successEdgeClock struct {
	*fakeVerifierClock
	deadline          time.Time
	finalObserved     bool
	postObservedReads int
}

func (clock *successEdgeClock) Now() time.Time {
	if !clock.finalObserved {
		return clock.fakeVerifierClock.Now()
	}
	clock.postObservedReads++
	if clock.postObservedReads <= 2 {
		return clock.deadline.Add(-time.Nanosecond)
	}
	return clock.deadline
}

func TestVerifierUsesOnePostObservationTimestampForDeadlineAndProof(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	timeout := time.Minute
	clock := &successEdgeClock{
		fakeVerifierClock: &fakeVerifierClock{now: startedAt},
		deadline:          startedAt.Add(timeout),
	}
	steps := []session.DetectionResult{
		{State: manifest.StateWorking, Evidence: session.EvidenceLive},
		{State: manifest.StateReady, Evidence: session.EvidenceLive},
		{State: manifest.StateReady, Evidence: session.EvidenceLive},
	}
	calls := 0
	observer := StateObserver(func(context.Context, VerificationTarget) (*session.DetectionResult, error) {
		result := steps[calls]
		calls++
		if calls == len(steps) {
			clock.finalObserved = true
		}
		return &result, nil
	})
	verifier := newVerifierWithClock(observer, clock, time.Second)

	proof, err := verifier.Verify(t.Context(), testVerificationTarget, timeout)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if proof.Elapsed >= timeout {
		t.Fatalf("Verify() elapsed = %s, want proof timestamp before %s deadline", proof.Elapsed, timeout)
	}
	if clock.postObservedReads != 1 {
		t.Fatalf("post-observation clock reads = %d, want exactly 1", clock.postObservedReads)
	}
}

func TestVerifierReturnsCancellationRaisedDuringFinalObservation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	steps := []session.DetectionResult{
		{State: manifest.StateWorking, Evidence: session.EvidenceLive},
		{State: manifest.StateReady, Evidence: session.EvidenceLive},
		{State: manifest.StateReady, Evidence: session.EvidenceLive},
	}
	calls := 0
	observer := StateObserver(func(context.Context, VerificationTarget) (*session.DetectionResult, error) {
		result := steps[calls]
		calls++
		if calls == len(steps) {
			cancel()
		}
		return &result, nil
	})
	verifier := newVerifierWithClock(observer, &fakeVerifierClock{now: time.Now()}, time.Second)

	_, err := verifier.Verify(ctx, testVerificationTarget, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Verify() error = %v, want context.Canceled", err)
	}
}

func TestVerifierOwnedDeadlineBoundsObservation(t *testing.T) {
	t.Parallel()

	observer := StateObserver(func(ctx context.Context, _ VerificationTarget) (*session.DetectionResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	verifier := NewVerifier(observer, time.Second)

	_, err := verifier.Verify(t.Context(), testVerificationTarget, 20*time.Millisecond)
	var unverified *UnverifiedError
	if !errors.As(err, &unverified) || unverified.Reason != UnverifiedTimeout {
		t.Fatalf("Verify() error = %v, want timeout UnverifiedError", err)
	}
}

func TestVerifierClassifiesObserverErrorAfterOwnedDeadlineAsTimeout(t *testing.T) {
	t.Parallel()

	clock := &fakeVerifierClock{now: time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)}
	observerFailure := errors.New("late detector failure")
	observer := StateObserver(func(context.Context, VerificationTarget) (*session.DetectionResult, error) {
		clock.now = clock.now.Add(time.Minute)
		return nil, observerFailure
	})
	verifier := newVerifierWithClock(observer, clock, time.Second)

	_, err := verifier.Verify(t.Context(), testVerificationTarget, time.Minute)
	var unverified *UnverifiedError
	if !errors.As(err, &unverified) || unverified.Reason != UnverifiedTimeout {
		t.Fatalf("Verify() error = %v, want timeout UnverifiedError", err)
	}
}
