package debug

import (
	"os"
	"path/filepath"
	"testing"
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
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

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
	debugDir := filepath.Join(tmpDir, ".agm", "debug")
	info, err := os.Stat(debugDir)
	if err != nil {
		t.Fatalf("debug directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("debug path should be a directory")
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
