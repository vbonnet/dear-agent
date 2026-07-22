package agent

import (
	"context"
	"time"
)

// Agent defines the interface all AI agents must implement.
//
// AGM uses this interface to give supported AI command-line harnesses a unified
// session management contract.
//
// Example usage:
//
//	harness, err := GetHarness("codex-cli")
//	if err != nil {
//	    return err
//	}
//	sessionID, err := harness.CreateSession(SessionContext{
//	    Name:             "my-session",
//	    WorkingDirectory: "~/project",
//	})
//	if err != nil {
//	    return err
//	}
//	err = harness.SendMessage(sessionID, Message{
//	    Role:    RoleUser,
//	    Content: "Hello, can you help me?",
//	})
//
// Implementations handle harness-specific command construction, authentication,
// process control, and session storage while conforming to this interface.
type Agent interface {
	// Name returns the canonical harness identifier.
	Name() string

	// Version returns the adapter's reported version or model identifier.
	Version() string

	// CreateSession creates a new agent session with the given context.
	//
	// The session context includes working directory, project info, and
	// pre-authorized directories. The returned SessionID is adapter-specific.
	//
	// Returns error if session creation fails (authentication, network,
	// invalid context).
	CreateSession(ctx SessionContext) (SessionID, error)

	// ResumeSession resumes an existing agent session by SessionID.
	//
	// An adapter may attach to an existing process or start its CLI resume path.
	//
	// Returns error if session not found or cannot be resumed.
	ResumeSession(sessionID SessionID) error

	// TerminateSession terminates an agent session.
	//
	// Active adapters terminate or detach their managed CLI process as defined by
	// the adapter's lifecycle contract.
	//
	// Returns error if session cannot be terminated.
	TerminateSession(sessionID SessionID) error

	// GetSessionStatus returns the current status of a session.
	//
	// Returns StatusActive, StatusIdle, StatusSuspended, or StatusTerminated.
	// Returns error if session not found.
	GetSessionStatus(sessionID SessionID) (Status, error)

	// SendMessage sends a message to the agent in the given session.
	//
	// Current adapters deliver through their managed CLI/tmux transport.
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

// ContextMessageSender is implemented by agents whose delivery surface can
// honor caller cancellation and deadlines. Callers should prefer this optional
// interface and fall back to Agent.SendMessage for legacy CLI adapters.
type ContextMessageSender interface {
	SendMessageContext(ctx context.Context, sessionID SessionID, message Message) error
}

// ContextSessionStatusGetter is implemented by agents whose readiness lookup
// can honor caller cancellation and deadlines. Pure API delivery requires this
// contract so a contended adapter store cannot pin the outer lifecycle lock.
type ContextSessionStatusGetter interface {
	GetSessionStatusContext(ctx context.Context, sessionID SessionID) (Status, error)
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

	// Model is the harness-native model alias or exact model label selected for
	// this session. Harnesses with a registry default may leave it empty.
	// Optional.
	Model string

	// InitialPrompt is the optional first user message. Harnesses that create
	// their durable native identity lazily may require it before CreateSession
	// can safely persist the session mapping.
	// Optional for harnesses with eager native identity creation.
	InitialPrompt string

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
	// SupportsSlashCommands indicates whether the harness supports translated
	// slash commands such as rename or clear.
	SupportsSlashCommands bool

	// SupportsHooks indicates if agent supports pre/post-command hooks.
	// May be AGM feature, not agent-specific.
	SupportsHooks bool

	// SupportsTools indicates whether the harness exposes tool/function calls.
	SupportsTools bool

	// SupportsVision indicates whether the selected harness/model accepts images.
	SupportsVision bool

	// SupportsMultimodal indicates whether the selected harness/model accepts
	// additional media such as audio or video.
	SupportsMultimodal bool

	// SupportsStreaming indicates whether response streaming is available.
	SupportsStreaming bool

	// SupportsSystemPrompts indicates whether system prompts are available.
	SupportsSystemPrompts bool

	// MaxContextWindow is the adapter-reported context limit in tokens.
	MaxContextWindow int

	// ModelName is the selected model identifier.
	ModelName string
}

// Command represents a generic agent operation.
//
// Commands are translated to agent-specific actions by adapters.
// Examples:
//   - CommandRename: translated to the harness-specific rename operation
//   - CommandSetDir: translated to the harness-specific directory operation
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
// It may be a native conversation ID or an AGM-owned mapping identifier.
type SessionID string

// Status represents the state of a session.
type Status string

const (
	// StatusActive indicates session is running and accepting messages.
	StatusActive Status = "active"

	// StatusIdle indicates the session is alive and waiting for input (e.g. a
	// Codex TUI showing its composer). It is a refinement of StatusActive used
	// by the supervisor to distinguish a worker that is ready for a prompt from
	// one that is busy processing.
	StatusIdle Status = "idle"

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
