// Package gpt provides a GPT adapter implementation for the agent.Agent interface.
//
// This adapter integrates with OpenAI's GPT API using sashabaranov/go-openai SDK.
// It implements full session management with in-memory storage.
//
// V1 Implementation:
//   - All 12 Agent interface methods implemented
//   - In-memory session storage (no persistence)
//   - JSONL export/import support
//   - Exponential backoff for rate limits
//
// V2 Roadmap:
//   - File-based session persistence
//   - Streaming support
//   - Tool/function calling
//   - Vision input handling
//
// Usage:
//
//	adapter, err := gpt.NewAdapter()
//	if err != nil {
//	    log.Fatal(err) // OPENAI_API_KEY not set
//	}
//
//	ctx := agent.SessionContext{
//	    Name:             "my-session",
//	    WorkingDirectory: "/home/user/project",
//	}
//	sessionID, err := adapter.CreateSession(ctx)
//	// ... use session
package gpt

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/agent"
)

// Compile-time interface compliance check.
var _ agent.Agent = (*Adapter)(nil)

// Adapter implements the agent.Agent interface for OpenAI GPT.
type Adapter struct {
	client   *openai.Client
	sessions map[agent.SessionID]*Session
	mu       sync.RWMutex
	model    string
}

// NewAdapter creates a new GPT adapter instance.
//
// Returns error if OPENAI_API_KEY environment variable is not set.
func NewAdapter() (agent.Agent, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, ErrAPIKeyNotSet
	}

	client := openai.NewClient(apiKey)

	return &Adapter{
		client:   client,
		sessions: make(map[agent.SessionID]*Session),
		model:    openai.GPT4o,
	}, nil
}

func init() {
	// Register GPT adapter (registration errors ignored, will fail at Get time)
	adapter, _ := NewAdapter()
	if adapter != nil {
		agent.Register("gpt", adapter)
	}
}

// Name returns the agent identifier.
func (a *Adapter) Name() string {
	return "gpt"
}

// Version returns the model name.
func (a *Adapter) Version() string {
	return a.model
}

// Capabilities returns the agent's feature capabilities.
func (a *Adapter) Capabilities() agent.Capabilities {
	return agent.Capabilities{
		SupportsSlashCommands: false,   // API adapter, not CLI
		SupportsHooks:         false,   // V1: not implemented
		SupportsTools:         true,    // GPT-4 supports function calling
		SupportsVision:        true,    // GPT-4V capable (not impl in V1)
		SupportsMultimodal:    false,   // V2 feature
		MaxContextWindow:      128000,  // gpt-4-turbo: 128K tokens
		ModelName:             a.model,
	}
}

// CreateSession creates a new agent session with the given context.
func (a *Adapter) CreateSession(ctx agent.SessionContext) (agent.SessionID, error) {
	if ctx.Name == "" {
		return "", errors.New("session name required")
	}
	if ctx.WorkingDirectory == "" {
		return "", errors.New("working directory required")
	}

	id := agent.SessionID(uuid.New().String())
	session := &Session{
		ID:        id,
		Context:   ctx,
		Messages:  []agent.Message{},
		Status:    agent.StatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	a.mu.Lock()
	a.sessions[id] = session
	a.mu.Unlock()

	return id, nil
}

// ResumeSession resumes an existing agent session by SessionID.
// For in-memory sessions, this is a no-op (sessions are always active).
func (a *Adapter) ResumeSession(sessionID agent.SessionID) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if _, exists := a.sessions[sessionID]; !exists {
		return ErrSessionNotFound
	}

	return nil
}

// TerminateSession terminates an agent session.
func (a *Adapter) TerminateSession(sessionID agent.SessionID) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.sessions[sessionID]; !exists {
		return ErrSessionNotFound
	}

	delete(a.sessions, sessionID)
	return nil
}

// GetSessionStatus returns the current status of a session.
func (a *Adapter) GetSessionStatus(sessionID agent.SessionID) (agent.Status, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if _, exists := a.sessions[sessionID]; exists {
		return agent.StatusActive, nil
	}
	return agent.StatusTerminated, nil
}

// SendMessage sends a message to the agent in the given session.
func (a *Adapter) SendMessage(sessionID agent.SessionID, message agent.Message) error {
	// 1. Get session (thread-safe read)
	a.mu.RLock()
	session, exists := a.sessions[sessionID]
	a.mu.RUnlock()

	if !exists {
		return ErrSessionNotFound
	}

	// 2. Add user message
	message.ID = uuid.New().String()
	message.Timestamp = time.Now()

	a.mu.Lock()
	session.Messages = append(session.Messages, message)
	session.UpdatedAt = time.Now()
	a.mu.Unlock()

	// 3. Build OpenAI request
	req := openai.ChatCompletionRequest{
		Model:    a.model,
		Messages: toOpenAIMessages(session.Messages),
	}

	// 4. Call API with retry
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	response, err := a.sendWithRetry(ctx, req)
	if err != nil {
		return err
	}

	// 5. Store assistant response
	assistantMsg := fromOpenAIMessage(response.Choices[0].Message, a.model)

	a.mu.Lock()
	session.Messages = append(session.Messages, assistantMsg)
	session.UpdatedAt = time.Now()
	a.mu.Unlock()

	return nil
}

// sendWithRetry calls OpenAI API with exponential backoff retry logic.
func (a *Adapter) sendWithRetry(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	maxRetries := 5
	baseDelay := 1 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err := a.client.CreateChatCompletion(ctx, req)

		if err == nil {
			return resp, nil
		}

		// Check error type
		var apiErr *openai.APIError
		if errors.As(err, &apiErr) {
			if apiErr.HTTPStatusCode == 429 { // Rate limit
				delay := baseDelay * time.Duration(1<<attempt) // Exponential: 1s, 2s, 4s, 8s, 16s
				select {
				case <-time.After(delay):
					continue
				case <-ctx.Done():
					return openai.ChatCompletionResponse{}, ctx.Err()
				}
			}
			if apiErr.HTTPStatusCode == 401 { // Auth error (non-retryable)
				return openai.ChatCompletionResponse{}, &APIError{
					Operation:  "sendMessage",
					StatusCode: 401,
					Message:    "authentication failed",
					Err:        err,
				}
			}
		}

		// Non-retryable error
		return openai.ChatCompletionResponse{}, err
	}

	return openai.ChatCompletionResponse{}, ErrMaxRetriesExceeded
}

// GetHistory retrieves conversation history for a session.
func (a *Adapter) GetHistory(sessionID agent.SessionID) ([]agent.Message, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	// Return copy to prevent external modification
	history := make([]agent.Message, len(session.Messages))
	copy(history, session.Messages)

	return history, nil
}

// ExportConversation exports conversation in the specified format.
func (a *Adapter) ExportConversation(sessionID agent.SessionID, format agent.ConversationFormat) ([]byte, error) {
	history, err := a.GetHistory(sessionID)
	if err != nil {
		return nil, err
	}

	switch format {
	case agent.FormatJSONL:
		return exportJSONL(history)
	case agent.FormatMarkdown:
		return exportMarkdown(history)
	default:
		return nil, ErrInvalidFormat
	}
}

// exportJSONL serializes messages to JSONL format (one JSON object per line).
func exportJSONL(messages []agent.Message) ([]byte, error) {
	var buf bytes.Buffer
	for _, msg := range messages {
		line, err := json.Marshal(msg)
		if err != nil {
			return nil, err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// exportMarkdown serializes messages to human-readable Markdown format.
func exportMarkdown(messages []agent.Message) ([]byte, error) {
	var buf bytes.Buffer
	for _, msg := range messages {
		buf.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", msg.Role, msg.Content))
	}
	return buf.Bytes(), nil
}

// ImportConversation imports conversation from serialized data.
func (a *Adapter) ImportConversation(data []byte, format agent.ConversationFormat) (agent.SessionID, error) {
	if format != agent.FormatJSONL {
		return "", errors.New("only JSONL import supported in V1")
	}

	messages, err := parseJSONL(data)
	if err != nil {
		return "", err
	}

	ctx := agent.SessionContext{
		Name:             "imported-session",
		WorkingDirectory: "/tmp",
	}
	sessionID, err := a.CreateSession(ctx)
	if err != nil {
		return "", err
	}

	a.mu.Lock()
	a.sessions[sessionID].Messages = messages
	a.mu.Unlock()

	return sessionID, nil
}

// parseJSONL parses JSONL data into a slice of agent.Message.
func parseJSONL(data []byte) ([]agent.Message, error) {
	var messages []agent.Message
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var msg agent.Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, scanner.Err()
}

// ExecuteCommand executes a generic command with agent-specific translation.
func (a *Adapter) ExecuteCommand(cmd agent.Command) error {
	switch cmd.Type {
	case agent.CommandRename:
		return a.executeRename(cmd)
	case agent.CommandSetDir:
		return a.executeSetDir(cmd)
	case agent.CommandAuthorize:
		// No-op for API agent (no directory restrictions)
		return nil
	case agent.CommandRunHook:
		return errors.New("hooks not supported for API agents")
	default:
		return fmt.Errorf("unsupported command: %s", cmd.Type)
	}
}

// executeRename handles the rename session command.
func (a *Adapter) executeRename(cmd agent.Command) error {
	name, ok := cmd.Params["name"].(string)
	if !ok || name == "" {
		return errors.New("parameter 'name' must be non-empty string")
	}

	sessionID, ok := cmd.Params["session_id"].(agent.SessionID)
	if !ok {
		return errors.New("parameter 'session_id' required")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	session.Context.Name = name
	return nil
}

// executeSetDir handles the set directory command.
func (a *Adapter) executeSetDir(cmd agent.Command) error {
	path, ok := cmd.Params["path"].(string)
	if !ok || path == "" {
		return errors.New("parameter 'path' must be non-empty string")
	}

	sessionID, ok := cmd.Params["session_id"].(agent.SessionID)
	if !ok {
		return errors.New("parameter 'session_id' required")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	session.Context.WorkingDirectory = path
	return nil
}
