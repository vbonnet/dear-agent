# AGM API Reference

Complete API reference for developers integrating with or extending AGM.

**Version**: 3.0
**Last Updated**: 2026-02-04

---

## Table of Contents

- [Go Package API](#go-package-api)
- [Agent Interface](#agent-interface)
- [CommandTranslator Interface](#commandtranslator-interface)
- [Session Manager API](#session-manager-api)
- [Manifest API](#manifest-api)
- [Tmux Integration API](#tmux-integration-api)
- [Message Logging API](#message-logging-api)
- [Agent Adapters](#agent-adapters)
- [Exit Codes](#exit-codes)

---

## Go Package API

### Importing Packages

```go
import (
    "github.com/vbonnet/dear-agent/agm/internal/agent"
    "github.com/vbonnet/dear-agent/agm/internal/command"
    "github.com/vbonnet/dear-agent/agm/internal/manifest"
    "github.com/vbonnet/dear-agent/agm/internal/session"
    "github.com/vbonnet/dear-agent/agm/internal/tmux"
)
```

---

## Agent Interface

### Interface Definition

```go
// Agent represents an AI agent (Claude, Gemini, GPT)
type Agent interface {
    // Start initializes and starts the agent session
    Start(ctx context.Context, sessionID string, opts *StartOptions) error

    // IsAvailable checks if agent is configured (API keys, CLI installed)
    IsAvailable() bool

    // GetMetadata returns agent metadata (name, version, capabilities)
    GetMetadata() *AgentMetadata

    // GetTranslator returns the command translator for this agent
    GetTranslator() command.Translator
}
```

### StartOptions

```go
type StartOptions struct {
    ProjectDir  string            // Working directory
    InitPrompt  string            // Initial prompt to send
    Environment map[string]string // Environment variables
    Detached    bool              // Create without attaching
}
```

### AgentMetadata

```go
type AgentMetadata struct {
    Name         string   // "claude", "gemini", "gpt"
    DisplayName  string   // "Claude (Anthropic)"
    Version      string   // Agent version
    ContextLimit int      // Max context tokens
    Capabilities []string // Supported features
    Available    bool     // API key configured
}
```

### Example Usage

```go
package main

import (
    "context"
    "github.com/vbonnet/dear-agent/agm/internal/agent"
)

func main() {
    // Get Claude agent
    claudeAgent := agent.NewClaudeAdapter()

    // Check availability
    if !claudeAgent.IsAvailable() {
        panic("Claude API key not configured")
    }

    // Start session
    opts := &agent.StartOptions{
        ProjectDir: "~/projects/myapp",
        InitPrompt: "Please review the authentication code",
    }

    ctx := context.Background()
    err := claudeAgent.Start(ctx, "my-session-uuid", opts)
    if err != nil {
        panic(err)
    }
}
```

---

## CommandTranslator Interface

### Interface Definition

```go
// Translator provides unified command interface across agents
type Translator interface {
    // RenameSession renames the agent's session/conversation
    RenameSession(ctx context.Context, sessionID, newName string) error

    // SetDirectory sets the working directory context
    SetDirectory(ctx context.Context, sessionID, dirPath string) error

    // RunHook executes agent-specific initialization hook
    RunHook(ctx context.Context, sessionID, hookType string) error
}
```

### Error Types

```go
var (
    // ErrNotSupported indicates command not supported by agent
    ErrNotSupported = errors.New("command not supported by agent")

    // ErrTimeout indicates command execution timeout
    ErrTimeout = errors.New("command execution timeout")

    // ErrSessionNotFound indicates session doesn't exist
    ErrSessionNotFound = errors.New("session not found")
)
```

### Example Usage

```go
package main

import (
    "context"
    "errors"
    "time"
    "github.com/vbonnet/dear-agent/agm/internal/command"
    "github.com/vbonnet/dear-agent/agm/internal/agent"
)

func renameSession(agent agent.Agent, sessionID, newName string) error {
    translator := agent.GetTranslator()

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    err := translator.RenameSession(ctx, sessionID, newName)
    if errors.Is(err, command.ErrNotSupported) {
        // Graceful degradation: update manifest only
        return updateManifestName(sessionID, newName)
    }
    return err
}
```

---

## Session Manager API

### SessionManager Type

```go
type SessionManager struct {
    sessionsDir string
    manifestReader *manifest.Reader
    manifestWriter *manifest.Writer
}

func NewSessionManager(sessionsDir string) *SessionManager
```

### Methods

#### Create Session

```go
func (sm *SessionManager) Create(opts *CreateOptions) (*manifest.Manifest, error)

type CreateOptions struct {
    Name       string   // Session name
    Agent      string   // "claude", "gemini", "gpt"
    ProjectDir string   // Working directory
    Tags       []string // Session tags
}
```

#### Get Session

```go
func (sm *SessionManager) Get(name string) (*manifest.Manifest, error)
```

#### List Sessions

```go
func (sm *SessionManager) List(filter *ListFilter) ([]*manifest.Manifest, error)

type ListFilter struct {
    IncludeArchived bool
    Agent           string // Filter by agent
    Tags            []string // Filter by tags
}
```

#### Archive Session

```go
func (sm *SessionManager) Archive(name string) error
```

#### Unarchive Session

```go
func (sm *SessionManager) Unarchive(name string) error
```

### Example Usage

```go
package main

import (
    "github.com/vbonnet/dear-agent/agm/internal/session"
)

func main() {
    manager := session.NewSessionManager("~/sessions")

    // Create session
    opts := &session.CreateOptions{
        Name:       "my-coding-session",
        Agent:      "claude",
        ProjectDir: "~/projects/myapp",
        Tags:       []string{"coding", "backend"},
    }

    manifest, err := manager.Create(opts)
    if err != nil {
        panic(err)
    }

    // List sessions
    filter := &session.ListFilter{
        IncludeArchived: false,
        Agent:           "claude",
    }

    sessions, err := manager.List(filter)
    if err != nil {
        panic(err)
    }

    for _, s := range sessions {
        fmt.Printf("Session: %s (%s)\n", s.TmuxSessionName, s.Agent)
    }
}
```

---

## Manifest API

### Manifest Schema

```go
type Manifest struct {
    Version         string            `yaml:"version"`         // "2.0"
    SessionID       string            `yaml:"session_id"`      // UUID
    TmuxSessionName string            `yaml:"tmux_session_name"`
    Agent           string            `yaml:"agent"`           // "claude", "gemini", "gpt"
    Lifecycle       string            `yaml:"lifecycle"`       // "active", "stopped", "archived"
    Context         Context           `yaml:"context"`
    Metadata        Metadata          `yaml:"metadata"`
    Claude          *ClaudeMetadata   `yaml:"claude,omitempty"`
    Gemini          *GeminiMetadata   `yaml:"gemini,omitempty"`
}

type Context struct {
    Project string   `yaml:"project"` // Project directory
    Tags    []string `yaml:"tags"`    // Session tags
}

type Metadata struct {
    CreatedAt time.Time `yaml:"created_at"`
    UpdatedAt time.Time `yaml:"updated_at"`
}

type ClaudeMetadata struct {
    UUID    string `yaml:"uuid"`    // Claude-specific UUID
    Version string `yaml:"version"` // Claude CLI version
}

type GeminiMetadata struct {
    ConversationID string `yaml:"conversation_id"`
    ModelVersion   string `yaml:"model_version"`
}
```

### Reader API

```go
type Reader struct {}

func NewReader() *Reader

// Read reads manifest from session directory
func (r *Reader) Read(sessionDir string) (*Manifest, error)

// ReadFile reads manifest from specific file path
func (r *Reader) ReadFile(filePath string) (*Manifest, error)

// Validate validates manifest schema
func (r *Reader) Validate(m *Manifest) error
```

### Writer API

```go
type Writer struct {}

func NewWriter() *Writer

// Write writes manifest to session directory
func (w *Writer) Write(sessionDir string, m *Manifest) error

// WriteFile writes manifest to specific file path
func (w *Writer) WriteFile(filePath string, m *Manifest) error

// CreateBackup creates numbered backup before writing
func (w *Writer) CreateBackup(sessionDir string) error
```

### Example Usage

```go
package main

import (
    "time"
    "github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func main() {
    // Create manifest
    m := &manifest.Manifest{
        Version:         "2.0",
        SessionID:       "abc-def-123",
        TmuxSessionName: "my-session",
        Agent:           "claude",
        Lifecycle:       "active",
        Context: manifest.Context{
            Project: "~/projects/myapp",
            Tags:    []string{"coding"},
        },
        Metadata: manifest.Metadata{
            CreatedAt: time.Now(),
            UpdatedAt: time.Now(),
        },
    }

    // Write manifest
    writer := manifest.NewWriter()
    err := writer.Write("~/sessions/my-session", m)
    if err != nil {
        panic(err)
    }

    // Read manifest
    reader := manifest.NewReader()
    loaded, err := reader.Read("~/sessions/my-session")
    if err != nil {
        panic(err)
    }

    // Validate
    if err := reader.Validate(loaded); err != nil {
        panic(err)
    }
}
```

---

## Tmux Integration API

### Core Functions

#### Session Management

```go
// HasSession checks if tmux session exists
func HasSession(name string) (bool, error)

// NewSession creates new tmux session
func NewSession(name, workDir string, detached bool) error

// KillSession terminates tmux session
func KillSession(name string) error

// AttachSession attaches to tmux session (requires TTY)
func AttachSession(name string) error

// ListSessions lists all tmux sessions
func ListSessions() ([]SessionInfo, error)

type SessionInfo struct {
    Name    string
    Created time.Time
    Windows int
}
```

#### Control Mode

```go
// ControlModeSession represents tmux control mode connection
type ControlModeSession struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout io.ReadCloser
    watcher *OutputWatcher
}

// StartControlMode starts tmux in control mode (-C)
func StartControlMode(sessionName string) (*ControlModeSession, error)

// SendCommand sends command to control mode session
func (c *ControlModeSession) SendCommand(command string) error

// Close terminates control mode session
func (c *ControlModeSession) Close() error
```

#### Prompt Injection

```go
// SendPromptLiteral sends prompt to tmux session (literal mode)
func SendPromptLiteral(target, prompt string) error

// SendPromptFromFile sends prompt from file (max 10KB)
func SendPromptFromFile(target, filePath string) error
```

### Example Usage

```go
package main

import (
    "github.com/vbonnet/dear-agent/agm/internal/tmux"
)

func main() {
    // Check if session exists
    exists, err := tmux.HasSession("my-session")
    if err != nil {
        panic(err)
    }

    if !exists {
        // Create new session
        err = tmux.NewSession("my-session", "~/projects/myapp", false)
        if err != nil {
            panic(err)
        }
    }

    // Send prompt to session
    err = tmux.SendPromptLiteral("my-session", "Please analyze the code")
    if err != nil {
        panic(err)
    }

    // List all sessions
    sessions, err := tmux.ListSessions()
    if err != nil {
        panic(err)
    }

    for _, s := range sessions {
        fmt.Printf("Session: %s (%d windows)\n", s.Name, s.Windows)
    }
}
```

---

## Message Logging API

### MessageLogger Type

```go
type MessageLogger struct {
    logsDir string
}

func NewMessageLogger(logsDir string) (*MessageLogger, error)
```

### MessageLogEntry

```go
type MessageLogEntry struct {
    MessageID string    `json:"message_id"` // Unique message ID
    Timestamp time.Time `json:"timestamp"`  // ISO 8601
    Sender    string    `json:"sender"`     // Sender name
    Recipient string    `json:"recipient"`  // Recipient session
    Message   string    `json:"message"`    // Message content
    ReplyTo   string    `json:"reply_to,omitempty"` // Reply-to message ID
}
```

### Methods

```go
// LogMessage appends message to daily log file
func (l *MessageLogger) LogMessage(entry *MessageLogEntry) error

// GetStats returns log statistics
func (l *MessageLogger) GetStats() (*LogStats, error)

type LogStats struct {
    TotalFiles    int
    TotalMessages int
    OldestLog     time.Time
    NewestLog     time.Time
    DiskUsage     int64 // Bytes
}

// CleanupOldLogs deletes logs older than retentionDays
func (l *MessageLogger) CleanupOldLogs(retentionDays int) (int, error)
```

### Example Usage

```go
package main

import (
    "time"
    "github.com/vbonnet/dear-agent/agm/internal/messages"
)

func main() {
    logger, err := messages.NewMessageLogger("~/.agm/logs/messages")
    if err != nil {
        panic(err)
    }

    // Log message
    entry := &messages.MessageLogEntry{
        MessageID: "1738612345678-agm-send-001",
        Timestamp: time.Now(),
        Sender:    "agm-send",
        Recipient: "my-session",
        Message:   "Please analyze the code",
        ReplyTo:   nil,
    }

    err = logger.LogMessage(entry)
    if err != nil {
        panic(err)
    }

    // Get stats
    stats, err := logger.GetStats()
    if err != nil {
        panic(err)
    }

    fmt.Printf("Total messages: %d\n", stats.TotalMessages)
    fmt.Printf("Disk usage: %d bytes\n", stats.DiskUsage)

    // Cleanup old logs (older than 90 days)
    deleted, err := logger.CleanupOldLogs(90)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Deleted %d log files\n", deleted)
}
```

---

## Agent Adapters

### Claude Adapter

```go
package agent

type ClaudeAdapter struct {
    apiKey string
}

func NewClaudeAdapter() *ClaudeAdapter

func (a *ClaudeAdapter) Start(ctx context.Context, sessionID string, opts *StartOptions) error
func (a *ClaudeAdapter) IsAvailable() bool
func (a *ClaudeAdapter) GetMetadata() *AgentMetadata
func (a *ClaudeAdapter) GetTranslator() command.Translator
```

### Gemini Adapter

```go
package agent

type GeminiAdapter struct {
    apiKey    string
    projectID string
}

func NewGeminiAdapter() *GeminiAdapter

func (a *GeminiAdapter) Start(ctx context.Context, sessionID string, opts *StartOptions) error
func (a *GeminiAdapter) IsAvailable() bool
func (a *GeminiAdapter) GetMetadata() *AgentMetadata
func (a *GeminiAdapter) GetTranslator() command.Translator
```

---

## Exit Codes

AGM uses standard exit codes for automation and scripting:

```go
const (
    ExitSuccess         = 0   // Success
    ExitGeneralError    = 1   // General error
    ExitUsageError      = 2   // Misuse of command (invalid arguments)
    ExitSessionNotFound = 3   // Session not found
    ExitLockFailed      = 4   // Lock acquisition failed
    ExitInterrupted     = 130 // Interrupted by user (Ctrl+C)
)
```

### Example Shell Usage

```bash
#!/bin/bash

agm resume my-session
EXIT_CODE=$?

case $EXIT_CODE in
    0)
        echo "Session resumed successfully"
        ;;
    3)
        echo "Session not found - creating new session"
        agm new my-session
        ;;
    4)
        echo "Session locked - waiting and retrying"
        sleep 5
        agm resume my-session
        ;;
    *)
        echo "Error: $EXIT_CODE"
        exit $EXIT_CODE
        ;;
esac
```

---

## JSON Output Format

Many commands support `--json` flag for machine-readable output:

### agm list --json

```json
[
  {
    "name": "my-coding-session",
    "status": "active",
    "agent": "claude",
    "project": "~/projects/myapp",
    "tags": ["coding", "backend"],
    "created_at": "2026-02-04T10:00:00Z",
    "updated_at": "2026-02-04T14:30:00Z",
    "session_id": "abc-def-123"
  },
  {
    "name": "research-task",
    "status": "stopped",
    "agent": "gemini",
    "project": "~/research",
    "tags": ["research"],
    "created_at": "2026-02-03T09:00:00Z",
    "updated_at": "2026-02-03T18:00:00Z",
    "session_id": "xyz-789-456"
  }
]
```

### agm doctor --json

```json
{
  "system_healthy": true,
  "checks": {
    "claude_installed": true,
    "tmux_installed": true,
    "tmux_version": "3.3a",
    "user_lingering": true,
    "total_sessions": 224
  },
  "unhealthy_sessions": [],
  "warnings": []
}
```

---

## Environment Variables

AGM respects these environment variables:

```go
const (
    // Debug logging
    EnvDebug = "AGM_DEBUG" // "true" or "1"

    // Accessibility
    EnvNoColor       = "NO_COLOR"         // "1"
    EnvScreenReader  = "AGM_SCREEN_READER" // "true" or "1"

    // Agent API keys
    EnvAnthropicKey = "ANTHROPIC_API_KEY"
    EnvGeminiKey    = "GEMINI_API_KEY"
    EnvOpenAIKey    = "OPENAI_API_KEY"

    // Google Cloud (for search)
    EnvGoogleProject = "GOOGLE_CLOUD_PROJECT"
    EnvGoogleCreds   = "GOOGLE_APPLICATION_CREDENTIALS"

    // Configuration
    EnvConfigPath = "AGM_CONFIG" // Path to config.yaml
)
```

---

## Testing API

### Test Helpers

```go
package testing

// CreateTestSession creates isolated test session
func CreateTestSession(t *testing.T, name string) (sessionDir string, cleanup func())

// MockAgent creates mock agent for testing
func MockAgent(t *testing.T, agentType string) agent.Agent

// MockTmux creates mock tmux environment
func MockTmux(t *testing.T) (socketPath string, cleanup func())
```

### Example Test

```go
package mypackage_test

import (
    "testing"
    "github.com/vbonnet/dear-agent/agm/internal/testing"
)

func TestSessionCreation(t *testing.T) {
    sessionDir, cleanup := testing.CreateTestSession(t, "test-session")
    defer cleanup()

    // Test session operations
    // ...
}
```

---

## Related Documentation

- **[Architecture Overview](ARCHITECTURE.md)** - System architecture
- **[Command Reference](AGM-COMMAND-REFERENCE.md)** - CLI commands
- **[Getting Started](GETTING-STARTED.md)** - Installation and setup
- **[Examples](EXAMPLES.md)** - Usage examples

---

**Maintained by**: Foundation Engineering
**License**: MIT
**Repository**: https://github.com/vbonnet/dear-agent
