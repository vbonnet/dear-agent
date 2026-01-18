package agent

import (
	"fmt"
)

// GeminiAdapter implements the Agent interface for Google Gemini
type GeminiAdapter struct {
	// Uses Google Gemini SDK with client-side history persistence
}

// NewGeminiAdapter creates a new Gemini adapter instance
func NewGeminiAdapter() Agent {
	return &GeminiAdapter{}
}

// Name returns the agent identifier
func (a *GeminiAdapter) Name() string {
	return "gemini"
}

// Version returns the model name
func (a *GeminiAdapter) Version() string {
	return "gemini-1.5-pro"
}

// CreateSession creates a new Gemini session
// TODO: Implement with Gemini SDK
func (a *GeminiAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
	return "", fmt.Errorf("not implemented: Gemini adapter CreateSession")
}

// ResumeSession resumes an existing Gemini session
// TODO: Implement with history loading
func (a *GeminiAdapter) ResumeSession(sessionID SessionID) error {
	return fmt.Errorf("not implemented: Gemini adapter ResumeSession")
}

// TerminateSession terminates a Gemini session
func (a *GeminiAdapter) TerminateSession(sessionID SessionID) error {
	return fmt.Errorf("not implemented: Gemini adapter TerminateSession")
}

// GetSessionStatus returns the status of a Gemini session
func (a *GeminiAdapter) GetSessionStatus(sessionID SessionID) (Status, error) {
	return StatusActive, fmt.Errorf("not implemented: Gemini adapter GetSessionStatus")
}

// SendMessage sends a message to Gemini
func (a *GeminiAdapter) SendMessage(sessionID SessionID, message Message) error {
	return fmt.Errorf("not implemented: Gemini adapter SendMessage")
}

// GetHistory retrieves conversation history
func (a *GeminiAdapter) GetHistory(sessionID SessionID) ([]Message, error) {
	return nil, fmt.Errorf("not implemented: Gemini adapter GetHistory")
}

// ExportConversation exports conversation in specified format
func (a *GeminiAdapter) ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error) {
	return nil, fmt.Errorf("not implemented: Gemini adapter ExportConversation")
}

// ImportConversation imports conversation from serialized data
func (a *GeminiAdapter) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
	return "", fmt.Errorf("not implemented: Gemini adapter ImportConversation")
}

// Capabilities returns Gemini's feature capabilities
func (a *GeminiAdapter) Capabilities() Capabilities {
	return Capabilities{
		SupportsSlashCommands: false, // API agent, no CLI slash commands
		SupportsHooks:         false,
		SupportsTools:         true,
		SupportsVision:        true,
		SupportsMultimodal:    false,
		MaxContextWindow:      1000000, // 1M tokens
		ModelName:             "gemini-1.5-pro",
	}
}

// ExecuteCommand executes a generic command
func (a *GeminiAdapter) ExecuteCommand(cmd Command) error {
	return fmt.Errorf("not implemented: Gemini adapter ExecuteCommand")
}
