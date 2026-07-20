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
