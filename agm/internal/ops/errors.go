// Package ops provides ops functionality.
package ops

import (
	"encoding/json"
	"fmt"
	"time"
)

// OpError implements RFC 7807 Problem Details for HTTP APIs,
// adapted for CLI/MCP/Skill error responses.
//
// Every error includes actionable suggestions telling agents
// what to do instead of the failed operation.
type OpError struct {
	// Status is the HTTP-equivalent status code (400, 404, 409, 500, etc.)
	Status int `json:"status"`

	// Type is a machine-readable error category: "session/not_found", "input/invalid", etc.
	Type string `json:"type"`

	// Code is a stable error code for programmatic handling: "AGM-001", "AGM-002", etc.
	Code string `json:"code"`

	// Title is a short human-readable summary.
	Title string `json:"title"`

	// Detail is a full explanation of what went wrong.
	Detail string `json:"detail"`

	// Instance identifies the specific operation that failed.
	Instance string `json:"instance,omitempty"`

	// Suggestions are concrete next actions the agent should take.
	Suggestions []string `json:"suggestions,omitempty"`

	// Parameters echo back the input that caused the error.
	Parameters map[string]string `json:"parameters,omitempty"`
}

func (e *OpError) Error() string {
	return fmt.Sprintf("[%s] %s: %s", e.Code, e.Title, e.Detail)
}

// JSON returns the error as a JSON byte slice for programmatic consumers.
func (e *OpError) JSON() []byte {
	data, _ := json.Marshal(e)
	return data
}

// Error catalog — stable codes that agents can match on.
const (
	ErrCodeSessionNotFound    = "AGM-001"
	ErrCodeSessionArchived    = "AGM-002"
	ErrCodeTmuxNotRunning     = "AGM-003"
	ErrCodeDoltUnavailable    = "AGM-004"
	ErrCodeInvalidInput       = "AGM-005"
	ErrCodePermissionDenied   = "AGM-006"
	ErrCodeSessionExists      = "AGM-007"
	ErrCodeHarnessUnavailable = "AGM-008"
	ErrCodeWorkspaceNotFound  = "AGM-009"
	ErrCodeUUIDNotAssociated  = "AGM-010"
	ErrCodeStorageError       = "AGM-011"
	ErrCodeVerificationFailed = "AGM-012"
	ErrCodeKillProtected      = "AGM-013"
	ErrCodeActiveSessionKill  = "AGM-014"
	ErrCodeDryRun             = "AGM-100"
)

// Constructor functions for common errors.

// ErrSessionNotFound returns an error indicating no session matches the given identifier.
func ErrSessionNotFound(identifier string) *OpError {
	return &OpError{
		Status:   404,
		Type:     "session/not_found",
		Code:     ErrCodeSessionNotFound,
		Title:    "Session not found",
		Detail:   fmt.Sprintf("No session matches identifier %q.", identifier),
		Instance: "session/get",
		Suggestions: []string{
			"Run `agm session list` to see available sessions.",
			"Check if the session was archived: `agm session list --all`.",
			"Use a session name, UUID, or UUID prefix as the identifier.",
		},
		Parameters: map[string]string{
			"identifier": identifier,
		},
	}
}

// ErrSessionArchived returns an error indicating the session is archived and cannot be modified.
func ErrSessionArchived(name string) *OpError {
	return &OpError{
		Status: 409,
		Type:   "session/archived",
		Code:   ErrCodeSessionArchived,
		Title:  "Session is archived",
		Detail: fmt.Sprintf("Session %q is archived and cannot be modified.", name),
		Suggestions: []string{
			fmt.Sprintf("Unarchive first: `agm session unarchive %s`.", name),
			"Create a new session instead: `agm session new <name>`.",
		},
		Parameters: map[string]string{
			"session_name": name,
		},
	}
}

// ErrInvalidInput returns an error for invalid input on the specified field.
func ErrInvalidInput(field, detail string) *OpError {
	return &OpError{
		Status: 400,
		Type:   "input/invalid",
		Code:   ErrCodeInvalidInput,
		Title:  "Invalid input",
		Detail: detail,
		Suggestions: []string{
			"Check the field value and try again.",
			"Run the command with `--schema` to see the expected input format.",
		},
		Parameters: map[string]string{
			"field": field,
		},
	}
}

// ErrStorageError returns an error indicating a storage operation failure.
func ErrStorageError(operation string, cause error) *OpError {
	return &OpError{
		Status:   500,
		Type:     "storage/error",
		Code:     ErrCodeStorageError,
		Title:    "Storage error",
		Detail:   fmt.Sprintf("Storage operation %q failed: %v", operation, cause),
		Instance: operation,
		Suggestions: []string{
			"Run `agm admin doctor` to check storage health.",
			"Verify Dolt server is running: `agm admin dolt-status`.",
		},
	}
}

// ErrKillProtected returns an error indicating the session was recently active and requires --force.
func ErrKillProtected(name string, lastActivity time.Time) *OpError {
	ago := time.Since(lastActivity).Truncate(time.Second)
	return &OpError{
		Status: 409,
		Type:   "session/kill_protected",
		Code:   ErrCodeKillProtected,
		Title:  "Session recently active",
		Detail: fmt.Sprintf("Session %q was active %s ago. Use --force to override.", name, ago),
		Suggestions: []string{
			fmt.Sprintf("Use --force to kill despite recent activity: `agm session kill --force %s`.", name),
			"Wait for the session to become idle before killing.",
		},
		Parameters: map[string]string{
			"session":       name,
			"last_activity": lastActivity.Format(time.RFC3339),
		},
	}
}

// ErrActiveSessionKill returns an error indicating the session is actively running
// and requires --confirmed-stuck to kill.
func ErrActiveSessionKill(name string) *OpError {
	return &OpError{
		Status: 409,
		Type:   "session/active_kill",
		Code:   ErrCodeActiveSessionKill,
		Title:  "Session is active",
		Detail: fmt.Sprintf("Session %q is actively running. Use --confirmed-stuck to kill an active session.", name),
		Suggestions: []string{
			fmt.Sprintf("If the session is stuck, use: `agm session kill --confirmed-stuck %s`.", name),
			"If the session is healthy, consider using `agm exit` instead for graceful shutdown.",
			fmt.Sprintf("Check session status first: `agm session status %s`.", name),
		},
		Parameters: map[string]string{
			"session": name,
		},
	}
}

// ErrTmuxNotRunning returns an error indicating no tmux server is running.
func ErrTmuxNotRunning() *OpError {
	return &OpError{
		Status: 503,
		Type:   "tmux/not_running",
		Code:   ErrCodeTmuxNotRunning,
		Title:  "Tmux not running",
		Detail: "No tmux server is running. AGM requires tmux for session management.",
		Suggestions: []string{
			"Start tmux: `tmux new-session -d -s default`.",
			"Install tmux if not available.",
			"Check if tmux socket is accessible: `tmux list-sessions`.",
		},
	}
}
