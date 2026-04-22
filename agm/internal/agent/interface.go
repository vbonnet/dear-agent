package agent

import (
	"time"
)

// Agent defines the interface all AI agents must implement.
//
// AGM (Agent Manager) uses this interface to support multiple AI providers
// (Claude, Gemini, GPT) with a unified session management experience.
//
// Example usage:
//
//	agent := claude.NewAdapter()
//	sessionID, err := agent.CreateSession(SessionContext{
//	    Name:             "my-session",
//	    WorkingDirectory: "~/project",
//	})
//	if err != nil {
//	    return err
//	}
//	err = agent.SendMessage(sessionID, Message{
//	    Role:    RoleUser,
//	    Content: "Hello, can you help me?",
//	})
//
// Implementations must handle agent-specific details (authentication,
// API endpoints, session storage) while conforming to this interface.
type Agent interface {
	// Name returns the agent identifier (e.g., "claude", "gemini", "gpt").
	Name() string

	// Version returns the agent version or model name (e.g., "claude-sonnet-4.5").
	Version() string

	// CreateSession creates a new agent session with the given context.
	//
	// The session context includes working directory, project info, and
	// pre-authorized directories. The returned SessionID is agent-specific
	// (e.g., Claude UUID, Gemini session ID).
	//
	// Returns error if session creation fails (authentication, network,
	// invalid context).
	CreateSession(ctx SessionContext) (SessionID, error)

	// ResumeSession resumes an existing agent session by SessionID.
	//
	// For CLI agents (Claude), this attaches to existing tmux session.
	// For API agents (Gemini, GPT), this loads conversation history.
	//
	// Returns error if session not found or cannot be resumed.
	ResumeSession(sessionID SessionID) error

	// TerminateSession terminates an agent session.
	//
	// For CLI agents, this exits the agent process.
	// For API agents, this may be a no-op (sessions are stateless).
	//
	// Returns error if session cannot be terminated.
	TerminateSession(sessionID SessionID) error

	// GetSessionStatus returns the current status of a session.
	//
	// Returns StatusActive, StatusSuspended, or StatusTerminated.
	// Returns error if session not found.
	GetSessionStatus(sessionID SessionID) (Status, error)

	// SendMessage sends a message to the agent in the given session.
	//
	// For CLI agents, this sends keys to tmux pane.
	// For API agents, this makes API call with message + history.
	//
	// Returns error if message cannot be sent.
	SendMessage(sessionID SessionID, message Message) error

	// GetHistory retrieves conversation history for a session.
	//
	// Returns all messages in the session's conversation history.
	// Returns error if history cannot be retrieved.
	GetHistory(sessionID SessionID) ([]Message, error)

	// ExportConversation exports conversation in the specified format.
	//
	// Supported formats: jsonl (universal), html (Claude), markdown (readable).
	// Returns serialized conversation data.
	// Returns error if format unsupported or export fails.
	ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error)

	// ImportConversation imports conversation from serialized data.
	//
	// Creates new session from exported conversation data.
	// Returns new SessionID and error if import fails.
	ImportConversation(data []byte, format ConversationFormat) (SessionID, error)

	// Capabilities returns the agent's feature capabilities.
	//
	// Used for runtime feature detection and graceful degradation.
	Capabilities() Capabilities

	// ExecuteCommand executes a generic command with agent-specific translation.
	//
	// Examples: rename_session, set_directory, authorize_directory.
	// Command translation happens in adapter implementation.
	// Returns error if command unsupported or execution fails.
	ExecuteCommand(cmd Command) error
}

// SessionContext provides parameters for creating a new agent session.
type SessionContext struct {
	// Name is the session name (used for tmux session name).
	// Required.
	Name string

	// WorkingDirectory is the initial working directory for the session.
	// Required.
	WorkingDirectory string

	// Project is the project identifier (e.g., "ai-tools", "engram").
	// Optional.
	Project string

	// AuthorizedDirs are directories pre-authorized for agent access.
	// Optional. If empty, agent may prompt for directory authorization.
	AuthorizedDirs []string

	// Environment contains environment variables for the session.
	// Optional.
	Environment map[string]string

	// WorkflowName specifies the execution mode for this session.
	// Examples: "deep-research", "code-review", "architect".
	// Optional. If empty, session runs in default conversational mode.
	WorkflowName string
}

// Message represents a single message in a conversation.
type Message struct {
	// ID is a unique message identifier (UUID).
	ID string

	// Role is the message sender (user or assistant).
	Role Role

	// Content is the message text content.
	Content string

	// Timestamp is when the message was created.
	Timestamp time.Time

	// Metadata contains agent-specific data (tool use, tokens, model info).
	// Optional.
	Metadata map[string]interface{}
}

// Role represents the sender of a message.
type Role string

const (
	// RoleUser represents a message from the user.
	RoleUser Role = "user"

	// RoleAssistant represents a message from the assistant.
	RoleAssistant Role = "assistant"
)

// Capabilities describes the features an agent supports.
//
// Used for runtime feature detection and graceful degradation.
type Capabilities struct {
	// SupportsSlashCommands indicates if agent supports slash commands
	// (e.g., /rename, /clear). True for Claude CLI, false for API agents.
	SupportsSlashCommands bool

	// SupportsHooks indicates if agent supports pre/post-command hooks.
	// May be AGM feature, not agent-specific.
	SupportsHooks bool

	// SupportsTools indicates if agent supports tool/function calls.
	// True for Claude, GPT; Gemini has similar (functions).
	SupportsTools bool

	// SupportsVision indicates if agent can process images.
	// True for modern models (Claude Opus/Sonnet, GPT-4V, Gemini Pro Vision).
	SupportsVision bool

	// SupportsMultimodal indicates if agent supports audio/video.
	// Future-proofing for next-gen models.
	SupportsMultimodal bool

	// SupportsStreaming indicates if agent supports streaming responses.
	// True for most modern APIs (Claude, GPT, Gemini).
	SupportsStreaming bool

	// SupportsSystemPrompts indicates if agent supports system prompts.
	// True for Claude, GPT-4; Gemini (via conversation prefix).
	SupportsSystemPrompts bool

	// MaxContextWindow is the maximum context window size in tokens.
	// Varies by model (Claude: 200K, GPT-4: 128K, Gemini: 1M+).
	MaxContextWindow int

	// ModelName is the underlying model identifier.
	// Examples: "claude-sonnet-4.5", "gpt-4-turbo", "gemini-2.0-flash".
	ModelName string
}

// Command represents a generic agent operation.
//
// Commands are translated to agent-specific actions by adapters.
// Examples:
//   - CommandRename: Claude → "/rename {name}\r", Gemini → API call
//   - CommandSetDir: Claude → "cd {path}\r", Gemini → update context
type Command struct {
	// Type is the command type.
	Type CommandType

	// Params contains command-specific parameters.
	Params map[string]interface{}
}

// CommandType identifies a generic operation.
type CommandType string

const (
	// CommandRename renames the current session.
	// Params: session_id (SessionID), name (string)
	CommandRename CommandType = "rename_session"

	// CommandSetDir changes the working directory.
	// Params: session_id (SessionID), path (string)
	CommandSetDir CommandType = "set_directory"

	// CommandRunHook executes a pre/post-command hook.
	// Params: hook_name (string), script (string)
	CommandRunHook CommandType = "run_hook"

	// CommandAuthorize authorizes a directory for agent access.
	// Params: session_id (SessionID), path (string)
	CommandAuthorize CommandType = "authorize_directory"

	// CommandClearHistory clears conversation history.
	// Params: session_id (SessionID)
	CommandClearHistory CommandType = "clear_history"

	// CommandSetSystemPrompt sets or updates system prompt.
	// Params: session_id (SessionID), prompt (string)
	CommandSetSystemPrompt CommandType = "set_system_prompt"
)

// SessionID is an opaque agent-specific session identifier.
//
// For Claude: UUID from history.jsonl (e.g., "550e8400-e29b-41d4-a716-446655440000")
// For Gemini: API session ID or file path
// For GPT: Thread ID or conversation ID
type SessionID string

// Status represents the state of a session.
type Status string

const (
	// StatusActive indicates session is running and accepting messages.
	StatusActive Status = "active"

	// StatusSuspended indicates session is paused (tmux detached).
	StatusSuspended Status = "suspended"

	// StatusTerminated indicates session has ended.
	StatusTerminated Status = "terminated"
)

// ConversationFormat specifies the serialization format for conversations.
type ConversationFormat string

const (
	// FormatJSONL is the universal JSON Lines format (agent-agnostic).
	// Primary format for AGM conversation storage.
	FormatJSONL ConversationFormat = "jsonl"

	// FormatHTML is the HTML transcript format (Claude-specific).
	// Used for legacy compatibility with existing AGM transcripts.
	FormatHTML ConversationFormat = "html"

	// FormatMarkdown is a human-readable markdown format.
	// Used for export/sharing.
	FormatMarkdown ConversationFormat = "markdown"

	// FormatNative is the agent-specific native format.
	// Claude: history.jsonl, Gemini: API JSON, GPT: Thread JSON
	FormatNative ConversationFormat = "native"
)
