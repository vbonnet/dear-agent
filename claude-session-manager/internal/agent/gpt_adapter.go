package agent

import (
	"fmt"
)

// GPTAdapter implements the Agent interface for OpenAI GPT
type GPTAdapter struct {
	// Uses OpenAI SDK with client-side history persistence
}

// NewGPTAdapter creates a new GPT adapter instance
func NewGPTAdapter() Agent {
	return &GPTAdapter{}
}

// Name returns the agent identifier
func (a *GPTAdapter) Name() string {
	return "gpt"
}

// Version returns the model name
func (a *GPTAdapter) Version() string {
	return "gpt-4o"
}

// CreateSession creates a new GPT session
// TODO: Implement with OpenAI SDK
func (a *GPTAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
	return "", fmt.Errorf("not implemented: GPT adapter CreateSession")
}

// ResumeSession resumes an existing GPT session
// TODO: Implement with history loading
func (a *GPTAdapter) ResumeSession(sessionID SessionID) error {
	return fmt.Errorf("not implemented: GPT adapter ResumeSession")
}

// TerminateSession terminates a GPT session
func (a *GPTAdapter) TerminateSession(sessionID SessionID) error {
	return fmt.Errorf("not implemented: GPT adapter TerminateSession")
}

// GetSessionStatus returns the status of a GPT session
func (a *GPTAdapter) GetSessionStatus(sessionID SessionID) (Status, error) {
	return StatusActive, fmt.Errorf("not implemented: GPT adapter GetSessionStatus")
}

// SendMessage sends a message to GPT
func (a *GPTAdapter) SendMessage(sessionID SessionID, message Message) error {
	return fmt.Errorf("not implemented: GPT adapter SendMessage")
}

// GetHistory retrieves conversation history
func (a *GPTAdapter) GetHistory(sessionID SessionID) ([]Message, error) {
	return nil, fmt.Errorf("not implemented: GPT adapter GetHistory")
}

// ExportConversation exports conversation in specified format
func (a *GPTAdapter) ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error) {
	return nil, fmt.Errorf("not implemented: GPT adapter ExportConversation")
}

// ImportConversation imports conversation from serialized data
func (a *GPTAdapter) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
	return "", fmt.Errorf("not implemented: GPT adapter ImportConversation")
}

// Capabilities returns GPT's feature capabilities
func (a *GPTAdapter) Capabilities() Capabilities {
	return Capabilities{
		SupportsSlashCommands: false, // API agent, no CLI slash commands
		SupportsHooks:         false,
		SupportsTools:         true,
		SupportsVision:        true,
		SupportsMultimodal:    false,
		MaxContextWindow:      128000, // 128K tokens
		ModelName:             "gpt-4o",
	}
}

// ExecuteCommand executes a generic command
func (a *GPTAdapter) ExecuteCommand(cmd Command) error {
	return fmt.Errorf("not implemented: GPT adapter ExecuteCommand")
}
