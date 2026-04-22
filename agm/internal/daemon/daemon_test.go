package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/logging"
	"github.com/vbonnet/dear-agent/agm/internal/messages"
)

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

func TestTruncateMessage(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short message",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length",
			input:  "helloworld",
			maxLen: 10,
			want:   "helloworld",
		},
		{
			name:   "truncate long message",
			input:  "hello world this is a long message",
			maxLen: 10,
			want:   "hello worl...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateMessage(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}
