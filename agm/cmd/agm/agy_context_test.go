package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitForResumedAgyUsesCallerContext(t *testing.T) {
	type callerContextKey struct{}
	callerCtx, cancel := context.WithCancel(context.WithValue(t.Context(), callerContextKey{}, "resume"))
	cancel()

	err := waitForResumedAgyWithWait(callerCtx, &HealthStatus{TmuxSessionName: "agy-resume"}, func(ctx context.Context, sessionName string, timeout time.Duration) error {
		if ctx != callerCtx {
			t.Fatalf("resume wait context identity changed")
		}
		if sessionName != "agy-resume" || timeout != 60*time.Second {
			t.Fatalf("resume wait = %q/%s, want agy-resume/60s", sessionName, timeout)
		}
		return ctx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForResumedAgyWithWait error = %v, want context.Canceled", err)
	}
}

func TestWaitForAgyMetadataBackfillUsesCallerContext(t *testing.T) {
	type callerContextKey struct{}
	callerCtx, cancel := context.WithCancel(context.WithValue(t.Context(), callerContextKey{}, "send"))
	cancel()

	err := waitForAgyMetadataBackfill(callerCtx, "agy-send", func(ctx context.Context, sessionName string, timeout time.Duration) error {
		if ctx != callerCtx {
			t.Fatalf("metadata wait context identity changed")
		}
		if sessionName != "agy-send" || timeout != 60*time.Second {
			t.Fatalf("metadata wait = %q/%s, want agy-send/60s", sessionName, timeout)
		}
		return ctx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForAgyMetadataBackfill error = %v, want context.Canceled", err)
	}
}

func TestRunAgyPostCreateReturnsCancellationBeforeSideEffects(t *testing.T) {
	t.Setenv("AGM_TEST_RUN_ID", "")
	t.Setenv("AGM_TEST_ENV", "")
	callerCtx, cancel := context.WithCancel(t.Context())
	cancel()

	var associated, delivered, retried bool
	err := runAgyPostCreateWithRuntime(callerCtx, "agy-create", agyPostCreateRuntime{
		wait: func(ctx context.Context, sessionName string, timeout time.Duration) error {
			if ctx != callerCtx {
				t.Fatalf("post-create wait context identity changed")
			}
			if sessionName != "agy-create" || timeout != 30*time.Second {
				t.Fatalf("post-create wait = %q/%s, want agy-create/30s", sessionName, timeout)
			}
			return ctx.Err()
		},
		associate:          func(string) { associated = true },
		deliver:            func(string, bool, bool) { delivered = true },
		associateWithRetry: func(string, int, time.Duration) { retried = true },
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runAgyPostCreateWithRuntime error = %v, want context.Canceled", err)
	}
	if associated || delivered || retried {
		t.Fatalf("post-cancellation side effects: associated=%t delivered=%t retried=%t", associated, delivered, retried)
	}
}
