package eventbus

import (
	"context"
	"log/slog"
)

// LogSink writes events to a structured logger (slog).
type LogSink struct {
	logger *slog.Logger
}

// NewLogSink creates a sink that logs events via the provided slog.Logger.
func NewLogSink(logger *slog.Logger) *LogSink {
	return &LogSink{logger: logger}
}

// Name returns the sink identifier "log".
func (s *LogSink) Name() string { return "log" }

// HandleEvent logs the event using structured logging.
func (s *LogSink) HandleEvent(_ context.Context, event *Event) error {
	s.logger.Log(context.Background(), event.Level,
		"event",
		"event_id", event.ID,
		"type", event.Type,
		"channel", event.Channel,
		"source", event.Source,
		"data", event.Data,
	)
	return nil
}

// Close is a no-op for the log sink.
func (s *LogSink) Close() error { return nil }
