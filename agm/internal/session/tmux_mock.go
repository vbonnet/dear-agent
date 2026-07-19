package session

import "time"

// MockTmux provides an in-memory mock implementation of TmuxInterface for testing
type MockTmux struct {
	// Sessions maps session name to whether it exists
	Sessions map[string]bool

	// CreatedSessions tracks the order in which sessions were created
	CreatedSessions []string

	// SentCommands tracks commands sent via SendKeys
	SentCommands []string
	// Readiness checks track shared lifecycle gating in tests.
	WaitedHarnesses      []string
	CheckedInputSessions []string
	InputReadiness       InputReadiness

	// Errors can be set to simulate tmux failures
	HasSessionError          error
	ListSessionsError        error
	CreateSessionError       error
	KillSessionError         error
	AttachSessionError       error
	SendKeysError            error
	WaitForHarnessReadyError error
	InputReadinessError      error
}

// NewMockTmux creates a new MockTmux instance
func NewMockTmux() *MockTmux {
	return &MockTmux{
		Sessions:        make(map[string]bool),
		CreatedSessions: []string{},
		SentCommands:    []string{},
		InputReadiness:  InputReadiness{Ready: true, State: "YES"},
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

// KillSession removes a mock session. It implements TmuxSessionKiller so
// creation rollback is contract-testable without a real tmux server.
func (m *MockTmux) KillSession(name string) error {
	if m.KillSessionError != nil {
		return m.KillSessionError
	}
	delete(m.Sessions, name)
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

// WaitForHarnessReady records the requested harness and returns the configured error.
func (m *MockTmux) WaitForHarnessReady(sessionName, harness string, _ time.Duration) error {
	m.WaitedHarnesses = append(m.WaitedHarnesses, sessionName+":"+harness)
	return m.WaitForHarnessReadyError
}

// CheckInputReadiness records the target and returns the configured readiness result.
func (m *MockTmux) CheckInputReadiness(sessionName, harness string) (InputReadiness, error) {
	m.CheckedInputSessions = append(m.CheckedInputSessions, sessionName+":"+harness)
	if m.InputReadinessError != nil {
		return InputReadiness{}, m.InputReadinessError
	}
	return m.InputReadiness, nil
}

// ListClients returns empty list in the mock (clients not tracked)
func (m *MockTmux) ListClients(sessionName string) ([]ClientInfo, error) {
	return []ClientInfo{}, nil
}
