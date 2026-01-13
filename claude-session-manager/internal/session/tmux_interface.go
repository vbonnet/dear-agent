package session

// SessionInfo holds information about a tmux session
type SessionInfo struct {
	Name            string
	AttachedClients int // Number of clients attached to this session
}

// TmuxInterface provides an abstraction for tmux operations
// This allows mocking tmux in tests without requiring real tmux to be installed
type TmuxInterface interface {
	// HasSession checks if a tmux session with the given name exists
	HasSession(name string) (bool, error)

	// ListSessions returns all active tmux session names
	ListSessions() ([]string, error)

	// ListSessionsWithInfo returns all active tmux sessions with attachment info
	ListSessionsWithInfo() ([]SessionInfo, error)

	// CreateSession creates a new tmux session with the given name and working directory
	CreateSession(name, workdir string) error

	// AttachSession attaches to or switches to the given tmux session
	AttachSession(name string) error

	// SendKeys sends keys (command) to the given tmux session
	SendKeys(session, keys string) error
}
