# AGM (Agent Gateway Manager) - Architecture

## Overview

AGM (Agent Gateway Manager) is a multi-agent session management system that provides a unified CLI interface for managing AI agent sessions across different providers (Claude, Gemini, GPT, OpenCode). The system implements an adapter pattern to abstract away CLI-specific differences and enable seamless multi-session orchestration.

## Architecture

### C4 Component Diagram

The following diagram shows the internal component architecture of AGM at the C4 Level 3 (Component) view:

![AGM Component Architecture](diagrams/rendered/c4-component-agm.svg)

[View PNG version](diagrams/rendered/c4-component-agm.png) | [View D2 Source](diagrams/c4-component-agm.d2)

### Key Components

#### 1. Command Layer (CLI Interface)
- **Command Interface (Cobra CLI)**: Parses commands, validates flags, routes to business logic
- **Session Commands**: Core session operations (new, resume, list, archive, kill, associate)
- **Admin Commands**: System maintenance (doctor, fix-uuid, clean, unlock, migrate, check-worktrees, cleanup-worktrees)
- **Workflow Commands**: Specialized workflows (deep-research, code-review, architect)

#### 2. Business Logic Layer
- **Session Manager**: Session lifecycle management, state tracking, validation
- **Command Translator**: Translates AGM commands to agent-specific actions
- **Identifier Resolver**: Multi-strategy session resolution (exact, UUID prefix, fuzzy, interactive)
- **Workflow Engine**: Executes specialized workflows with agent coordination

#### 3. Shared Operations Layer (`internal/ops/`)
All three API surfaces (CLI, MCP, Skills) share a common operations layer:

- **OpContext**: Dependency injection container holding storage, tmux, config, and output preferences
- **Operations**: ListSessions, GetSession, SearchSessions, GetStatus (more being added)
- **RFC 7807 Errors**: Structured error responses with stable error codes (AGM-001 through AGM-100) and actionable suggestions
- **Field Masks**: Token-efficient output filtering via `--fields` flag
- **Output Formatting**: `--output json` for programmatic consumers, text for humans

```
CLI (Cobra)    →  internal/ops  →  Dolt Storage
MCP (JSON-RPC) →  internal/ops  →  Dolt Storage
Skills (.md)   →  CLI --json    →  internal/ops  →  Dolt Storage
```

See `docs/AGENTIC-API.md` for the complete agentic API reference.

#### 4. Adapter Registry (Multi-CLI Support)
The adapter pattern enables AGM to support multiple AI agents with a unified interface:

- **Claude Adapter**: CLI integration, UUID detection, slash commands, history.jsonl parsing
- **Gemini Adapter**: API integration, session file management, command translation
- **Codex Adapter**: OpenAI API integration, thread management
- **OpenCode Adapter**: SSE event stream integration, server port management, real-time state updates

Each adapter implements the unified `Agent` interface, allowing AGM to:
- Abstract away CLI-specific differences
- Provide consistent session management across providers
- Enable seamless switching between agents
- Translate generic commands to agent-specific actions

#### 5. Coordination & Storage Layer
- **Dolt Storage**: Session metadata persistence with transactional database operations (Git-like versioned SQL)
- **Coordination Daemon**: Multi-session orchestration with 30-second polling interval
- **Message Queue**: Async communication with retry logic and acknowledgments
- **State Monitor (Astrocyte)**: Real-time state detection (READY, THINKING, PERMISSION_PROMPT, COMPACTING, OFFLINE)

#### 6. Backend Abstraction
- **Tmux Backend**: Session management, pane control, key sending
- **Temporal Backend**: Workflow orchestration, durable execution (future)

#### 7. UI & Presentation Layer
- **Interactive UI (Huh TUI)**: Session picker, forms, confirmations, fuzzy search
- **Error Presentation**: Layered error handling with remediation suggestions

### Adapter Pattern Implementation

The adapter pattern is central to AGM's architecture:

```
Generic AGM Command → Command Translator → Agent Adapter → Agent-Specific Action
     "resume"              ↓                      ↓               ↓
                    Analyze session       Claude Adapter:   `/resume {uuid}\r`
                    metadata              Gemini Adapter:   API call with history
                                         Codex Adapter:    Thread continuation
```

**Benefits:**
- **Extensibility**: Adding new agents requires only implementing the `Agent` interface
- **Maintainability**: Agent-specific logic is isolated in adapters
- **Testability**: Each adapter can be tested independently with mocks
- **Flexibility**: Users can switch agents without changing workflows

### Session Lifecycle Flow

1. **Session Creation**: User executes `agm new my-session`
2. **CLI Validation**: Command layer validates flags and parameters
3. **UUID Generation**: SessionManager generates unique session ID
4. **Adapter Selection**: Appropriate adapter chosen based on agent flag
5. **Agent Startup**: Adapter starts agent CLI via backend
6. **Database Persistence**: Dolt adapter inserts session metadata into database
7. **Backend Attachment**: TmuxBackend attaches to session

### Session Hierarchy and Parent-Child Relationships

AGM tracks parent-child relationships between sessions to maintain continuity when Claude Code's "Clear Context and Execute Plan" feature creates new sessions:

**Problem**: When a planning session uses "Clear Context and Execute Plan", Claude Code creates a new execution session with a different UUID, breaking AGM's session continuity.

**Solution**: Session hierarchy tracking via `parent_session_id` field:

1. **Automatic Detection**: SessionStart hook detects execution sessions created 1-30 seconds after planning sessions with matching CWD
2. **Parent Linking**: Child session's `parent_session_id` set to parent's `id`
3. **Name Inheritance**: Execution sessions inherit parent's name with `-exec` suffix (e.g., `my-project` → `my-project-exec`)
4. **Smart Resume**: `agm session resume <name>` automatically prefers execution sessions over planning sessions
5. **Backfill**: Admin command `agm admin backfill-plan-sessions` fixes orphaned historical sessions

**Database Schema**:
```sql
ALTER TABLE agm_sessions
  ADD COLUMN parent_session_id VARCHAR(255) NULL,
  ADD CONSTRAINT fk_parent_session FOREIGN KEY (parent_session_id) REFERENCES agm_sessions(id),
  ADD INDEX idx_parent_session_id (parent_session_id);
```

**Hierarchy Methods**:
- `GetParent(sessionID)`: Retrieve parent session
- `GetChildren(sessionID)`: Retrieve all child sessions
- `GetSessionTree(sessionID)`: Retrieve full session tree

**Implementation**: Migration 007, commit 75be3d7

### Worktree Lifecycle Management

AGM tracks git worktree creation and removal to prevent orphaned worktrees from accumulating:

1. **Tracking**: The `posttool-worktree-tracker` hook detects `git worktree add/remove` in Bash tool
   calls and records events in the `agm_worktrees` Dolt table.
2. **Exit Gate**: `agm admin check-worktrees` runs as a session exit gate, warning when worktrees
   created during a session have not been removed.
3. **Cleanup**: `agm admin cleanup-worktrees` finds and removes orphaned worktrees (worktrees whose
   sessions have ended without removing them).

### Async Message Delivery Flow

1. **Message Enqueueing**: Session A enqueues message for Session B
2. **Daemon Polling**: CoordinationDaemon polls queue every 30 seconds
3. **State Detection**: StateMonitor checks Session B's current state
4. **Conditional Delivery**: If state is READY, message is delivered
5. **State Update**: Session B's state updated to THINKING
6. **Acknowledgment**: Delivery acknowledged and removed from queue

### Multi-Session Orchestration

AGM supports coordinated multi-session workflows through:

- **Coordination Daemon**: Background process managing message delivery
- **State-Aware Routing**: Messages only delivered when sessions are READY
- **Retry Logic**: Failed deliveries retry with exponential backoff (max 3 attempts)
- **Acknowledgment System**: Ensures reliable message delivery
- **SSE Integration**: Real-time state updates from OpenCode sessions

## Pending Message System (`internal/messages/pending.go`)

The pending message system provides file-based message delivery for inter-agent communication via Claude Code's PreToolUse hook mechanism.

### How It Works

1. **Write side**: `WritePendingFile(sessionName, messageID, formattedMessage)` writes a `.msg` file to `~/.agm/pending/{session}/`
2. **Read side**: The `pretool-message-check` hook (in the engram repo) reads `.msg` files on every tool call and injects them into the agent's context via stderr
3. **Cleanup**: After delivery, the hook removes the `.msg` files

### File Format

- **Directory**: `~/.agm/pending/{sessionName}/`
- **Filename**: `{unix-nanoseconds}-{messageID-prefix}.msg` (chronological sorting)
- **Content**: Plain UTF-8 text (the formatted message)
- **Permissions**: Directory 0700, files 0600

### Design Decisions

- File-based rather than database-backed for simplicity and cross-process compatibility
- Timestamp-based filenames ensure chronological ordering without an index
- Best-effort delivery: the SQLite queue + daemon path remains the primary delivery mechanism
- See [ADR-017](docs/adr/ADR-017-pending-message-files.md) for the full decision record

## Advisory File Reservations (`internal/reservation/store.go`)

Advisory file reservations prevent parallel agents from destructively editing the same files during swarm operations.

### How It Works

1. **Reserve**: An agent calls `Reserve(sessionID, patterns, ttl)` to declare intent to edit files matching glob patterns
2. **Check**: Before editing, an agent calls `Check(sessionID, filePath)` to see if another agent holds a conflicting reservation
3. **Release**: When done, the agent calls `Release(sessionID)` to free its reservations
4. **Auto-expire**: Reservations have a TTL (default 2 hours) and are automatically cleaned up on every store operation

### Storage

- **File**: `~/.agm/reservations.json` (single JSON file, atomic writes)
- **Concurrency**: Mutex-protected in-process; atomic file writes for cross-process safety
- **Glob matching**: Supports `*`, `?`, character classes, and `**` patterns via `filepath.Match`

### Advisory Nature

Reservations are advisory, not enforced. An agent can proceed despite a conflict -- the system provides visibility, not hard locks. This avoids deadlocks and keeps the system simple.

See [ADR-018](docs/adr/ADR-018-advisory-file-reservations.md) for the full decision record.

## A2A Agent Cards (`internal/a2a/`)

AGM generates [A2A Protocol](https://github.com/a2aproject/a2a-spec) Agent Cards from session manifests, enabling standardized agent discovery.

### Components

- **`cards.go`**: `GenerateCard(manifest)` creates an `a2a.AgentCard` struct using the official `a2aproject/a2a-go` SDK. Skills are inferred from harness type, manifest tags, and session name patterns.
- **`registry.go`**: `Registry` manages Agent Card lifecycle on disk at `~/.agm/a2a/cards/`. Supports CRUD operations and `SyncFromManifests` for bulk reconciliation (adds new, keeps existing, removes orphaned/archived).

### Card Generation

Cards are derived from session metadata:
- **Name**: From session manifest name
- **Description**: Falls back through purpose, harness description, and generic text
- **Skills**: Inferred from harness type (e.g., `claude-code`), manifest tags (e.g., `golang`, `backend`), and name patterns (e.g., `review`, `fix`, `test`)
- **Protocol version**: Set to the SDK's `a2a.Version` constant
- **Input/Output modes**: Default to `text/plain`

### Current Limitations

- File-based registry only (local machine). HTTP serving at `/.well-known/agent.json` is not yet implemented.
- No authentication or authorization on card access.

See [ADR-019](docs/adr/ADR-019-a2a-agent-cards.md) for the full decision record.

## Design Principles

### 1. Adapter Pattern for Multi-CLI Support
All agent-specific logic is encapsulated in adapters implementing a common interface, enabling seamless multi-agent support.

### 2. Smart Identifier Resolution
Multi-strategy resolution algorithm (exact → UUID prefix → fuzzy → interactive) eliminates typing exact names.

### 3. Layered Error Handling
Errors handled at validation, execution, and presentation layers with user-friendly remediation suggestions.

### 4. Configuration Cascade
CLI flags → Environment variables → Config file → Smart defaults.

### 5. Dependency Injection for Testability
External dependencies (tmux client, file system) injected via `ExecuteWithDeps` for unit testing.

## State Management

Sessions can be in the following states:

- **READY**: Session idle, ready to receive commands
- **THINKING**: Session processing a command
- **PERMISSION_PROMPT**: Session waiting for user permission
- **COMPACTING**: Session compacting context window
- **OFFLINE**: Session not running or unreachable

State transitions are driven by hook-based manifest state detection: the Stop hook sets state to DONE, PostToolUse sets state to WORKING, and the OpenCode adapter uses SSE events.

## Storage Architecture

### Manifest Schema Versioning

AGM supports multiple manifest schema versions:

- **v2 (Legacy)**: Original AGM schema
- **v3 (Current)**: Enhanced with state tracking, workspace support, agent field

Manifests are automatically upgraded to v3 on first write (backward compatible).

### Storage Modes

- **Dotfile Mode**: Manifests stored in `~/.agm/sessions/`
- **Workspace Mode**: Manifests stored in `{workspace_root}/.agm/sessions/`
- **Centralized Mode**: Symlink from `~/.agm` to centralized location

## Test Infrastructure

### Test Isolation Architecture

AGM implements fail-fast test isolation to prevent production workspace contamination:

#### Enforcement Layers

1. **Infrastructure-Level Blocking** (`internal/dolt/adapter.go`)
   - Detects test execution context (`.test` in executable name)
   - Requires `ENGRAM_TEST_MODE=1` environment variable
   - Blocks production workspaces: oss, acme, prod, production, main
   - Fails immediately with actionable error messages

2. **Test Utilities** (`internal/testutil/environment.go`)
   - `SetupTestEnvironment(t)` - One-line test isolation setup
   - Auto-configures: ENGRAM_TEST_MODE, ENGRAM_TEST_WORKSPACE, WORKSPACE
   - Registers cleanup to restore environment

3. **Interactive Enforcement** (`cmd/agm/new.go`)
   - Detects "test" anywhere in session name (case-insensitive)
   - Forces `--test` flag for test-named sessions
   - No bypass mechanism - scripts must use explicit flag

#### Test Workspace Isolation

**Workspace Model**: Tests use workspace name "test" (not file paths)

```
Production: WORKSPACE=oss → Dolt database "oss"
Tests:      WORKSPACE=test → Dolt database "test"
```

**Environment Variables**:
- `ENGRAM_TEST_MODE=1` - Signals test execution context
- `ENGRAM_TEST_WORKSPACE=test` - Test workspace name
- `WORKSPACE=test` - Dolt workspace selector

#### Test Patterns

**Recommended**:
```go
func TestExample(t *testing.T) {
    testutil.SetupTestEnvironment(t)  // One line
    // Test uses workspace="test" automatically
}
```

**CI/CD Integration**:
```yaml
test:
  env:
    ENGRAM_TEST_MODE: "1"
    ENGRAM_TEST_WORKSPACE: "test"
    WORKSPACE: "test"
  run: go test -v ./...
```

### Test Coverage

- **Unit Tests**: Package-level validation with mocked dependencies
- **Integration Tests**: Multi-component interaction testing
- **E2E Tests**: Full workflow validation (requires tmux)
- **Documentation Tests**: Living documentation via test assertions

See [TESTING.md](TESTING.md) for complete test isolation guide.

## Related Documentation

- [AGM CLI Architecture](../agm-session-lifecycle/agm/cmd/agm/ARCHITECTURE.md) - Detailed CLI architecture
- [AGM Specification](SPEC.md) - Complete system specification
- [Backend Implementation](BACKEND_IMPLEMENTATION.md) - Backend abstraction details
- [Capability Matrix](CAPABILITY-MATRIX.md) - Feature comparison across agents
- [Testing Guide](TESTING.md) - Test isolation and best practices
- [ADR-006](cmd/agm/ADR-006-test-isolation-enforcement.md) - Test isolation enforcement decision
