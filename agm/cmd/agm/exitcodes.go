package main

import (
	"errors"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// Process exit codes for agent consumers.
//
// Agents driving agm can branch on $? without parsing stderr. The codes map
// the HTTP-equivalent OpError.Status (404/409/etc.) onto a small, stable
// failure taxonomy:
//
//	0  success
//	1  generic / unknown error (cobra default)
//	2  auth failure        (unauthenticated / missing API key / permission denied)
//	3  bad input           (invalid args, validation failure)
//	4  state conflict      (archived, already-running, confirmation required)
//	5  not found           (session / resource does not exist)
const (
	ExitOK            = 0
	ExitGeneric       = 1
	ExitAuth          = 2
	ExitBadInput      = 3
	ExitStateConflict = 4
	ExitNotFound      = 5
)

// exitError carries an explicit process exit code alongside an error message.
// The rendered marker distinguishes errors returned after an envelope was
// emitted from early root errors that still need an output owner. handleError
// wraps OpErrors in this so the top-level run() can recover the exit code even
// though the original OpError type has been consumed for printing.
type exitError struct {
	code     int
	msg      string
	rendered bool
}

func (e *exitError) Error() string { return e.msg }

// ExitCode reports the process exit code this error should produce.
func (e *exitError) ExitCode() int { return e.code }

// exitCodeForStatus translates an OpError.Status (HTTP-equivalent code) into
// an AGM exit code. Unmapped statuses (500, 503, etc.) fall through to the
// generic code — they are real failures but not ones agents branch on.
func exitCodeForStatus(status int) int {
	switch status {
	case 401, 403:
		return ExitAuth
	case 400, 422:
		return ExitBadInput
	case 404:
		return ExitNotFound
	case 409:
		return ExitStateConflict
	default:
		return ExitGeneric
	}
}

// exitCodeFromError resolves the process exit code for a finished command.
// It first honours an explicit exitError (the path most CLI errors take via
// handleError), then falls back to inspecting a raw OpError for commands that
// return one directly. Anything else is a generic failure.
func exitCodeFromError(err error) int {
	if err == nil {
		return ExitOK
	}
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	var opErr *ops.OpError
	if errors.As(err, &opErr) {
		return exitCodeForStatus(opErr.Status)
	}
	return ExitGeneric
}
