package tmux

import (
	"context"
	"time"
)

// sleepWithContext waits for the polling interval or returns as soon as the
// caller cancels. Readiness loops use it so command-scoped SIGINT/SIGTERM
// cancellation cannot be hidden behind an otherwise harmless time.Sleep.
func sleepWithContext(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
