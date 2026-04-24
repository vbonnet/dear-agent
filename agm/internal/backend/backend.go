package backend

// Backend defines the unified interface for session management backends
// This interface abstracts session operations across different implementations
// (tmux, Temporal, etc.) allowing AGM to switch between backends via configuration
type Backend interface {
	// HasSession checks if a session with the given name exists
	HasSession(name string) (bool, error)

	// ListSessions returns all active session names
	ListSessions() ([]string, error)

	// ListSessionsWithInfo returns all active sessions with attachment info
	ListSessionsWithInfo() ([]SessionInfo, error)

	// ListClients returns all clients attached to a specific session
	ListClients(sessionName string) ([]ClientInfo, error)

	// CreateSession creates a new session with the given name and working directory
	CreateSession(name, workdir string) error

	// AttachSession attaches to or switches to the given session
	AttachSession(name string) error

	// SendKeys sends keys (command) to the given session
	SendKeys(session, keys string) error
}

// SessionInfo holds information about a session
// This is a backend-agnostic structure that all backends must support
type SessionInfo struct {
	Name            string
	AttachedClients int    // Number of clients attached to this session
	AttachedList    string // Comma-separated list of attached client IDs/TTYs
}

// ClientInfo holds information about a client attached to a session
// This is a backend-agnostic structure that all backends must support
type ClientInfo struct {
	SessionName string
	TTY         string
	PID         int
}
