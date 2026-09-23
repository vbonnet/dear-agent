package safegit

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// A canceled caller must not turn an accepted merge into a reported failure.
//
// Cancellation is one of the ways a merge becomes indeterminate: the request
// can be accepted and the caller canceled before the response lands. While the
// probe was skipped whenever ctx was already canceled, confirm() ran with that
// same canceled context, returned on its first read of the normally transient
// OPEN state, and safe-merge reported failure, skipped cleanup, and let watch
// mode retry a merge that had already happened.
func TestCanceledCallerStillProbesIndeterminateMerge(t *testing.T) {
	// Cancellation has to arrive WHILE the provider command is running, which
	// is the race the finding describes. A context already canceled on entry
	// is refused before any mutation starts, which is a different and correct
	// behavior.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	probed := false
	confirmCalls := 0
	failure := runProviderMergeTransaction(
		ctx,
		"",
		[]string{"sleep", "5"}, // killed by the cancellation, exits nonzero
		func() error { confirmCalls++; return errors.New("confirm must not be reached") },
		nil,
		func() error { probed = true; return nil },
	)

	if ctx.Err() == nil {
		t.Fatal("fixture did not cancel while the provider command was running")
	}

	if !probed {
		t.Error("the indeterminate probe was skipped because the caller was canceled")
	}
	if failure != nil {
		t.Errorf("failure = %+v, want the probe's confirmation to stand", failure)
	}
	if confirmCalls != 0 {
		t.Errorf("confirm called %d times, want 0: a successful probe is itself the confirmation", confirmCalls)
	}
}

// An exact-head mismatch found by the rescue probe is terminal and must stay
// terminal. Only an error still carrying errMergeHeadChanged stops watch mode,
// so discarding it in favour of the command error downgrades a safety
// violation into a retryable provider failure.
func TestRescueProbeHeadMismatchIsPreserved(t *testing.T) {
	mismatch := fmt.Errorf("%w: expected abc, got def", errMergeHeadChanged)

	failure := runProviderMergeTransaction(
		context.Background(),
		"",
		[]string{"false"},
		func() error { return errors.New("confirm must not be reached") },
		nil,
		func() error { return mismatch },
	)

	if failure == nil {
		t.Fatal("a head mismatch must fail the transaction")
	}
	if !errors.Is(failure.err, errMergeHeadChanged) {
		t.Errorf("failure.err = %v, want it to keep the errMergeHeadChanged sentinel", failure.err)
	}
	if failure.stage != providerMergeConfirmationStage {
		t.Errorf("failure.stage = %v, want the confirmation stage", failure.stage)
	}
}

// An ordinary probe failure still reports the provider command's own error,
// which is the one that describes what the operator asked for.
func TestUnconfirmedProbeReportsTheCommandError(t *testing.T) {
	failure := runProviderMergeTransaction(
		context.Background(),
		"",
		[]string{"false"},
		func() error { return errors.New("confirm must not be reached") },
		nil,
		func() error { return errMergePending },
	)
	if failure == nil || failure.stage != providerMergeCommandStage {
		t.Fatalf("failure = %+v, want a command-stage failure", failure)
	}
}

// A credential that will not start working must fail immediately rather than
// be retried until the window expires.
func TestWaitForMergeCompletionStopsOnAuthDenial(t *testing.T) {
	for _, denial := range []string{
		"confirming merge completion: exit status 1\nstderr: HTTP 401: Bad credentials",
		"confirming merge completion: exit status 1\nstderr: HTTP 403: Resource not accessible by integration",
		"confirming merge completion: gh auth login required",
	} {
		calls := 0
		start := time.Now()
		err := waitForMergeCompletion(context.Background(), time.Minute, 10*time.Millisecond, func() error {
			calls++
			return errors.New(denial)
		})
		if !errors.Is(err, errProviderAuthDenied) {
			t.Errorf("waitForMergeCompletion(%q) = %v, want an access denial", denial, err)
		}
		if calls != 1 {
			t.Errorf("check called %d times for %q, want 1", calls, denial)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Errorf("polled for %s before reporting a denial", elapsed)
		}
	}
}

// A transient failure is still retried, so the denial classifier did not turn
// every confirmation error into a hard stop.
func TestWaitForMergeCompletionStillRetriesTransientFailures(t *testing.T) {
	calls := 0
	err := waitForMergeCompletion(context.Background(), 5*time.Second, time.Millisecond, func() error {
		calls++
		if calls < 3 {
			return fmt.Errorf("%w: accepted but PR remains open", errMergePending)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("waitForMergeCompletion() = %v, want success after retries", err)
	}
	if calls != 3 {
		t.Errorf("check called %d times, want 3", calls)
	}
}
