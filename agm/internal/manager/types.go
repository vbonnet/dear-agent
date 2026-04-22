// Package manager defines the core abstractions for agent session management.
//
// Three interfaces decouple AGM from any specific backend (tmux, process, etc.):
//   - SessionManager: lifecycle operations (create, terminate, list, archive)
//   - MessageBroker: communication (send message, read output, interrupt)
//   - StateReader: observation (get state, health check, detect permissions)
//
// Backends implement these interfaces and register via the Registry.
package manager

import (
	"time"
)

// SessionID is an opaque identifier for a session within a backend.
type SessionID string

// State represents the current state of an agent session.
type State string

const (
	StateCreating         State = "CREATING"
	StateIdle             State = "IDLE"
	StateWorking          State = "WORKING"
	StatePermissionPrompt State = "PERMISSION_PROMPT"
	StateCompacting       State = "COMPACTING"
	StateArchived         State = "ARCHIVED"
	StateOffline          State = "OFFLINE"
	StateError            State = "ERROR"
)

// SessionConfig holds configuration for creating a new session.
type SessionConfig struct {
	// Name is the human-readable session name.
	Name string

	// WorkingDirectory is the initial working directory for the agent.
	WorkingDirectory string

	// Harness identifies the agent runtime: "claude-code", "gemini-cli", etc.
	Harness string

	// Environment holds additional environment variables for the session.
	Environment map[string]string

	// InitialPrompt is the first message to send after creation (optional).
	InitialPrompt string

	// Detached controls whether to attach a terminal to the session.
	Detached bool
}

// SessionInfo holds metadata about a session.
type SessionInfo struct {
	ID        SessionID
	Name      string
	State     State
	CreatedAt time.Time
	Harness   string
	Attached  bool
}

// SessionFilter defines criteria for listing sessions.
type SessionFilter struct {
	States    []State
	Harnesses []string
	NameMatch string // Glob pattern
	Limit     int
}

// SendResult indicates the outcome of sending a message.
type SendResult struct {
	Delivered bool
	Error     error
}

// StateResult contains state detection results with confidence scoring.
type StateResult struct {
	State      State
	Confidence float64 // 0.0-1.0, where 1.0 = certain
	Evidence   string  // Human-readable explanation of detection method
}

// CanReceive indicates whether a session can accept input right now.
type CanReceive int

const (
	// CanReceiveYes means the session is idle and ready for input.
	CanReceiveYes CanReceive = iota
	// CanReceiveNo means the session is blocked (e.g., permission prompt).
	CanReceiveNo
	// CanReceiveQueue means the session is busy; queue for later delivery.
	CanReceiveQueue
	// CanReceiveNotFound means the session does not exist.
	CanReceiveNotFound
)

// BackendCapabilities declares what a backend supports.
type BackendCapabilities struct {
	// SupportsAttach indicates the backend can attach a terminal for human viewing.
	SupportsAttach bool

	// SupportsStructuredIO indicates structured JSON I/O instead of terminal scraping.
	SupportsStructuredIO bool

	// SupportsInterrupt indicates the backend can interrupt a running operation.
	SupportsInterrupt bool

	// MaxConcurrentSessions is the maximum concurrent sessions (0 = unlimited).
	MaxConcurrentSessions int
}
