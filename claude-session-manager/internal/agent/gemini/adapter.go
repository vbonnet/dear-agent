// Package gemini provides a stub implementation of the agent.Agent interface
// for Google Gemini.
//
// This is a scaffold implementation (V1). All methods return ErrNotImplemented
// except for metadata methods (Name, Version, Capabilities) which return
// accurate Gemini information.
//
// V2 Implementation Plan:
//   - Integrate Gemini API client
//   - Implement session management via Gemini API
//   - Add configuration loading (API key, project ID)
//   - Add error handling and retries
//
// Usage:
//
//	adapter := gemini.NewAdapter()
//	name := adapter.Name() // Returns "gemini"
//	// All other methods return ErrNotImplemented
//
// Example (V2):
//
//	// Future usage once V2 is implemented
//	adapter := gemini.NewAdapter(apiKey, projectID, "gemini-2.0-flash")
//	sessionID, err := adapter.CreateSession(ctx)
package gemini

import (
	"errors"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/agent"
)

// Compile-time interface compliance check
var _ agent.Agent = (*Adapter)(nil)

// ErrNotImplemented is returned by all Adapter methods in V1 scaffold.
// V2 will replace with actual Gemini API implementation.
var ErrNotImplemented = errors.New("not implemented")

// Adapter implements the agent.Agent interface for Google Gemini.
//
// This is a scaffold implementation (V1). All methods return ErrNotImplemented.
// V2 will integrate with the Gemini API client.
type Adapter struct {
	// Configuration fields (placeholders for V2)
	apiKey    string // Gemini API key
	model     string // Model name (e.g., "gemini-2.0-flash")
	projectID string // Google Cloud project ID
}

// NewAdapter creates a new Gemini adapter instance.
//
// Returns an adapter with default model "gemini-2.0-flash".
// All methods return ErrNotImplemented in V1 scaffold.
func NewAdapter() agent.Agent {
	return &Adapter{
		model: "gemini-2.0-flash", // Default model for V2
	}
}

// Name returns the agent identifier.
func (a *Adapter) Name() string {
	return "gemini"
}

// Version returns the model name.
func (a *Adapter) Version() string {
	return a.model
}

// CreateSession is not implemented in V1 scaffold.
func (a *Adapter) CreateSession(ctx agent.SessionContext) (agent.SessionID, error) {
	return "", ErrNotImplemented
}

// ResumeSession is not implemented in V1 scaffold.
func (a *Adapter) ResumeSession(sessionID agent.SessionID) error {
	return ErrNotImplemented
}

// TerminateSession is not implemented in V1 scaffold.
func (a *Adapter) TerminateSession(sessionID agent.SessionID) error {
	return ErrNotImplemented
}

// GetSessionStatus is not implemented in V1 scaffold.
func (a *Adapter) GetSessionStatus(sessionID agent.SessionID) (agent.Status, error) {
	return "", ErrNotImplemented
}

// SendMessage is not implemented in V1 scaffold.
func (a *Adapter) SendMessage(sessionID agent.SessionID, message agent.Message) error {
	return ErrNotImplemented
}

// GetHistory is not implemented in V1 scaffold.
func (a *Adapter) GetHistory(sessionID agent.SessionID) ([]agent.Message, error) {
	return nil, ErrNotImplemented
}

// ExportConversation is not implemented in V1 scaffold.
func (a *Adapter) ExportConversation(sessionID agent.SessionID, format agent.ConversationFormat) ([]byte, error) {
	return nil, ErrNotImplemented
}

// ImportConversation is not implemented in V1 scaffold.
func (a *Adapter) ImportConversation(data []byte, format agent.ConversationFormat) (agent.SessionID, error) {
	return "", ErrNotImplemented
}

// Capabilities returns Gemini's feature set.
//
// Even in V1 scaffold, this returns accurate metadata for Gemini 2.0.
func (a *Adapter) Capabilities() agent.Capabilities {
	return agent.Capabilities{
		SupportsSlashCommands: false,   // API adapter, not CLI
		SupportsHooks:         false,   // V2 feature
		SupportsTools:         true,    // Gemini supports function calling
		SupportsVision:        true,    // Gemini 2.0 has vision
		SupportsMultimodal:    false,   // V2 feature (audio/video)
		MaxContextWindow:      1000000, // Gemini 2.0: 1M+ tokens
		ModelName:             a.model,
	}
}

// ExecuteCommand is not implemented in V1 scaffold.
func (a *Adapter) ExecuteCommand(cmd agent.Command) error {
	return ErrNotImplemented
}
