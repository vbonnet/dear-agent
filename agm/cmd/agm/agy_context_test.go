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

func TestSendViaTmuxUsesCallerContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	original := sendMultiLinePromptSafeContext
	t.Cleanup(func() { sendMultiLinePromptSafeContext = original })

	type callerContextKey struct{}
	callerCtx, cancel := context.WithCancel(context.WithValue(t.Context(), callerContextKey{}, "direct-send"))
	sendMultiLinePromptSafeContext = func(ctx context.Context, sessionName, message string, shouldInterrupt bool) error {
		if ctx != callerCtx {
			t.Fatal("tmux delivery did not receive the caller context")
		}
		if sessionName != "agy-send" || message != "message" || shouldInterrupt {
			t.Fatalf("tmux delivery = %q/%q/%t", sessionName, message, shouldInterrupt)
		}
		cancel()
		return ctx.Err()
	}

	err := sendViaTmux(callerCtx, "agy-send", "sender", "message-id", "message", "", false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("sendViaTmux error = %v, want context.Canceled", err)
	}
}

func TestWaitForAgyAssociationRetryDelayUsesCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	timer := time.AfterFunc(10*time.Millisecond, cancel)
	t.Cleanup(func() { timer.Stop() })
	started := time.Now()
	err := waitForAgyAssociationRetryDelay(ctx, time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("retry delay error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("retry delay returned after %s, want prompt cancellation", elapsed)
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
		associateWithRetry: func(context.Context, string, int, time.Duration) error { retried = true; return nil },
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runAgyPostCreateWithRuntime error = %v, want context.Canceled", err)
	}
	if associated || delivered || retried {
		t.Fatalf("post-cancellation side effects: associated=%t delivered=%t retried=%t", associated, delivered, retried)
	}
}

func TestRunAgyPostCreateMetadataRetryUsesCallerContext(t *testing.T) {
	t.Setenv("AGM_TEST_RUN_ID", "")
	t.Setenv("AGM_TEST_ENV", "")
	originalPrompt, originalPromptFile := prompt, promptFile
	prompt, promptFile = "startup prompt", ""
	t.Cleanup(func() { prompt, promptFile = originalPrompt, originalPromptFile })

	type callerContextKey struct{}
	callerCtx := context.WithValue(t.Context(), callerContextKey{}, "post-create-retry")
	err := runAgyPostCreateWithRuntime(callerCtx, "agy-create", agyPostCreateRuntime{
		wait:      func(context.Context, string, time.Duration) error { return nil },
		associate: func(string) {},
		deliver:   func(string, bool, bool) {},
		associateWithRetry: func(ctx context.Context, sessionName string, attempts int, delay time.Duration) error {
			if ctx != callerCtx {
				t.Fatal("metadata retry did not receive the caller context")
			}
			if sessionName != "agy-create" || attempts != 20 || delay != 500*time.Millisecond {
				t.Fatalf("metadata retry = %q/%d/%s", sessionName, attempts, delay)
			}
			return context.Canceled
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runAgyPostCreateWithRuntime error = %v, want context.Canceled", err)
	}
}
