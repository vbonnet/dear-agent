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
func (a *GPTAdapter) generateResponse(session *Session, message string) string {
	msgLower := strings.ToLower(message)

	// Pattern 1: "My name is X" → remember name
	if strings.Contains(msgLower, "my name is") {
		return "Nice to meet you! I'll remember your name."
	}

	// Pattern 2: "What is my name?" → recall from history
	if strings.Contains(msgLower, "what is my name") {
		name := extractNameFromHistoryGPT(session.History)
		if name != "" {
			return fmt.Sprintf("Your name is %s.", name)
		}
		return "I don't know your name. You haven't told me yet."
	}

	// Pattern 3: "explain" → verbose multi-line response (GPT-specific)
	if strings.Contains(msgLower, "explain") {
		topic := extractTopicAfterExplain(message)
		return fmt.Sprintf(`GPT received: %s

Let me break this down:
1. GPT provides structured information
2. This demonstrates GPT's verbose style
3. You asked about: %s`, message, topic)
	}

	// Pattern 4: "recall first" → reference first message in history
	if strings.Contains(msgLower, "recall first") {
		firstMsg := extractFirstUserMessage(session.History)
		if firstMsg != "" {
			return fmt.Sprintf("GPT received: %s\n\nThe first message was: %s", message, firstMsg)
		}
		return fmt.Sprintf("GPT received: %s\n\nNo previous messages found.", message)
	}

	// Default: Echo response
	return fmt.Sprintf("GPT received: %s", message)
}

// extractNameFromHistoryGPT extracts a name from conversation history
func extractNameFromHistoryGPT(history []Message) string {
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

// extractTopicAfterExplain extracts topic from "explain X" message
func extractTopicAfterExplain(message string) string {
	msgLower := strings.ToLower(message)
	idx := strings.Index(msgLower, "explain")
	if idx == -1 {
		return "the topic"
	}

	// Get text after "explain"
	after := strings.TrimSpace(message[idx+7:])
	if after == "" {
		return "this concept"
	}

	// Take first word/phrase
	words := strings.Fields(after)
	if len(words) > 0 {
		return words[0]
	}

	return "this"
}

// extractFirstUserMessage gets the first user message from history
func extractFirstUserMessage(history []Message) string {
	for _, msg := range history {
		if msg.Role == RoleUser {
			return msg.Content
		}
	}
	return ""
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
