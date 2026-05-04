package notify

import (
	"context"
	"log/slog"
)

// LogDispatcher writes notifications to a structured logger.
type LogDispatcher struct {
	logger *slog.Logger
}

// NewLogDispatcher creates a dispatcher that logs notifications via slog.
func NewLogDispatcher(logger *slog.Logger) *LogDispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogDispatcher{logger: logger}
}

// Name returns the dispatcher name "log".
func (d *LogDispatcher) Name() string { return "log" }

// Dispatch logs the notification via the configured slog.Logger.
func (d *LogDispatcher) Dispatch(_ context.Context, n *Notification) error {
	d.logger.Log(context.Background(), n.Level,
		"notification",
		"id", n.ID,
		"title", n.Title,
		"body", n.Body,
		"source", n.Source,
	)
	return nil
}

// Close is a no-op for LogDispatcher.
func (d *LogDispatcher) Close() error { return nil }
