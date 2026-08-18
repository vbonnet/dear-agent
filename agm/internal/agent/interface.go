package agent

import (
	"context"
	"time"
)

// Harness describes one adapter for heterogeneous discovery and conformance.
// Lifecycle and message behavior stays on concrete adapters or behind narrow
// interfaces defined by the consumer that owns the operation.
//
// Example usage:
//
//	harness, err := GetHarness("codex-cli")
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("%s %s\n", harness.Name(), harness.Version())
//
// Concrete adapters continue to expose harness-specific lifecycle mechanisms,
// while internal/ops owns cross-surface lifecycle transactions.
type Harness interface {
	// Name returns the canonical harness identifier.
	Name() string

	// Version returns the adapter's reported version or model identifier.
	Version() string

	// Capabilities returns descriptive feature and model metadata.
	Capabilities() Capabilities
}

var (
	_ Harness = (*ClaudeAdapter)(nil)
	_ Harness = (*GeminiCLIAdapter)(nil)
	_ Harness = (*CodexCLIAdapter)(nil)
	_ Harness = (*OpenCodeAdapter)(nil)
	_ Harness = (*AgyAdapter)(nil)
	_ Harness = (*PiAdapter)(nil)
	_ Harness = (*OpenAIAdapter)(nil)
)

// ContextMessageSender is implemented by adapters whose delivery surface can
// honor caller cancellation and deadlines.
type ContextMessageSender interface {
	SendMessageContext(ctx context.Context, sessionID SessionID, message Message) error
}

// ContextSessionStatusGetter is implemented by adapters whose readiness lookup
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
