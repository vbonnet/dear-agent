package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/send"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
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

func TestWaitForResumedAgyPropagatesOnboardingRequired(t *testing.T) {
	err := waitForResumedAgyWithWait(t.Context(), &HealthStatus{TmuxSessionName: "agy-resume"}, func(context.Context, string, time.Duration) error {
		return fmt.Errorf("onboarding: %w", tmux.ErrAgyOnboardingRequired)
	})
	if !errors.Is(err, tmux.ErrAgyOnboardingRequired) {
		t.Fatalf("waitForResumedAgyWithWait error = %v, want ErrAgyOnboardingRequired", err)
	}
}

func TestWaitForResumedAgyToleratesSlowStartup(t *testing.T) {
	err := waitForResumedAgyWithWait(t.Context(), &HealthStatus{TmuxSessionName: "agy-resume"}, func(context.Context, string, time.Duration) error {
		return errors.New("readiness timeout")
	})
	if err != nil {
		t.Fatalf("waitForResumedAgyWithWait error = %v, want slow startup to remain non-fatal", err)
	}
}

func TestResumeSessionStopsCancellationAfterManifestRead(t *testing.T) {
	for _, tmuxExists := range []bool{false, true} {
		t.Run(map[bool]string{false: "cold-resume", true: "warm-resume"}[tmuxExists], func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			mutatedTmux := false

			err := resumeSessionWithRuntime(ctx, nil, "agy-session", "", "agy", &HealthStatus{
				TmuxExists:      tmuxExists,
				TmuxSessionName: "agy-resume",
			}, resumeSessionRuntime{
				loadManifest: func(context.Context, *dolt.Adapter, string, string) (*manifest.Manifest, error) {
					cancel()
					return &manifest.Manifest{}, nil
				},
				createTmux: func(string, string) (tmux.SessionIdentity, error) {
					mutatedTmux = true
					return tmux.SessionIdentity{}, nil
				},
				attach: func(string) error {
					mutatedTmux = true
					return nil
				},
			})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("resumeSessionWithRuntime() error = %v, want context.Canceled", err)
			}
			if mutatedTmux {
				t.Fatal("resume mutated tmux after the manifest read canceled its caller")
			}
		})
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
	original := sendMultiLinePromptSafeForHarnessContext
	t.Cleanup(func() { sendMultiLinePromptSafeForHarnessContext = original })

	type callerContextKey struct{}
	callerCtx, cancel := context.WithCancel(context.WithValue(t.Context(), callerContextKey{}, "direct-send"))
	sendMultiLinePromptSafeForHarnessContext = func(ctx context.Context, sessionName, message string, shouldInterrupt bool, harness string) error {
		if ctx != callerCtx {
			t.Fatal("tmux delivery did not receive the caller context")
		}
		if sessionName != "agy-send" || message != "header\nmessage body" || shouldInterrupt || harness != "agy" {
			t.Fatalf("tmux delivery = %q/%q/%t/%q", sessionName, message, shouldInterrupt, harness)
		}
		cancel()
		return ctx.Err()
	}

	err := sendViaTmux(callerCtx, "agy-send", "sender", "message-id", "header\nmessage body", "", false, "agy")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("sendViaTmux error = %v, want context.Canceled", err)
	}
}

func TestStructuredPromptUsesCallerContext(t *testing.T) {
	originalSend := sendMultiLinePromptSafeForHarnessContext
	originalResolve := resolveSendRecipientHarness
	t.Cleanup(func() {
		sendMultiLinePromptSafeForHarnessContext = originalSend
		resolveSendRecipientHarness = originalResolve
	})
	type callerContextKey struct{}
	callerCtx, cancel := context.WithCancel(context.WithValue(t.Context(), callerContextKey{}, "structured-send"))
	resolveSendRecipientHarness = func(recipient string) string {
		if recipient != "recipient" {
			t.Fatalf("resolve recipient = %q, want recipient", recipient)
		}
		return "agy"
	}
	sendMultiLinePromptSafeForHarnessContext = func(ctx context.Context, sessionName, message string, shouldInterrupt bool, harness string) error {
		if ctx != callerCtx {
			t.Fatal("structured delivery did not receive the caller context")
		}
		if sessionName != "recipient" || message != "payload" || !shouldInterrupt || harness != "agy" {
			t.Fatalf("structured delivery = %q/%q/%t/%q", sessionName, message, shouldInterrupt, harness)
		}
		cancel()
		return ctx.Err()
	}

	err := sendStructuredPrompt(callerCtx, "recipient", "payload", true)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("sendStructuredPrompt() error = %v, want context.Canceled", err)
	}
}

func TestDeliveryFuncPreservesAgyHarness(t *testing.T) {
	originalHasSession := hasTmuxSessionForDelivery
	originalSend := sendDeliveryViaTmux
	t.Cleanup(func() {
		hasTmuxSessionForDelivery = originalHasSession
		sendDeliveryViaTmux = originalSend
	})
	hasTmuxSessionForDelivery = func(sessionName string) (bool, error) {
		return sessionName == "agy-fanout", nil
	}
	sendDeliveryViaTmux = func(ctx context.Context, recipient, sender, messageID, message, promptFile string, shouldInterrupt bool, harness string) error {
		if ctx != t.Context() || recipient != "agy-fanout" || sender != "codex" || messageID != "msg-1" || message != "header\nbody" || promptFile != "prompt.txt" || shouldInterrupt || harness != "agy" {
			t.Fatalf("fan-out delivery = context:%t %q/%q/%q/%q/%q/%t/%q", ctx == t.Context(), recipient, sender, messageID, message, promptFile, shouldInterrupt, harness)
		}
		return nil
	}
	err := deliveryFunc(t.Context(), &send.DeliveryJob{
		Recipient: "agy-fanout", Sender: "codex", MessageID: "msg-1",
		FormattedMessage: "header\nbody", PromptFile: "prompt.txt", Harness: "agy",
	})
	if err != nil {
		t.Fatalf("deliveryFunc() error = %v", err)
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

func TestRunClaudePostCreateReturnsCallerCancellationBeforeSideEffects(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := runClaudePostCreate(ctx, "claude-create", false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runClaudePostCreate() error = %v, want context.Canceled", err)
	}
}

func TestDeliverInitialPromptReturnsCallerCancellation(t *testing.T) {
	originalPrompt, originalPromptFile := prompt, promptFile
	prompt, promptFile = "startup prompt", ""
	t.Cleanup(func() { prompt, promptFile = originalPrompt, originalPromptFile })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := deliverInitialPrompt(ctx, "claude-create", true, true)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("deliverInitialPrompt() error = %v, want context.Canceled", err)
	}
}

func TestRunAgyPostCreateReturnsCancellationBeforeSideEffects(t *testing.T) {
	t.Setenv("AGM_TEST_RUN_ID", "")
	t.Setenv("AGM_TEST_ENV", "")
	callerCtx, cancel := context.WithCancel(t.Context())
	cancel()

	var associated, delivered, retried bool
	err := runAgyPostCreateWithRuntime(callerCtx, "agy-create", false, agyPostCreateRuntime{
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
		deliver:            func(context.Context, string, bool, bool) error { delivered = true; return nil },
		associateWithRetry: func(context.Context, string, int, time.Duration) error { retried = true; return nil },
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runAgyPostCreateWithRuntime error = %v, want context.Canceled", err)
	}
	if associated || delivered || retried {
		t.Fatalf("post-cancellation side effects: associated=%t delivered=%t retried=%t", associated, delivered, retried)
	}
}

func TestRunAgyPostCreatePropagatesReadinessFailure(t *testing.T) {
	t.Setenv("AGM_TEST_RUN_ID", "")
	t.Setenv("AGM_TEST_ENV", "")
	wantErr := errors.New("AGY composer unavailable")
	var associated, delivered bool

	err := runAgyPostCreateWithRuntime(t.Context(), "agy-create", false, agyPostCreateRuntime{
		wait:               func(context.Context, string, time.Duration) error { return wantErr },
		associate:          func(string) { associated = true },
		deliver:            func(context.Context, string, bool, bool) error { delivered = true; return nil },
		associateWithRetry: func(context.Context, string, int, time.Duration) error { return nil },
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("runAgyPostCreateWithRuntime() error = %v, want %v", err, wantErr)
	}
	if associated || delivered {
		t.Fatalf("readiness failure side effects: associated=%t delivered=%t", associated, delivered)
	}
}

func TestRunAgyPostCreatePropagatesPostPromptReadinessFailure(t *testing.T) {
	t.Setenv("AGM_TEST_RUN_ID", "")
	t.Setenv("AGM_TEST_ENV", "")
	originalPrompt, originalPromptFile := prompt, promptFile
	prompt, promptFile = "startup prompt", ""
	t.Cleanup(func() { prompt, promptFile = originalPrompt, originalPromptFile })
	wantErr := errors.New("AGY response readiness unavailable")
	startupWaits := 0
	postInputWaits := 0
	retried := false

	err := runAgyPostCreateWithRuntime(t.Context(), "agy-create", false, agyPostCreateRuntime{
		wait: func(context.Context, string, time.Duration) error {
			startupWaits++
			return nil
		},
		waitAfterInput: func(context.Context, string, time.Duration) error {
			postInputWaits++
			return wantErr
		},
		associate: func(string) {},
		deliver:   func(context.Context, string, bool, bool) error { return nil },
		associateWithRetry: func(context.Context, string, int, time.Duration) error {
			retried = true
			return nil
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("runAgyPostCreateWithRuntime() error = %v, want %v", err, wantErr)
	}
	if retried {
		t.Fatal("metadata retry ran after post-prompt readiness failure")
	}
	if startupWaits != 1 || postInputWaits != 1 {
		t.Fatalf("readiness waits = startup %d/post-input %d, want 1/1", startupWaits, postInputWaits)
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
	err := runAgyPostCreateWithRuntime(callerCtx, "agy-create", false, agyPostCreateRuntime{
		wait:      func(context.Context, string, time.Duration) error { return nil },
		associate: func(string) {},
		deliver:   func(context.Context, string, bool, bool) error { return nil },
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

func TestRunAgyPostCreateSkipsPromptAlreadyDeliveredForIdentityBootstrap(t *testing.T) {
	t.Setenv("AGM_TEST_RUN_ID", "")
	t.Setenv("AGM_TEST_ENV", "")
	sideEffects := 0
	err := runAgyPostCreateWithRuntime(t.Context(), "agy-create", true, agyPostCreateRuntime{
		wait: func(context.Context, string, time.Duration) error { sideEffects++; return nil },
		waitAfterInput: func(context.Context, string, time.Duration) error {
			sideEffects++
			return nil
		},
		associate: func(string) { sideEffects++ },
		deliver: func(context.Context, string, bool, bool) error {
			sideEffects++
			return nil
		},
		associateWithRetry: func(context.Context, string, int, time.Duration) error {
			sideEffects++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sideEffects != 0 {
		t.Fatalf("legacy AGY completion ran %d side effect(s) after pre-registration prompt delivery", sideEffects)
	}
}
