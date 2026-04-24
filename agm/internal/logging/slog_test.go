package logging

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
)

func TestDefaultLogger(t *testing.T) {
	logger := DefaultLogger()
	if logger == nil {
		t.Fatal("DefaultLogger() returned nil")
	}

	// Verify it can log without error
	logger.Info("test message", "key", "value")

	// Verify handler is enabled at Info level
	if !logger.Handler().Enabled(context.Background(), slog.LevelInfo) {
		t.Error("DefaultLogger should be enabled at INFO level")
	}

	// Verify DEBUG is not enabled (default is INFO)
	if logger.Handler().Enabled(context.Background(), slog.LevelDebug) {
		t.Error("DefaultLogger should not be enabled at DEBUG level")
	}
}

func TestJSONLogger(t *testing.T) {
	logger := JSONLogger()
	if logger == nil {
		t.Fatal("JSONLogger() returned nil")
	}

	// Verify it can log without error
	logger.Info("test message")

	// Verify handler is enabled at Info level
	if !logger.Handler().Enabled(context.Background(), slog.LevelInfo) {
		t.Error("JSONLogger should be enabled at INFO level")
	}

	// Verify DEBUG is not enabled
	if logger.Handler().Enabled(context.Background(), slog.LevelDebug) {
		t.Error("JSONLogger should not be enabled at DEBUG level")
	}
}

func TestDebugLogger(t *testing.T) {
	logger := DebugLogger()
	if logger == nil {
		t.Fatal("DebugLogger() returned nil")
	}

	// Verify handler is enabled at DEBUG level
	if !logger.Handler().Enabled(context.Background(), slog.LevelDebug) {
		t.Error("DebugLogger should be enabled at DEBUG level")
	}

	// Verify INFO is also enabled
	if !logger.Handler().Enabled(context.Background(), slog.LevelInfo) {
		t.Error("DebugLogger should be enabled at INFO level")
	}
}

func TestNewTextLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := NewTextLogger(&buf)
	if logger == nil {
		t.Fatal("NewTextLogger() returned nil")
	}

	logger.Info("hello", "key", "value")

	output := buf.String()
	if len(output) == 0 {
		t.Error("NewTextLogger should produce output")
	}

	// Verify output contains the message
	if !bytes.Contains([]byte(output), []byte("hello")) {
		t.Errorf("expected output to contain 'hello', got: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte("key=value")) {
		t.Errorf("expected output to contain 'key=value', got: %s", output)
	}
}

func TestNewTextLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := NewTextLogger(&buf)

	// DEBUG should not be logged (default level is INFO)
	logger.Debug("debug message")
	if buf.Len() > 0 {
		t.Error("NewTextLogger should not log DEBUG messages by default")
	}

	// INFO should be logged
	logger.Info("info message")
	if buf.Len() == 0 {
		t.Error("NewTextLogger should log INFO messages")
	}
}
