package helpers

import (
	"context"
	"fmt"
	"time"

	"github.com/Netflix/go-expect"
)

// ExpectHelper wraps go-expect console for easier testing.
type ExpectHelper struct {
	console *expect.Console
}

// NewExpectConsole creates a console for interactive command testing.
//
// This is useful for testing interactive CLI commands that require
// user input or produce output over time. It replaces brittle time.Sleep
// calls with explicit expectation matching.
//
// Example usage:
//
//	console, err := NewExpectConsole(cmd)
//	require.NoError(t, err)
//	defer console.Close()
//
//	// Wait for specific output instead of time.Sleep
//	err = console.ExpectString("Ready", 5*time.Second)
//	require.NoError(t, err)
func NewExpectConsole(cmd interface{}) (*ExpectHelper, error) {
	// Note: This is a placeholder for the full implementation
	// go-expect needs to be added to go.mod first:
	//   go get github.com/Netflix/go-expect
	//
	// Then this can be used to wrap commands with pseudo-terminal support
	return nil, fmt.Errorf("go-expect not yet integrated - use polling helpers instead")
}

// ExpectString waits for expected string to appear in output.
func (h *ExpectHelper) ExpectString(expected string, timeout time.Duration) error {
	if h.console == nil {
		return fmt.Errorf("console not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, err := h.console.Expect(expect.String(expected), expect.WithTimeout(timeout))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout waiting for %q after %v", expected, timeout)
		}
		return fmt.Errorf("failed to find %q: %w", expected, err)
	}

	return nil
}

// Close closes the console and cleans up resources.
func (h *ExpectHelper) Close() error {
	if h.console != nil {
		return h.console.Close()
	}
	return nil
}

// NOTE: For most test cases, use the Poll* functions in polling.go instead.
// go-expect is primarily useful for:
//   1. Interactive CLI programs that require stdin/stdout interaction
//   2. Programs that use terminal control sequences
//   3. Testing prompts and user input flows
//
// For waiting on file system changes, process completion, or API calls,
// use Poll/PollUntil which are simpler and more reliable.
