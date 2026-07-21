package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitForResumedClaudeUsesCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	started := time.Now()
	if err := waitForResumedClaude(ctx, &HealthStatus{TmuxSessionName: "missing"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("canceled Claude resume wait returned after %v, want under 1s", elapsed)
	}
}

func TestWaitForResumedCodexUsesCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	started := time.Now()
	if err := waitForResumedCodex(ctx, &HealthStatus{TmuxSessionName: "missing"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("canceled Codex resume wait returned after %v, want under 1s", elapsed)
	}
}

func TestRunCodexPostCreateReturnsCancellationBeforePromptDelivery(t *testing.T) {
	t.Setenv("AGM_TEST_RUN_ID", "")
	t.Setenv("AGM_TEST_ENV", "")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	previousDetached, previousPrompt, previousPromptFile := detached, prompt, promptFile
	detached, prompt, promptFile = false, "must-not-be-delivered", ""
	t.Cleanup(func() {
		detached, prompt, promptFile = previousDetached, previousPrompt, previousPromptFile
	})

	if err := runCodexPostCreate(ctx, "missing"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
}
