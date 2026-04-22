package manager

import "context"

// SessionManager handles session lifecycle operations.
// Implementations must be safe for concurrent use.
type SessionManager interface {
	// CreateSession spawns a new agent session with the given configuration.
	// Returns the session ID assigned by the backend.
	CreateSession(ctx context.Context, config SessionConfig) (SessionID, error)

	// TerminateSession stops a running session. The session process is killed
	// but session metadata may be preserved for archival.
	TerminateSession(ctx context.Context, id SessionID) error

	// ListSessions returns sessions matching the given filter criteria.
	ListSessions(ctx context.Context, filter SessionFilter) ([]SessionInfo, error)

	// GetSession returns metadata for a single session.
	GetSession(ctx context.Context, id SessionID) (SessionInfo, error)

	// RenameSession changes the human-readable name of a session.
	RenameSession(ctx context.Context, id SessionID, name string) error
}

// MessageBroker handles communication with agent sessions.
// Implementations must be safe for concurrent use.
type MessageBroker interface {
	// SendMessage delivers a prompt/message to the agent session.
	// Blocks until delivery is confirmed or fails.
	SendMessage(ctx context.Context, id SessionID, message string) (SendResult, error)

	// ReadOutput returns recent output from the session.
	// The lines parameter controls how many lines of history to return.
	ReadOutput(ctx context.Context, id SessionID, lines int) (string, error)

	// Interrupt sends a cancel/interrupt signal to the session,
	// stopping the current operation.
	Interrupt(ctx context.Context, id SessionID) error
}

// StateReader provides observation of session state.
// Implementations must be safe for concurrent use.
type StateReader interface {
	// GetState returns the current state of a session with confidence scoring.
	// For terminal-based backends, confidence reflects parsing heuristic quality.
	// For structured-IO backends, confidence is always 1.0.
	GetState(ctx context.Context, id SessionID) (StateResult, error)

	// CheckDelivery determines if a session can receive input right now.
	// This is the authority for message delivery decisions.
	CheckDelivery(ctx context.Context, id SessionID) (CanReceive, error)

	// HealthCheck verifies the backend is operational.
	HealthCheck(ctx context.Context) error
}

// AttachableBackend extends SessionManager for backends that support
// direct user interaction (e.g., tmux terminal attachment).
type AttachableBackend interface {
	SessionManager

	// AttachSession connects the current terminal to the session for
	// interactive viewing/control.
	AttachSession(ctx context.Context, id SessionID) error
}

// Backend combines all three core interfaces plus identity.
// This is the interface that backend implementations register.
type Backend interface {
	SessionManager
	MessageBroker
	StateReader

	// Name returns the backend identifier (e.g., "tmux", "process").
	Name() string

	// Capabilities returns what this backend supports.
	Capabilities() BackendCapabilities
}
