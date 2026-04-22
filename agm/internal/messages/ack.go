package messages

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// AckTimeout is the default timeout for waiting for acknowledgments
const AckTimeout = 60 * time.Second

// PendingAck tracks an acknowledgment that is being waited for
type PendingAck struct {
	MessageID string
	SentAt    time.Time
	Timeout   time.Duration
	Done      chan struct{} // Closed when ack is received or timeout occurs
	Result    error         // nil if acknowledged, timeout error otherwise
}

// AckManager manages message acknowledgments and timeouts
type AckManager struct {
	mu      sync.RWMutex
	pending map[string]*PendingAck
	queue   *MessageQueue
}

// NewAckManager creates a new acknowledgment manager
func NewAckManager(queue *MessageQueue) *AckManager {
	return &AckManager{
		pending: make(map[string]*PendingAck),
		queue:   queue,
	}
}

// WaitForAck blocks until the message is acknowledged or timeout occurs.
// Returns nil if acknowledged successfully, error if timeout or other failure.
func (a *AckManager) WaitForAck(messageID string, timeout time.Duration) error {
	// Register pending ack
	ack := &PendingAck{
		MessageID: messageID,
		SentAt:    time.Now(),
		Timeout:   timeout,
		Done:      make(chan struct{}),
	}

	a.mu.Lock()
	a.pending[messageID] = ack
	a.mu.Unlock()

	// Setup timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case <-ack.Done:
		// Ack received or error occurred
		a.mu.Lock()
		delete(a.pending, messageID)
		a.mu.Unlock()
		return ack.Result

	case <-ctx.Done():
		// Timeout occurred
		a.mu.Lock()
		delete(a.pending, messageID)
		a.mu.Unlock()

		// Mark as timeout in queue
		if err := a.handleTimeout(messageID); err != nil {
			return fmt.Errorf("timeout and failed to update queue: %w", err)
		}

		return fmt.Errorf("acknowledgment timeout after %v", timeout)
	}
}

// SendAck acknowledges delivery of a message
func (a *AckManager) SendAck(messageID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Mark as acknowledged in queue
	if a.queue != nil {
		if err := a.queue.MarkAcknowledged(messageID); err != nil {
			return fmt.Errorf("failed to mark message as acknowledged: %w", err)
		}
	}

	// Notify any waiting goroutine
	if ack, exists := a.pending[messageID]; exists {
		ack.Result = nil
		close(ack.Done)
	}

	return nil
}

// CheckTimeout scans all pending acks and handles timeouts
// Returns the number of messages that timed out
func (a *AckManager) CheckTimeout() (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	timedOut := 0

	for messageID, ack := range a.pending {
		if now.Sub(ack.SentAt) > ack.Timeout {
			// Handle timeout
			if err := a.handleTimeout(messageID); err != nil {
				// Log error but continue checking other messages
				continue
			}

			// Notify waiting goroutine
			ack.Result = fmt.Errorf("acknowledgment timeout")
			close(ack.Done)

			delete(a.pending, messageID)
			timedOut++
		}
	}

	return timedOut, nil
}

// handleTimeout processes a timed-out message
func (a *AckManager) handleTimeout(messageID string) error {
	if a.queue == nil {
		return nil
	}

	// Mark timeout in database
	if err := a.queue.MarkTimeout(messageID); err != nil {
		return fmt.Errorf("failed to mark timeout: %w", err)
	}

	// Check if message should be retried or moved to DLQ
	entry, err := a.queue.GetByMessageID(messageID)
	if err != nil {
		return fmt.Errorf("failed to get message: %w", err)
	}

	// If this was a redelivery attempt, increment attempt count
	if entry.AttemptCount > 0 {
		if err := a.queue.IncrementAttempt(messageID); err != nil {
			return fmt.Errorf("failed to increment attempt: %w", err)
		}
	}

	return nil
}

// GetDLQ retrieves all messages in the dead letter queue
// Messages end up in DLQ after max retries or permanent failures
func (a *AckManager) GetDLQ() ([]*QueueEntry, error) {
	if a.queue == nil {
		return nil, fmt.Errorf("queue not configured")
	}

	return a.queue.GetDLQ()
}

// GetPendingCount returns the number of messages waiting for acknowledgment
func (a *AckManager) GetPendingCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.pending)
}

// CancelAck cancels waiting for an acknowledgment with the given error
func (a *AckManager) CancelAck(messageID string, reason error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if ack, exists := a.pending[messageID]; exists {
		ack.Result = reason
		close(ack.Done)
		delete(a.pending, messageID)
	}
}

// GetPendingAcks returns a list of all message IDs waiting for acknowledgment
func (a *AckManager) GetPendingAcks() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	messageIDs := make([]string, 0, len(a.pending))
	for messageID := range a.pending {
		messageIDs = append(messageIDs, messageID)
	}
	return messageIDs
}

// Cleanup removes any stale pending acks and performs periodic maintenance
func (a *AckManager) Cleanup() error {
	// Check for timeouts
	timedOut, err := a.CheckTimeout()
	if err != nil {
		return fmt.Errorf("failed to check timeouts: %w", err)
	}

	if timedOut > 0 {
		// Log or return count for monitoring
		_ = timedOut
	}

	return nil
}
