package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// AgyAdapter implements Agent interface for the Antigravity (agy) CLI.
//
// It wraps existing AGM tmux-based session management and provides
// the Agent interface abstraction for agy sessions.
type AgyAdapter struct {
	sessionStore SessionStore
}

var (
	agyHasSession    = tmux.HasSession
	agyNewSession    = tmux.NewSession
	agySendCommand   = tmux.SendCommand
	agyWaitForPrompt = tmux.WaitForAgyPrompt
)

// NewAgyAdapter creates a new Agy adapter instance.
//
// If sessionStore is nil, creates a default JSON-backed store at ~/.agm/sessions.json.
func NewAgyAdapter(sessionStore SessionStore) (Agent, error) {
	if sessionStore == nil {
		store, err := NewJSONSessionStore("")
		if err != nil {
			return nil, fmt.Errorf("failed to create session store: %w", err)
		}
		sessionStore = store
	}

	return &AgyAdapter{
		sessionStore: sessionStore,
	}, nil
}

// Name returns the agent identifier
func (a *AgyAdapter) Name() string {
	return "agy"
}

// Version returns the model name or version
func (a *AgyAdapter) Version() string {
	return "antigravity-1.0"
}

// CreateSession creates a new Agy session.
//
// Creates a tmux session with Agy CLI and stores the SessionID mapping.
func (a *AgyAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
	// Generate unique SessionID
	sessionID := SessionID(uuid.New().String())

	// Use session name as tmux session name (or generate one)
	tmuxName := ctx.Name
	if tmuxName == "" {
		tmuxName = fmt.Sprintf("agy-%s", time.Now().Format("20060102-150405"))
	}
	workDir := ctx.WorkingDirectory
	if workDir == "" {
		workDir = "."
	}

	// Check if tmux session already exists
	exists, err := agyHasSession(tmuxName)
	if err != nil {
		return "", fmt.Errorf("failed to check tmux session: %w", err)
	}

	if !exists {
		// Create new tmux session
		if err := agyNewSession(tmuxName, workDir); err != nil {
			return "", fmt.Errorf("failed to create tmux session: %w", err)
		}
	}

	// Build Agy command (navigate to dir and run)
	agyCmd := fmt.Sprintf("cd %s && agy%s && exit", shellQuote(workDir), agyPermissionFlag(ctx.Environment["AGM_PERMISSION_MODE"]))

	// Start Agy in the tmux session
	if err := agySendCommand(tmuxName, agyCmd); err != nil {
		// Clean up tmux session on error if we created it
		if !exists {
			_ = agySendCommand(tmuxName, "exit\r")
		}
		return "", fmt.Errorf("failed to start Agy in tmux session: %w", err)
	}

	// Wait for Agy to be ready
	if err := agyWaitForPrompt(tmuxName, 30*time.Second); err != nil {
		// Non-fatal warning
		fmt.Fprintf(os.Stderr, "Warning: Agy prompt not detected (still initializing)\n")
	}

	// Store session metadata
	metadata := &SessionMetadata{
		TmuxName:   tmuxName,
		Title:      ctx.Name, // Set initial title from session name
		CreatedAt:  time.Now(),
		WorkingDir: workDir,
		Project:    ctx.Project,
		UUID:       ctx.Environment["AGY_CONVERSATION_ID"],
	}

	if err := a.sessionStore.Set(sessionID, metadata); err != nil {
		// Clean up tmux session on error
		_ = agySendCommand(tmuxName, "exit\r")
		return "", fmt.Errorf("failed to store session metadata: %w", err)
	}

	return sessionID, nil
}

// ResumeSession resumes an existing Agy session.
func (a *AgyAdapter) ResumeSession(sessionID SessionID) error {
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
		// Assume IsClaudeRunning works as a generic check or add IsAgyRunning later
		running, err := tmux.IsClaudeRunning(metadata.TmuxName)
		switch {
		case err != nil:
			sendCommands = false
		case running:
			sendCommands = false
		default:
			sendCommands = true
		}
	}

	// Send resume command if needed
	if sendCommands {
		// Build combined command. AGY resumes saved conversations with
		// --conversation; metadata.UUID stores the native AGY conversation ID
		// when AGM imported or captured one.
		conversationID := metadata.UUID
		if conversationID == "" {
			conversationID = string(sessionID)
		}
		fullCmd := fmt.Sprintf("cd %s && agy --conversation %s --add-dir %s && exit",
			shellQuote(metadata.WorkingDir),
			shellQuote(conversationID),
			shellQuote(metadata.WorkingDir))

		if err := tmux.SendCommand(metadata.TmuxName, fullCmd); err != nil {
			return fmt.Errorf("failed to send resume command: %w", err)
		}

		_ = tmux.WaitForAgyPrompt(metadata.TmuxName, 5*time.Second)
	}

	// Attach to tmux session (skip if already in tmux)
	if os.Getenv("TMUX") == "" {
		if err := tmux.AttachSession(metadata.TmuxName); err != nil {
			return fmt.Errorf("failed to attach to tmux session: %w", err)
		}
	}

	return nil
}

func agyPermissionFlag(mode string) string {
	if mode == "auto" {
		return " --dangerously-skip-permissions"
	}
	return ""
}

// TerminateSession terminates an Agy session.
func (a *AgyAdapter) TerminateSession(sessionID SessionID) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	if err := tmux.SendCommand(metadata.TmuxName, "exit\r"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to send exit to session: %v\n", err)
	}

	if err := a.sessionStore.Delete(sessionID); err != nil {
		return fmt.Errorf("failed to remove session from store: %w", err)
	}

	return nil
}

// GetSessionStatus returns the status of an Agy session.
func (a *AgyAdapter) GetSessionStatus(sessionID SessionID) (Status, error) {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return StatusTerminated, err
	}

	exists, err := tmux.HasSession(metadata.TmuxName)
	if err != nil {
		return StatusTerminated, fmt.Errorf("failed to check tmux session: %w", err)
	}

	if !exists {
		return StatusTerminated, nil
	}

	return StatusActive, nil
}

// SendMessage sends a message to Agy.
func (a *AgyAdapter) SendMessage(sessionID SessionID, message Message) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	if err := tmux.SendCommand(metadata.TmuxName, message.Content); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// GetHistory retrieves conversation history.
//
//nolint:dupl // adapter interface requires identical method signature on each harness type; shared logic extracted where practical
func (a *AgyAdapter) GetHistory(sessionID SessionID) ([]Message, error) {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Assuming similar history structure
	historyPath := filepath.Join(homeDir, ".agy", "sessions", metadata.TmuxName, "history.jsonl")

	if _, err := os.Stat(historyPath); os.IsNotExist(err) {
		return []Message{}, nil
	}

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
func (a *AgyAdapter) ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error) {
	messages, err := a.GetHistory(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}

	switch format {
	case FormatJSONL:
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
		var sb strings.Builder
		for _, msg := range messages {
			role := "User"
			if msg.Role == RoleAssistant {
				role = "Assistant"
			}
			fmt.Fprintf(&sb, "## %s (%s)\n\n%s\n\n", role, msg.Timestamp.Format(time.RFC3339), msg.Content)
		}
		return []byte(sb.String()), nil

	case FormatHTML:
		return nil, fmt.Errorf("HTML export not yet implemented")

	case FormatNative:
		return nil, fmt.Errorf("native format export not yet implemented")

	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// ImportConversation imports conversation from serialized data.
func (a *AgyAdapter) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
	return "", fmt.Errorf("conversation import not yet implemented")
}

// Capabilities returns Agy's feature capabilities
func (a *AgyAdapter) Capabilities() Capabilities {
	return Capabilities{
		SupportsSlashCommands: true,
		SupportsHooks:         false,
		SupportsTools:         true,
		SupportsVision:        true,
		SupportsMultimodal:    false,
		SupportsStreaming:     true,
		SupportsSystemPrompts: true,
		MaxContextWindow:      200000,
		ModelName:             "antigravity-1.0",
	}
}

// ExecuteCommand executes a generic command.
//
//nolint:dupl // adapter interface requires identical method signature on each harness type; shared logic extracted where practical
func (a *AgyAdapter) ExecuteCommand(cmd Command) error {
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
		newName, err := getStringParam(cmd.Params, "name")
		if err != nil {
			return fmt.Errorf("rename command: %w", err)
		}

		if err := tmux.SendCommand(metadata.TmuxName, fmt.Sprintf("/rename %s\r", newName)); err != nil {
			return fmt.Errorf("failed to send rename command: %w", err)
		}

		metadata.Title = newName
		if err := a.sessionStore.Set(SessionID(sessionIDStr), metadata); err != nil {
			return fmt.Errorf("failed to update session title: %w", err)
		}

		return nil

	case CommandSetDir:
		newPath, err := getStringParam(cmd.Params, "path")
		if err != nil {
			return fmt.Errorf("setdir command: %w", err)
		}
		if err := ValidateSendDirPath(newPath); err != nil {
			return fmt.Errorf("setdir command: %w", err)
		}
		if err := tmux.SendCommand(metadata.TmuxName, fmt.Sprintf("cd %s\r", newPath)); err != nil {
			return fmt.Errorf("failed to send cd command: %w", err)
		}
		return nil

	case CommandAuthorize:
		return fmt.Errorf("authorize command not yet implemented")

	case CommandRunHook:
		return fmt.Errorf("run_hook command not yet implemented")

	case CommandClearHistory:
		return fmt.Errorf("clear_history command not yet implemented")

	case CommandSetSystemPrompt:
		return fmt.Errorf("set_system_prompt command not yet implemented")

	default:
		return fmt.Errorf("unsupported command type: %s", cmd.Type)
	}
}
