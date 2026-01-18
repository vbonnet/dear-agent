// Package claude provides a stub implementation of the agent.Agent interface
// for Anthropic Claude.
//
// This is a scaffold implementation (V1). All methods return ErrNotImplemented
// except for metadata methods (Name, Version, Capabilities) which return
// accurate Claude information.
package claude

import (
	"errors"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/agent"
)

// Compile-time interface compliance check
var _ agent.Agent = (*Adapter)(nil)

// ErrNotImplemented is returned by all Adapter methods in V1 scaffold.
var ErrNotImplemented = errors.New("not implemented")

// Adapter implements the agent.Agent interface for Anthropic Claude.
//
// This is a scaffold implementation (V1). All methods return ErrNotImplemented.
type Adapter struct {
	model string // Model name (e.g., "claude-sonnet-4.5")
}

// NewAdapter creates a new Claude adapter instance.
//
// Returns an adapter with default model "claude-sonnet-4.5".
// All methods return ErrNotImplemented in V1 scaffold.
func NewAdapter() agent.Agent {
	return &Adapter{
		model: "claude-sonnet-4.5", // Default model
	}
}

// Name returns the agent identifier.
func (a *Adapter) Name() string {
	return "claude"
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

// Capabilities returns Claude's feature set.
func (a *Adapter) Capabilities() agent.Capabilities {
	return agent.Capabilities{
		SupportsSlashCommands: true,   // Claude CLI supports slash commands
		SupportsHooks:         true,   // AGM supports hooks for Claude
		SupportsTools:         true,   // Claude supports tool use
		SupportsVision:        true,   // Claude Opus/Sonnet have vision
		SupportsMultimodal:    false,  // V2 feature (audio/video)
		MaxContextWindow:      200000, // Claude: 200K tokens
		ModelName:             a.model,
	}
}

// ExecuteCommand is not implemented in V1 scaffold.
func (a *Adapter) ExecuteCommand(cmd agent.Command) error {
	return ErrNotImplemented
}

func init() {
	agent.Register("claude", NewAdapter())
}
