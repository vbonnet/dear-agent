package notify

import (
	"context"
	"log/slog"
	"time"
)

// Notification is the payload delivered to dispatchers.
type Notification struct {
	// ID is a unique identifier for this notification.
	ID string `json:"id"`
	// Title is a short summary suitable for display.
	Title string `json:"title"`
	// Body contains the full notification text.
	Body string `json:"body"`
	// Level indicates severity.
	Level slog.Level `json:"level"`
	// Source identifies the originating component.
	Source string `json:"source"`
	// Timestamp is when the notification was created.
	Timestamp time.Time `json:"timestamp"`
	// Meta carries arbitrary key-value data from the event.
	Meta map[string]interface{} `json:"meta,omitempty"`
}

// Dispatcher delivers notifications to a specific backend.
type Dispatcher interface {
	// Name returns a human-readable identifier for this dispatcher.
	Name() string
	// Dispatch sends the notification. Implementations should respect
	// context cancellation and return errors for transient failures.
	Dispatch(ctx context.Context, n *Notification) error
	// Close releases resources held by the dispatcher.
	Close() error
}
