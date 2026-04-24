package logging

import (
	"log/slog"
	"testing"
)

func TestDefaultLogger(t *testing.T) {
	logger := DefaultLogger()
	if logger == nil {
		t.Fatal("DefaultLogger returned nil")
	}

	// Verify it's enabled at Info level
	if !logger.Enabled(nil, slog.LevelInfo) {
		t.Error("expected Info level to be enabled")
	}
	// Debug should not be enabled at default level
	if logger.Enabled(nil, slog.LevelDebug) {
		t.Error("expected Debug level to be disabled")
	}
}

func TestJSONLogger(t *testing.T) {
	logger := JSONLogger()
	if logger == nil {
		t.Fatal("JSONLogger returned nil")
	}

	if !logger.Enabled(nil, slog.LevelInfo) {
		t.Error("expected Info level to be enabled")
	}
	if !logger.Enabled(nil, slog.LevelError) {
		t.Error("expected Error level to be enabled")
	}
	if logger.Enabled(nil, slog.LevelDebug) {
		t.Error("expected Debug level to be disabled")
	}
}

func TestDebugLogger(t *testing.T) {
	logger := DebugLogger()
	if logger == nil {
		t.Fatal("DebugLogger returned nil")
	}

	// Debug logger should enable all levels
	if !logger.Enabled(nil, slog.LevelDebug) {
		t.Error("expected Debug level to be enabled")
	}
	if !logger.Enabled(nil, slog.LevelInfo) {
		t.Error("expected Info level to be enabled")
	}
	if !logger.Enabled(nil, slog.LevelWarn) {
		t.Error("expected Warn level to be enabled")
	}
	if !logger.Enabled(nil, slog.LevelError) {
		t.Error("expected Error level to be enabled")
	}
}
