package trace

import "errors"

// ErrSinkClosed is returned when HandleEvent is called on a closed AuditSink.
var ErrSinkClosed = errors.New("audit sink is closed")
