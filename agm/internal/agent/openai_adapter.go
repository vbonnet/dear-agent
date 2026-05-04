package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/agent/openai"
)

// OpenAIAdapter implements Agent interface for OpenAI API.
//
// It uses the OpenAI API (via go-openai SDK) and manages conversation
// sessions through the SessionManager. Unlike CLI adapters (Claude, Gemini),
// this is a pure API-based adapter with no tmux integration.
type OpenAIAdapter struct {
	client         openai.ClientInterface
	sessionManager *openai.SessionManager
	model          string
}

// OpenAIConfig holds configuration for creating an OpenAI adapter.
type OpenAIConfig struct {
	// APIKey is the OpenAI API key.
	// If empty, will be read from OPENAI_API_KEY environment variable.
	APIKey string

	// Model is the OpenAI model to use.
	// Defaults to gpt-4-turbo-preview if empty.
	// Supported: gpt-4, gpt-4-turbo, gpt-4-turbo-preview, gpt-3.5-turbo, etc.
	Model string

	// Temperature controls randomness (0.0-2.0).
	// Defaults to 0.7 if not set.
	Temperature float32

	// MaxTokens is the maximum tokens to generate.
	// Defaults to 1000 if not set.
	MaxTokens int

	// SessionsDir is the base directory for session storage.
	// If empty, defaults to ~/.agm/openai-sessions/
	SessionsDir string

	// BaseURL is the OpenAI API base URL (for custom endpoints).
	// Optional. Defaults to standard OpenAI API.
	BaseURL string

	// IsAzure indicates if this is an Azure OpenAI endpoint.
	IsAzure bool

	// AzureAPIVersion is the API version for Azure OpenAI.
	// Only used when IsAzure is true.
	AzureAPIVersion string
}

// NewOpenAIAdapter creates a new OpenAI adapter instance.
//
// If config is nil, uses default configuration (gpt-4-turbo-preview).
// Returns error if API key is missing or client initialization fails.
func NewOpenAIAdapter(ctx context.Context, config *OpenAIConfig) (Agent, error) {
	if config == nil {
		config = &OpenAIConfig{
			Model:       "gpt-4-turbo-preview",
			Temperature: 0.7,
			MaxTokens:   1000,
		}
	}

	// Read API key from environment if not provided
	apiKey := config.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	// Create OpenAI client
	clientConfig := openai.Config{
		APIKey:          apiKey,
		Model:           config.Model,
		Temperature:     config.Temperature,
		MaxTokens:       config.MaxTokens,
		BaseURL:         config.BaseURL,
		IsAzure:         config.IsAzure,
		AzureAPIVersion: config.AzureAPIVersion,
	}

	client, err := openai.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI client: %w", err)
	}

	// Create session manager
	sessionManager, err := openai.NewSessionManager(config.SessionsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create session manager: %w", err)
	}

	// Determine model name
	model := config.Model
	if model == "" {
		model = "gpt-4-turbo-preview"
	}

	return &OpenAIAdapter{
		client:         client,
		sessionManager: sessionManager,
		model:          model,
	}, nil
}

// newOpenAIAdapterWithClient creates an adapter with a custom client (for testing).
// This is an unexported function used by tests to inject mock clients.
func newOpenAIAdapterWithClient(client openai.ClientInterface, sessionManager *openai.SessionManager) *OpenAIAdapter {
	return &OpenAIAdapter{
		client:         client,
		sessionManager: sessionManager,
		model:          "gpt-4",
	}
}

// Name returns the agent identifier
func (a *OpenAIAdapter) Name() string {
	return "openai"
}

// Version returns the model name
func (a *OpenAIAdapter) Version() string {
	return a.model
}

// CreateSession creates a new OpenAI conversation session.
//
// Creates a new session with the given context and stores metadata.
// Unlike CLI adapters, no tmux session is created.
func (a *OpenAIAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
	// Generate unique SessionID
	sessionID := SessionID(uuid.New().String())

	// Determine working directory
	workingDir := ctx.WorkingDirectory
	if workingDir == "" {
		workingDir = "."
	}

	// Create session via SessionManager
	_, err := a.sessionManager.CreateSession(
		string(sessionID),
		a.model,
		workingDir,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	// Update title if provided
	if ctx.Name != "" {
		if err := a.sessionManager.UpdateTitle(string(sessionID), ctx.Name); err != nil {
			return "", fmt.Errorf("failed to set session title: %w", err)
		}
	}

	// Add system message if workflow is specified
	if ctx.WorkflowName != "" {
		systemMsg := openai.Message{
			Role:      "system",
			Content:   fmt.Sprintf("You are running in workflow mode: %s", ctx.WorkflowName),
			Timestamp: time.Now(),
		}
		if err := a.sessionManager.AddMessage(string(sessionID), systemMsg); err != nil {
			return "", fmt.Errorf("failed to add system message: %w", err)
		}
	}

	// Trigger SessionStart hook after session creation
	// Get the session info we just created
	sessionInfo, err := a.sessionManager.GetSession(string(sessionID))
	if err != nil {
		// Log warning but don't fail session creation
		fmt.Fprintf(os.Stderr, "Warning: failed to get session info for SessionStart hook: %v\n", err)
	} else {
		// Execute SessionStart hook (non-fatal if it fails)
		_ = a.executeHook(sessionID, sessionInfo, "SessionStart")
	}

	return sessionID, nil
}

// ResumeSession resumes an existing OpenAI session.
//
// For API-based sessions, this simply validates that the session exists
// and loads its conversation history into memory.
func (a *OpenAIAdapter) ResumeSession(sessionID SessionID) error {
	// Verify session exists
	_, err := a.sessionManager.GetSession(string(sessionID))
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// For API-based sessions, no additional action needed
	// History will be loaded on-demand during SendMessage
	return nil
}

// TerminateSession terminates an OpenAI session.
//
// For API-based sessions, this is effectively a delete operation.
// The session is removed from storage.
func (a *OpenAIAdapter) TerminateSession(sessionID SessionID) error {
	// Trigger SessionEnd hook before session cleanup
	// Get session info before deletion
	sessionInfo, err := a.sessionManager.GetSession(string(sessionID))
	if err != nil {
		// Log warning but continue with deletion
		fmt.Fprintf(os.Stderr, "Warning: failed to get session info for SessionEnd hook: %v\n", err)
	} else {
		// Execute SessionEnd hook (non-fatal if it fails)
		_ = a.executeHook(sessionID, sessionInfo, "SessionEnd")
	}

	// Delete the session
	if err := a.sessionManager.DeleteSession(string(sessionID)); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// GetSessionStatus returns the status of an OpenAI session.
//
// For API-based sessions, status is either Active (exists) or Terminated (doesn't exist).
// Suspended status is not applicable to stateless API sessions.
func (a *OpenAIAdapter) GetSessionStatus(sessionID SessionID) (Status, error) {
	_, err := a.sessionManager.GetSession(string(sessionID))
	if err != nil {
		// Session not found = terminated
		return StatusTerminated, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	// Session exists = active
	return StatusActive, nil
}

// SendMessage sends a message to OpenAI and stores both the user message
// and assistant response in the conversation history.
//
// This method:
// 1. Adds user message to session history
// 2. Retrieves full conversation history
// 3. Sends to OpenAI API
// 4. Stores assistant response
func (a *OpenAIAdapter) SendMessage(sessionID SessionID, message Message) error {
	// Verify session exists
	_, err := a.sessionManager.GetSession(string(sessionID))
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Add user message to history
	userMsg := openai.Message{
		Role:      string(message.Role),
		Content:   message.Content,
		Timestamp: time.Now(),
	}

	if err := a.sessionManager.AddMessage(string(sessionID), userMsg); err != nil {
		return fmt.Errorf("failed to add user message: %w", err)
	}

	// Get full conversation history for API call
	history, err := a.sessionManager.GetMessages(string(sessionID))
	if err != nil {
		return fmt.Errorf("failed to get conversation history: %w", err)
	}

	// Send to OpenAI API
	ctx := context.Background()
	response, err := a.client.CreateChatCompletion(ctx, history)
	if err != nil {
		return fmt.Errorf("OpenAI API call failed: %w", err)
	}

	// Store assistant response
	assistantMsg := openai.Message{
		Role:      "assistant",
		Content:   response.Content,
		Timestamp: time.Now(),
	}

	if err := a.sessionManager.AddMessage(string(sessionID), assistantMsg); err != nil {
		return fmt.Errorf("failed to add assistant response: %w", err)
	}

	return nil
}

// GetHistory retrieves conversation history for a session.
//
// Returns all messages in the session's conversation history.
func (a *OpenAIAdapter) GetHistory(sessionID SessionID) ([]Message, error) {
	// Get messages from SessionManager
	openaiMessages, err := a.sessionManager.GetMessages(string(sessionID))
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	// Convert to agent.Message format
	messages := make([]Message, len(openaiMessages))
	for i, msg := range openaiMessages {
		messages[i] = Message{
			ID:        fmt.Sprintf("%s-%d", sessionID, i),
			Role:      Role(msg.Role),
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		}
	}

	return messages, nil
}

// ExportConversation exports conversation in specified format.
//
// Supports JSONL and Markdown formats. HTML format is not supported
// for OpenAI adapter (returns error).
func (a *OpenAIAdapter) ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error) {
	messages, err := a.GetHistory(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}

	switch format {
	case FormatJSONL:
		// Export as JSONL (one JSON object per line)
		result := make([]byte, 0)
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
		var builder strings.Builder
		fmt.Fprintf(&builder, "# OpenAI Conversation\n\nSession ID: %s\nModel: %s\n\n", sessionID, a.model)
		for _, msg := range messages {
			role := "User"
			if msg.Role == RoleAssistant {
				role = "Assistant"
			}
			fmt.Fprintf(&builder, "## %s (%s)\n\n%s\n\n", role, msg.Timestamp.Format(time.RFC3339), msg.Content)
		}
		return []byte(builder.String()), nil

	case FormatHTML:
		return nil, fmt.Errorf("HTML export not supported for OpenAI adapter")

	case FormatNative:
		// Native format is the same as JSONL for OpenAI
		return a.ExportConversation(sessionID, FormatJSONL)

	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// ImportConversation imports conversation from serialized data.
//
// Creates a new session and populates it with imported messages.
// Currently only supports JSONL format.
func (a *OpenAIAdapter) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
	if format != FormatJSONL {
		return "", fmt.Errorf("only JSONL import format is supported, got: %s", format)
	}

	// Parse messages
	lines := splitLinesOpenAI(string(data))
	var messages []Message

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

	// Create new session
	sessionID, err := a.CreateSession(SessionContext{
		Name:             fmt.Sprintf("imported-%s", time.Now().Format("20060102-150405")),
		WorkingDirectory: ".",
	})
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	// Import messages
	for _, msg := range messages {
		openaiMsg := openai.Message{
			Role:      string(msg.Role),
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		}

		if err := a.sessionManager.AddMessage(string(sessionID), openaiMsg); err != nil {
			return "", fmt.Errorf("failed to import message: %w", err)
		}
	}

	return sessionID, nil
}

// Capabilities returns OpenAI's feature capabilities.
//
// Note: SupportsHooks is true for synthetic hook support.
// OpenAI is API-based but supports AGM-level lifecycle hooks.
func (a *OpenAIAdapter) Capabilities() Capabilities {
	// Determine context window based on model
	contextWindow := 8192 // Default for gpt-3.5-turbo

	switch a.model {
	case "gpt-4":
		contextWindow = 8192
	case "gpt-4-32k":
		contextWindow = 32768
	case "gpt-4-turbo", "gpt-4-turbo-preview":
		contextWindow = 128000
	case "gpt-3.5-turbo":
		contextWindow = 16385
	case "gpt-3.5-turbo-16k":
		contextWindow = 16385
	}

	// Determine vision support
	supportsVision := a.model == "gpt-4-turbo" || a.model == "gpt-4-turbo-preview" || a.model == "gpt-4-vision-preview"

	return Capabilities{
		SupportsSlashCommands: false, // API-based, no slash commands
		SupportsHooks:         true,  // Synthetic hooks supported
		SupportsTools:         true,  // GPT supports function calling
		SupportsVision:        supportsVision,
		SupportsMultimodal:    false, // No audio/video support yet
		SupportsStreaming:     true,  // OpenAI API supports streaming
		SupportsSystemPrompts: true,  // GPT supports system prompts
		MaxContextWindow:      contextWindow,
		ModelName:             a.model,
	}
}

// ExecuteCommand executes a generic command.
//
// Translates generic commands to OpenAI API operations.
// Most commands update session metadata rather than sending API calls.
func (a *OpenAIAdapter) ExecuteCommand(cmd Command) error {
	// Validate session_id parameter
	sessionIDStr, err := getStringParam(cmd.Params, "session_id")
	if err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}

	// Verify session exists
	_, err = a.sessionManager.GetSession(sessionIDStr)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	switch cmd.Type {
	case CommandRename:
		// Update session title
		newName, err := getStringParam(cmd.Params, "name")
		if err != nil {
			return fmt.Errorf("rename command: %w", err)
		}

		if err := a.sessionManager.UpdateTitle(sessionIDStr, newName); err != nil {
			return fmt.Errorf("failed to update session title: %w", err)
		}

		return nil

	case CommandSetDir:
		// Update working directory
		newPath, err := getStringParam(cmd.Params, "path")
		if err != nil {
			return fmt.Errorf("setdir command: %w", err)
		}

		if err := a.sessionManager.UpdateWorkingDirectory(sessionIDStr, newPath); err != nil {
			return fmt.Errorf("failed to update working directory: %w", err)
		}

		return nil

	case CommandAuthorize:
		// OpenAI API doesn't have directory authorization
		// This is a no-op for API-based adapters
		return nil

	case CommandClearHistory:
		// Clear conversation history
		// We do this by deleting and recreating the session
		sessionInfo, err := a.sessionManager.GetSession(sessionIDStr)
		if err != nil {
			return fmt.Errorf("failed to get session info: %w", err)
		}

		// Delete old session
		if err := a.sessionManager.DeleteSession(sessionIDStr); err != nil {
			return fmt.Errorf("failed to delete session: %w", err)
		}

		// Recreate with same ID
		_, err = a.sessionManager.CreateSession(
			sessionIDStr,
			sessionInfo.Model,
			sessionInfo.WorkingDirectory,
		)
		if err != nil {
			return fmt.Errorf("failed to recreate session: %w", err)
		}

		// Restore title
		if sessionInfo.Title != "" {
			if err := a.sessionManager.UpdateTitle(sessionIDStr, sessionInfo.Title); err != nil {
				return fmt.Errorf("failed to restore session title: %w", err)
			}
		}

		return nil

	case CommandSetSystemPrompt:
		// Add system prompt message
		prompt, err := getStringParam(cmd.Params, "prompt")
		if err != nil {
			return fmt.Errorf("set_system_prompt command: %w", err)
		}

		systemMsg := openai.Message{
			Role:      "system",
			Content:   prompt,
			Timestamp: time.Now(),
		}

		if err := a.sessionManager.AddMessage(sessionIDStr, systemMsg); err != nil {
			return fmt.Errorf("failed to add system prompt: %w", err)
		}

		return nil

	case CommandRunHook:
		// Execute hook via OpenAI adapter lifecycle
		hookName, err := getStringParam(cmd.Params, "hook_name")
		if err != nil {
			return fmt.Errorf("run_hook command: %w", err)
		}

		// Get session info for hook execution
		sessionInfo, err := a.sessionManager.GetSession(sessionIDStr)
		if err != nil {
			return fmt.Errorf("failed to get session info: %w", err)
		}

		return a.executeHook(SessionID(sessionIDStr), sessionInfo, hookName)

	default:
		return fmt.Errorf("unsupported command type: %s", cmd.Type)
	}
}

// RunHook executes a session lifecycle hook for OpenAI.
//
// Triggers OpenAI lifecycle hooks (SessionStart, SessionEnd) via synthetic execution.
// Hook context is written to files that external hook scripts can consume.
//
// Hook failures are logged but don't block the session (graceful degradation).
func (a *OpenAIAdapter) RunHook(sessionID SessionID, hookName string) error {
	// Get session info
	sessionInfo, err := a.sessionManager.GetSession(string(sessionID))
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	return a.executeHook(sessionID, sessionInfo, hookName)
}

// executeHook runs an OpenAI lifecycle hook and creates hook context files.
//
// Hooks are synthetic for API-based adapters. The hook:
// 1. Receives hook name and session context
// 2. Creates a hook context file with session metadata
// 3. Can be consumed by external scripts for integration
//
// Errors are logged but don't fail the operation (graceful degradation).
func (a *OpenAIAdapter) executeHook(sessionID SessionID, sessionInfo *openai.SessionInfo, hookName string) error {
	// Create hook ready-file directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to get home directory for hook: %v\n", err)
		return nil // Non-fatal: hooks are optional
	}

	hookDir := filepath.Join(homeDir, ".agm", "openai-hooks")
	if err := os.MkdirAll(hookDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create hook directory: %v\n", err)
		return nil // Non-fatal
	}

	// Create hook execution marker
	hookFile := filepath.Join(hookDir, fmt.Sprintf("%s-%s.json", string(sessionID), hookName))

	// Prepare hook context data
	hookContext := map[string]any{
		"session_id":   string(sessionID),
		"hook_name":    hookName,
		"session_name": sessionInfo.Title,
		"working_dir":  sessionInfo.WorkingDirectory,
		"model":        sessionInfo.Model,
		"timestamp":    time.Now().Format(time.RFC3339),
	}

	// Write hook context to file
	contextData, err := json.MarshalIndent(hookContext, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to marshal hook context: %v\n", err)
		return nil // Non-fatal
	}

	if err := os.WriteFile(hookFile, contextData, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write hook context: %v\n", err)
		return nil // Non-fatal
	}

	// Log hook execution
	fmt.Fprintf(os.Stderr, "[OpenAI Hook] Executed %s hook for session %s\n", hookName, sessionInfo.Title)

	return nil
}

// splitLinesOpenAI splits a string into lines, preserving empty lines at the end.
// Note: Renamed to avoid conflict with gemini_cli_adapter.splitLines
func splitLinesOpenAI(s string) []string {
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
