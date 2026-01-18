package agent

import "errors"

// ErrNotImplemented is returned by stub adapter methods that are not yet implemented.
var ErrNotImplemented = errors.New("not implemented")
