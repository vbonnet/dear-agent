package agent

import (
	"context"
	"encoding/json"
	"errors"
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
	runtimeConfig  openai.SessionRuntimeConfig
}

var (
	_ Agent                      = (*OpenAIAdapter)(nil)
	_ ContextMessageSender       = (*OpenAIAdapter)(nil)
	_ ContextSessionStatusGetter = (*OpenAIAdapter)(nil)
)

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
	resolvedConfig := resolveOpenAIConfig(config)
	return newOpenAIAdapter(ctx, resolvedConfig)
}

// NewOpenAIAdapterForSession reconstructs an adapter from a session's
// persisted non-secret runtime configuration. API credentials are intentionally
// resolved from config or the environment on each process invocation.
func NewOpenAIAdapterForSession(ctx context.Context, sessionID SessionID, config *OpenAIConfig) (Agent, error) {
	resolvedConfig := resolveOpenAIConfig(config)
	sessionManager, err := openai.NewSessionManager(resolvedConfig.SessionsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create session manager: %w", err)
	}

	info, err := sessionManager.GetSessionContext(ctx, string(sessionID))
	if err != nil {
		return nil, fmt.Errorf("load OpenAI session %q: %w", sessionID, err)
	}
	resolvedConfig.Model = info.Model
	if info.RuntimeConfig != nil {
		resolvedConfig.Temperature = info.RuntimeConfig.Temperature
		resolvedConfig.MaxTokens = info.RuntimeConfig.MaxTokens
		resolvedConfig.BaseURL = info.RuntimeConfig.BaseURL
		resolvedConfig.IsAzure = info.RuntimeConfig.IsAzure
		resolvedConfig.AzureAPIVersion = info.RuntimeConfig.AzureAPIVersion
	}

	return newOpenAIAdapter(ctx, resolvedConfig)
}

func resolveOpenAIConfig(config *OpenAIConfig) OpenAIConfig {
	resolved := OpenAIConfig{}
	if config != nil {
		resolved = *config
	}

	// Read API key from environment if not provided
	if resolved.APIKey == "" {
		resolved.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	if resolved.Model == "" {
		resolved.Model = os.Getenv("OPENAI_MODEL")
		if resolved.Model == "" {
			resolved.Model = "gpt-4-turbo-preview"
		}
	}
	if resolved.Temperature == 0 {
		resolved.Temperature = 0.7
	}
	if resolved.MaxTokens == 0 {
		resolved.MaxTokens = 1000
	}
	if resolved.IsAzure && resolved.AzureAPIVersion == "" {
		resolved.AzureAPIVersion = "2024-02-15-preview"
	}
	return resolved
}

func newOpenAIAdapter(ctx context.Context, config OpenAIConfig) (*OpenAIAdapter, error) {
	// Create OpenAI client
	clientConfig := openai.Config{
		APIKey:          config.APIKey,
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
	return &OpenAIAdapter{
		client:         client,
		sessionManager: sessionManager,
		model:          config.Model,
		runtimeConfig: openai.SessionRuntimeConfig{
			Temperature:     config.Temperature,
			MaxTokens:       config.MaxTokens,
			BaseURL:         config.BaseURL,
			IsAzure:         config.IsAzure,
			AzureAPIVersion: config.AzureAPIVersion,
		},
	}, nil
}

// newOpenAIAdapterWithClient creates an adapter with a custom client (for testing).
// This is an unexported function used by tests to inject mock clients.
func newOpenAIAdapterWithClient(client openai.ClientInterface, sessionManager *openai.SessionManager) *OpenAIAdapter {
	return &OpenAIAdapter{
		client:         client,
		sessionManager: sessionManager,
		model:          "gpt-4",
		runtimeConfig: openai.SessionRuntimeConfig{
			Temperature: 0.7,
			MaxTokens:   1000,
		},
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
	if err := a.sessionManager.UpdateRuntimeConfig(string(sessionID), a.runtimeConfig); err != nil {
		_ = a.sessionManager.DeleteSession(string(sessionID))
		return "", fmt.Errorf("failed to persist session runtime configuration: %w", err)
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
// For pure API-based sessions, status is either Active (exists) or Terminated
// (doesn't exist) — Suspended is not applicable to stateless API sessions.
// The tmux-backed codex-cli harness has its own adapter and status projection;
// this pure API adapter must never capture or otherwise depend on a pane.
func (a *OpenAIAdapter) GetSessionStatus(sessionID SessionID) (Status, error) {
	return a.GetSessionStatusContext(context.Background(), sessionID)
}

// GetSessionStatusContext reports pure API readiness without tmux while
// honoring cancellation during authoritative store-lock acquisition.
func (a *OpenAIAdapter) GetSessionStatusContext(ctx context.Context, sessionID SessionID) (Status, error) {
	_, err := a.sessionManager.GetSessionContext(ctx, string(sessionID))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		if errors.Is(err, os.ErrNotExist) {
			return StatusTerminated, nil
		}
		return "", fmt.Errorf("load OpenAI session status: %w", err)
	}

	// Session exists = active
	return StatusActive, nil
}

// SendMessage sends a message to OpenAI and stores both the user message
// and assistant response in the conversation history.
//
// The complete history read, provider completion, and completed-turn commit are
// serialized across adapter instances and processes. A failed completion does
// not persist its provisional user message.
func (a *OpenAIAdapter) SendMessage(sessionID SessionID, message Message) error {
	return a.SendMessageContext(context.Background(), sessionID, message)
}

// OpenAICompletionTimeout is the maximum duration of one provider-backed
// completed-turn transaction, including a contended store-lock wait.
const OpenAICompletionTimeout = 2 * time.Minute

// OpenAIDeliveryTimeout bounds the surrounding reconstruction, readiness, and
// stable lifecycle transaction while leaving the adapter its full completion
// ceiling after preflight.
const OpenAIDeliveryTimeout = OpenAICompletionTimeout + time.Minute

// SendMessageContext is the request-aware OpenAI delivery transaction. The
// adapter applies a finite provider ceiling even when a legacy direct caller
// supplies a background context, so a stalled provider cannot retain the
// cross-process session lock indefinitely.
func (a *OpenAIAdapter) SendMessageContext(ctx context.Context, sessionID SessionID, message Message) error {
	if ctx == nil {
		ctx = context.Background()
	}
	completionCtx, cancel := context.WithTimeout(ctx, OpenAICompletionTimeout)
	defer cancel()

	return a.sessionManager.WithSessionLockContext(completionCtx, string(sessionID), func() error {
		history, err := a.sessionManager.GetMessagesUnderLock(string(sessionID))
		if err != nil {
			return fmt.Errorf("failed to revalidate session and get conversation history: %w", err)
		}
		userMsg := openai.Message{
			Role:      string(message.Role),
			Content:   message.Content,
			Timestamp: time.Now(),
		}
		requestHistory := append(append([]openai.Message(nil), history...), userMsg)

		response, err := a.client.CreateChatCompletion(completionCtx, requestHistory)
		if err != nil {
			return fmt.Errorf("OpenAI API call failed: %w", err)
		}
		assistantMsg := openai.Message{
			Role:      "assistant",
			Content:   response.Content,
			Timestamp: time.Now(),
		}
		if err := a.sessionManager.AddMessagesUnderLock(string(sessionID), userMsg, assistantMsg); err != nil {
			return fmt.Errorf("failed to commit completed OpenAI turn: %w", err)
		}
		return nil
	})
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

	if err := addImportedOpenAIMessages(messages, func(imported ...openai.Message) error {
		return a.sessionManager.AddMessages(string(sessionID), imported...)
	}); err != nil {
		return "", fmt.Errorf("failed to import messages: %w", err)
	}

	return sessionID, nil
}

func addImportedOpenAIMessages(messages []Message, add func(...openai.Message) error) error {
	if len(messages) == 0 {
		return nil
	}
	imported := make([]openai.Message, 0, len(messages))
	for _, msg := range messages {
		imported = append(imported, openai.Message{
			Role:      string(msg.Role),
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		})
	}
	return add(imported...)
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
		return a.openAIRename(cmd, sessionIDStr)
	case CommandSetDir:
		return a.openAISetDir(cmd, sessionIDStr)
	case CommandAuthorize:
		return nil
	case CommandClearHistory:
		return a.openAIClearHistory(sessionIDStr)
	case CommandSetSystemPrompt:
		return a.openAISetSystemPrompt(cmd, sessionIDStr)
	case CommandRunHook:
		return a.openAIRunHook(cmd, sessionIDStr)
	default:
		return fmt.Errorf("unsupported command type: %s", cmd.Type)
	}
}

func (a *OpenAIAdapter) openAIRename(cmd Command, sessionIDStr string) error {
	newName, err := getStringParam(cmd.Params, "name")
	if err != nil {
		return fmt.Errorf("rename command: %w", err)
	}
	if err := a.sessionManager.UpdateTitle(sessionIDStr, newName); err != nil {
		return fmt.Errorf("failed to update session title: %w", err)
	}
	return nil
}

func (a *OpenAIAdapter) openAISetDir(cmd Command, sessionIDStr string) error {
	newPath, err := getStringParam(cmd.Params, "path")
	if err != nil {
		return fmt.Errorf("setdir command: %w", err)
	}
	if err := a.sessionManager.UpdateWorkingDirectory(sessionIDStr, newPath); err != nil {
		return fmt.Errorf("failed to update working directory: %w", err)
	}
	return nil
}

func (a *OpenAIAdapter) openAIClearHistory(sessionIDStr string) error {
	clearCtx, cancel := context.WithTimeout(context.Background(), OpenAICompletionTimeout)
	defer cancel()
	return a.sessionManager.WithSessionLockContext(clearCtx, sessionIDStr, func() error {
		if err := a.sessionManager.ClearMessagesUnderLock(sessionIDStr); err != nil {
			return fmt.Errorf("failed to clear session history: %w", err)
		}
		return nil
	})
}

func (a *OpenAIAdapter) openAISetSystemPrompt(cmd Command, sessionIDStr string) error {
	prompt, err := getStringParam(cmd.Params, "prompt")
	if err != nil {
		return fmt.Errorf("set_system_prompt command: %w", err)
	}
	systemMsg := openai.Message{Role: "system", Content: prompt, Timestamp: time.Now()}
	if err := a.sessionManager.AddMessage(sessionIDStr, systemMsg); err != nil {
		return fmt.Errorf("failed to add system prompt: %w", err)
	}
	return nil
}

func (a *OpenAIAdapter) openAIRunHook(cmd Command, sessionIDStr string) error {
	hookName, err := getStringParam(cmd.Params, "hook_name")
	if err != nil {
		return fmt.Errorf("run_hook command: %w", err)
	}
	sessionInfo, err := a.sessionManager.GetSession(sessionIDStr)
	if err != nil {
		return fmt.Errorf("failed to get session info: %w", err)
	}
	return a.executeHook(SessionID(sessionIDStr), sessionInfo, hookName)
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
