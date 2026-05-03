package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// ClaudeAdapter implements Agent interface for Claude CLI.
//
// It wraps existing AGM tmux-based session management and provides
// the Agent interface abstraction for Claude sessions.
type ClaudeAdapter struct {
	sessionStore SessionStore
}

// NewClaudeAdapter creates a new Claude adapter instance.
//
// If sessionStore is nil, creates a default JSON-backed store at ~/.agm/sessions.json.
func NewClaudeAdapter(sessionStore SessionStore) (Agent, error) {
	if sessionStore == nil {
		store, err := NewJSONSessionStore("")
		if err != nil {
			return nil, fmt.Errorf("failed to create session store: %w", err)
		}
		sessionStore = store
	}

	return &ClaudeAdapter{
		sessionStore: sessionStore,
	}, nil
}

// Name returns the agent identifier
func (a *ClaudeAdapter) Name() string {
	return "claude"
}

// Version returns the model name
func (a *ClaudeAdapter) Version() string {
	return "claude-sonnet-4.5"
}

// CreateSession creates a new Claude session.
//
// Creates a tmux session with Claude CLI and stores the SessionID mapping.
func (a *ClaudeAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
	// Generate unique SessionID
	sessionID := SessionID(uuid.New().String())

	// Use session name as tmux session name (or generate one)
	tmuxName := ctx.Name
	if tmuxName == "" {
		tmuxName = fmt.Sprintf("claude-%s", time.Now().Format("20060102-150405"))
	}

	// Check if tmux session already exists
	exists, err := tmux.HasSession(tmuxName)
	if err != nil {
		return "", fmt.Errorf("failed to check tmux session: %w", err)
	}

	if !exists {
		// Create new tmux session
		if err := tmux.NewSession(tmuxName, ctx.WorkingDirectory); err != nil {
			return "", fmt.Errorf("failed to create tmux session: %w", err)
		}
	}

	// Build Claude command with directory authorization
	// Use --add-dir to pre-approve workspace and avoid trust prompt
	claudeCmd := fmt.Sprintf("claude --add-dir '%s'", ctx.WorkingDirectory)

	// Add additional authorized directories
	for _, dir := range ctx.AuthorizedDirs {
		if dir != ctx.WorkingDirectory {
			claudeCmd += fmt.Sprintf(" --add-dir '%s'", dir)
		}
	}

	claudeCmd += " && exit"

	// Start Claude in the tmux session
	if err := tmux.SendCommand(tmuxName, claudeCmd); err != nil {
		// Clean up tmux session on error if we created it
		if !exists {
			_ = tmux.SendCommand(tmuxName, "exit\r")
		}
		return "", fmt.Errorf("failed to start Claude in tmux session: %w", err)
	}

	// Wait for Claude to be ready (prompt appears)
	// This ensures subsequent commands go to Claude, not bash
	if err := tmux.WaitForClaudeReady(tmuxName, 30*time.Second); err != nil {
		// Non-fatal warning - Claude may still be initializing
		fmt.Fprintf(os.Stderr, "Warning: Claude prompt not detected (still initializing)\n")
	}

	// Store session metadata
	metadata := &SessionMetadata{
		TmuxName:   tmuxName,
		Title:      ctx.Name, // Set initial title from session name
		CreatedAt:  time.Now(),
		WorkingDir: ctx.WorkingDirectory,
		Project:    ctx.Project,
	}

	if err := a.sessionStore.Set(sessionID, metadata); err != nil {
		// Clean up tmux session on error
		_ = tmux.SendCommand(tmuxName, "exit\r")
		return "", fmt.Errorf("failed to store session metadata: %w", err)
	}

	return sessionID, nil
}

// ResumeSession resumes an existing Claude session.
//
// Attaches to the tmux session associated with the SessionID.
// If the tmux session doesn't exist, creates it and resumes the Claude session.
func (a *ClaudeAdapter) ResumeSession(sessionID SessionID) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Check if tmux session exists
	exists, err := tmux.HasSession(metadata.TmuxName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}

	sendCommands := false
	if !exists {
		// Create new tmux session
		if err := tmux.NewSession(metadata.TmuxName, metadata.WorkingDir); err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}
		sendCommands = true
	} else {
		// Check if Claude is already running
		claudeRunning, err := tmux.IsClaudeRunning(metadata.TmuxName)
		if err != nil {
			// Detection failed - skip commands for safety
			sendCommands = false
		} else if claudeRunning {
			// Claude already running - skip commands
			sendCommands = false
		} else {
			// Claude not running - send commands
			sendCommands = true
		}
	}

	// Send resume command if needed
	if sendCommands {
		// Build combined command: cd <workdir> && claude --resume <uuid> && exit
		// Note: We use string(sessionID) as the Claude UUID
		// TODO: If we need to support separate Claude UUID, store it in metadata
		fullCmd := fmt.Sprintf("cd '%s' && claude --resume %s && exit",
			metadata.WorkingDir,
			string(sessionID))

		if err := tmux.SendCommand(metadata.TmuxName, fullCmd); err != nil {
			return fmt.Errorf("failed to send resume command: %w", err)
		}

		// Wait for Claude to be ready
		_ = tmux.WaitForClaudeReady(metadata.TmuxName, 5*time.Second)
	}

	// Attach to tmux session (skip if already in tmux)
	if os.Getenv("TMUX") == "" {
		if err := tmux.AttachSession(metadata.TmuxName); err != nil {
			return fmt.Errorf("failed to attach to tmux session: %w", err)
		}
	}

	return nil
}

// TerminateSession terminates a Claude session.
//
// Kills the tmux session and removes the SessionID mapping.
func (a *ClaudeAdapter) TerminateSession(sessionID SessionID) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Terminate tmux session by sending exit command
	// Note: We use SendCommand instead of direct kill to allow graceful shutdown
	if err := tmux.SendCommand(metadata.TmuxName, "exit\r"); err != nil {
		// Continue even if send fails (session may already be dead)
		fmt.Fprintf(os.Stderr, "Warning: failed to send exit to session: %v\n", err)
	}

	// Remove from session store
	if err := a.sessionStore.Delete(sessionID); err != nil {
		return fmt.Errorf("failed to remove session from store: %w", err)
	}

	return nil
}

// GetSessionStatus returns the status of a Claude session.
//
// Queries tmux to determine if session is active, suspended, or terminated.
func (a *ClaudeAdapter) GetSessionStatus(sessionID SessionID) (Status, error) {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		// Session not in store = terminated
		return StatusTerminated, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	// Check if tmux session exists
	exists, err := tmux.HasSession(metadata.TmuxName)
	if err != nil {
		return StatusTerminated, fmt.Errorf("failed to check tmux session: %w", err)
	}

	if !exists {
		return StatusTerminated, nil
	}

	// TODO: Differentiate between active and suspended
	// For now, if tmux session exists, consider it active
	return StatusActive, nil
}

// SendMessage sends a message to Claude.
//
// Uses tmux send-keys to deliver the message to the Claude CLI.
func (a *ClaudeAdapter) SendMessage(sessionID SessionID, message Message) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Send message content to tmux pane
	if err := tmux.SendCommand(metadata.TmuxName, message.Content); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// GetHistory retrieves conversation history.
//
// Parses the history.jsonl file for the session.
func (a *ClaudeAdapter) GetHistory(sessionID SessionID) ([]Message, error) {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	// Find history.jsonl file for this session
	// Convention: ~/.claude/sessions/<tmux-name>/history.jsonl
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	historyPath := filepath.Join(homeDir, ".claude", "sessions", metadata.TmuxName, "history.jsonl")

	// Check if history file exists
	if _, err := os.Stat(historyPath); os.IsNotExist(err) {
		// No history yet (new session)
		return []Message{}, nil
	}

	// Parse JSONL file
	file, err := os.Open(historyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	var messages []Message
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var msg Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			// Skip malformed lines
			continue
		}
		messages = append(messages, msg)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read history file: %w", err)
	}

	return messages, nil
}

// ExportConversation exports conversation in specified format.
//
// Supports JSONL (direct copy), HTML, and Markdown formats.
func (a *ClaudeAdapter) ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error) {
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
		var result string
		for _, msg := range messages {
			role := "User"
			if msg.Role == RoleAssistant {
				role = "Assistant"
			}
			result += fmt.Sprintf("## %s (%s)\n\n%s\n\n", role, msg.Timestamp.Format(time.RFC3339), msg.Content)
		}
		return []byte(result), nil

	case FormatHTML:
		// TODO: Implement HTML export (use existing AGM HTML generation)
		return nil, fmt.Errorf("HTML export not yet implemented")

	case FormatNative:
		return nil, fmt.Errorf("native format export not yet implemented")

	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// ImportConversation imports conversation from serialized data.
//
// Creates a new session with the imported conversation history.
func (a *ClaudeAdapter) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
	// TODO: Implement conversation import
	// This would involve:
	// 1. Parse conversation data
	// 2. Create new session
	// 3. Inject conversation history into new session
	return "", fmt.Errorf("conversation import not yet implemented")
}

// Capabilities returns Claude's feature capabilities
func (a *ClaudeAdapter) Capabilities() Capabilities {
	return Capabilities{
		SupportsSlashCommands: true,   // Claude CLI supports /rename, /clear, etc.
		SupportsHooks:         false,  // AGM-level feature, not agent-specific
		SupportsTools:         true,   // Claude supports MCP tools
		SupportsVision:        true,   // Claude Sonnet/Opus support vision
		SupportsMultimodal:    false,  // No audio/video support yet
		SupportsStreaming:     true,   // Claude CLI supports streaming
		SupportsSystemPrompts: true,   // Claude supports system prompts
		MaxContextWindow:      200000, // 200K tokens
		ModelName:             "claude-sonnet-4.5",
	}
}

// getStringParam safely extracts a string parameter from the command params map.
//
// Returns an error if the parameter is missing or not a string.
func getStringParam(params map[string]interface{}, key string) (string, error) {
	val, ok := params[key]
	if !ok {
		return "", fmt.Errorf("missing required parameter: %s", key)
	}

	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("parameter %s must be a string, got %T", key, val)
	}

	return str, nil
}

// ExecuteCommand executes a generic command.
//
// Translates generic commands to Claude CLI-specific operations.
func (a *ClaudeAdapter) ExecuteCommand(cmd Command) error {
	// Validate session_id parameter
	sessionIDStr, err := getStringParam(cmd.Params, "session_id")
	if err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}

	metadata, err := a.sessionStore.Get(SessionID(sessionIDStr))
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	switch cmd.Type {
	case CommandRename:
		// Send /rename command to Claude CLI
		newName, err := getStringParam(cmd.Params, "name")
		if err != nil {
			return fmt.Errorf("rename command: %w", err)
		}

		// 1. Send to Claude CLI (updates Claude's internal name)
		if err := tmux.SendCommand(metadata.TmuxName, fmt.Sprintf("/rename %s\r", newName)); err != nil {
			return fmt.Errorf("failed to send rename command: %w", err)
		}

		// 2. Update AGM metadata (dual tracking)
		metadata.Title = newName
		if err := a.sessionStore.Set(SessionID(sessionIDStr), metadata); err != nil {
			return fmt.Errorf("failed to update session title: %w", err)
		}

		return nil

	case CommandSetDir:
		// Send cd command to change working directory
		newPath, err := getStringParam(cmd.Params, "path")
		if err != nil {
			return fmt.Errorf("setdir command: %w", err)
		}
		if err := tmux.SendCommand(metadata.TmuxName, fmt.Sprintf("cd %s\r", newPath)); err != nil {
			return fmt.Errorf("failed to send cd command: %w", err)
		}
		return nil

	case CommandAuthorize:
		// TODO: Send directory authorization command
		return fmt.Errorf("authorize command not yet implemented")

	case CommandRunHook:
		// TODO: Execute pre/post-command hooks
		return fmt.Errorf("run_hook command not yet implemented")

	case CommandClearHistory:
		return fmt.Errorf("clear_history command not yet implemented")

	case CommandSetSystemPrompt:
		return fmt.Errorf("set_system_prompt command not yet implemented")

	default:
		return fmt.Errorf("unsupported command type: %s", cmd.Type)
	}
}
