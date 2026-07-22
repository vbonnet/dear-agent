package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
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

func TestSendViaSharedOperationsUsesCallerContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := dolt.NewMockAdapter()
	if err := storage.CreateSession(&manifest.Manifest{
		SessionID: "agy-send-id",
		Name:      "agy-send",
		Harness:   "agy",
		Tmux:      manifest.Tmux{SessionName: "agy-send-tmux"},
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	tmuxClient := session.NewMockTmux()
	tmuxClient.InputReadiness = session.InputReadiness{Ready: true, State: "YES", PaneID: "%7"}

	type callerContextKey struct{}
	callerCtx := context.WithValue(t.Context(), callerContextKey{}, "direct-send")

	err := sendViaSharedOperations(callerCtx, "agy-send", "sender", "message-id", "header\nmessage body", "", false, storage, tmuxClient)
	if err != nil {
		t.Fatalf("sendViaSharedOperations() error = %v", err)
	}
	if tmuxClient.InputContext != callerCtx || tmuxClient.PaneSendContext != callerCtx {
		t.Fatal("atomic readiness and delivery did not receive the caller context")
	}
	if got, want := tmuxClient.AtomicInputChecks, []string{"agy-send-tmux:agy"}; !slices.Equal(got, want) {
		t.Fatalf("atomic input checks = %v, want %v", got, want)
	}
	if got, want := tmuxClient.ExactPaneDeliveries, []string{"%7"}; !slices.Equal(got, want) {
		t.Fatalf("exact-pane deliveries = %v, want %v", got, want)
	}
	if got, want := tmuxClient.SentCommands, []string{"header\nmessage body"}; !slices.Equal(got, want) {
		t.Fatalf("sent commands = %v, want %v", got, want)
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
		if sessionName != "recipient" || message != "header\npayload" || !shouldInterrupt || harness != "agy" {
			t.Fatalf("structured delivery = %q/%q/%t/%q", sessionName, message, shouldInterrupt, harness)
		}
		cancel()
		return ctx.Err()
	}

	err := sendStructuredPrompt(callerCtx, "recipient", "header\npayload", true)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("sendStructuredPrompt() error = %v, want context.Canceled", err)
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

func TestDeliverInitialPromptUsesAtomicExactPaneReadiness(t *testing.T) {
	originalPrompt, originalPromptFile, originalHarness := prompt, promptFile, harnessName
	prompt, promptFile, harnessName = "startup prompt", "", "codex-cli"
	t.Cleanup(func() { prompt, promptFile, harnessName = originalPrompt, originalPromptFile, originalHarness })
	tmuxClient := session.NewMockTmux()
	tmuxClient.InputReadiness = session.InputReadiness{Ready: true, State: "YES", PaneID: "%9"}

	callerCtx := context.WithValue(t.Context(), struct{ name string }{"startup"}, "caller")
	if err := deliverInitialPromptWithSender(callerCtx, "codex-create", false, false, tmuxClient); err != nil {
		t.Fatalf("deliverInitialPromptWithSender() error = %v", err)
	}
	if tmuxClient.InputContext != callerCtx || tmuxClient.PaneSendContext != callerCtx {
		t.Fatal("startup prompt atomic delivery did not preserve the caller context")
	}
	if got, want := tmuxClient.AtomicInputChecks, []string{"codex-create:codex-cli"}; !slices.Equal(got, want) {
		t.Fatalf("startup prompt readiness checks = %v, want %v", got, want)
	}
	if got, want := tmuxClient.ExactPaneDeliveries, []string{"%9"}; !slices.Equal(got, want) {
		t.Fatalf("startup prompt exact-pane deliveries = %v, want %v", got, want)
	}
	if got, want := tmuxClient.SentCommands, []string{"startup prompt"}; !slices.Equal(got, want) {
		t.Fatalf("startup prompt commands = %v, want %v", got, want)
	}
}

func TestDeliverInitialPromptFileUsesAtomicExactPaneReadiness(t *testing.T) {
	originalPrompt, originalPromptFile, originalHarness := prompt, promptFile, harnessName
	prompt = ""
	promptFile = filepath.Join(t.TempDir(), "startup.md")
	harnessName = "claude-code"
	t.Cleanup(func() { prompt, promptFile, harnessName = originalPrompt, originalPromptFile, originalHarness })
	if err := os.WriteFile(promptFile, []byte("first line\nsecond line"), 0o600); err != nil {
		t.Fatalf("write startup prompt file: %v", err)
	}
	tmuxClient := session.NewMockTmux()
	tmuxClient.InputReadiness = session.InputReadiness{Ready: true, State: "YES", PaneID: "%10"}

	if err := deliverInitialPromptWithSender(t.Context(), "claude-create", true, false, tmuxClient); err != nil {
		t.Fatalf("deliverInitialPromptWithSender(file) error = %v", err)
	}
	if got, want := tmuxClient.ExactPaneDeliveries, []string{"%10"}; !slices.Equal(got, want) {
		t.Fatalf("startup prompt file exact-pane deliveries = %v, want %v", got, want)
	}
	if got, want := tmuxClient.SentCommands, []string{"first line\nsecond line"}; !slices.Equal(got, want) {
		t.Fatalf("startup prompt file commands = %q, want %q", got, want)
	}
}

func TestDeliverInitialPromptFailsClosedWhenHarnessDoesNotOwnTerminal(t *testing.T) {
	tmuxClient := session.NewMockTmux()
	tmuxClient.InputReadiness = session.InputReadiness{State: "WRONG_HARNESS", PaneID: "%4"}

	err := sendInitialPromptAtomically(t.Context(), tmuxClient, "claude-create", "claude-code", "do not inject")
	if err == nil || !strings.Contains(err.Error(), "WRONG_HARNESS") {
		t.Fatalf("sendInitialPromptAtomically() error = %v, want WRONG_HARNESS", err)
	}
	if len(tmuxClient.SentCommands) != 0 || len(tmuxClient.ExactPaneDeliveries) != 0 {
		t.Fatalf("unready startup prompt was delivered: commands=%v panes=%v", tmuxClient.SentCommands, tmuxClient.ExactPaneDeliveries)
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
