package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// captureLogger creates a logger that writes to a buffer for testing
func captureLogger(level LogLevel) (*Logger, *bytes.Buffer) {
	buf := new(bytes.Buffer)
	opts := &slog.HandlerOptions{
		Level: level.toSlogLevel(),
	}
	handler := slog.NewJSONHandler(buf, opts)
	return &Logger{
		logger: slog.New(handler),
		level:  level,
	}, buf
}

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LogLevel(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("LogLevel.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLogLevel_toSlogLevel(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected slog.Level
	}{
		{LevelDebug, slog.LevelDebug},
		{LevelInfo, slog.LevelInfo},
		{LevelWarn, slog.LevelWarn},
		{LevelError, slog.LevelError},
		{LogLevel(999), slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			if got := tt.level.toSlogLevel(); got != tt.expected {
				t.Errorf("LogLevel.toSlogLevel() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNewLogger(t *testing.T) {
	logger := NewLogger(LevelDebug)
	if logger == nil {
		t.Fatal("NewLogger() returned nil")
	}
	if logger.level != LevelDebug {
		t.Errorf("NewLogger() level = %v, want %v", logger.level, LevelDebug)
	}
	if logger.logger == nil {
		t.Error("NewLogger() logger.logger is nil")
	}
}

func TestNewDefaultLogger(t *testing.T) {
	logger := NewDefaultLogger()
	if logger == nil {
		t.Fatal("NewDefaultLogger() returned nil")
	}
	if logger.level != LevelInfo {
		t.Errorf("NewDefaultLogger() level = %v, want %v", logger.level, LevelInfo)
	}
}

func TestLogger_Debug(t *testing.T) {
	logger, buf := captureLogger(LevelDebug)
	ctx := context.Background()

	logger.Debug(ctx, "test debug message", WithPlugin("test-plugin"))

	output := buf.String()
	if !strings.Contains(output, "test debug message") {
		t.Errorf("Debug() output missing message: %s", output)
	}
	if !strings.Contains(output, "test-plugin") {
		t.Errorf("Debug() output missing plugin name: %s", output)
	}

	// Verify it's at debug level
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}
	if logEntry["level"] != "DEBUG" {
		t.Errorf("Debug() level = %v, want DEBUG", logEntry["level"])
	}
}

func TestLogger_Info(t *testing.T) {
	logger, buf := captureLogger(LevelInfo)
	ctx := context.Background()

	logger.Info(ctx, "test info message", WithPlugin("test-plugin"))

	output := buf.String()
	if !strings.Contains(output, "test info message") {
		t.Errorf("Info() output missing message: %s", output)
	}
	if !strings.Contains(output, "test-plugin") {
		t.Errorf("Info() output missing plugin name: %s", output)
	}
}

func TestLogger_Warn(t *testing.T) {
	logger, buf := captureLogger(LevelWarn)
	ctx := context.Background()
	testErr := errors.New("test warning error")

	logger.Warn(ctx, "test warning message", WithPlugin("test-plugin"), testErr)

	output := buf.String()
	if !strings.Contains(output, "test warning message") {
		t.Errorf("Warn() output missing message: %s", output)
	}
	if !strings.Contains(output, "test-plugin") {
		t.Errorf("Warn() output missing plugin name: %s", output)
	}
	if !strings.Contains(output, "test warning error") {
		t.Errorf("Warn() output missing error: %s", output)
	}
}

func TestLogger_Error(t *testing.T) {
	logger, buf := captureLogger(LevelError)
	ctx := context.Background()
	testErr := errors.New("test error")

	logger.Error(ctx, "test error message", WithPlugin("test-plugin"), testErr)

	output := buf.String()
	if !strings.Contains(output, "test error message") {
		t.Errorf("Error() output missing message: %s", output)
	}
	if !strings.Contains(output, "test-plugin") {
		t.Errorf("Error() output missing plugin name: %s", output)
	}
	if !strings.Contains(output, "test error") {
		t.Errorf("Error() output missing error: %s", output)
	}
}

func TestLogger_LevelFiltering(t *testing.T) {
	// Logger set to INFO level should not log DEBUG messages
	logger, buf := captureLogger(LevelInfo)
	ctx := context.Background()

	logger.Debug(ctx, "debug message", WithPlugin("test"))
	logger.Info(ctx, "info message", WithPlugin("test"))

	output := buf.String()
	if strings.Contains(output, "debug message") {
		t.Error("Info-level logger logged DEBUG message")
	}
	if !strings.Contains(output, "info message") {
		t.Error("Info-level logger did not log INFO message")
	}
}

func TestErrorContext_WithExtra(t *testing.T) {
	ctx := WithPlugin("test-plugin")
	ctx = ctx.WithExtra("key1", "value1")
	ctx = ctx.WithExtra("key2", 42)

	if ctx.Extra["key1"] != "value1" {
		t.Errorf("WithExtra() key1 = %v, want value1", ctx.Extra["key1"])
	}
	if ctx.Extra["key2"] != 42 {
		t.Errorf("WithExtra() key2 = %v, want 42", ctx.Extra["key2"])
	}
}

func TestErrorContext_Merge(t *testing.T) {
	ctx1 := ErrorContext{
		Plugin:    "plugin1",
		Operation: "op1",
		Extra:     map[string]interface{}{"key1": "value1"},
	}

	ctx2 := ErrorContext{
		Command:   "cmd1",
		Operation: "op2",
		Extra:     map[string]interface{}{"key2": "value2"},
	}

	merged := ctx1.Merge(ctx2)

	if merged.Plugin != "plugin1" {
		t.Errorf("Merge() plugin = %v, want plugin1", merged.Plugin)
	}
	if merged.Command != "cmd1" {
		t.Errorf("Merge() command = %v, want cmd1", merged.Command)
	}
	if merged.Operation != "op2" {
		t.Errorf("Merge() operation = %v, want op2 (should override)", merged.Operation)
	}
	if merged.Extra["key1"] != "value1" {
		t.Errorf("Merge() extra key1 = %v, want value1", merged.Extra["key1"])
	}
	if merged.Extra["key2"] != "value2" {
		t.Errorf("Merge() extra key2 = %v, want value2", merged.Extra["key2"])
	}
}

func TestWithHelpers(t *testing.T) {
	t.Run("WithPlugin", func(t *testing.T) {
		ctx := WithPlugin("test-plugin")
		if ctx.Plugin != "test-plugin" {
			t.Errorf("WithPlugin() = %v, want test-plugin", ctx.Plugin)
		}
	})

	t.Run("WithCommand", func(t *testing.T) {
		ctx := WithCommand("test-plugin", "test-cmd")
		if ctx.Plugin != "test-plugin" {
			t.Errorf("WithCommand() plugin = %v, want test-plugin", ctx.Plugin)
		}
		if ctx.Command != "test-cmd" {
			t.Errorf("WithCommand() command = %v, want test-cmd", ctx.Command)
		}
	})

	t.Run("WithOperation", func(t *testing.T) {
		ctx := WithOperation("test-op")
		if ctx.Operation != "test-op" {
			t.Errorf("WithOperation() = %v, want test-op", ctx.Operation)
		}
	})

	t.Run("WithPath", func(t *testing.T) {
		ctx := WithPath("/test/path")
		if ctx.PluginPath != "/test/path" {
			t.Errorf("WithPath() = %v, want /test/path", ctx.PluginPath)
		}
	})

	t.Run("WithSearchPath", func(t *testing.T) {
		ctx := WithSearchPath("/search/path")
		if ctx.SearchPath != "/search/path" {
			t.Errorf("WithSearchPath() = %v, want /search/path", ctx.SearchPath)
		}
	})
}

func TestFormatError(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		errCtx   ErrorContext
		err      error
		contains []string
	}{
		{
			name:     "error with plugin context",
			msg:      "operation failed",
			errCtx:   WithPlugin("test-plugin"),
			err:      errors.New("underlying error"),
			contains: []string{"plugin=test-plugin", "operation failed", "underlying error"},
		},
		{
			name:     "error with command context",
			msg:      "command failed",
			errCtx:   WithCommand("test-plugin", "test-cmd"),
			err:      errors.New("cmd error"),
			contains: []string{"command=test-cmd", "command failed", "cmd error"},
		},
		{
			name:     "error with operation context",
			msg:      "operation failed",
			errCtx:   WithOperation("load"),
			err:      errors.New("load error"),
			contains: []string{"operation=load", "operation failed", "load error"},
		},
		{
			name:     "error without underlying error",
			msg:      "simple error",
			errCtx:   ErrorContext{},
			err:      nil,
			contains: []string{"simple error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FormatError(tt.msg, tt.errCtx, tt.err)
			errStr := err.Error()

			for _, substr := range tt.contains {
				if !strings.Contains(errStr, substr) {
					t.Errorf("FormatError() = %v, want to contain %v", errStr, substr)
				}
			}
		})
	}
}

func TestLogger_AllContextFields(t *testing.T) {
	logger, buf := captureLogger(LevelInfo)
	ctx := context.Background()

	errCtx := ErrorContext{
		Plugin:     "test-plugin",
		Command:    "test-cmd",
		SearchPath: "/search/path",
		PluginPath: "/plugin/path",
		Operation:  "test-op",
		Extra: map[string]interface{}{
			"custom_key": "custom_value",
		},
	}

	logger.Info(ctx, "test message", errCtx)

	output := buf.String()

	// Check all fields are present in JSON output
	expectedFields := []string{
		"test-plugin",
		"test-cmd",
		"/search/path",
		"/plugin/path",
		"test-op",
		"custom_key",
		"custom_value",
	}

	for _, field := range expectedFields {
		if !strings.Contains(output, field) {
			t.Errorf("Logger output missing field %q: %s", field, output)
		}
	}
}

func TestLogger_NilError(t *testing.T) {
	logger, buf := captureLogger(LevelError)
	ctx := context.Background()

	// Should not panic with nil error
	logger.Error(ctx, "error message", WithPlugin("test"), nil)

	output := buf.String()
	if !strings.Contains(output, "error message") {
		t.Errorf("Error() with nil error missing message: %s", output)
	}
}

func TestLogger_EmptyContext(t *testing.T) {
	logger, buf := captureLogger(LevelInfo)
	ctx := context.Background()

	// Should work with empty context
	logger.Info(ctx, "test message", ErrorContext{})

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Info() with empty context missing message: %s", output)
	}
}
