package main

import (
	"context"
	"errors"
	"testing"
)

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
