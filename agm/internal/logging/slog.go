// Package logging provides structured logging.
package logging

import (
	"io"
	"log/slog"
	"os"
)

// DefaultLogger returns a configured slog logger for agm
// Uses text format for CLI-friendly output
func DefaultLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo, // Default to INFO level
	}
	handler := slog.NewTextHandler(os.Stderr, opts)
	return slog.New(handler)
}

// JSONLogger returns a JSON-formatted logger for daemon output
func JSONLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	handler := slog.NewJSONHandler(os.Stderr, opts)
	return slog.New(handler)
}

// DebugLogger returns a logger with DEBUG level enabled
func DebugLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	handler := slog.NewTextHandler(os.Stderr, opts)
	return slog.New(handler)
}

// NewTextLogger returns a text-formatted logger that writes to the given writer
func NewTextLogger(w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	handler := slog.NewTextHandler(w, opts)
	return slog.New(handler)
}
