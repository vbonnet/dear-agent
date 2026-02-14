package temporal

// SessionInfo holds information about a temporal session
// This mirrors the tmux SessionInfo structure for compatibility
type SessionInfo struct {
	Name            string
	AttachedClients int    // Number of clients attached to this session
	AttachedList    string // Comma-separated list of attached client IDs
}

// ClientInfo holds information about a temporal client
// This mirrors the tmux ClientInfo structure for compatibility
type ClientInfo struct {
	SessionName string
	TTY         string
	PID         int
}

// TemporalInterface provides an abstraction for temporal session operations
// This interface mirrors TmuxInterface to provide a compatible backend
// for managing Claude Code sessions via Temporal workflows instead of tmux
type TemporalInterface interface {
	// HasSession checks if a temporal session with the given name exists
	HasSession(name string) (bool, error)

	// ListSessions returns all active temporal session names
	ListSessions() ([]string, error)

	// ListSessionsWithInfo returns all active temporal sessions with attachment info
	ListSessionsWithInfo() ([]SessionInfo, error)

	// ListClients returns all clients attached to a specific session
	ListClients(sessionName string) ([]ClientInfo, error)

	// CreateSession creates a new temporal session with the given name and working directory
	CreateSession(name, workdir string) error

	// AttachSession attaches to or switches to the given temporal session
	AttachSession(name string) error

	// SendKeys sends keys (command) to the given temporal session
	SendKeys(session, keys string) error
}
