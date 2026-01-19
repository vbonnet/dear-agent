package mock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// GeminiAdapter is a mock implementation of the Gemini adapter
type GeminiAdapter struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewGeminiAdapter creates a new Gemini adapter
func NewGeminiAdapter() *GeminiAdapter {
	return &GeminiAdapter{
		sessions: make(map[string]*Session),
	}
}

// Name returns the adapter name
func (a *GeminiAdapter) Name() string {
	return "gemini"
}

// CreateSession creates a new session
func (a *GeminiAdapter) CreateSession(ctx context.Context, req CreateSessionRequest) (*Session, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Add agent tag
	_ = append(req.Tags, "agent:gemini") // tags unused in mock

	session := &Session{
		ID:        uuid.NewString(),
		Name:      req.Name,
		Agent:     "gemini",
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
func (a *GeminiAdapter) SendMessage(ctx context.Context, req SendMessageRequest) (*Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	session, exists := a.sessions[req.SessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", req.SessionID)
	}

	if session.State == StateArchived {
		return nil, fmt.Errorf("session %s is archived", req.SessionID)
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
		responseContent = fmt.Sprintf("Gemini received: %s", req.Content)
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
func (a *GeminiAdapter) GetHistory(ctx context.Context, sessionID string) ([]Message, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	return session.History, nil
}

// PauseSession pauses a session
func (a *GeminiAdapter) PauseSession(ctx context.Context, sessionID string) error {
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
func (a *GeminiAdapter) ResumeSession(ctx context.Context, sessionID string) (*Session, error) {
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
func (a *GeminiAdapter) ArchiveSession(ctx context.Context, sessionID string) error {
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
func (a *GeminiAdapter) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	return session, nil
}
