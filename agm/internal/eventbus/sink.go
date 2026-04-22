package eventbus

// Sink receives events from the eventbus for processing.
// Implementations handle event persistence, forwarding, or transformation.
type Sink interface {
	// HandleEvent processes a single event. Implementations should be safe
	// for concurrent use.
	HandleEvent(event *Event) error

	// Close flushes any buffered data and releases resources.
	Close() error
}
