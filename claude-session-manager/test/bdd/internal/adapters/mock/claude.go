package mock

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ClaudeAdapter is a mock implementation of the Claude adapter
type ClaudeAdapter struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewClaudeAdapter creates a new Claude adapter
func NewClaudeAdapter() *ClaudeAdapter {
	return &ClaudeAdapter{
		sessions: make(map[string]*Session),
	}
}

// Name returns the adapter name
func (a *ClaudeAdapter) Name() string {
	return "claude"
}

// CreateSession creates a new session
func (a *ClaudeAdapter) CreateSession(ctx context.Context, req CreateSessionRequest) (*Session, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Add agent tag
	_ = append(req.Tags, "agent:claude") // tags unused in mock

	session := &Session{
		ID:        uuid.NewString(),
		Name:      req.Name,
		Agent:     "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		State:     StateActive,
		History:   []Message{},
	}

	a.sessions[session.ID] = session
	return session, nil
}

// SendMessage sends a message and returns response
func (a *ClaudeAdapter) SendMessage(ctx context.Context, req SendMessageRequest) (*Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	session, exists := a.sessions[req.SessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", req.SessionID)
	}

	if session.State == StateArchived {
		return nil, fmt.Errorf("session %s is archived", req.SessionID)
	}

	// Append user message
	userMsg := Message{
		Role:      RoleUser,
		Content:   req.Content,
		Timestamp: time.Now(),
	}
	session.History = append(session.History, userMsg)

	// Generate response (deterministic)
	responseContent := a.generateResponse(session, req.Content)

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

// generateResponse generates deterministic responses based on message content
func (a *ClaudeAdapter) generateResponse(session *Session, message string) string {
	// Pattern 1: "My name is X" → remember name
	if strings.Contains(strings.ToLower(message), "my name is") {
		return "Nice to meet you! I'll remember your name."
	}

	// Pattern 2: "What is my name?" → recall from history
	if strings.Contains(strings.ToLower(message), "what is my name") {
		name := extractNameFromHistory(session.History)
		if name != "" {
			return fmt.Sprintf("Your name is %s.", name)
		}
		return "I don't know your name. You haven't told me yet."
	}

	// Default: Echo response
	return fmt.Sprintf("Claude received: %s", message)
}

// extractNameFromHistory extracts a name from conversation history
func extractNameFromHistory(history []Message) string {
	for _, msg := range history {
		if msg.Role == RoleUser {
			// Simple pattern: "My name is Alice"
			re := regexp.MustCompile(`my name is (\w+)`)
			matches := re.FindStringSubmatch(strings.ToLower(msg.Content))
			if len(matches) > 1 {
				// Capitalize first letter
				name := matches[1]
				return strings.ToUpper(name[:1]) + name[1:]
			}
		}
	}
	return ""
}

// GetHistory retrieves conversation history
func (a *ClaudeAdapter) GetHistory(ctx context.Context, sessionID string) ([]Message, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	return session.History, nil
}

// PauseSession pauses a session
func (a *ClaudeAdapter) PauseSession(ctx context.Context, sessionID string) error {
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
func (a *ClaudeAdapter) ResumeSession(ctx context.Context, sessionID string) (*Session, error) {
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
func (a *ClaudeAdapter) ArchiveSession(ctx context.Context, sessionID string) error {
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
func (a *ClaudeAdapter) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	return session, nil
}
