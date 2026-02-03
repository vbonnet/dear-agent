package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/google/uuid"
	"google.golang.org/api/option"
)

// GeminiAdapter implements the Agent interface for Google Gemini.
//
// It uses the Google Generative AI Go SDK with client-side history persistence.
// Conversation history is stored in ~/.csm/gemini/<session-id>/history.jsonl.
type GeminiAdapter struct {
	sessionStore SessionStore
	modelName    string
	apiKey       string
}

// GeminiConfig holds configuration for Gemini adapter.
type GeminiConfig struct {
	// ModelName is the Gemini model to use (default: gemini-2.0-flash-exp).
	ModelName string

	// APIKey is the Google AI API key. If empty, reads from GEMINI_API_KEY env var.
	APIKey string

	// SessionStore is the session metadata storage. If nil, uses default JSON store.
	SessionStore SessionStore
}

// NewGeminiAdapter creates a new Gemini adapter instance.
//
// If config is nil, uses default configuration:
// - Model: gemini-2.0-flash-exp
// - API Key: from GEMINI_API_KEY environment variable
// - Session Store: default JSON store at ~/.csm/sessions.json
func NewGeminiAdapter(config *GeminiConfig) (Agent, error) {
	if config == nil {
		config = &GeminiConfig{}
	}

	// Set default model
	if config.ModelName == "" {
		config.ModelName = "gemini-2.0-flash-exp"
	}

	// Get API key from config or environment
	apiKey := config.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable not set")
	}

	// Initialize session store
	sessionStore := config.SessionStore
	if sessionStore == nil {
		store, err := NewJSONSessionStore("")
		if err != nil {
			return nil, fmt.Errorf("failed to create session store: %w", err)
		}
		sessionStore = store
	}

	return &GeminiAdapter{
		sessionStore: sessionStore,
		modelName:    config.ModelName,
		apiKey:       apiKey,
	}, nil
}

// Name returns the agent identifier.
func (a *GeminiAdapter) Name() string {
	return "gemini"
}

// Version returns the model name.
func (a *GeminiAdapter) Version() string {
	return a.modelName
}

// CreateSession creates a new Gemini session.
//
// Creates a session directory at ~/.csm/gemini/<session-id>/ to store conversation history.
func (a *GeminiAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
	// Generate unique SessionID
	sessionID := SessionID(uuid.New().String())

	// Create session directory
	sessionDir, err := a.getSessionDir(sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get session directory: %w", err)
	}

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create session directory: %w", err)
	}

	// Store session metadata
	metadata := &SessionMetadata{
		TmuxName:   string(sessionID), // For API agents, we reuse TmuxName field for session ID
		CreatedAt:  time.Now(),
		WorkingDir: ctx.WorkingDirectory,
		Project:    ctx.Project,
	}

	if err := a.sessionStore.Set(sessionID, metadata); err != nil {
		// Clean up session directory on error
		_ = os.RemoveAll(sessionDir)
		return "", fmt.Errorf("failed to store session metadata: %w", err)
	}

	return sessionID, nil
}

// ResumeSession resumes an existing Gemini session.
//
// Verifies that the session exists in the store and has a valid history file.
func (a *GeminiAdapter) ResumeSession(sessionID SessionID) error {
	// Verify session exists in store
	_, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Verify session directory exists
	sessionDir, err := a.getSessionDir(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session directory: %w", err)
	}

	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		return fmt.Errorf("session directory not found: %s", sessionDir)
	}

	return nil
}

// TerminateSession terminates a Gemini session.
//
// Removes the session from the store but preserves the conversation history on disk.
func (a *GeminiAdapter) TerminateSession(sessionID SessionID) error {
	// Remove from session store
	if err := a.sessionStore.Delete(sessionID); err != nil {
		return fmt.Errorf("failed to remove session from store: %w", err)
	}

	// Note: We preserve the session directory for historical purposes
	// Users can manually delete ~/.csm/gemini/<session-id>/ if needed

	return nil
}

// GetSessionStatus returns the status of a Gemini session.
//
// For API agents, sessions are always active if they exist in the store.
func (a *GeminiAdapter) GetSessionStatus(sessionID SessionID) (Status, error) {
	_, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return StatusTerminated, nil
	}

	return StatusActive, nil
}

// SendMessage sends a message to Gemini.
//
// Makes an API call to Gemini with the full conversation history and appends
// the user message and assistant response to history.jsonl.
func (a *GeminiAdapter) SendMessage(sessionID SessionID, message Message) error {
	// Verify session exists
	if _, err := a.sessionStore.Get(sessionID); err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Load conversation history
	history, err := a.GetHistory(sessionID)
	if err != nil {
		return fmt.Errorf("failed to load history: %w", err)
	}

	// Create Gemini client
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(a.apiKey))
	if err != nil {
		return fmt.Errorf("failed to create Gemini client: %w", err)
	}
	defer client.Close()

	// Get the model
	model := client.GenerativeModel(a.modelName)

	// Convert history to Gemini format
	var geminiHistory []*genai.Content
	for _, msg := range history {
		role := "user"
		if msg.Role == RoleAssistant {
			role = "model"
		}
		geminiHistory = append(geminiHistory, &genai.Content{
			Role: role,
			Parts: []genai.Part{
				genai.Text(msg.Content),
			},
		})
	}

	// Start chat session with history
	chat := model.StartChat()
	chat.History = geminiHistory

	// Send message
	resp, err := chat.SendMessage(ctx, genai.Text(message.Content))
	if err != nil {
		return fmt.Errorf("failed to send message to Gemini: %w", err)
	}

	// Extract response text
	var responseText string
	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		for _, part := range resp.Candidates[0].Content.Parts {
			if text, ok := part.(genai.Text); ok {
				responseText += string(text)
			}
		}
	}

	// Append user message to history
	userMsg := Message{
		ID:        uuid.New().String(),
		Role:      RoleUser,
		Content:   message.Content,
		Timestamp: time.Now(),
	}
	if err := a.appendToHistory(sessionID, userMsg); err != nil {
		return fmt.Errorf("failed to save user message: %w", err)
	}

	// Append assistant response to history
	assistantMsg := Message{
		ID:        uuid.New().String(),
		Role:      RoleAssistant,
		Content:   responseText,
		Timestamp: time.Now(),
	}
	if err := a.appendToHistory(sessionID, assistantMsg); err != nil {
		return fmt.Errorf("failed to save assistant response: %w", err)
	}

	return nil
}

// GetHistory retrieves conversation history.
//
// Parses the history.jsonl file for the session.
func (a *GeminiAdapter) GetHistory(sessionID SessionID) ([]Message, error) {
	historyPath, err := a.getHistoryPath(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get history path: %w", err)
	}

	// If history file doesn't exist, return empty history
	if _, err := os.Stat(historyPath); os.IsNotExist(err) {
		return []Message{}, nil
	}

	// Read and parse JSONL file
	data, err := os.ReadFile(historyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read history file: %w", err)
	}

	var messages []Message
	lines := splitLines(string(data))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var msg Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// Skip malformed lines
			continue
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

// ExportConversation exports conversation in specified format.
func (a *GeminiAdapter) ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error) {
	messages, err := a.GetHistory(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}

	switch format {
	case FormatJSONL:
		// Export as JSONL (one JSON object per line)
		var result []byte
		for _, msg := range messages {
			data, err := json.Marshal(msg)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal message: %w", err)
			}
			result = append(result, data...)
			result = append(result, '\n')
		}
		return result, nil

	case FormatMarkdown:
		// Export as Markdown
		var result string
		result += fmt.Sprintf("# Gemini Conversation\n\nSession ID: %s\n\n", sessionID)
		for _, msg := range messages {
			role := "User"
			if msg.Role == RoleAssistant {
				role = "Assistant"
			}
			result += fmt.Sprintf("## %s (%s)\n\n%s\n\n", role, msg.Timestamp.Format(time.RFC3339), msg.Content)
		}
		return []byte(result), nil

	case FormatHTML:
		return nil, fmt.Errorf("HTML export not supported for Gemini adapter")

	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// ImportConversation imports conversation from serialized data.
//
// Creates a new session with the imported conversation history.
func (a *GeminiAdapter) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
	// Parse messages based on format
	var messages []Message
	switch format {
	case FormatJSONL:
		lines := splitLines(string(data))
		for _, line := range lines {
			if line == "" {
				continue
			}
			var msg Message
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				return "", fmt.Errorf("failed to parse message: %w", err)
			}
			messages = append(messages, msg)
		}

	default:
		return "", fmt.Errorf("unsupported import format: %s", format)
	}

	// Create new session
	sessionID, err := a.CreateSession(SessionContext{
		Name:             fmt.Sprintf("imported-%s", time.Now().Format("20060102-150405")),
		WorkingDirectory: os.TempDir(), // Default to temp directory for imports
	})
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	// Write messages to history file
	historyPath, err := a.getHistoryPath(sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get history path: %w", err)
	}

	file, err := os.Create(historyPath)
	if err != nil {
		return "", fmt.Errorf("failed to create history file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, msg := range messages {
		if err := encoder.Encode(msg); err != nil {
			return "", fmt.Errorf("failed to write message: %w", err)
		}
	}

	return sessionID, nil
}

// Capabilities returns Gemini's feature capabilities.
func (a *GeminiAdapter) Capabilities() Capabilities {
	return Capabilities{
		SupportsSlashCommands: false,   // API agent, no CLI slash commands
		SupportsHooks:         false,   // AGM-level feature, not agent-specific
		SupportsTools:         true,    // Gemini supports function calling
		SupportsVision:        true,    // Gemini 2.0 supports vision
		SupportsMultimodal:    true,    // Gemini 2.0 supports audio/video
		SupportsStreaming:     true,    // Gemini API supports streaming
		SupportsSystemPrompts: true,    // Gemini supports system instructions
		MaxContextWindow:      1000000, // 1M tokens (2M for 2.0 Flash)
		ModelName:             a.modelName,
	}
}

// ExecuteCommand executes a generic command.
//
// For API agents like Gemini, most commands are no-ops or managed via metadata.
func (a *GeminiAdapter) ExecuteCommand(cmd Command) error {
	switch cmd.Type {
	case CommandRename:
		// For API agents, renaming is just updating metadata
		// We could store this in session metadata if needed
		return nil

	case CommandSetDir:
		// API agents don't have a working directory in the traditional sense
		// Could update metadata if needed
		return nil

	case CommandAuthorize:
		// API agents don't have directory authorization
		return nil

	case CommandRunHook:
		return fmt.Errorf("hooks not supported for API agents")

	default:
		return fmt.Errorf("unsupported command type: %s", cmd.Type)
	}
}

// Helper methods

// getSessionDir returns the session directory path for a given SessionID.
func (a *GeminiAdapter) getSessionDir(sessionID SessionID) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".csm", "gemini", string(sessionID)), nil
}

// getHistoryPath returns the history.jsonl file path for a session.
func (a *GeminiAdapter) getHistoryPath(sessionID SessionID) (string, error) {
	sessionDir, err := a.getSessionDir(sessionID)
	if err != nil {
		return "", err
	}
	return filepath.Join(sessionDir, "history.jsonl"), nil
}

// appendToHistory appends a message to the session's history.jsonl file.
func (a *GeminiAdapter) appendToHistory(sessionID SessionID, message Message) error {
	historyPath, err := a.getHistoryPath(sessionID)
	if err != nil {
		return err
	}

	// Open file in append mode
	file, err := os.OpenFile(historyPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	// Encode and write message
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(message); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// splitLines splits a string into lines, preserving empty lines at the end.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	// Add remaining content (even if empty when string ends with newline)
	if start <= len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
