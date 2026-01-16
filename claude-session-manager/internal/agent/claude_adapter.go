package agent

import (
	"fmt"
)

// ClaudeAdapter wraps existing Claude CLI integration
type ClaudeAdapter struct {
	// Uses existing AGM tmux + claude CLI infrastructure
}

// NewClaudeAdapter creates a new Claude adapter instance
func NewClaudeAdapter() Agent {
	return &ClaudeAdapter{}
}

// Name returns the agent identifier
func (a *ClaudeAdapter) Name() string {
	return "claude"
}

// Version returns the model name
func (a *ClaudeAdapter) Version() string {
	return "claude-sonnet-4.5"
}

// CreateSession creates a new Claude session
// TODO: Delegate to existing AGM session creation logic
func (a *ClaudeAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
	return "", fmt.Errorf("not implemented: Claude adapter CreateSession")
}

// ResumeSession resumes an existing Claude session
// TODO: Delegate to existing AGM resume logic
func (a *ClaudeAdapter) ResumeSession(sessionID SessionID) error {
	return fmt.Errorf("not implemented: Claude adapter ResumeSession")
}

// TerminateSession terminates a Claude session
func (a *ClaudeAdapter) TerminateSession(sessionID SessionID) error {
	return fmt.Errorf("not implemented: Claude adapter TerminateSession")
}

// GetSessionStatus returns the status of a Claude session
func (a *ClaudeAdapter) GetSessionStatus(sessionID SessionID) (Status, error) {
	return StatusActive, fmt.Errorf("not implemented: Claude adapter GetSessionStatus")
}

// SendMessage sends a message to Claude
func (a *ClaudeAdapter) SendMessage(sessionID SessionID, message Message) error {
	return fmt.Errorf("not implemented: Claude adapter SendMessage")
}

// GetHistory retrieves conversation history
func (a *ClaudeAdapter) GetHistory(sessionID SessionID) ([]Message, error) {
	return nil, fmt.Errorf("not implemented: Claude adapter GetHistory")
}

// ExportConversation exports conversation in specified format
func (a *ClaudeAdapter) ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error) {
	return nil, fmt.Errorf("not implemented: Claude adapter ExportConversation")
}

// ImportConversation imports conversation from serialized data
func (a *ClaudeAdapter) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
	return "", fmt.Errorf("not implemented: Claude adapter ImportConversation")
}

// Capabilities returns Claude's feature capabilities
func (a *ClaudeAdapter) Capabilities() Capabilities {
	return Capabilities{
		SupportsSlashCommands: true, // Claude CLI supports /rename, /clear, etc.
		SupportsHooks:         false,
		SupportsTools:         true,
		SupportsVision:        true,
		SupportsMultimodal:    false,
		MaxContextWindow:      200000, // 200K tokens
		ModelName:             "claude-sonnet-4.5",
	}
}

// ExecuteCommand executes a generic command
func (a *ClaudeAdapter) ExecuteCommand(cmd Command) error {
	return fmt.Errorf("not implemented: Claude adapter ExecuteCommand")
}
