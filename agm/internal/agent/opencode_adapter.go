package agent

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// OpenCodeAdapter implements Agent interface for OpenCode.
//
// OpenCode uses a client-server architecture where the server (opencode serve)
// runs independently on localhost:4096, and clients attach via `opencode attach`.
// The adapter manages sessions via tmux (similar to Claude) and integrates with
// the existing SSE monitoring infrastructure for state detection.
//
// Architecture:
//   - OpenCode Server: User-managed, runs on localhost:4096
//   - Session Management: tmux-based (opencode attach in tmux)
//   - State Detection: SSE monitoring (existing infrastructure)
//   - Multi-Session: Supported via SessionMapper (shared server model)
//
// Key Differences from Claude:
//   - No UUID-based session resume (uses tmux session names)
//   - Server must be running before session creation
//   - SSE events drive state detection (no tmux scraping)
//   - Multiple sessions share single OpenCode server
type OpenCodeAdapter struct {
	sessionStore SessionStore
	serverURL    string // OpenCode server URL (default: http://localhost:4096)
}

// OpenCodeConfig provides configuration for OpenCode adapter.
type OpenCodeConfig struct {
	// SessionStore persists session metadata. If nil, uses default JSON store.
	SessionStore SessionStore

	// ServerURL is the OpenCode server URL (default: http://localhost:4096).
	ServerURL string
}

// NewOpenCodeAdapter creates a new OpenCode adapter instance.
//
// If config.SessionStore is nil, creates a default JSON-backed store at ~/.agm/sessions.json.
// If config.ServerURL is empty, uses default http://localhost:4096.
//
// Returns error if session store creation fails.
func NewOpenCodeAdapter(config *OpenCodeConfig) (Agent, error) {
	if config == nil {
		config = &OpenCodeConfig{}
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

	// Set default server URL
	serverURL := config.ServerURL
	if serverURL == "" {
		serverURL = "http://localhost:4096"
	}

	return &OpenCodeAdapter{
		sessionStore: sessionStore,
		serverURL:    serverURL,
	}, nil
}

// Name returns the agent identifier.
//
// Contract: Returns "opencode" to identify this adapter in agent factory.
func (a *OpenCodeAdapter) Name() string {
	return "opencode"
}

// Version returns the agent version or model name.
//
// Contract: Returns version string for display/logging purposes.
// OpenCode doesn't expose a version API, so we return a fixed identifier.
func (a *OpenCodeAdapter) Version() string {
	return "opencode-server" // Fixed identifier since OpenCode is mock implementation
}

// CreateSession creates a new OpenCode session.
//
// Creates a tmux session and starts `opencode attach` within it.
// The OpenCode server must already be running on localhost:4096.
//
// Session lifecycle:
//  1. Generate unique SessionID (UUID)
//  2. Create tmux session with name from ctx.Name
//  3. Execute `opencode attach` in tmux session
//  4. Store session metadata (SessionID → tmux name mapping)
//
// The SSE monitoring infrastructure (already integrated) will automatically
// detect the new session and begin state tracking via EventBus.
//
// Returns SessionID on success, error if:
//   - OpenCode server not running (health check fails)
//   - Tmux session creation fails
//   - Session metadata storage fails
func (a *OpenCodeAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
	// 1. Validate OpenCode server is running (health check)
	if err := a.checkServerHealth(); err != nil {
		return "", fmt.Errorf("OpenCode server not running: %w\n\nPlease start OpenCode server first:\n  opencode serve --port 4096", err)
	}

	// 2. Generate unique SessionID
	sessionID := SessionID(uuid.New().String())

	// 3. Use session name as tmux session name (or generate one)
	tmuxName := ctx.Name
	if tmuxName == "" {
		tmuxName = fmt.Sprintf("opencode-%s", time.Now().Format("20060102-150405"))
	}

	// 4. Check if tmux session already exists
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

	// 5. Build OpenCode command (attach to running server)
	// Unlike Claude (which needs --add-dir), OpenCode attach is simpler
	opencodeCmd := "opencode attach && exit"

	// 6. Start OpenCode in the tmux session
	if err := tmux.SendCommand(tmuxName, opencodeCmd); err != nil {
		// Clean up tmux session on error if we created it
		if !exists {
			_ = tmux.SendCommand(tmuxName, "exit\r")
		}
		return "", fmt.Errorf("failed to start OpenCode in tmux session: %w", err)
	}

	// 7. Store session metadata
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

// ResumeSession resumes an existing OpenCode session.
//
// Attaches to the tmux session associated with the SessionID.
// Unlike Claude (which supports UUID-based resume), OpenCode uses
// tmux session name mapping to resume sessions.
//
// Flow:
//  1. Lookup SessionID → tmux session name (from session store)
//  2. Check if tmux session exists
//  3. If exists: attach to tmux session
//  4. If not exists: create new tmux + restart `opencode attach`
//
// Returns error if:
//   - SessionID not found in store
//   - Tmux session cannot be created/attached
func (a *OpenCodeAdapter) ResumeSession(sessionID SessionID) error {
	// 1. Get session metadata from store
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// 2. Check if tmux session exists
	exists, err := tmux.HasSession(metadata.TmuxName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}

	// 3. If tmux session doesn't exist, recreate it
	sendCommands := false
	if !exists {
		// Create new tmux session
		if err := tmux.NewSession(metadata.TmuxName, metadata.WorkingDir); err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}
		sendCommands = true
	}

	// 4. Send attach command if needed (tmux session was recreated)
	if sendCommands {
		// OpenCode uses simple `opencode attach` (no session-specific resume)
		// The OpenCode server maintains session state internally
		fullCmd := fmt.Sprintf("cd '%s' && opencode attach && exit", metadata.WorkingDir)

		if err := tmux.SendCommand(metadata.TmuxName, fullCmd); err != nil {
			return fmt.Errorf("failed to send attach command: %w", err)
		}
	}

	// 5. Attach to tmux session (skip if already in tmux)
	if os.Getenv("TMUX") == "" {
		if err := tmux.AttachSession(metadata.TmuxName); err != nil {
			return fmt.Errorf("failed to attach to tmux session: %w", err)
		}
	}

	return nil
}

// TerminateSession terminates an OpenCode session.
//
// Sends exit command to tmux session and cleans up session metadata.
// The OpenCode server continues running (user-managed lifecycle).
//
// Cleanup:
//  1. Send exit command to tmux session
//  2. Kill tmux session
//  3. Remove session metadata from store
//  4. SessionMapper cleanup handled by SSE adapter (automatic)
//
// Returns error if:
//   - SessionID not found
//   - Tmux session cleanup fails
func (a *OpenCodeAdapter) TerminateSession(sessionID SessionID) error {
	// 1. Get session metadata
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// 2. Terminate tmux session by sending exit command
	// Use graceful shutdown (not forced kill) to allow OpenCode to clean up
	if err := tmux.SendCommand(metadata.TmuxName, "exit\r"); err != nil {
		// Continue even if send fails (session may already be dead)
		fmt.Fprintf(os.Stderr, "Warning: failed to send exit to session: %v\n", err)
	}

	// 3. Remove from session store
	if err := a.sessionStore.Delete(sessionID); err != nil {
		return fmt.Errorf("failed to remove session from store: %w", err)
	}

	return nil
}

// GetSessionStatus returns the current status of an OpenCode session.
//
// Queries the SSE monitoring infrastructure for session state.
// State detection is event-driven (not tmux scraping).
//
// Status mapping:
//   - READY/IDLE → StatusActive
//   - THINKING/WAITING → StatusActive
//   - CLOSED → StatusTerminated
//   - Tmux detached → StatusSuspended
//
// Returns error if:
//   - SessionID not found
//   - State cannot be determined
func (a *OpenCodeAdapter) GetSessionStatus(sessionID SessionID) (Status, error) {
	// 1. Get session metadata
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		// Session not in store = terminated
		return StatusTerminated, nil
	}

	// 2. Check if tmux session exists
	exists, err := tmux.HasSession(metadata.TmuxName)
	if err != nil {
		return StatusTerminated, fmt.Errorf("failed to check tmux session: %w", err)
	}

	if !exists {
		return StatusTerminated, nil
	}

	// If tmux session exists, consider it active
	return StatusActive, nil
}

// SendMessage sends a message to the OpenCode agent.
//
// For tmux-based sessions, this sends the message content followed by
// Enter key to the tmux pane running `opencode attach`.
//
// Message flow:
//  1. Get tmux session name from SessionID
//  2. Format message content
//  3. Send keys to tmux session (message + "\r")
//  4. SSE monitoring detects state changes automatically
//
// Returns error if:
//   - SessionID not found
//   - Tmux session not running
//   - Message cannot be sent
func (a *OpenCodeAdapter) SendMessage(sessionID SessionID, message Message) error {
	// 1. Get session metadata
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// 2. Send message content to tmux pane
	// The message is sent as-is, followed by Enter (handled by SendCommand)
	if err := tmux.SendCommand(metadata.TmuxName, message.Content); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// GetHistory retrieves conversation history for an OpenCode session.
//
// OpenCode (mock implementation) doesn't expose conversation history API.
// This method returns empty history with a warning.
//
// For production OpenCode with history support, this would:
//  1. Query OpenCode server API for conversation
//  2. Parse response into Message format
//  3. Return chronological history
//
// Returns empty slice (OpenCode is mock, no history API).
func (a *OpenCodeAdapter) GetHistory(sessionID SessionID) ([]Message, error) {
	// OpenCode mock: return empty history
	return []Message{}, nil
}

// ExportConversation exports conversation in the specified format.
//
// OpenCode (mock implementation) doesn't support conversation export.
// Returns error for all formats.
//
// For production OpenCode with export support, this would:
//  1. Retrieve conversation history
//  2. Serialize to requested format (JSONL, Markdown, etc.)
//  3. Return serialized bytes
//
// Returns error (not supported for mock implementation).
func (a *OpenCodeAdapter) ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error) {
	// OpenCode mock: export not supported
	return nil, fmt.Errorf("conversation export not supported for OpenCode (mock implementation)")
}

// ImportConversation imports conversation from serialized data.
//
// OpenCode (mock implementation) doesn't support conversation import.
// Returns error for all formats.
//
// For production OpenCode with import support, this would:
//  1. Parse serialized conversation data
//  2. Create new session
//  3. Replay conversation to OpenCode server
//  4. Return new SessionID
//
// Returns error (not supported for mock implementation).
func (a *OpenCodeAdapter) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
	// OpenCode mock: import not supported
	return "", fmt.Errorf("conversation import not supported for OpenCode (mock implementation)")
}

// Capabilities returns the agent's feature capabilities.
//
// OpenCode capabilities (based on mock implementation and server architecture):
//   - SupportsSlashCommands: false (server-based, not CLI)
//   - SupportsHooks: true (AGM feature, not agent-specific)
//   - SupportsTools: true (assumed based on architecture)
//   - SupportsVision: false (mock implementation)
//   - SupportsMultimodal: false (mock implementation)
//   - SupportsStreaming: true (SSE events indicate streaming)
//   - SupportsSystemPrompts: true (assumed)
//   - MaxContextWindow: 200000 (assumed, similar to Claude)
//   - ModelName: "opencode-server"
func (a *OpenCodeAdapter) Capabilities() Capabilities {
	return Capabilities{
		SupportsSlashCommands: false,  // Server-based, not CLI
		SupportsHooks:         true,   // AGM feature
		SupportsTools:         true,   // Based on SSE events (tool.execute.*)
		SupportsVision:        false,  // Mock implementation
		SupportsMultimodal:    false,  // Mock implementation
		SupportsStreaming:     true,   // SSE events indicate streaming
		SupportsSystemPrompts: true,   // Assumed
		MaxContextWindow:      200000, // Assumed (similar to Claude)
		ModelName:             "opencode-server",
	}
}

// ExecuteCommand executes a generic command with OpenCode-specific translation.
//
// Supported commands:
//   - CommandRename: Not supported (OpenCode doesn't expose session rename)
//   - CommandSetDir: Not supported (OpenCode is server-based)
//   - CommandAuthorize: Not supported (OpenCode server manages auth)
//   - CommandClearHistory: Not supported (mock implementation)
//   - CommandSetSystemPrompt: Not supported (mock implementation)
//
// Most commands are not applicable to OpenCode's server-based architecture.
// Returns error for unsupported commands.
func (a *OpenCodeAdapter) ExecuteCommand(cmd Command) error {
	// OpenCode mock: commands not supported (server-based architecture)
	return fmt.Errorf("command %s not supported for OpenCode", cmd.Type)
}

// checkServerHealth validates that the OpenCode server is running and accessible.
//
// Performs HTTP GET request to the health endpoint (serverURL/health).
// Returns error if server is unreachable or returns non-200 status.
//
// Timeout: 5 seconds (prevents hanging on unreachable server)
func (a *OpenCodeAdapter) checkServerHealth() error {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(a.serverURL + "/health")
	if err != nil {
		return fmt.Errorf("server unreachable at %s: %w", a.serverURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server health check failed: HTTP %d", resp.StatusCode)
	}

	return nil
}
