// Package testenv provides test environment management for BDD tests
package testenv

import (
	"fmt"
	"testing"

	"github.com/vbonnet/dear-agent/agm/test/bdd/internal/adapters/mock"
)

// Type aliases for easier use in step definitions
type CreateSessionRequest = mock.CreateSessionRequest
type SendMessageRequest = mock.SendMessageRequest

// Environment manages test state and adapters
type Environment struct {
	T                  *testing.T
	ClaudeAdapter      mock.Adapter
	GeminiAdapter      mock.Adapter
	CodexAdapter       mock.Adapter
	OpenCodeAdapter    mock.Adapter
	CurrentAdapter     mock.Adapter // Currently selected adapter
	CurrentSession     *mock.Session
	Sessions           map[string]*mock.Session // Track all sessions by name
	LastResponse       *mock.Response
	LastError          error
	FirstMessage       string      // Store first message for sequential message tests
	AssociationContext interface{} // Context for association tests
}

// NewEnvironment creates a new test environment
func NewEnvironment(t *testing.T) *Environment {
	return &Environment{
		T:               t,
		ClaudeAdapter:   mock.NewClaudeAdapter(),
		GeminiAdapter:   mock.NewGeminiAdapter(),
		CodexAdapter:    mock.NewCodexAdapter(),
		OpenCodeAdapter: mock.NewOpenCodeAdapter(),
		Sessions:        make(map[string]*mock.Session),
	}
}

// GetAdapter returns the adapter for the given agent name
func (e *Environment) GetAdapter(name string) (mock.Adapter, error) {
	switch name {
	case "claude":
		return e.ClaudeAdapter, nil
	case "gemini":
		return e.GeminiAdapter, nil
	case "codex":
		return e.CodexAdapter, nil
	case "opencode":
		return e.OpenCodeAdapter, nil
	default:
		return nil, fmt.Errorf("unknown adapter: %s", name)
	}
}

// Cleanup resets state between scenarios
func (e *Environment) Cleanup() {
	e.CurrentSession = nil
	e.Sessions = make(map[string]*mock.Session)
	e.LastResponse = nil
	e.LastError = nil
	e.FirstMessage = ""
	e.AssociationContext = nil
}
