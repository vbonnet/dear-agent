package messages

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewAckManager tests creating an ack manager
func TestNewAckManager(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	ackMgr := NewAckManager(queue)
	require.NotNil(t, ackMgr)
	assert.NotNil(t, ackMgr.queue)
	assert.NotNil(t, ackMgr.pending)
	assert.Equal(t, 0, ackMgr.GetPendingCount())
}

// TestSendAck tests acknowledging a message
func TestSendAck(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	// Enqueue a message
	entry := &QueueEntry{
		MessageID: "test-ack-001",
		From:      "session-a",
		To:        "session-b",
		Message:   "Test message",
		Priority:  PriorityMedium,
		QueuedAt:  time.Now(),
	}
	err := queue.Enqueue(entry)
	require.NoError(t, err)

	// Mark as delivered
	err = queue.MarkDelivered("test-ack-001")
	require.NoError(t, err)

	// Create ack manager and send ack
	ackMgr := NewAckManager(queue)
	err = ackMgr.SendAck("test-ack-001")
	assert.NoError(t, err)

	// Verify ack was recorded in database
	retrieved, err := queue.GetByMessageID("test-ack-001")
	require.NoError(t, err)
	assert.True(t, retrieved.AckReceived)
}

// TestWaitForAckSuccess tests waiting for an acknowledgment that arrives
func TestWaitForAckSuccess(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	ackMgr := NewAckManager(queue)
	messageID := "test-wait-001"

	// Enqueue and mark delivered
	entry := &QueueEntry{
		MessageID: messageID,
		From:      "session-a",
		To:        "session-b",
		Message:   "Test message",
		Priority:  PriorityMedium,
		QueuedAt:  time.Now(),
	}
	err := queue.Enqueue(entry)
	require.NoError(t, err)
	err = queue.MarkDelivered(messageID)
	require.NoError(t, err)

	// Start waiting for ack in goroutine
	done := make(chan error)
	go func() {
		err := ackMgr.WaitForAck(messageID, 5*time.Second)
		done <- err
	}()

	// Send ack after short delay
	time.Sleep(100 * time.Millisecond)
	err = ackMgr.SendAck(messageID)
	require.NoError(t, err)

	// Wait for WaitForAck to complete
	err = <-done
	assert.NoError(t, err, "should receive ack successfully")

	// Verify pending count is 0
	assert.Equal(t, 0, ackMgr.GetPendingCount())
}

// TestWaitForAckTimeout tests waiting for an acknowledgment that times out
func TestWaitForAckTimeout(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	ackMgr := NewAckManager(queue)
	messageID := "test-timeout-001"

	// Enqueue and mark delivered
	entry := &QueueEntry{
		MessageID: messageID,
		From:      "session-a",
		To:        "session-b",
		Message:   "Test message",
		Priority:  PriorityMedium,
		QueuedAt:  time.Now(),
	}
	err := queue.Enqueue(entry)
	require.NoError(t, err)
	err = queue.MarkDelivered(messageID)
	require.NoError(t, err)

	// Wait for ack with short timeout (no ack sent)
	start := time.Now()
	err = ackMgr.WaitForAck(messageID, 200*time.Millisecond)
	elapsed := time.Since(start)

	// Should timeout
	assert.Error(t, err, "should timeout")
	assert.Contains(t, err.Error(), "timeout")

	// Should take approximately the timeout duration
	assert.True(t, elapsed >= 200*time.Millisecond, "should wait for timeout duration")
	assert.True(t, elapsed < 500*time.Millisecond, "should not wait too long")

	// Verify timeout was recorded in database
	retrieved, err := queue.GetByMessageID(messageID)
	require.NoError(t, err)
	assert.NotNil(t, retrieved.AckTimeout, "timeout should be recorded")
}

// TestCheckTimeout tests manual timeout checking
func TestCheckTimeout(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	ackMgr := NewAckManager(queue)

	// Create multiple pending acks with different timeouts
	for i := 1; i <= 3; i++ {
		messageID := fmt.Sprintf("test-check-timeout-%03d", i)

		// Enqueue and mark delivered
		entry := &QueueEntry{
			MessageID: messageID,
			From:      "session-a",
			To:        "session-b",
			Message:   "Test message",
			Priority:  PriorityMedium,
			QueuedAt:  time.Now(),
		}
		err := queue.Enqueue(entry)
		require.NoError(t, err)
		err = queue.MarkDelivered(messageID)
		require.NoError(t, err)

		// Create pending ack manually (simulate waiting)
		timeout := time.Duration(i*100) * time.Millisecond
		ack := &PendingAck{
			MessageID: messageID,
			SentAt:    time.Now().Add(-timeout - 10*time.Millisecond), // Already timed out
			Timeout:   timeout,
			Done:      make(chan struct{}),
		}
		ackMgr.pending[messageID] = ack
	}

	// Initial pending count
	assert.Equal(t, 3, ackMgr.GetPendingCount())

	// Check timeouts
	timedOut, err := ackMgr.CheckTimeout()
	require.NoError(t, err)
	assert.Equal(t, 3, timedOut, "all 3 should timeout")

	// Verify pending count is 0
	assert.Equal(t, 0, ackMgr.GetPendingCount())

	// Verify timeouts were recorded
	for i := 1; i <= 3; i++ {
		messageID := fmt.Sprintf("test-check-timeout-%03d", i)
		retrieved, err := queue.GetByMessageID(messageID)
		require.NoError(t, err)
		assert.NotNil(t, retrieved.AckTimeout, "timeout should be recorded for %s", messageID)
	}
}

// TestGetDLQ tests retrieving dead letter queue messages
func TestGetDLQ(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	ackMgr := NewAckManager(queue)

	// Add some failed messages
	for i := 1; i <= 3; i++ {
		entry := &QueueEntry{
			MessageID: fmt.Sprintf("test-dlq-%03d", i),
			From:      "session-a",
			To:        "session-b",
			Message:   "Failed message",
			Priority:  PriorityMedium,
			QueuedAt:  time.Now(),
		}
		err := queue.Enqueue(entry)
		require.NoError(t, err)

		// Mark as permanently failed
		err = queue.MarkPermanentlyFailed(entry.MessageID)
		require.NoError(t, err)
	}

	// Add a successful message (should not be in DLQ)
	successEntry := &QueueEntry{
		MessageID: "test-success-001",
		From:      "session-a",
		To:        "session-b",
		Message:   "Success message",
		Priority:  PriorityMedium,
		QueuedAt:  time.Now(),
	}
	err := queue.Enqueue(successEntry)
	require.NoError(t, err)
	err = queue.MarkDelivered(successEntry.MessageID)
	require.NoError(t, err)

	// Get DLQ
	dlq, err := ackMgr.GetDLQ()
	require.NoError(t, err)
	assert.Len(t, dlq, 3, "should have 3 failed messages")

	// Verify all are failed status
	for _, entry := range dlq {
		assert.Equal(t, StatusFailed, entry.Status)
	}
}

// TestCancelAck tests cancelling an acknowledgment wait
func TestCancelAck(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	ackMgr := NewAckManager(queue)
	messageID := "test-cancel-001"

	// Enqueue and mark delivered
	entry := &QueueEntry{
		MessageID: messageID,
		From:      "session-a",
		To:        "session-b",
		Message:   "Test message",
		Priority:  PriorityMedium,
		QueuedAt:  time.Now(),
	}
	err := queue.Enqueue(entry)
	require.NoError(t, err)
	err = queue.MarkDelivered(messageID)
	require.NoError(t, err)

	// Start waiting for ack
	done := make(chan error)
	go func() {
		err := ackMgr.WaitForAck(messageID, 5*time.Second)
		done <- err
	}()

	// Cancel after short delay
	time.Sleep(100 * time.Millisecond)
	ackMgr.CancelAck(messageID, fmt.Errorf("cancelled by user"))

	// Wait should complete with error
	err = <-done
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")

	// Verify pending count is 0
	assert.Equal(t, 0, ackMgr.GetPendingCount())
}

// TestGetPendingAcks tests retrieving list of pending acks
func TestGetPendingAcks(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	ackMgr := NewAckManager(queue)

	// Add multiple pending acks
	messageIDs := []string{"msg-001", "msg-002", "msg-003"}
	for _, msgID := range messageIDs {
		// Enqueue and mark delivered
		entry := &QueueEntry{
			MessageID: msgID,
			From:      "session-a",
			To:        "session-b",
			Message:   "Test message",
			Priority:  PriorityMedium,
			QueuedAt:  time.Now(),
		}
		err := queue.Enqueue(entry)
		require.NoError(t, err)
		err = queue.MarkDelivered(msgID)
		require.NoError(t, err)

		// Start waiting (in goroutine to avoid blocking)
		go ackMgr.WaitForAck(msgID, 10*time.Second)
	}

	// Wait a bit for goroutines to register
	time.Sleep(50 * time.Millisecond)

	// Get pending acks
	pending := ackMgr.GetPendingAcks()
	assert.Len(t, pending, 3)

	// Verify all message IDs are present
	for _, msgID := range messageIDs {
		assert.Contains(t, pending, msgID)
	}
}

// TestCleanup tests the cleanup operation
func TestCleanup(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	ackMgr := NewAckManager(queue)

	// Create some pending acks that will timeout
	for i := 1; i <= 2; i++ {
		messageID := fmt.Sprintf("test-cleanup-%03d", i)

		// Enqueue and mark delivered
		entry := &QueueEntry{
			MessageID: messageID,
			From:      "session-a",
			To:        "session-b",
			Message:   "Test message",
			Priority:  PriorityMedium,
			QueuedAt:  time.Now(),
		}
		err := queue.Enqueue(entry)
		require.NoError(t, err)
		err = queue.MarkDelivered(messageID)
		require.NoError(t, err)

		// Create pending ack that already timed out
		ack := &PendingAck{
			MessageID: messageID,
			SentAt:    time.Now().Add(-2 * time.Minute), // 2 minutes ago
			Timeout:   60 * time.Second,                 // 60 second timeout
			Done:      make(chan struct{}),
		}
		ackMgr.pending[messageID] = ack
	}

	// Run cleanup
	err := ackMgr.Cleanup()
	assert.NoError(t, err)

	// Verify pending acks were cleaned up
	assert.Equal(t, 0, ackMgr.GetPendingCount())
}

// TestGetUnacknowledged tests retrieving unacknowledged messages
func TestGetUnacknowledged(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	// Add delivered but unacknowledged messages
	for i := 1; i <= 3; i++ {
		entry := &QueueEntry{
			MessageID: fmt.Sprintf("test-unacked-%03d", i),
			From:      "session-a",
			To:        "session-b",
			Message:   "Unacked message",
			Priority:  PriorityMedium,
			QueuedAt:  time.Now(),
		}
		err := queue.Enqueue(entry)
		require.NoError(t, err)

		// Mark as delivered but not acknowledged
		err = queue.MarkDelivered(entry.MessageID)
		require.NoError(t, err)
	}

	// Add an acknowledged message (should not appear)
	ackedEntry := &QueueEntry{
		MessageID: "test-acked-001",
		From:      "session-a",
		To:        "session-b",
		Message:   "Acked message",
		Priority:  PriorityMedium,
		QueuedAt:  time.Now(),
	}
	err := queue.Enqueue(ackedEntry)
	require.NoError(t, err)
	err = queue.MarkDelivered(ackedEntry.MessageID)
	require.NoError(t, err)
	err = queue.MarkAcknowledged(ackedEntry.MessageID)
	require.NoError(t, err)

	// Get unacknowledged messages
	unacked, err := queue.GetUnacknowledged()
	require.NoError(t, err)
	assert.Len(t, unacked, 3, "should have 3 unacknowledged messages")

	// Verify all require ack but haven't received it
	for _, entry := range unacked {
		assert.True(t, entry.AckRequired)
		assert.False(t, entry.AckReceived)
		assert.Equal(t, StatusDelivered, entry.Status)
	}
}

// TestAckWithRedelivery tests acknowledgment with message redelivery
func TestAckWithRedelivery(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	ackMgr := NewAckManager(queue)
	messageID := "test-redeliver-001"

	// Enqueue message
	entry := &QueueEntry{
		MessageID: messageID,
		From:      "session-a",
		To:        "session-b",
		Message:   "Test message",
		Priority:  PriorityMedium,
		QueuedAt:  time.Now(),
	}
	err := queue.Enqueue(entry)
	require.NoError(t, err)

	// Mark as delivered
	err = queue.MarkDelivered(messageID)
	require.NoError(t, err)

	// Simulate failed delivery (increment attempt)
	err = queue.IncrementAttempt(messageID)
	require.NoError(t, err)

	// Wait for ack with timeout (simulate timeout)
	err = ackMgr.WaitForAck(messageID, 100*time.Millisecond)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")

	// Verify attempt count was incremented due to timeout handling
	retrieved, err := queue.GetByMessageID(messageID)
	require.NoError(t, err)
	assert.Greater(t, retrieved.AttemptCount, 0, "attempt count should be incremented")
	assert.NotNil(t, retrieved.AckTimeout, "timeout should be recorded")
}

// TestConcurrentAcks tests concurrent acknowledgment operations
func TestConcurrentAcks(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	ackMgr := NewAckManager(queue)

	// Create multiple messages
	numMessages := 10
	for i := 1; i <= numMessages; i++ {
		messageID := fmt.Sprintf("test-concurrent-%03d", i)

		entry := &QueueEntry{
			MessageID: messageID,
			From:      "session-a",
			To:        "session-b",
			Message:   "Test message",
			Priority:  PriorityMedium,
			QueuedAt:  time.Now(),
		}
		err := queue.Enqueue(entry)
		require.NoError(t, err)
		err = queue.MarkDelivered(messageID)
		require.NoError(t, err)
	}

	// Start waiting for all acks concurrently
	done := make(chan error, numMessages)
	for i := 1; i <= numMessages; i++ {
		messageID := fmt.Sprintf("test-concurrent-%03d", i)
		go func(msgID string) {
			err := ackMgr.WaitForAck(msgID, 2*time.Second)
			done <- err
		}(messageID)
	}

	// Wait for all to register
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, numMessages, ackMgr.GetPendingCount())

	// Send acks concurrently
	for i := 1; i <= numMessages; i++ {
		messageID := fmt.Sprintf("test-concurrent-%03d", i)
		go func(msgID string) {
			time.Sleep(50 * time.Millisecond) // Small delay
			ackMgr.SendAck(msgID)
		}(messageID)
	}

	// Collect all results
	errors := 0
	for i := 0; i < numMessages; i++ {
		err := <-done
		if err != nil {
			errors++
		}
	}

	// All should succeed
	assert.Equal(t, 0, errors, "all acks should succeed")
	assert.Equal(t, 0, ackMgr.GetPendingCount())
}
