// Package mock provides mock adapters for BDD testing
package mock

import (
	"context"
	"time"
)

// Adapter represents an agent adapter (Claude, Gemini, etc.)
type Adapter interface {
	// Name returns the adapter name ("claude", "gemini")
	Name() string

	// CreateSession creates a new session
	CreateSession(ctx context.Context, req CreateSessionRequest) (*Session, error)

	// SendMessage sends a message and returns response
	SendMessage(ctx context.Context, req SendMessageRequest) (*Response, error)

	// GetHistory retrieves conversation history
	GetHistory(ctx context.Context, sessionID string) ([]Message, error)

	// PauseSession pauses a session (saves state)
	PauseSession(ctx context.Context, sessionID string) error

	// ResumeSession resumes a paused session
	ResumeSession(ctx context.Context, sessionID string) (*Session, error)

	// ArchiveSession archives a session
	ArchiveSession(ctx context.Context, sessionID string) error

	// GetSession retrieves session metadata
	GetSession(ctx context.Context, sessionID string) (*Session, error)
}

// CreateSessionRequest holds session creation parameters
type CreateSessionRequest struct {
	Name    string
	Project string
	Tags    []string
}

// SendMessageRequest holds message parameters
type SendMessageRequest struct {
	SessionID string
	Content   string
}

// Session represents a session
type Session struct {
	ID        string
	Name      string
	Agent     string
	CreatedAt time.Time
	UpdatedAt time.Time
	State     SessionState
	History   []Message
	Context   *SessionContext // Context for conversation state
}

// SessionState represents session lifecycle state
type SessionState string

const (
	// StateActive represents an active session
	StateActive SessionState = "active"
	// StatePaused represents a paused session
	StatePaused SessionState = "paused"
	// StateArchived represents an archived session
	StateArchived SessionState = "archived"
)

// Message represents a conversation message
type Message struct {
	Role      MessageRole
	Content   string
	Timestamp time.Time
}

// MessageRole represents message sender
type MessageRole string

const (
	// RoleUser represents a user message
	RoleUser MessageRole = "user"
	// RoleAssistant represents an assistant message
	RoleAssistant MessageRole = "assistant"
)

// Response represents an agent response
type Response struct {
	Content   string
	Timestamp time.Time
}
