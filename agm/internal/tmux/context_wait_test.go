package tmux

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCommandScopedReadinessWaitsReturnCallerCancellation(t *testing.T) {
	t.Parallel()

	tests := map[string]func(context.Context) error{
		"process": func(ctx context.Context) error {
			return WaitForProcessReadyContext(ctx, "missing", "missing", time.Minute)
		},
		"claude prompt": func(ctx context.Context) error {
			return WaitForClaudePromptContext(ctx, "missing", time.Minute)
		},
		"resume prompt": func(ctx context.Context) error {
			return WaitForPromptOrResumeFailureContext(ctx, "missing", time.Minute)
		},
		"codex prompt": func(ctx context.Context) error {
			return WaitForCodexPromptContext(ctx, "missing", time.Minute)
		},
		"generic prompt": func(ctx context.Context) error {
			return WaitForPromptSimpleContext(ctx, "missing", time.Minute)
		},
	}

	for name, wait := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			started := time.Now()
			if err := wait(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context cancellation", err)
			}
			if elapsed := time.Since(started); elapsed > time.Second {
				t.Fatalf("canceled wait returned after %v, want under 1s", elapsed)
			}
		})
	}
}
