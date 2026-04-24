package notify

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// NotificationSink implements eventbus.Sink by converting events on the
// notification channel into Notification values and fanning them out to
// registered Dispatchers.
type NotificationSink struct {
	mu          sync.RWMutex
	dispatchers []Dispatcher
	logger      *slog.Logger
}

// NewNotificationSink creates a sink that fans out to the given dispatchers.
func NewNotificationSink(logger *slog.Logger, dispatchers ...Dispatcher) *NotificationSink {
	if logger == nil {
		logger = slog.Default()
	}
	return &NotificationSink{
		dispatchers: dispatchers,
		logger:      logger,
	}
}

// Name implements eventbus.Sink.
func (s *NotificationSink) Name() string { return "notify" }

// HandleEvent implements eventbus.Sink. It converts the event into a
// Notification and dispatches it to all registered dispatchers.
func (s *NotificationSink) HandleEvent(ctx context.Context, event *eventbus.Event) error {
	if event.Channel != eventbus.ChannelNotification {
		return nil
	}

	n := eventToNotification(event)

	s.mu.RLock()
	dispatchers := s.dispatchers
	s.mu.RUnlock()

	var firstErr error
	for _, d := range dispatchers {
		if err := d.Dispatch(ctx, n); err != nil {
			s.logger.Warn("dispatcher failed",
				"dispatcher", d.Name(),
				"notification_id", n.ID,
				"error", err,
			)
			if firstErr == nil {
				firstErr = fmt.Errorf("dispatcher %s: %w", d.Name(), err)
			}
		}
	}
	return firstErr
}

// Close implements eventbus.Sink. It closes all dispatchers.
func (s *NotificationSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var firstErr error
	for _, d := range s.dispatchers {
		if err := d.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close dispatcher %s: %w", d.Name(), err)
		}
	}
	return firstErr
}

// eventToNotification converts an eventbus.Event to a Notification.
func eventToNotification(e *eventbus.Event) *Notification {
	n := &Notification{
		ID:        e.ID,
		Level:     e.Level,
		Source:    e.Source,
		Timestamp: e.Timestamp,
		Meta:      e.Data,
	}
	if t, ok := e.Data["title"].(string); ok {
		n.Title = t
	} else {
		n.Title = e.Type
	}
	if b, ok := e.Data["body"].(string); ok {
		n.Body = b
	}
	return n
}
