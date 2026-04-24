package session

// MockTmux provides an in-memory mock implementation of TmuxInterface for testing
type MockTmux struct {
	// Sessions maps session name to whether it exists
	Sessions map[string]bool

	// CreatedSessions tracks the order in which sessions were created
	CreatedSessions []string

	// SentCommands tracks commands sent via SendKeys
	SentCommands []string

	// Errors can be set to simulate tmux failures
	HasSessionError    error
	ListSessionsError  error
	CreateSessionError error
	AttachSessionError error
	SendKeysError      error
}

// NewMockTmux creates a new MockTmux instance
func NewMockTmux() *MockTmux {
	return &MockTmux{
		Sessions:        make(map[string]bool),
		CreatedSessions: []string{},
		SentCommands:    []string{},
	}
}

// HasSession checks if a session exists in the mock
func (m *MockTmux) HasSession(name string) (bool, error) {
	if m.HasSessionError != nil {
		return false, m.HasSessionError
	}

	exists, ok := m.Sessions[name]
	if !ok {
		return false, nil
	}
	return exists, nil
}

// ListSessions returns all active sessions in the mock
func (m *MockTmux) ListSessions() ([]string, error) {
	if m.ListSessionsError != nil {
		return nil, m.ListSessionsError
	}

	sessions := []string{}
	for name, exists := range m.Sessions {
		if exists {
			sessions = append(sessions, name)
		}
	}
	return sessions, nil
}

// ListSessionsWithInfo returns all active sessions with attachment info (mock returns 0 attached)
func (m *MockTmux) ListSessionsWithInfo() ([]SessionInfo, error) {
	if m.ListSessionsError != nil {
		return nil, m.ListSessionsError
	}

	sessions := []SessionInfo{}
	for name, exists := range m.Sessions {
		if exists {
			sessions = append(sessions, SessionInfo{
				Name:            name,
				AttachedClients: 0,  // Mock doesn't track attachment
				AttachedList:    "", // Mock doesn't track TTYs
			})
		}
	}
	return sessions, nil
}

// CreateSession creates a session in the mock
func (m *MockTmux) CreateSession(name, workdir string) error {
	if m.CreateSessionError != nil {
		return m.CreateSessionError
	}

	m.Sessions[name] = true
	m.CreatedSessions = append(m.CreatedSessions, name)
	return nil
}

// AttachSession is a no-op in the mock
func (m *MockTmux) AttachSession(name string) error {
	if m.AttachSessionError != nil {
		return m.AttachSessionError
	}
	return nil
}

// SendKeys records the command in the mock
func (m *MockTmux) SendKeys(session, keys string) error {
	if m.SendKeysError != nil {
		return m.SendKeysError
	}

	m.SentCommands = append(m.SentCommands, keys)
	return nil
}

// ListClients returns empty list in the mock (clients not tracked)
func (m *MockTmux) ListClients(sessionName string) ([]ClientInfo, error) {
	return []ClientInfo{}, nil
}
