package gateway

import "fmt"

// ErrorCode is a small enumeration adapters can switch on to map
// gateway errors into their wire format (HTTP status, JSON-RPC code,
// chat ack symbol).
type ErrorCode string

const (
	// CodeUnknownCommand means the command type has no registered
	// handler. Adapters should surface this as a client error
	// (HTTP 400, JSON-RPC -32601).
	CodeUnknownCommand ErrorCode = "unknown_command"

	// CodeInvalidArgs means the handler received an Args map that
	// violated the documented contract. Surface as a client error.
	CodeInvalidArgs ErrorCode = "invalid_args"

	// CodeNotFound means a referenced object (run_id, approval_id) does
	// not exist.
	CodeNotFound ErrorCode = "not_found"

	// CodeConflict means the operation collides with current state
	// (gate already resolved, run already terminal).
	CodeConflict ErrorCode = "conflict"

	// CodeUnauthorized means the caller is not allowed to perform the
	// action (role mismatch on HITL, anonymous caller on a write).
	CodeUnauthorized ErrorCode = "unauthorized"

	// CodeUnavailable means a dependency the handler needs is not
	// configured (no Runner, no audit store).
	CodeUnavailable ErrorCode = "unavailable"

	// CodeInternal is the catch-all for handler bugs and
	// infrastructure failures.
	CodeInternal ErrorCode = "internal"
)

// Error is the structured error returned in Response.Err. Cause is the
// underlying Go error if the handler had one — kept around for logging
// but never serialised over the wire.
type Error struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Cause   error     `json:"-"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap exposes the underlying cause to errors.Is / errors.As.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Errorf is a convenience that builds an Error with a formatted message
// and no underlying cause.
func Errorf(code ErrorCode, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// WrapError attaches an underlying cause. Useful when a handler wants
// to surface a structured code while preserving the original error for
// logs.
func WrapError(code ErrorCode, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

// errorResponse is the convenience helper handlers use to return an
// Error inside a Response.
func errorResponse(cmdID string, err *Error) Response {
	return Response{CommandID: cmdID, Err: err}
}
