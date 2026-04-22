// Package agent provides agent functionality.
package agent

import "errors"

// Session errors
var (
	// ErrSessionNotFound indicates the session does not exist.
	ErrSessionNotFound = errors.New("session not found")

	// ErrSessionExists indicates a session with the same name already exists.
	ErrSessionExists = errors.New("session already exists")

	// ErrSessionTerminated indicates the session has been terminated.
	ErrSessionTerminated = errors.New("session has been terminated")

	// ErrInvalidContext indicates invalid session context parameters.
	ErrInvalidContext = errors.New("invalid session context")
)

// Message errors
var (
	// ErrInvalidRole indicates an invalid message role.
	ErrInvalidRole = errors.New("invalid message role")

	// ErrEmptyContent indicates message content cannot be empty.
	ErrEmptyContent = errors.New("message content cannot be empty")
)

// Command errors
var (
	// ErrUnsupportedCommand indicates the command is not supported by this agent.
	ErrUnsupportedCommand = errors.New("command not supported by this agent")

	// ErrInvalidParams indicates invalid command parameters.
	ErrInvalidParams = errors.New("invalid command parameters")
)

// Network errors
var (
	// ErrNetworkError indicates a network error occurred.
	ErrNetworkError = errors.New("network error")

	// ErrAuthFailed indicates authentication failed.
	ErrAuthFailed = errors.New("authentication failed")

	// ErrRateLimited indicates the API rate limit was exceeded.
	ErrRateLimited = errors.New("rate limit exceeded")
)

// Format errors
var (
	// ErrUnsupportedFormat indicates the format is not supported.
	ErrUnsupportedFormat = errors.New("unsupported format")

	// ErrInvalidFormat indicates invalid format or corrupted data.
	ErrInvalidFormat = errors.New("invalid format or corrupted data")

	// ErrHistoryCorrupted indicates the conversation history is corrupted.
	ErrHistoryCorrupted = errors.New("conversation history corrupted")
)

// tmux errors (CLI agents)
var (
	// ErrTmuxSessionNotFound indicates the tmux session was not found.
	ErrTmuxSessionNotFound = errors.New("tmux session not found")
)

// Implementation errors
var (
	// ErrNotImplemented is returned by stub adapter methods that are not yet implemented.
	ErrNotImplemented = errors.New("not implemented")
)
