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
			Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  csm list         # Verify recovery",
			Duration: timeout,
		}
	}

	return output, err
}

// global timeout configuration
var globalTimeout = 5 * time.Second

// SetTimeout sets the global timeout for all tmux commands
func SetTimeout(timeout time.Duration) {
	globalTimeout = timeout
}

// GetTimeout returns the current global timeout
func GetTimeout() time.Duration {
	return globalTimeout
}
