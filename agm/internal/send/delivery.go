// Package send provides send functionality.
package send

import (
	"context"
	"fmt"
	"time"
)

// DeliveryJob represents a message delivery task
type DeliveryJob struct {
	Recipient        string
	Sender           string
	MessageID        string
	FormattedMessage string
	PromptFile       string
	ShouldInterrupt  bool
	SessionsDir      string
}

// DeliveryResult represents delivery outcome
type DeliveryResult struct {
	Recipient string
	Success   bool
	Error     error
	Duration  time.Duration
	MessageID string
	Method    string // "tmux" or "api"
}

// SequentialDeliver delivers messages to multiple recipients one at a time.
//
// Bug fix (2026-04-03): Replaced ParallelDeliver. The tmux server lock is a
// process-global singleton — concurrent goroutines calling withTmuxLock caused
// "double lock" errors (4/7 deliveries failed). Sequential delivery is correct
// because the lock serializes tmux writes anyway, so parallelism provided no
// actual throughput benefit.
func SequentialDeliver(ctx context.Context, jobs []*DeliveryJob, deliverFunc DeliveryFunc) []*DeliveryResult {
	if len(jobs) == 0 {
		return nil
	}

	// Use default delivery function if none provided
	if deliverFunc == nil {
		deliverFunc = DefaultDeliveryFunc
	}

	var results []*DeliveryResult

	for _, job := range jobs {
		// Check context cancellation before each delivery
		select {
		case <-ctx.Done():
			results = append(results, &DeliveryResult{
				Recipient: job.Recipient,
				Success:   false,
				Error:     ctx.Err(),
				Duration:  0,
				MessageID: job.MessageID,
				Method:    "cancelled",
			})
			continue
		default:
		}

		// Execute delivery
		start := time.Now()
		err := deliverFunc(job)
		duration := time.Since(start)

		results = append(results, &DeliveryResult{
			Recipient: job.Recipient,
			Success:   err == nil,
			Error:     err,
			Duration:  duration,
			MessageID: job.MessageID,
			Method:    "tmux",
		})
	}

	return results
}

// DeliveryFunc is a function type for delivering a message
// Allows dependency injection for testing
type DeliveryFunc func(job *DeliveryJob) error

// DefaultDeliveryFunc is the default delivery implementation
// Uses tmux for message delivery (matches existing sendViaTmux behavior)
var DefaultDeliveryFunc DeliveryFunc = func(job *DeliveryJob) error {
	// Import cycle prevention: this will be set by cmd/agm/send_msg.go
	// during initialization to avoid circular dependency
	return fmt.Errorf("delivery function not initialized")
}

// SetDefaultDeliveryFunc sets the default delivery function
// Called by cmd/agm/send_msg.go to inject the actual delivery implementation
func SetDefaultDeliveryFunc(fn DeliveryFunc) {
	DefaultDeliveryFunc = fn
}
