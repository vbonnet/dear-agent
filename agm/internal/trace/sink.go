package trace

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
	"github.com/vbonnet/dear-agent/agm/internal/logging"
)

// AuditSink implements eventbus.Sink by fanning out events to one or more Backends.
type AuditSink struct {
	backends []Backend
	logger   *slog.Logger
	mu       sync.RWMutex
	closed   bool
}

// AuditSinkOption configures an AuditSink.
type AuditSinkOption func(*AuditSink)

// WithLogger sets a custom logger.
func WithLogger(l *slog.Logger) AuditSinkOption {
	return func(s *AuditSink) { s.logger = l }
}

// NewAuditSink creates an AuditSink that writes to the given backends.
func NewAuditSink(backends []Backend, opts ...AuditSinkOption) *AuditSink {
	s := &AuditSink{
		backends: backends,
		logger:   logging.DefaultLogger(),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// HandleEvent converts the event to a TraceRecord and writes it to all backends.
// Errors from individual backends are logged but do not stop delivery to others.
func (s *AuditSink) HandleEvent(event *eventbus.Event) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrSinkClosed
	}
	s.mu.RUnlock()

	rec, err := RecordFromEvent(event)
	if err != nil {
		s.logger.Warn("Failed to convert event to trace record", "error", err, "event_type", event.Type)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var firstErr error
	for _, b := range s.backends {
		if werr := b.Write(ctx, rec); werr != nil {
			s.logger.Warn("Backend write failed", "error", werr, "event_type", event.Type)
			if firstErr == nil {
				firstErr = werr
			}
		}
	}
	return firstErr
}

// Close flushes and closes all backends.
func (s *AuditSink) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	var firstErr error
	for _, b := range s.backends {
		if err := b.Flush(context.Background()); err != nil {
			s.logger.Warn("Backend flush failed on close", "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
		if err := b.Close(); err != nil {
			s.logger.Warn("Backend close failed", "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// compile-time check
var _ eventbus.Sink = (*AuditSink)(nil)
