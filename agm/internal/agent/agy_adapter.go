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
// agy is a Claude Code-compatible harness for Gemini models via Google AI Ultra.
// It runs in tmux (like Claude) and supports the same --add-dir / --conversation
// flags for workspace authorization and session resumption.
type AgyAdapter struct {
	sessionStore SessionStore
}

// NewAgyAdapter creates a new agy adapter instance.
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

// Name returns the agent identifier.
func (a *AgyAdapter) Name() string {
	return "agy"
}

// Version returns the default model name.
func (a *AgyAdapter) Version() string {
	return "gemini-3.5-flash"
}

// CreateSession creates a new agy session.
//
// Creates a tmux session with the agy CLI and stores the SessionID mapping.
func (a *AgyAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
	sessionID := SessionID(uuid.New().String())

	tmuxName := ctx.Name
	if tmuxName == "" {
		tmuxName = fmt.Sprintf("agy-%s", time.Now().Format("20060102-150405"))
	}

	exists, err := tmux.HasSession(tmuxName)
	if err != nil {
		return "", fmt.Errorf("failed to check tmux session: %w", err)
	}

	if !exists {
		if err := tmux.NewSession(tmuxName, ctx.WorkingDirectory); err != nil {
			return "", fmt.Errorf("failed to create tmux session: %w", err)
		}
	}

	// Build agy command; --add-dir pre-approves directories to avoid the trust prompt.
	var b strings.Builder
	fmt.Fprintf(&b, "agy --add-dir '%s'", ctx.WorkingDirectory)
	for _, dir := range ctx.AuthorizedDirs {
		if dir != ctx.WorkingDirectory {
			fmt.Fprintf(&b, " --add-dir '%s'", dir)
		}
	}
	b.WriteString(" && exit")
	agyCmd := b.String()

	if err := tmux.SendCommand(tmuxName, agyCmd); err != nil {
		if !exists {
			_ = tmux.SendCommand(tmuxName, "exit\r")
		}
		return "", fmt.Errorf("failed to start agy in tmux session: %w", err)
	}

	if err := tmux.WaitForProcessReady(tmuxName, "agy", 30*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: agy prompt not detected (still initializing)\n")
	}

	metadata := &SessionMetadata{
		TmuxName:   tmuxName,
		Title:      ctx.Name,
		CreatedAt:  time.Now(),
		WorkingDir: ctx.WorkingDirectory,
		Project:    ctx.Project,
	}

	if err := a.sessionStore.Set(sessionID, metadata); err != nil {
		_ = tmux.SendCommand(tmuxName, "exit\r")
		return "", fmt.Errorf("failed to store session metadata: %w", err)
	}

	return sessionID, nil
}

// ResumeSession resumes an existing agy session.
//
// Attaches to the tmux session. If the session is no longer alive, creates a
// fresh tmux session and resumes agy via --continue (most recent conversation).
// When a conversation UUID is stored in metadata, uses --conversation <uuid> instead.
func (a *AgyAdapter) ResumeSession(sessionID SessionID) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	exists, err := tmux.HasSession(metadata.TmuxName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}

	sendCommands := false
	if !exists {
		if err := tmux.NewSession(metadata.TmuxName, metadata.WorkingDir); err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}
		sendCommands = true
	} else {
		running, err := tmux.IsProcessRunning(metadata.TmuxName, "agy")
		if err != nil {
			sendCommands = false
		} else if !running {
			sendCommands = true
		}
	}

	if sendCommands {
		var resumeCmd string
		if metadata.UUID != "" {
			resumeCmd = fmt.Sprintf("cd '%s' && agy --conversation %s && exit",
				metadata.WorkingDir, metadata.UUID)
		} else {
			resumeCmd = fmt.Sprintf("cd '%s' && agy --continue && exit", metadata.WorkingDir)
		}

		if err := tmux.SendCommand(metadata.TmuxName, resumeCmd); err != nil {
			return fmt.Errorf("failed to resume agy: %w", err)
		}

		if err := tmux.WaitForProcessReady(metadata.TmuxName, "agy", 30*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: agy prompt not detected after resume\n")
		}
	}

	if os.Getenv("TMUX") == "" {
		if err := tmux.AttachSession(metadata.TmuxName); err != nil {
			return fmt.Errorf("failed to attach to tmux session: %w", err)
		}
	}

	return nil
}

// TerminateSession terminates an agy session.
//
// Sends exit to the CLI and removes the session from the store.
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

// GetSessionStatus returns the status of an agy session.
func (a *AgyAdapter) GetSessionStatus(sessionID SessionID) (Status, error) {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return StatusTerminated, nil //nolint:nilerr // intentional: missing record means terminated
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

// SendMessage sends a message to agy via tmux.
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
// agy session storage format is not yet reverse-engineered; returns empty for now.
//
//nolint:dupl // intentional: each adapter owns its own storage path and history format
func (a *AgyAdapter) GetHistory(sessionID SessionID) ([]Message, error) {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// agy sessions are expected under ~/.agy/sessions/<tmux-name>/history.jsonl.
	// Adjust when the storage format is confirmed.
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

// ExportConversation exports conversation in the specified format.
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
		var b strings.Builder
		fmt.Fprintf(&b, "# agy Conversation\n\nSession ID: %s\n\n", sessionID)
		for _, msg := range messages {
			role := "User"
			if msg.Role == RoleAssistant {
				role = "Assistant"
			}
			fmt.Fprintf(&b, "## %s (%s)\n\n%s\n\n", role, msg.Timestamp.Format(time.RFC3339), msg.Content)
		}
		return []byte(b.String()), nil

	case FormatHTML:
		return nil, fmt.Errorf("HTML export not supported for agy adapter")

	case FormatNative:
		return nil, fmt.Errorf("native format export not supported for agy adapter")

	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// ImportConversation imports conversation from serialized data.
func (a *AgyAdapter) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
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

	case FormatHTML, FormatMarkdown, FormatNative:
		return "", fmt.Errorf("unsupported import format: %s", format)

	default:
		return "", fmt.Errorf("unsupported import format: %s", format)
	}

	sessionID, err := a.CreateSession(SessionContext{
		Name:             fmt.Sprintf("imported-%s", time.Now().Format("20060102-150405")),
		WorkingDirectory: os.TempDir(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	_ = messages // TODO: replay messages into agy once storage format is known

	return sessionID, nil
}

// Capabilities returns agy feature capabilities.
func (a *AgyAdapter) Capabilities() Capabilities {
	return Capabilities{
		SupportsSlashCommands: true,
		SupportsHooks:         true,
		SupportsTools:         true,
		SupportsVision:        true,
		SupportsMultimodal:    true,
		SupportsStreaming:     true,
		SupportsSystemPrompts: true,
		MaxContextWindow:      1048576, // 1M tokens (Gemini 3.5 Flash)
		ModelName:             "gemini-3.5-flash",
	}
}

// ExecuteCommand executes a generic command against an agy session.
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
		return a.cmdRename(cmd, sessionIDStr, metadata)
	case CommandSetDir:
		return a.cmdSetDir(cmd, sessionIDStr, metadata)
	case CommandAuthorize:
		// agy authorizes directories via --add-dir at session creation; no runtime path.
		return nil
	case CommandClearHistory:
		return a.cmdClearHistory(metadata)
	case CommandSetSystemPrompt:
		return a.cmdSetSystemPrompt(cmd, sessionIDStr, metadata)
	case CommandRunHook:
		return a.cmdRunHook(cmd, metadata)
	default:
		return fmt.Errorf("unsupported command type: %s", cmd.Type)
	}
}

func (a *AgyAdapter) cmdRename(cmd Command, sessionIDStr string, metadata *SessionMetadata) error {
	newName, err := getStringParam(cmd.Params, "name")
	if err != nil {
		return fmt.Errorf("rename command: %w", err)
	}
	metadata.Title = newName
	if err := a.sessionStore.Set(SessionID(sessionIDStr), metadata); err != nil {
		return fmt.Errorf("failed to update session title: %w", err)
	}
	return nil
}

func (a *AgyAdapter) cmdSetDir(cmd Command, sessionIDStr string, metadata *SessionMetadata) error {
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
	metadata.WorkingDir = newPath
	if err := a.sessionStore.Set(SessionID(sessionIDStr), metadata); err != nil {
		return fmt.Errorf("failed to update working directory: %w", err)
	}
	return nil
}

func (a *AgyAdapter) cmdClearHistory(metadata *SessionMetadata) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	historyPath := filepath.Join(homeDir, ".agy", "sessions", metadata.TmuxName, "history.jsonl")
	if err := os.Remove(historyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove history file: %w", err)
	}
	return nil
}

func (a *AgyAdapter) cmdSetSystemPrompt(cmd Command, sessionIDStr string, metadata *SessionMetadata) error {
	prompt, err := getStringParam(cmd.Params, "prompt")
	if err != nil {
		return fmt.Errorf("set_system_prompt command: %w", err)
	}
	metadata.SystemPrompt = prompt
	if err := a.sessionStore.Set(SessionID(sessionIDStr), metadata); err != nil {
		return fmt.Errorf("failed to update system prompt: %w", err)
	}
	return nil
}

func (a *AgyAdapter) cmdRunHook(cmd Command, metadata *SessionMetadata) error {
	hookName, err := getStringParam(cmd.Params, "hook_name")
	if err != nil {
		return fmt.Errorf("run_hook command: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[agy hook] %s for session %s\n", hookName, metadata.Title)
	return nil
}
