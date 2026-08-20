package debug

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInit_Disabled(t *testing.T) {
	// Reset global state
	mu.Lock()
	oldLogger := globalLogger
	globalLogger = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		globalLogger = oldLogger
		mu.Unlock()
	}()

	err := Init(false, "test-session")
	if err != nil {
		t.Fatalf("Init(false, ...) returned error: %v", err)
	}

	mu.Lock()
	if globalLogger == nil {
		mu.Unlock()
		t.Fatal("globalLogger should not be nil after Init")
	}
	if globalLogger.enabled {
		mu.Unlock()
		t.Error("globalLogger.enabled should be false")
	}
	mu.Unlock()
}

func TestInit_Enabled(t *testing.T) {
	// Reset global state
	mu.Lock()
	oldLogger := globalLogger
	globalLogger = nil
	mu.Unlock()
	defer func() {
		Close()
		mu.Lock()
		globalLogger = oldLogger
		mu.Unlock()
	}()

	// Override HOME so we write to a temp dir
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	debugDir := filepath.Join(tmpDir, ".agm", "debug")
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		t.Fatalf("pre-create broad debug directory: %v", err)
	}

	err := Init(true, "test-session")
	if err != nil {
		t.Fatalf("Init(true, ...) returned error: %v", err)
	}

	mu.Lock()
	if globalLogger == nil {
		mu.Unlock()
		t.Fatal("globalLogger should not be nil")
	}
	if !globalLogger.enabled {
		mu.Unlock()
		t.Error("globalLogger.enabled should be true")
	}
	if globalLogger.file == nil {
		mu.Unlock()
		t.Error("globalLogger.file should not be nil")
	}
	mu.Unlock()

	// Verify debug directory was created
	info, err := os.Stat(debugDir)
	if err != nil {
		t.Fatalf("debug directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("debug path should be a directory")
	}
	if got := info.Mode().Perm(); got != 0700 {
		t.Errorf("debug directory mode = %04o, want 0700", got)
	}

	mu.Lock()
	logPath := globalLogger.file.Name()
	mu.Unlock()
	logInfo, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("debug log not created: %v", err)
	}
	if got := logInfo.Mode().Perm(); got != 0600 {
		t.Errorf("debug log mode = %04o, want 0600", got)
	}
}

func TestLog_WhenDisabled(t *testing.T) {
	mu.Lock()
	oldLogger := globalLogger
	globalLogger = &Logger{enabled: false}
	mu.Unlock()
	defer func() {
		mu.Lock()
		globalLogger = oldLogger
		mu.Unlock()
	}()

	// Should not panic when disabled
	Log("test message %d", 42)
}

func TestLog_WhenNilLogger(t *testing.T) {
	mu.Lock()
	oldLogger := globalLogger
	globalLogger = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		globalLogger = oldLogger
		mu.Unlock()
	}()

	// Should not panic with nil logger
	Log("test message %d", 42)
}

func TestLog_WhenEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("failed to create log file: %v", err)
	}

	mu.Lock()
	oldLogger := globalLogger
	globalLogger = &Logger{
		file:    f,
		enabled: true,
	}
	mu.Unlock()
	defer func() {
		f.Close()
		mu.Lock()
		globalLogger = oldLogger
		mu.Unlock()
	}()

	Log("hello %s", "world")

	// Read file content
	f.Sync()
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Error("log file should not be empty after Log()")
	}
}

func TestPhase(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("failed to create log file: %v", err)
	}

	mu.Lock()
	oldLogger := globalLogger
	globalLogger = &Logger{
		file:    f,
		enabled: true,
	}
	mu.Unlock()
	defer func() {
		f.Close()
		mu.Lock()
		globalLogger = oldLogger
		mu.Unlock()
	}()

	Phase("test-phase")

	f.Sync()
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Error("log file should not be empty after Phase()")
	}
}

func TestClose_NilLogger(t *testing.T) {
	mu.Lock()
	oldLogger := globalLogger
	globalLogger = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		globalLogger = oldLogger
		mu.Unlock()
	}()

	// Should not panic
	Close()
}

func TestClose_DisabledLogger(t *testing.T) {
	mu.Lock()
	oldLogger := globalLogger
	globalLogger = &Logger{enabled: false}
	mu.Unlock()
	defer func() {
		mu.Lock()
		globalLogger = oldLogger
		mu.Unlock()
	}()

	// Should not panic
	Close()
}

func TestClose_ReleasesHandleAndIsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("failed to create log file: %v", err)
	}

	mu.Lock()
	oldLogger := globalLogger
	globalLogger = &Logger{file: f, enabled: true, startTime: time.Now()}
	mu.Unlock()
	defer func() {
		mu.Lock()
		globalLogger = oldLogger
		mu.Unlock()
	}()

	Close()

	mu.Lock()
	handle := globalLogger.file
	stillEnabled := globalLogger.enabled
	mu.Unlock()
	if handle != nil {
		t.Error("Close() should drop the file handle so a later write cannot reach a closed descriptor")
	}
	if stillEnabled {
		t.Error("Close() should clear enabled so Log() short-circuits instead of touching a nil file")
	}

	// The descriptor really is closed, not just forgotten.
	if _, err := f.Write([]byte("after close")); err == nil {
		t.Error("write after Close() succeeded; the descriptor was never closed")
	}

	// DBG-04: repeated close and post-close logging stay no-ops.
	Close()
	Log("after close %d", 1)

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if got := string(data); !strings.Contains(got, "Debug session ended") {
		t.Errorf("closing log should be flushed to disk, got %q", got)
	}
	if got := string(data); strings.Contains(got, "after close") {
		t.Error("Log() after Close() must not append to the closed log")
	}
}
