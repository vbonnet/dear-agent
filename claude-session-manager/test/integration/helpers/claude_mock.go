//go:build integration

package helpers

import (
	"sync"

	"github.com/google/uuid"
)

// MockClaude implements ClaudeInterface for testing without real Claude
type MockClaude struct {
	StartedSessions map[string]string // sessionName -> UUID
	mu              sync.RWMutex
}

// Start simulates Claude starting in a session
func (m *MockClaude) Start(sessionName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.StartedSessions == nil {
		m.StartedSessions = make(map[string]string)
	}

	// Simulate Claude starting (instant in mock)
	m.StartedSessions[sessionName] = uuid.New().String()
	return nil
}

// IsReady checks if Claude is ready (simulated)
func (m *MockClaude) IsReady(sessionName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, started := m.StartedSessions[sessionName]
	return started
}

// Stop simulates stopping Claude in a session
func (m *MockClaude) Stop(sessionName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.StartedSessions, sessionName)
	return nil
}

// GetSessionUUID returns the mock UUID for a session (test helper)
func (m *MockClaude) GetSessionUUID(sessionName string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.StartedSessions[sessionName]
}
