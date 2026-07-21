package tmux

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSendMultiLinePromptSafeContextReturnsCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()

	err := SendMultiLinePromptSafeContext(ctx, "missing-canceled-session", "do not send", false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SendMultiLinePromptSafeContext() error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("canceled multiline delivery took %v", elapsed)
	}
}
