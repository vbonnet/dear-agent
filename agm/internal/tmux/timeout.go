package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// TimeoutError represents a command timeout error with recovery guidance
type TimeoutError struct {
	Problem  string
	Recovery string
	Duration time.Duration
}

func (e *TimeoutError) Error() string {
	msg := fmt.Sprintf("Error: %s\n\n", e.Problem)
	if e.Recovery != "" {
		msg += fmt.Sprintf("Recovery:\n%s\n", e.Recovery)
	}
	return msg
}

// CommandWithTimeout creates a command with timeout context.
// The command will be automatically killed if it exceeds the timeout.
func CommandWithTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)

	// Create command with context (auto-kills on timeout)
	cmd := exec.CommandContext(timeoutCtx, name, args...)

	return cmd, cancel
}

// RunWithTimeout runs a command with timeout and returns output.
// Returns TimeoutError if command exceeds timeout duration.
func RunWithTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create command with context
	cmd := exec.CommandContext(timeoutCtx, name, args...)

	// Run command
	output, err := cmd.CombinedOutput()

	// Check if timeout occurred
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return nil, &TimeoutError{
			Problem:  fmt.Sprintf("tmux command timed out after %v (server may be hung)", timeout),
			Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  agm session list         # Verify recovery",
			Duration: timeout,
		}
	}

	return output, err
}

// global timeout configuration
//
// Bug fix (2026-04-02): Increased from 5s to 10s. Under heavy load with 15+
// concurrent sessions, 5s was insufficient and caused cascading timeouts that
// left orphaned state (buffers, pipes) and contributed to tmux server crashes.
var globalTimeout = 10 * time.Second

// SetTimeout sets the global timeout for all tmux commands
func SetTimeout(timeout time.Duration) {
	globalTimeout = timeout
}

// GetTimeout returns the current global timeout
func GetTimeout() time.Duration {
	return globalTimeout
}

// getAdaptiveTimeout returns a timeout scaled by the current number of concurrent
// tmux operations. Under heavy load, tmux operations take longer because the server
// is handling more requests. Scaling the timeout prevents premature timeouts that
// leave orphaned state.
//
// Base timeout: globalTimeout (10s)
// Scale: +50% per 5 concurrent operations, up to 3x base
//
// Examples:
//   - 0-4 concurrent ops: 10s (1.0x)
//   - 5-9 concurrent ops: 15s (1.5x)
//   - 10-14 concurrent ops: 20s (2.0x)
//   - 15+ concurrent ops: 30s (3.0x cap)
func getAdaptiveTimeout() time.Duration {
	concurrent := TmuxConcurrentOps()
	multiplier := 1.0 + float64(concurrent/5)*0.5
	if multiplier > 3.0 {
		multiplier = 3.0
	}
	return time.Duration(float64(globalTimeout) * multiplier)
}
