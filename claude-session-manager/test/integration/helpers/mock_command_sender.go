//go:build integration

package helpers

import (
	"sync"
)

// MockCommandSender records commands sent to tmux for test verification
type MockCommandSender struct {
	CommandsSent    []string
	UsedLiteralMode bool
	mutex           sync.Mutex
}

// SendCommand records a command without executing it
func (m *MockCommandSender) SendCommand(session string, cmd string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.CommandsSent = append(m.CommandsSent, cmd)
	return nil
}

// SendPromptLiteral records a literal mode send
func (m *MockCommandSender) SendPromptLiteral(sessionName string, text string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.UsedLiteralMode = true
	m.CommandsSent = append(m.CommandsSent, text)
	return nil
}

// Reset clears recorded commands
func (m *MockCommandSender) Reset() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.CommandsSent = []string{}
	m.UsedLiteralMode = false
}
