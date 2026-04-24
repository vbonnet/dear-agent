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

func (d *LogDispatcher) Name() string { return "log" }

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

func (d *LogDispatcher) Close() error { return nil }
