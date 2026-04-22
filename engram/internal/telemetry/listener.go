package telemetry

// EventListener receives telemetry events asynchronously.
//
// Listeners are called in goroutines, so OnEvent must be thread-safe.
// Panics are recovered and logged, errors are logged but don't block
// other listeners.
//
// Example implementation:
//
//	type MyListener struct{}
//
//	func (l *MyListener) MinLevel() Level {
//	    return LevelWarn  // Only WARN, ERROR, CRITICAL
//	}
//
//	func (l *MyListener) OnEvent(event *Event) error {
//	    // Thread-safe processing
//	    log.Printf("Received event: %s (level %d)", event.Type, event.Level)
//	    return nil
//	}
type EventListener interface {
	// OnEvent is called when an event is recorded (if level >= MinLevel).
	// Called asynchronously in a goroutine.
	// Panics are recovered and logged.
	// Errors are logged but don't block other listeners.
	OnEvent(event *Event) error

	// MinLevel returns the minimum severity level this listener accepts.
	// Events with level < MinLevel are filtered out before calling OnEvent.
	// Called once during registration (result is cached).
	MinLevel() Level
}
