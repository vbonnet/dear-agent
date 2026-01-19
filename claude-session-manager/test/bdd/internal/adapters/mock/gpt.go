package mock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// GPTAdapter is a mock implementation of the GPT adapter
type GPTAdapter struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewGPTAdapter creates a new GPT adapter
func NewGPTAdapter() *GPTAdapter {
	return &GPTAdapter{
		sessions: make(map[string]*Session),
	}
}

// Name returns the adapter name
func (a *GPTAdapter) Name() string {
	return "gpt"
}

// CreateSession creates a new session
func (a *GPTAdapter) CreateSession(ctx context.Context, req CreateSessionRequest) (*Session, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Add agent tag
	_ = append(req.Tags, "agent:gpt") // tags unused in mock

	session := &Session{
		ID:        uuid.NewString(),
		Name:      req.Name,
		Agent:     "gpt",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		State:     StateActive,
		History:   []Message{},
		Context: &SessionContext{
			Attributes: make(map[string]string),
			Messages:   []string{},
		},
	}

	a.sessions[session.ID] = session
	return session, nil
}

// SendMessage sends a message and returns response
func (a *GPTAdapter) SendMessage(ctx context.Context, req SendMessageRequest) (*Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	session, exists := a.sessions[req.SessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found. Verify the session ID with 'csm list'", req.SessionID)
	}

	if session.State == StateArchived {
		return nil, fmt.Errorf("session %s is archived and cannot accept messages. Use 'csm resume' to reactivate", req.SessionID)
	}

	// Store user message in context for recall
	session.Context.Messages = append(session.Context.Messages, req.Content)

	// Append user message to history
	userMsg := Message{
		Role:      RoleUser,
		Content:   req.Content,
		Timestamp: time.Now(),
	}
	session.History = append(session.History, userMsg)

	// Generate response using shared pattern matching
	responseContent, matched := GenerateContextualResponse(session, req.Content)
	if !matched {
		// Fallback: echo with agent name
		responseContent = fmt.Sprintf("GPT received: %s", req.Content)
	}

	// Append assistant message
	assistantMsg := Message{
		Role:      RoleAssistant,
		Content:   responseContent,
		Timestamp: time.Now(),
	}
	session.History = append(session.History, assistantMsg)

	session.UpdatedAt = time.Now()

	return &Response{
		Content:   responseContent,
		Timestamp: time.Now(),
	}, nil
}

// GetHistory retrieves conversation history
func (a *GPTAdapter) GetHistory(ctx context.Context, sessionID string) ([]Message, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	return session.History, nil
}

// PauseSession pauses a session
func (a *GPTAdapter) PauseSession(ctx context.Context, sessionID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	session.State = StatePaused
	session.UpdatedAt = time.Now()
	return nil
}

// ResumeSession resumes a paused session
func (a *GPTAdapter) ResumeSession(ctx context.Context, sessionID string) (*Session, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	if session.State == StateArchived {
		return nil, fmt.Errorf("cannot resume archived session %s", sessionID)
	}

	session.State = StateActive
	session.UpdatedAt = time.Now()
	return session, nil
}

// ArchiveSession archives a session
func (a *GPTAdapter) ArchiveSession(ctx context.Context, sessionID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	session.State = StateArchived
	session.UpdatedAt = time.Now()
	return nil
}

// GetSession retrieves session metadata
func (a *GPTAdapter) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	return session, nil
}
