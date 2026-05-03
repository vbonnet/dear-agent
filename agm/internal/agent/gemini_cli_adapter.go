package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// GeminiCLIAdapter implements Agent interface for Gemini CLI.
//
// It runs Gemini CLI in tmux (like Claude) and provides the Agent interface
// abstraction for Gemini sessions.
type GeminiCLIAdapter struct {
	sessionStore SessionStore
}

// NewGeminiCLIAdapter creates a new Gemini CLI adapter instance.
//
// If sessionStore is nil, creates a default JSON-backed store at ~/.agm/sessions.json.
func NewGeminiCLIAdapter(sessionStore SessionStore) (Agent, error) {
	if sessionStore == nil {
		store, err := NewJSONSessionStore("")
		if err != nil {
			return nil, fmt.Errorf("failed to create session store: %w", err)
		}
		sessionStore = store
	}

	return &GeminiCLIAdapter{
		sessionStore: sessionStore,
	}, nil
}

// Name returns the agent identifier
func (a *GeminiCLIAdapter) Name() string {
	return "gemini"
}

// Version returns the model name
func (a *GeminiCLIAdapter) Version() string {
	return "gemini-2.0-flash-exp"
}

// CreateSession creates a new Gemini session.
//
// Creates a tmux session with Gemini CLI and stores the SessionID mapping.
func (a *GeminiCLIAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
	// Generate unique SessionID
	sessionID := SessionID(uuid.New().String())

	// Use session name as tmux session name (or generate one)
	tmuxName := ctx.Name
	if tmuxName == "" {
		tmuxName = fmt.Sprintf("gemini-%s", time.Now().Format("20060102-150405"))
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

	// Build Gemini command with directory authorization
	// Use --include-directories to pre-approve workspace and avoid trust prompt
	geminiCmd := fmt.Sprintf("gemini --include-directories '%s'", ctx.WorkingDirectory)

	// Add additional authorized directories
	for _, dir := range ctx.AuthorizedDirs {
		if dir != ctx.WorkingDirectory {
			geminiCmd += fmt.Sprintf(" --include-directories '%s'", dir)
		}
	}

	geminiCmd += " && exit"

	// Start Gemini CLI in tmux
	if err := tmux.SendCommand(tmuxName, geminiCmd); err != nil {
		// Clean up tmux session on error if we created it
		if !exists {
			_ = tmux.SendCommand(tmuxName, "exit\r")
		}
		return "", fmt.Errorf("failed to start Gemini in tmux session: %w", err)
	}

	// Wait for Gemini to be ready (prompt appears)
	if err := tmux.WaitForProcessReady(tmuxName, "gemini", 30*time.Second); err != nil {
		// Non-fatal warning - Gemini may still be initializing
		fmt.Fprintf(os.Stderr, "Warning: Gemini prompt not detected (still initializing)\n")
	}

	// Extract Gemini session UUID from --list-sessions output
	// This UUID is needed for --resume flag to restore session state
	geminiUUID, err := a.extractLatestGeminiUUID(ctx.WorkingDirectory)
	if err != nil {
		// Non-fatal: UUID extraction failure doesn't block session creation
		// Resume will fall back to "latest" if UUID not available
		fmt.Fprintf(os.Stderr, "Warning: failed to extract Gemini UUID: %v\n", err)
		geminiUUID = "" // Empty UUID means resume will use "latest"
	}

	// Store session metadata
	metadata := &SessionMetadata{
		TmuxName:   tmuxName,
		Title:      ctx.Name, // Set initial title from session name
		CreatedAt:  time.Now(),
		WorkingDir: ctx.WorkingDirectory,
		Project:    ctx.Project,
		UUID:       geminiUUID, // Store Gemini's native UUID
	}

	if err := a.sessionStore.Set(sessionID, metadata); err != nil {
		// Clean up tmux session on error
		_ = tmux.SendCommand(tmuxName, "exit\r")
		return "", fmt.Errorf("failed to store session metadata: %w", err)
	}

	return sessionID, nil
}

// ResumeSession resumes an existing Gemini session.
//
// Attaches to the tmux session associated with the SessionID.
func (a *GeminiCLIAdapter) ResumeSession(sessionID SessionID) error {
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
		// Check if Gemini is already running
		geminiRunning, err := tmux.IsProcessRunning(metadata.TmuxName, "gemini")
		if err != nil {
			// Detection failed - skip commands for safety
			sendCommands = false
		} else if !geminiRunning {
			sendCommands = true
		}
	}

	if sendCommands {
		// Build resume command with UUID
		// If UUID is stored, use it. Otherwise fall back to "latest"
		var resumeCmd string
		if metadata.UUID != "" {
			// Resume specific session by UUID
			resumeCmd = fmt.Sprintf("cd '%s' && gemini --resume %s && exit",
				metadata.WorkingDir,
				metadata.UUID)
		} else {
			// No UUID stored - use "latest" as fallback
			resumeCmd = fmt.Sprintf("cd '%s' && gemini --resume latest && exit",
				metadata.WorkingDir)
		}

		if err := tmux.SendCommand(metadata.TmuxName, resumeCmd); err != nil {
			return fmt.Errorf("failed to resume Gemini: %w", err)
		}

		// Wait for ready
		if err := tmux.WaitForProcessReady(metadata.TmuxName, "gemini", 30*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Gemini prompt not detected\n")
		}
	}

	return nil
}

// TerminateSession terminates a Gemini session.
//
// Sends exit command to Gemini and removes from session store.
func (a *GeminiCLIAdapter) TerminateSession(sessionID SessionID) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Send exit to Gemini (graceful shutdown)
	if err := tmux.SendCommand(metadata.TmuxName, "exit\r"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to send exit to session: %v\n", err)
	}

	// Remove from session store
	if err := a.sessionStore.Delete(sessionID); err != nil {
		return fmt.Errorf("failed to remove session from store: %w", err)
	}

	return nil
}

// GetSessionStatus returns the status of a Gemini session.
//
// Queries tmux to determine if session is active or terminated.
func (a *GeminiCLIAdapter) GetSessionStatus(sessionID SessionID) (Status, error) {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		// Session not in store = terminated
		return StatusTerminated, nil
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
	return StatusActive, nil
}

// SendMessage sends a message to Gemini.
//
// Uses tmux send-keys to deliver the message to the Gemini CLI.
func (a *GeminiCLIAdapter) SendMessage(sessionID SessionID, message Message) error {
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
func (a *GeminiCLIAdapter) GetHistory(sessionID SessionID) ([]Message, error) {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	// Find history file for this session
	// Note: Gemini CLI stores sessions in ~/.gemini/tmp/<project_hash>/chats/
	// For now, we'll use a simplified approach similar to Claude
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	historyPath := filepath.Join(homeDir, ".gemini", "sessions", metadata.TmuxName, "history.jsonl")

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
func (a *GeminiCLIAdapter) ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error) {
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

	case FormatNative:
		return nil, fmt.Errorf("native format export not supported for Gemini adapter")

	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// ImportConversation imports conversation from serialized data.
func (a *GeminiCLIAdapter) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
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

	case FormatHTML, FormatMarkdown, FormatNative:
		return "", fmt.Errorf("unsupported import format: %s", format)

	default:
		return "", fmt.Errorf("unsupported import format: %s", format)
	}

	// Create new session
	sessionID, err := a.CreateSession(SessionContext{
		Name:             fmt.Sprintf("imported-%s", time.Now().Format("20060102-150405")),
		WorkingDirectory: os.TempDir(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	// TODO: Write messages to history file
	// For now, just return the session ID

	return sessionID, nil
}

// Capabilities returns Gemini's feature capabilities.
func (a *GeminiCLIAdapter) Capabilities() Capabilities {
	return Capabilities{
		SupportsSlashCommands: true,    // Gemini CLI supports /chat, /memory, etc.
		SupportsHooks:         true,    // Gemini CLI supports hooks (SessionStart, SessionEnd, etc.)
		SupportsTools:         true,    // Gemini supports function calling
		SupportsVision:        true,    // Gemini 2.0 supports vision
		SupportsMultimodal:    true,    // Gemini 2.0 supports audio/video
		SupportsStreaming:     true,    // Gemini CLI supports streaming
		SupportsSystemPrompts: true,    // Gemini supports system instructions
		MaxContextWindow:      1000000, // 1M tokens (2M for 2.0 Flash)
		ModelName:             "gemini-2.0-flash-exp",
	}
}

// ExecuteCommand executes a generic command.
//
// Translates generic commands to Gemini CLI-specific operations.
func (a *GeminiCLIAdapter) ExecuteCommand(cmd Command) error {
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
		// Send /chat save command to Gemini CLI
		newName, err := getStringParam(cmd.Params, "name")
		if err != nil {
			return fmt.Errorf("rename command: %w", err)
		}

		// 1. Send to Gemini CLI (creates checkpoint with name)
		if err := tmux.SendCommand(metadata.TmuxName, fmt.Sprintf("/chat save %s\r", newName)); err != nil {
			return fmt.Errorf("failed to send chat save command: %w", err)
		}

		// 2. Update AGM metadata (dual tracking)
		metadata.Title = newName
		if err := a.sessionStore.Set(SessionID(sessionIDStr), metadata); err != nil {
			return fmt.Errorf("failed to update session title: %w", err)
		}

		return nil

	case CommandSetDir:
		// Send cd command to Gemini session and update metadata
		newPath, err := getStringParam(cmd.Params, "path")
		if err != nil {
			return fmt.Errorf("setdir command: %w", err)
		}

		// 1. Send cd command to tmux session
		if err := tmux.SendCommand(metadata.TmuxName, fmt.Sprintf("cd %s\r", newPath)); err != nil {
			return fmt.Errorf("failed to send cd command: %w", err)
		}

		// 2. Update AGM metadata
		metadata.WorkingDir = newPath
		if err := a.sessionStore.Set(SessionID(sessionIDStr), metadata); err != nil {
			return fmt.Errorf("failed to update working directory: %w", err)
		}

		return nil

	case CommandAuthorize:
		// Gemini CLI doesn't have runtime directory authorization
		// (directories are pre-authorized via --include-directories at session creation)
		return nil

	case CommandClearHistory:
		// Remove Gemini history file for this session
		historyPath, err := a.getHistoryPath(metadata)
		if err != nil {
			return fmt.Errorf("failed to get history path: %w", err)
		}

		// Remove history file (ignore if doesn't exist)
		if err := os.Remove(historyPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove history file: %w", err)
		}

		return nil

	case CommandSetSystemPrompt:
		// Update system prompt in session metadata
		prompt, err := getStringParam(cmd.Params, "prompt")
		if err != nil {
			return fmt.Errorf("set_system_prompt command: %w", err)
		}

		metadata.SystemPrompt = prompt
		if err := a.sessionStore.Set(SessionID(sessionIDStr), metadata); err != nil {
			return fmt.Errorf("failed to update system prompt: %w", err)
		}

		return nil

	case CommandRunHook:
		// Execute hook via Gemini CLI lifecycle
		hookName, err := getStringParam(cmd.Params, "hook_name")
		if err != nil {
			return fmt.Errorf("run_hook command: %w", err)
		}
		return a.executeHook(SessionID(sessionIDStr), metadata.TmuxName, hookName)

	default:
		return fmt.Errorf("unsupported command type: %s", cmd.Type)
	}
}

// RunHook executes a session lifecycle hook for the Gemini CLI.
//
// Triggers Gemini CLI hooks (SessionStart, SessionEnd, BeforeAgent, AfterAgent)
// via subprocess execution. Hooks output JSON that gets injected into session context.
//
// Hook failures are logged but don't block the session (graceful degradation).
func (a *GeminiCLIAdapter) RunHook(sessionID SessionID, hookName string) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	return a.executeHook(sessionID, metadata.TmuxName, hookName)
}

// executeHook runs a Gemini CLI hook and processes its output.
//
// Hooks are triggered via the Gemini CLI lifecycle. The hook script:
// 1. Receives hook name and session context as environment variables
// 2. Executes custom logic (e.g., log session start, update metadata)
// 3. Outputs JSON with context updates
//
// Hook output is parsed and injected into session context.
// Errors are logged but don't fail the operation (graceful degradation).
func (a *GeminiCLIAdapter) executeHook(sessionID SessionID, tmuxName, hookName string) error {
	// Get session metadata for hook context
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session metadata: %w", err)
	}

	// Build hook execution environment
	// Gemini CLI hooks are typically configured in ~/.gemini/config.yaml
	// and triggered via lifecycle events (SessionStart, SessionEnd, etc.)
	//
	// For now, we simulate hook execution by:
	// 1. Creating a hook ready-file signal (similar to Claude's approach)
	// 2. Allowing hooks to output JSON for context injection

	// Create hook ready-file directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to get home directory for hook: %v\n", err)
		return nil // Non-fatal: hooks are optional
	}

	hookDir := filepath.Join(homeDir, ".agm", "gemini-hooks")
	if err := os.MkdirAll(hookDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create hook directory: %v\n", err)
		return nil // Non-fatal
	}

	// Create hook execution marker
	hookFile := filepath.Join(hookDir, fmt.Sprintf("%s-%s.json", string(sessionID), hookName))

	// Prepare hook context data
	hookContext := map[string]interface{}{
		"session_id":   string(sessionID),
		"hook_name":    hookName,
		"session_name": metadata.Title,
		"working_dir":  metadata.WorkingDir,
		"project":      metadata.Project,
		"tmux_session": tmuxName,
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
	fmt.Fprintf(os.Stderr, "[Gemini Hook] Executed %s hook for session %s\n", hookName, metadata.Title)

	// TODO: In a future implementation, we could:
	// 1. Execute actual hook scripts via subprocess
	// 2. Parse JSON output from hooks
	// 3. Inject parsed data into session metadata
	// 4. Handle hook timeouts and failures
	//
	// For now, we create the hook context file which can be:
	// - Read by external hook scripts
	// - Used for debugging and testing
	// - Extended with actual subprocess execution

	return nil
}

// Helper functions

// getHistoryPath returns the path to the history file for a given session.
func (a *GeminiCLIAdapter) getHistoryPath(metadata *SessionMetadata) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".gemini", "sessions", metadata.TmuxName, "history.jsonl"), nil
}

// extractLatestGeminiUUID extracts the most recent Gemini session UUID for the given project directory.
//
// It runs `gemini --list-sessions` in the project directory and parses the output to find the latest UUID.
// Returns empty string if extraction fails (non-fatal - resume will use "latest" fallback).
func (a *GeminiCLIAdapter) extractLatestGeminiUUID(workingDir string) (string, error) {
	// Run gemini --list-sessions in the working directory
	// Output format (from investigation):
	// 0: Wed, Feb 26, 2025, 01:06:06 PM [23a6e871-bb1f-48ec-bdbe-1f6ae90f9686]
	// 1: Wed, Feb 26, 2025, 01:05:57 PM [8c123456-abcd-1234-5678-9012345678ab]
	//
	// Latest session is index 0 (most recent first)

	// Use exec.Command to run gemini --list-sessions
	cmd := exec.Command("gemini", "--list-sessions")
	cmd.Dir = workingDir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run gemini --list-sessions: %w", err)
	}

	// Parse output to extract UUID from first line
	// Expected format: "0: <timestamp> [<uuid>]"
	lines := strings.Split(string(output), "\n")
	if len(lines) == 0 {
		return "", fmt.Errorf("no sessions found in output")
	}

	// Find first line with UUID pattern [...]
	uuidPattern := regexp.MustCompile(`\[([a-f0-9-]+)\]`)
	for _, line := range lines {
		matches := uuidPattern.FindStringSubmatch(line)
		if len(matches) >= 2 {
			// matches[0] is full match "[uuid]", matches[1] is captured UUID
			return matches[1], nil
		}
	}

	return "", fmt.Errorf("no UUID found in --list-sessions output")
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
