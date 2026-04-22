// Package helpers provides helpers functionality.
package helpers

import (
	"context"
	"fmt"
	"time"
)

// PollCondition is a function that returns true when a condition is met.
type PollCondition func() (bool, error)

// PollConfig configures polling behavior.
type PollConfig struct {
	Timeout  time.Duration // Maximum time to wait
	Interval time.Duration // Time between checks
}

// DefaultPollConfig returns sensible defaults for polling.
func DefaultPollConfig() PollConfig {
	return PollConfig{
		Timeout:  5 * time.Second,
		Interval: 100 * time.Millisecond,
	}
}

// Poll waits for a condition to become true.
//
// Repeatedly calls condition() at interval until it returns true or timeout expires.
// This is a more robust alternative to time.Sleep for waiting on asynchronous operations.
//
// Parameters:
//   - ctx: Context for cancellation (use context.Background() if not needed)
//   - config: Polling configuration (timeout and interval)
//   - condition: Function that returns (true, nil) when ready
//
// Returns:
//   - error if timeout expires or condition returns error
//
// Example replacing time.Sleep:
//
//	// OLD:
//	time.Sleep(500 * time.Millisecond)
//	// assume session is ready
//
//	// NEW:
//	err := Poll(context.Background(), DefaultPollConfig(), func() (bool, error) {
//	    exists, err := HasTmuxSession(sessionName)
//	    return exists, err
//	})
func Poll(ctx context.Context, config PollConfig, condition PollCondition) error {
	deadline := time.Now().Add(config.Timeout)
	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()

	// Check immediately
	if ready, err := condition(); err != nil {
		return fmt.Errorf("condition check failed: %w", err)
	} else if ready {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("polling cancelled: %w", ctx.Err())
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("polling timeout after %v", config.Timeout)
			}

			ready, err := condition()
			if err != nil {
				return fmt.Errorf("condition check failed: %w", err)
			}
			if ready {
				return nil
			}
		}
	}
}

// PollUntil is a simpler version that polls until condition is true.
//
// Uses default timeout of 5 seconds and interval of 100ms.
//
// Example:
//
//	err := PollUntil(func() (bool, error) {
//	    _, err := os.Stat(filePath)
//	    return err == nil, nil
//	})
func PollUntil(condition PollCondition) error {
	return Poll(context.Background(), DefaultPollConfig(), condition)
}

// PollUntilWithTimeout polls with custom timeout.
//
// Example:
//
//	err := PollUntilWithTimeout(2*time.Second, func() (bool, error) {
//	    return processCompleted(), nil
//	})
func PollUntilWithTimeout(timeout time.Duration, condition PollCondition) error {
	config := PollConfig{
		Timeout:  timeout,
		Interval: 100 * time.Millisecond,
	}
	return Poll(context.Background(), config, condition)
}
