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

func TestCommandScopedSafeDeliveryReturnsCallerCancellation(t *testing.T) {
	tests := map[string]func(context.Context) error{
		"prompt file": func(ctx context.Context) error {
			return SendPromptFileSafeContext(ctx, "missing-canceled-session", "missing-prompt-file", false)
		},
		"slash command": func(ctx context.Context) error {
			return SendSlashCommandSafeContext(ctx, "missing-canceled-session", "/model test")
		},
		"pane capture": func(ctx context.Context) error {
			_, err := CapturePaneOutputContext(ctx, "missing-canceled-session", 10)
			return err
		},
		"delivery verification": func(ctx context.Context) error {
			_, err := VerifyPromptDeliveryContext(ctx, "missing-canceled-session", "do not send", func() error {
				t.Fatal("canceled verification must not retry delivery")
				return nil
			}, 3)
			return err
		},
	}
	for name, deliver := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			started := time.Now()
			if err := deliver(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
			if elapsed := time.Since(started); elapsed > time.Second {
				t.Fatalf("canceled delivery took %v", elapsed)
			}
		})
	}
}
