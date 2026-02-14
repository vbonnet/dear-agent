package temporal

// Compile-time check to ensure TemporalClient implements TemporalInterface
var _ TemporalInterface = (*TemporalClient)(nil)

// TemporalClient implements the TemporalInterface using Temporal workflows
// This is a stub implementation that will be completed in future tasks
type TemporalClient struct {
	// sessions holds the in-memory session state for stub implementation
	sessions map[string]*sessionState
}

// sessionState holds internal state for a temporal session
type sessionState struct {
	name     string
	workdir  string
	clients  []ClientInfo
	attached bool
}

// NewTemporalClient creates a new TemporalClient instance
// This initializes the client with empty state for stub implementation
func NewTemporalClient() *TemporalClient {
	return &TemporalClient{
		sessions: make(map[string]*sessionState),
	}
}

// HasSession checks if a temporal session exists
// Stub implementation: returns false for all sessions
func (c *TemporalClient) HasSession(name string) (bool, error) {
	_, exists := c.sessions[name]
	return exists, nil
}

// ListSessions returns all active temporal session names
// Stub implementation: returns empty list
func (c *TemporalClient) ListSessions() ([]string, error) {
	sessions := make([]string, 0, len(c.sessions))
	for name := range c.sessions {
		sessions = append(sessions, name)
	}
	return sessions, nil
}

// ListSessionsWithInfo returns all active temporal sessions with attachment info
// Stub implementation: returns empty list
func (c *TemporalClient) ListSessionsWithInfo() ([]SessionInfo, error) {
	sessions := make([]SessionInfo, 0, len(c.sessions))
	for _, state := range c.sessions {
		sessions = append(sessions, SessionInfo{
			Name:            state.name,
			AttachedClients: len(state.clients),
			AttachedList:    "", // Will be populated with client IDs in full implementation
		})
	}
	return sessions, nil
}

// ListClients returns all clients attached to a specific session
// Stub implementation: returns empty list
func (c *TemporalClient) ListClients(sessionName string) ([]ClientInfo, error) {
	state, exists := c.sessions[sessionName]
	if !exists {
		return []ClientInfo{}, nil
	}
	return state.clients, nil
}

// CreateSession creates a new temporal session with the given name and working directory
// Stub implementation: no-op, returns nil
func (c *TemporalClient) CreateSession(name, workdir string) error {
	c.sessions[name] = &sessionState{
		name:     name,
		workdir:  workdir,
		clients:  []ClientInfo{},
		attached: false,
	}
	return nil
}

// AttachSession attaches to or switches to the given temporal session
// Stub implementation: no-op, returns nil
func (c *TemporalClient) AttachSession(name string) error {
	state, exists := c.sessions[name]
	if !exists {
		return nil
	}
	state.attached = true
	return nil
}

// SendKeys sends keys (command) to the given temporal session
// Stub implementation: no-op, returns nil
func (c *TemporalClient) SendKeys(session, keys string) error {
	// Stub: In full implementation, this would send commands via Temporal workflow
	return nil
}
