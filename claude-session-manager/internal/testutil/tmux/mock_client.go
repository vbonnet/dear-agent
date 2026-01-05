package tmux

import "time"

// MockClient is a mock implementation of Client for testing
type MockClient struct {
	CreateSessionFunc  func(name, workingDir string) error
	SendKeysFunc       func(sessionName, keys string) error
	CapturePaneFunc    func(sessionName string, lines int) (string, error)
	KillSessionFunc    func(sessionName string) error
	HasSessionFunc     func(sessionName string) bool
	WaitForStartupFunc func(sessionName string, timeout time.Duration) error

	// Call tracking
	CreateSessionCalls  []CreateSessionCall
	SendKeysCalls       []SendKeysCall
	CapturePaneCalls    []CapturePaneCall
	KillSessionCalls    []string
	HasSessionCalls     []string
	WaitForStartupCalls []WaitForStartupCall
}

type CreateSessionCall struct {
	Name       string
	WorkingDir string
}

type SendKeysCall struct {
	SessionName string
	Keys        string
}

type CapturePaneCall struct {
	SessionName string
	Lines       int
}

type WaitForStartupCall struct {
	SessionName string
	Timeout     time.Duration
}

// NewMock creates a new mock client with default no-op implementations
func NewMock() *MockClient {
	return &MockClient{
		CreateSessionFunc:  func(string, string) error { return nil },
		SendKeysFunc:       func(string, string) error { return nil },
		CapturePaneFunc:    func(string, int) (string, error) { return "", nil },
		KillSessionFunc:    func(string) error { return nil },
		HasSessionFunc:     func(string) bool { return false },
		WaitForStartupFunc: func(string, time.Duration) error { return nil },
	}
}

func (m *MockClient) CreateSession(name, workingDir string) error {
	m.CreateSessionCalls = append(m.CreateSessionCalls, CreateSessionCall{
		Name:       name,
		WorkingDir: workingDir,
	})
	return m.CreateSessionFunc(name, workingDir)
}

func (m *MockClient) SendKeys(sessionName, keys string) error {
	m.SendKeysCalls = append(m.SendKeysCalls, SendKeysCall{
		SessionName: sessionName,
		Keys:        keys,
	})
	return m.SendKeysFunc(sessionName, keys)
}

func (m *MockClient) CapturePane(sessionName string, lines int) (string, error) {
	m.CapturePaneCalls = append(m.CapturePaneCalls, CapturePaneCall{
		SessionName: sessionName,
		Lines:       lines,
	})
	return m.CapturePaneFunc(sessionName, lines)
}

func (m *MockClient) KillSession(sessionName string) error {
	m.KillSessionCalls = append(m.KillSessionCalls, sessionName)
	return m.KillSessionFunc(sessionName)
}

func (m *MockClient) HasSession(sessionName string) bool {
	m.HasSessionCalls = append(m.HasSessionCalls, sessionName)
	return m.HasSessionFunc(sessionName)
}

func (m *MockClient) WaitForStartup(sessionName string, timeout time.Duration) error {
	m.WaitForStartupCalls = append(m.WaitForStartupCalls, WaitForStartupCall{
		SessionName: sessionName,
		Timeout:     timeout,
	})
	return m.WaitForStartupFunc(sessionName, timeout)
}
