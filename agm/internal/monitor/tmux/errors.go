// Package tmux provides tmux session management.
package tmux

import "errors"

var (
	// ErrSessionNotFound is returned when the specified tmux session does not exist
	ErrSessionNotFound = errors.New("tmux session not found")

	// ErrTmuxNotRunning is returned when the tmux server is not running
	ErrTmuxNotRunning = errors.New("tmux server is not running")

	// ErrPermissionDenied is returned when permission is denied to access the tmux session
	ErrPermissionDenied = errors.New("permission denied to access tmux session")
)
