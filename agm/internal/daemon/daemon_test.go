package daemon

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/logging"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/messages"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func newDaemonDeliveryQueue(t *testing.T, entry messages.QueueEntry) *messages.MessageQueue {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	queue, err := messages.NewMessageQueue()
	if err != nil {
		t.Fatalf("NewMessageQueue() error: %v", err)
	}
	t.Cleanup(func() { _ = queue.Close() })
	if err := queue.Enqueue(&entry); err != nil {
		t.Fatalf("Enqueue() error: %v", err)
	}
	return queue
}

func TestDaemonDeliveryUsesSharedOperationAndStableResult(t *testing.T) {
	entry := messages.QueueEntry{
		MessageID: "message-id",
		From:      "sender",
		To:        "recipient",
		Message:   "header\nmessage body",
		Priority:  messages.PriorityMedium,
		QueuedAt:  time.Now(),
	}
	queue := newDaemonDeliveryQueue(t, entry)
	storage, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	if err := storage.CreateSession(&manifest.Manifest{
		SessionID: "stable-session-id",
		Name:      "recipient",
		Harness:   "agy",
		State:     manifest.StateReady,
	}); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}

	d := NewDaemon(Config{Queue: queue, Logger: logging.NewTextLogger(io.Discard), DoltAdapter: storage})
	calls := 0
	updatedSessionID := ""
	d.updateState = func(_ string, state, source, sessionID string, adapter *dolt.Adapter) error {
		if state != manifest.StateWorking || source != "daemon" || adapter != storage {
			t.Fatalf("state update = %q/%q/%p, want WORKING/daemon/%p", state, source, adapter, storage)
		}
		updatedSessionID = sessionID
		return nil
	}
	d.deliverDirect = func(opCtx *ops.OpContext, req *ops.SendMessageRequest) (*ops.SendMessageResult, error) {
		calls++
		if opCtx.Context != d.ctx || opCtx.Storage != storage || opCtx.Tmux == nil {
			t.Fatalf("operation context = %#v, want daemon context/storage/tmux", opCtx)
		}
		if req.Recipient != entry.To || req.Message != entry.Message {
			t.Fatalf("operation request = %#v, want queue entry", req)
		}
		return &ops.SendMessageResult{
			Operation:       "send_message",
			Recipient:       entry.To,
			SessionID:       "stable-session-id",
			Delivered:       true,
			ResponsePending: true,
		}, nil
	}

	if err := d.deliverMessage(entry); err != nil {
		t.Fatalf("deliverMessage() error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("shared operation calls = %d, want 1", calls)
	}
	if updatedSessionID != "stable-session-id" {
		t.Fatalf("updated session ID = %q, want stable-session-id", updatedSessionID)
	}
	pending, err := queue.GetAllPending()
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending after delivery = %#v, %v; want empty", pending, err)
	}
}

func TestDaemonDeliveryDoesNotMarkCompletedAPITurnWorking(t *testing.T) {
	entry := messages.QueueEntry{
		MessageID: "api-message-id",
		From:      "sender",
		To:        "api-recipient",
		Message:   "message body",
		Priority:  messages.PriorityMedium,
		QueuedAt:  time.Now(),
	}
	queue := newDaemonDeliveryQueue(t, entry)
	d := NewDaemon(Config{Queue: queue, Logger: logging.NewTextLogger(io.Discard)})
	d.updateState = func(string, string, string, string, *dolt.Adapter) error {
		t.Fatal("completed API turn was marked WORKING")
		return nil
	}
	d.deliverDirect = func(*ops.OpContext, *ops.SendMessageRequest) (*ops.SendMessageResult, error) {
		return &ops.SendMessageResult{
			Operation: "send_message",
			Recipient: entry.To,
			SessionID: "api-session-id",
			Delivered: true,
		}, nil
	}

	if err := d.deliverMessage(entry); err != nil {
		t.Fatalf("deliverMessage() error: %v", err)
	}
	pending, err := queue.GetAllPending()
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending after API delivery = %#v, %v; want empty", pending, err)
	}
}

func TestDaemonDisplayStateUsesManifestForNonTmuxSession(t *testing.T) {
	storage, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	if err := storage.CreateSession(&manifest.Manifest{
		SessionID: "api-session-id",
		Name:      "api-recipient",
		Harness:   "openai",
		State:     manifest.StateReady,
	}); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}

	d := NewDaemon(Config{Logger: logging.NewTextLogger(io.Discard), DoltAdapter: storage})
	// Session state is now persisted at creation (the completion pipeline
	// depends on it), so a non-tmux session surfaces its manifest state
	// instead of the old "unknown" that unpersisted state produced.
	if got := d.recordDisplayState("api-recipient"); got != string(manifest.StateReady) {
		t.Fatalf("recordDisplayState() = %q, want %q from the persisted manifest state", got, manifest.StateReady)
	}
	metrics := d.GetMetrics()
	if len(metrics.StateDetectionAccuracy) != 1 {
		t.Fatalf("display-state detections = %v, want exactly the manifest-state detection", metrics.StateDetectionAccuracy)
	}
	if metrics.StateDetectionErrors != 0 {
		t.Fatalf("display-state errors = %d, want 0", metrics.StateDetectionErrors)
	}
}

func TestDaemonDeliveryDefersTypedNotReadyWithoutRetry(t *testing.T) {
	for _, readiness := range []string{"QUEUE", "QUEUED_AGM", "PERMISSION", "OVERLAY", "ONBOARDING", "REVIEW_REQUIRED", "WRONG_HARNESS", "UNKNOWN"} {
		t.Run(readiness, func(t *testing.T) {
			entry := messages.QueueEntry{
				MessageID: "message-" + readiness,
				From:      "sender",
				To:        "recipient",
				Message:   "message",
				Priority:  messages.PriorityMedium,
				QueuedAt:  time.Now(),
			}
			queue := newDaemonDeliveryQueue(t, entry)
			d := NewDaemon(Config{Queue: queue, Logger: logging.NewTextLogger(io.Discard)})
			d.deliverDirect = func(*ops.OpContext, *ops.SendMessageRequest) (*ops.SendMessageResult, error) {
				return &ops.SendMessageResult{Operation: "send_message", Recipient: entry.To}, ops.ErrSessionNotReady(entry.To, readiness)
			}

			err := d.deliverMessage(entry)
			if !errors.Is(err, errDeferred) {
				t.Fatalf("deliverMessage(%s) error = %v, want errDeferred", readiness, err)
			}
			pending, getErr := queue.GetAllPending()
			if getErr != nil || len(pending) != 1 || pending[0].AttemptCount != 0 {
				t.Fatalf("pending after defer = %#v, %v; want one attempt 0", pending, getErr)
			}
		})
	}
}

func TestDaemonDeliveryRetriesMissingAndOperationFailures(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "not found", err: ops.ErrSessionNotReady("recipient", "NOT_FOUND")},
		{name: "operation failure", err: errors.New("provider unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := messages.QueueEntry{
				MessageID: "message-" + test.name,
				From:      "sender",
				To:        "recipient",
				Message:   "message",
				Priority:  messages.PriorityMedium,
				QueuedAt:  time.Now(),
			}
			queue := newDaemonDeliveryQueue(t, entry)
			d := NewDaemon(Config{Queue: queue, Logger: logging.NewTextLogger(io.Discard)})
			d.deliverDirect = func(*ops.OpContext, *ops.SendMessageRequest) (*ops.SendMessageResult, error) {
				return nil, test.err
			}

			err := d.deliverMessage(entry)
			if err == nil || errors.Is(err, errDeferred) {
				t.Fatalf("deliverMessage(%s) error = %v, want retry error", test.name, err)
			}
			pending, getErr := queue.GetAllPending()
			if getErr != nil || len(pending) != 1 || pending[0].AttemptCount != 1 {
				t.Fatalf("pending after retry = %#v, %v; want one attempt 1", pending, getErr)
			}
		})
	}
}

func TestNewDaemon(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "daemon.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}
	defer func() { _ = logFile.Close() }()

	logger := logging.NewTextLogger(logFile)

	queue, err := messages.NewMessageQueue()
	if err != nil {
		t.Fatalf("Failed to create message queue: %v", err)
	}
	defer func() { _ = queue.Close() }()

	cfg := Config{
		BaseDir: tmpDir,
		LogDir:  tmpDir,
		PIDFile: filepath.Join(tmpDir, "daemon.pid"),
		Queue:   queue,
		Logger:  logger,
	}

	d := NewDaemon(cfg)

	if d == nil {
		t.Fatal("NewDaemon returned nil")
		return
	}

	if d.cfg.BaseDir != tmpDir {
		t.Errorf("Expected BaseDir %s, got %s", tmpDir, d.cfg.BaseDir)
	}

	if d.ctx == nil {
		t.Error("Context not initialized")
	}

	if d.cancel == nil {
		t.Error("Cancel func not initialized")
	}
}

func TestDaemon_WritePIDFile(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "daemon.pid")
	logPath := filepath.Join(tmpDir, "daemon.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}
	defer func() { _ = logFile.Close() }()

	logger := logging.NewTextLogger(logFile)

	queue, err := messages.NewMessageQueue()
	if err != nil {
		t.Fatalf("Failed to create message queue: %v", err)
	}
	defer func() { _ = queue.Close() }()

	cfg := Config{
		BaseDir: tmpDir,
		LogDir:  tmpDir,
		PIDFile: pidFile,
		Queue:   queue,
		Logger:  logger,
	}

	d := NewDaemon(cfg)

	// Write PID file
	if err := d.writePIDFile(); err != nil {
		t.Fatalf("writePIDFile failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		t.Error("PID file was not created")
	}

	// Read and verify PID
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("Failed to read PID file: %v", err)
	}

	var readPID int
	if _, err := fmt.Sscanf(string(pidBytes), "%d", &readPID); err != nil {
		t.Fatalf("Failed to parse PID: %v", err)
	}

	if readPID != os.Getpid() {
		t.Errorf("Expected PID %d, got %d", os.Getpid(), readPID)
	}

	// Cleanup
	d.removePIDFile()
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("PID file should be removed")
	}
}

func TestDaemon_StopCancelsContext(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "daemon.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}
	defer func() { _ = logFile.Close() }()

	logger := logging.NewTextLogger(logFile)

	queue, err := messages.NewMessageQueue()
	if err != nil {
		t.Fatalf("Failed to create message queue: %v", err)
	}
	defer func() { _ = queue.Close() }()

	cfg := Config{
		BaseDir: tmpDir,
		LogDir:  tmpDir,
		PIDFile: filepath.Join(tmpDir, "daemon.pid"),
		Queue:   queue,
		Logger:  logger,
	}

	d := NewDaemon(cfg)

	// Verify context is not cancelled initially
	select {
	case <-d.ctx.Done():
		t.Fatal("Context should not be cancelled initially")
	default:
		// Good
	}

	// Stop daemon
	d.Stop()

	// Verify context is now cancelled
	select {
	case <-d.ctx.Done():
		// Good
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Context should be cancelled after Stop()")
	}
}

func TestIsRunning(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "daemon.pid")

	// Test when PID file doesn't exist
	running := IsRunning(pidFile)
	if running {
		t.Error("IsRunning should return false when PID file doesn't exist")
	}

	// Create PID file with current process PID
	pid := os.Getpid()
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", pid)), 0644); err != nil {
		t.Fatalf("Failed to write PID file: %v", err)
	}

	// Test when process is running
	running = IsRunning(pidFile)
	if !running {
		t.Error("IsRunning should return true for current process")
	}

	// Create PID file with non-existent PID
	nonExistentPID := 999999
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", nonExistentPID)), 0644); err != nil {
		t.Fatalf("Failed to write PID file: %v", err)
	}

	// Test when process doesn't exist
	running = IsRunning(pidFile)
	if running {
		t.Error("IsRunning should return false for non-existent process")
	}
}

func TestDaemon_DeliverPending_EmptyQueue(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "daemon.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}
	defer func() { _ = logFile.Close() }()

	logger := logging.NewTextLogger(logFile)

	queue, err := messages.NewMessageQueue()
	if err != nil {
		t.Fatalf("Failed to create message queue: %v", err)
	}
	defer func() { _ = queue.Close() }()

	cfg := Config{
		BaseDir: tmpDir,
		LogDir:  tmpDir,
		PIDFile: filepath.Join(tmpDir, "daemon.pid"),
		Queue:   queue,
		Logger:  logger,
	}

	d := NewDaemon(cfg)

	// Deliver pending (should return without error on empty queue)
	if err := d.deliverPending(); err != nil {
		t.Errorf("deliverPending should not error on empty queue: %v", err)
	}
}
