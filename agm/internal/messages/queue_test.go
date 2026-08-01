package messages

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewMessageQueue tests queue creation
func TestNewMessageQueue(t *testing.T) {
	// Create temp config dir
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	queue, err := NewMessageQueue()
	require.NoError(t, err)
	require.NotNil(t, queue)
	defer queue.Close()

	// Verify database file created
	dbPath := filepath.Join(tmpDir, ".config", "agm", "message_queue.db")
	_, err = os.Stat(dbPath)
	assert.NoError(t, err, "database file should exist")
}

// TestEnqueue tests adding messages to queue
func TestEnqueue(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	entry := &QueueEntry{
		MessageID: "test-msg-001",
		From:      "session-a",
		To:        "session-b",
		Message:   "Test message",
		Priority:  PriorityHigh,
		QueuedAt:  time.Now(),
	}

	err := queue.Enqueue(entry)
	assert.NoError(t, err)

	// Verify entry was added
	pending, err := queue.GetPending("session-b")
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "test-msg-001", pending[0].MessageID)
	assert.Equal(t, "session-a", pending[0].From)
	assert.Equal(t, PriorityHigh, pending[0].Priority)
}

func TestEnqueueRejectsInvalidPriorityBeforeSQLite(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	entry := &QueueEntry{
		MessageID: "invalid-priority",
		From:      "session-a",
		To:        "session-b",
		Message:   "must not be persisted",
		Priority:  Priority("URGENT"),
		QueuedAt:  time.Now(),
	}

	err := queue.Enqueue(entry)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid message priority "URGENT"`)

	entries, listErr := queue.GetQueueList("", 10)
	require.NoError(t, listErr)
	assert.Empty(t, entries)
}

func TestEnqueueRejectsNilEntryBeforeSQLite(t *testing.T) {
	t.Parallel()

	var queue MessageQueue
	err := queue.Enqueue(nil)
	require.EqualError(t, err, "failed to enqueue message: entry is nil")
}

func TestQueueStateParser(t *testing.T) {
	for _, state := range []QueueState{
		QueueStateQueued,
		QueueStateDelivered,
		QueueStateFailed,
	} {
		parsed, err := ParseQueueState(string(state))
		require.NoError(t, err)
		assert.Equal(t, state, parsed)
	}

	_, err := ParseQueueState("waiting")
	assert.ErrorContains(t, err, "invalid queue state")
}

func TestLegacyStatusConstantsRemainUntyped(t *testing.T) {
	var state QueueState = StatusQueued
	counts := map[string]int{StatusQueued: 1}

	assert.Equal(t, QueueStateQueued, state)
	assert.Equal(t, 1, counts[StatusQueued])
}

func TestScannedQueueEntriesRejectMalformedDomains(t *testing.T) {
	tests := []struct {
		name     string
		priority string
		state    string
		read     func(*MessageQueue) ([]*QueueEntry, error)
		domain   string
	}{
		{
			name:     "priority in pending row",
			priority: "IMMEDIATE",
			state:    string(QueueStateQueued),
			read: func(q *MessageQueue) ([]*QueueEntry, error) {
				return q.GetPending("session-b")
			},
			domain: "priority domain",
		},
		{
			name:     "state in queue list row",
			priority: string(PriorityMedium),
			state:    "discarded",
			read: func(q *MessageQueue) ([]*QueueEntry, error) {
				return q.GetQueueList("", 10)
			},
			domain: "state domain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue := setupTestQueue(t)
			defer queue.Close()

			const messageID = "invalid-domain-entry"
			const body = "persisted body that must not appear in errors"
			_, err := queue.db.Exec(`
				INSERT INTO message_queue
					(message_id, from_session, to_session, message, priority, queued_at, status)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, messageID, "session-a", "session-b", body, tt.priority, time.Now(), tt.state)
			require.NoError(t, err)

			_, err = tt.read(queue)
			require.Error(t, err)
			assert.Contains(t, err.Error(), messageID)
			assert.Contains(t, err.Error(), tt.domain)
			assert.NotContains(t, err.Error(), body)
		})
	}
}

func TestPendingQueriesRejectUnknownState(t *testing.T) {
	tests := []struct {
		name string
		read func(*MessageQueue) ([]*QueueEntry, error)
	}{
		{
			name: "target session pending",
			read: func(q *MessageQueue) ([]*QueueEntry, error) {
				return q.GetPending("target-session")
			},
		},
		{
			name: "all pending",
			read: func(q *MessageQueue) ([]*QueueEntry, error) {
				return q.GetAllPending()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue := setupTestQueue(t)
			defer queue.Close()

			const messageID = "unknown-pending-state"
			const body = "persisted body that must not appear in errors"
			_, err := queue.db.Exec(`
				INSERT INTO message_queue
					(message_id, from_session, to_session, message, priority, queued_at, status)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, messageID, "session-a", "target-session", body, PriorityMedium, time.Now(), "waiting")
			require.NoError(t, err)

			_, err = tt.read(queue)
			require.Error(t, err)
			assert.Contains(t, err.Error(), messageID)
			assert.Contains(t, err.Error(), "state domain")
			assert.NotContains(t, err.Error(), body)
		})
	}
}

func TestGetPendingKeepsTargetAndKnownStateScope(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	for _, entry := range []struct {
		messageID string
		to        string
		state     QueueState
	}{
		{messageID: "delivered-target", to: "target-session", state: QueueStateDelivered},
		{messageID: "failed-target", to: "target-session", state: QueueStateFailed},
		{messageID: "queued-other", to: "other-session", state: QueueStateQueued},
	} {
		_, err := queue.db.Exec(`
			INSERT INTO message_queue
				(message_id, from_session, to_session, message, priority, queued_at, status)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, entry.messageID, "session-a", entry.to, "valid nonpending body", PriorityMedium, time.Now(), entry.state)
		require.NoError(t, err)
	}

	entries, err := queue.GetPending("target-session")
	require.NoError(t, err)
	assert.Empty(t, entries)

	entries, err = queue.GetAllPending()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "queued-other", entries[0].MessageID)
}

func TestLegacyQueueVocabularyRowsDecode(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	_, err := queue.db.Exec(`
		INSERT INTO message_queue
			(message_id, from_session, to_session, message, priority, queued_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "legacy-queue-entry", "session-a", "session-b", "legacy persisted body", "CRITICAL", time.Now(), "queued")
	require.NoError(t, err)

	entries, err := queue.GetPending("session-b")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, PriorityCritical, entries[0].Priority)
	assert.Equal(t, QueueStateQueued, entries[0].Status)
}

// TestEnqueueDuplicate tests duplicate message handling
func TestEnqueueDuplicate(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	entry := &QueueEntry{
		MessageID: "test-msg-001",
		From:      "session-a",
		To:        "session-b",
		Message:   "Test message",
		Priority:  PriorityMedium,
		QueuedAt:  time.Now(),
	}

	// First enqueue should succeed
	err := queue.Enqueue(entry)
	assert.NoError(t, err)

	// Second enqueue with same message_id should fail (UNIQUE constraint)
	err = queue.Enqueue(entry)
	assert.Error(t, err, "duplicate message_id should fail")
}

// TestGetPendingPriorityOrder tests priority-based ordering
func TestGetPendingPriorityOrder(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	baseTime := time.Now()

	// Add messages with different priorities (in random order)
	messages := []*QueueEntry{
		{MessageID: "msg-low", From: "a", To: "b", Message: "low", Priority: PriorityLow, QueuedAt: baseTime.Add(1 * time.Second)},
		{MessageID: "msg-critical", From: "a", To: "b", Message: "critical", Priority: PriorityCritical, QueuedAt: baseTime.Add(2 * time.Second)},
		{MessageID: "msg-medium", From: "a", To: "b", Message: "medium", Priority: PriorityMedium, QueuedAt: baseTime.Add(3 * time.Second)},
		{MessageID: "msg-high", From: "a", To: "b", Message: "high", Priority: PriorityHigh, QueuedAt: baseTime.Add(4 * time.Second)},
	}

	for _, msg := range messages {
		err := queue.Enqueue(msg)
		require.NoError(t, err)
	}

	// Get pending - should be ordered CRITICAL > HIGH > MEDIUM > LOW
	pending, err := queue.GetPending("b")
	require.NoError(t, err)
	require.Len(t, pending, 4)

	assert.Equal(t, "msg-critical", pending[0].MessageID)
	assert.Equal(t, "msg-high", pending[1].MessageID)
	assert.Equal(t, "msg-medium", pending[2].MessageID)
	assert.Equal(t, "msg-low", pending[3].MessageID)
}

// TestGetPendingTimeOrder tests time-based ordering within same priority
func TestGetPendingTimeOrder(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	baseTime := time.Now()

	// Add 3 HIGH priority messages at different times
	messages := []*QueueEntry{
		{MessageID: "msg-3", From: "a", To: "b", Message: "third", Priority: PriorityHigh, QueuedAt: baseTime.Add(3 * time.Second)},
		{MessageID: "msg-1", From: "a", To: "b", Message: "first", Priority: PriorityHigh, QueuedAt: baseTime.Add(1 * time.Second)},
		{MessageID: "msg-2", From: "a", To: "b", Message: "second", Priority: PriorityHigh, QueuedAt: baseTime.Add(2 * time.Second)},
	}

	for _, msg := range messages {
		err := queue.Enqueue(msg)
		require.NoError(t, err)
	}

	// Get pending - should be ordered by time (oldest first) within same priority
	pending, err := queue.GetPending("b")
	require.NoError(t, err)
	require.Len(t, pending, 3)

	assert.Equal(t, "msg-1", pending[0].MessageID)
	assert.Equal(t, "msg-2", pending[1].MessageID)
	assert.Equal(t, "msg-3", pending[2].MessageID)
}

// TestMarkDelivered tests marking message as delivered
func TestMarkDelivered(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	entry := &QueueEntry{
		MessageID: "test-msg-001",
		From:      "session-a",
		To:        "session-b",
		Message:   "Test message",
		Priority:  PriorityMedium,
		QueuedAt:  time.Now(),
	}

	err := queue.Enqueue(entry)
	require.NoError(t, err)

	// Mark as delivered
	err = queue.MarkDelivered("test-msg-001")
	assert.NoError(t, err)

	// Should no longer appear in pending
	pending, err := queue.GetPending("session-b")
	require.NoError(t, err)
	assert.Len(t, pending, 0)

	// Verify status in database
	stats, err := queue.GetStats()
	require.NoError(t, err)
	assert.Equal(t, 1, stats[StatusDelivered])
	assert.Equal(t, 0, stats[StatusQueued])
}

// TestMarkFailed tests marking message as failed (retry increment)
func TestMarkFailed(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	entry := &QueueEntry{
		MessageID: "test-msg-001",
		From:      "session-a",
		To:        "session-b",
		Message:   "Test message",
		Priority:  PriorityMedium,
		QueuedAt:  time.Now(),
	}

	err := queue.Enqueue(entry)
	require.NoError(t, err)

	// Mark as failed (increments attempt_count)
	err = queue.MarkFailed("test-msg-001")
	assert.NoError(t, err)

	// Should still appear in pending (status still 'queued')
	pending, err := queue.GetPending("session-b")
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, 1, pending[0].AttemptCount)
	assert.NotNil(t, pending[0].LastAttempt)

	// Mark failed again
	err = queue.MarkFailed("test-msg-001")
	assert.NoError(t, err)

	pending, err = queue.GetPending("session-b")
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, 2, pending[0].AttemptCount)
}

// TestMarkPermanentlyFailed tests marking message as permanently failed
func TestMarkPermanentlyFailed(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	entry := &QueueEntry{
		MessageID: "test-msg-001",
		From:      "session-a",
		To:        "session-b",
		Message:   "Test message",
		Priority:  PriorityMedium,
		QueuedAt:  time.Now(),
	}

	err := queue.Enqueue(entry)
	require.NoError(t, err)

	// Mark as permanently failed
	err = queue.MarkPermanentlyFailed("test-msg-001")
	assert.NoError(t, err)

	// Should NOT appear in pending
	pending, err := queue.GetPending("session-b")
	require.NoError(t, err)
	assert.Len(t, pending, 0)

	// Verify status
	stats, err := queue.GetStats()
	require.NoError(t, err)
	assert.Equal(t, 1, stats[StatusFailed])
}

// TestCleanupOld tests cleanup of old delivered/failed messages
func TestCleanupOld(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	// Add old delivered message (8 days ago)
	oldTime := time.Now().AddDate(0, 0, -8)
	oldEntry := &QueueEntry{
		MessageID: "old-msg-001",
		From:      "session-a",
		To:        "session-b",
		Message:   "Old message",
		Priority:  PriorityMedium,
		QueuedAt:  oldTime,
	}
	err := queue.Enqueue(oldEntry)
	require.NoError(t, err)
	err = queue.MarkDelivered("old-msg-001")
	require.NoError(t, err)

	// Add recent delivered message (2 days ago)
	recentTime := time.Now().AddDate(0, 0, -2)
	recentEntry := &QueueEntry{
		MessageID: "recent-msg-001",
		From:      "session-a",
		To:        "session-b",
		Message:   "Recent message",
		Priority:  PriorityMedium,
		QueuedAt:  recentTime,
	}
	err = queue.Enqueue(recentEntry)
	require.NoError(t, err)
	err = queue.MarkDelivered("recent-msg-001")
	require.NoError(t, err)

	// Cleanup messages older than 7 days
	deleted, err := queue.CleanupOld(7)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted, "should delete 1 old message")

	// Verify stats
	stats, err := queue.GetStats()
	require.NoError(t, err)
	assert.Equal(t, 1, stats[StatusDelivered], "should have 1 recent message remaining")
}

// TestGetStats tests queue statistics
func TestGetStats(t *testing.T) {
	queue := setupTestQueue(t)
	defer queue.Close()

	// Add messages with different statuses
	for i := 0; i < 3; i++ {
		entry := &QueueEntry{
			MessageID: fmt.Sprintf("queued-msg-%d", i),
			From:      "session-a",
			To:        "session-b",
			Message:   "Queued",
			Priority:  PriorityMedium,
			QueuedAt:  time.Now(),
		}
		err := queue.Enqueue(entry)
		require.NoError(t, err)
	}

	for i := 0; i < 2; i++ {
		entry := &QueueEntry{
			MessageID: fmt.Sprintf("delivered-msg-%d", i),
			From:      "session-a",
			To:        "session-b",
			Message:   "Delivered",
			Priority:  PriorityMedium,
			QueuedAt:  time.Now(),
		}
		err := queue.Enqueue(entry)
		require.NoError(t, err)
		err = queue.MarkDelivered(entry.MessageID)
		require.NoError(t, err)
	}

	entry := &QueueEntry{
		MessageID: "failed-msg-001",
		From:      "session-a",
		To:        "session-b",
		Message:   "Failed",
		Priority:  PriorityMedium,
		QueuedAt:  time.Now(),
	}
	err := queue.Enqueue(entry)
	require.NoError(t, err)
	err = queue.MarkPermanentlyFailed(entry.MessageID)
	require.NoError(t, err)

	// Check stats
	stats, err := queue.GetStats()
	require.NoError(t, err)
	assert.Equal(t, 3, stats[StatusQueued])
	assert.Equal(t, 2, stats[StatusDelivered])
	assert.Equal(t, 1, stats[StatusFailed])
}

// setupTestQueue creates a test queue with temp database
func setupTestQueue(t *testing.T) *MessageQueue {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	t.Cleanup(func() {
		t.Setenv("HOME", origHome)
	})

	queue, err := NewMessageQueue()
	require.NoError(t, err)
	require.NotNil(t, queue)

	return queue
}
