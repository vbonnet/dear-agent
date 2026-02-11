# ADR-001: Multi-Agent Architecture

**Status:** Accepted
**Date:** 2026-01-15
**Deciders:** Foundation Engineering Team
**Related:** CSM to AGM rename decision

---

## Context

Claude Session Manager (CSM) was originally designed to manage Claude CLI sessions exclusively. As AI landscape evolved, users expressed need to manage multiple AI agents (Gemini, GPT) with same tooling. Three architectural approaches were considered for multi-agent support.

### Problem Statement

**User Need**: Developers want unified session management across multiple AI providers without learning separate CLIs for each agent.

**Business Driver**: As AI agents proliferate, providing multi-agent support increases AGM's value proposition and differentiates from single-agent tools.

**Technical Constraint**: Must maintain backward compatibility with existing CSM sessions while enabling extensibility for new agents.

---

## Decision

We will implement **multi-agent support via Agent Adapter pattern** with command translation layer, while maintaining backward compatibility through manifest versioning.

**Architecture**:
1. **Agent Interface**: Define common operations (Start, IsAvailable, GetMetadata, GetTranslator)
2. **Agent Adapters**: Per-agent implementations (ClaudeAdapter, GeminiAdapter, GPTAdapter)
3. **Command Translator**: Abstract agent-specific commands into unified interface
4. **Manifest v3**: Add `agent` field to distinguish sessions by provider

---

## Alternatives Considered

### Alternative 1: Multi-Binary Approach

**Approach**: Separate binaries for each agent (`agm-claude`, `agm-gemini`, `agm-gpt`)

**Pros**:
- Clean separation of concerns
- No shared codebase complexity
- Easy to test in isolation

**Cons**:
- User must install multiple binaries
- No unified session list across agents
- Duplicated code for common functionality (tmux, manifest, UI)
- Poor user experience (which binary to run?)

**Verdict**: Rejected. UX suffers too much, code duplication is anti-DRY.

---

### Alternative 2: Monolithic Agent Support (No Abstraction)

**Approach**: Hard-code agent-specific logic throughout codebase with if/switch statements

**Pros**:
- Simple to implement initially
- No abstraction overhead
- Direct control over each agent's behavior

**Cons**:
- Code explosion as agents added (O(n) complexity per feature)
- Testing nightmare (test matrix explodes)
- Violates Open-Closed Principle (modify existing code for new agents)
- Difficult to add custom agents (no extension point)

**Verdict**: Rejected. Doesn't scale, high maintenance burden.

---

### Alternative 3: Agent Adapter Pattern (CHOSEN)

**Approach**: Define Agent interface, implement per-agent adapters, use registry for lookup

**Pros**:
- Clean abstraction (Agent interface)
- Extensible (new agents via new adapters)
- Testable (mock agents for tests)
- Single binary (good UX)
- Unified session management

**Cons**:
- Upfront abstraction cost
- Some commands may not map cleanly across agents
- Requires graceful degradation for unsupported features

**Verdict**: ACCEPTED. Best balance of extensibility, maintainability, and UX.

---

## Implementation Details

### Agent Interface

```go
type Agent interface {
    // Start agent CLI session
    Start(ctx context.Context, sessionID string, opts *StartOptions) error

    // Check if agent is available (API keys configured)
    IsAvailable() bool

    // Get agent metadata
    GetMetadata() *AgentMetadata

    // Get command translator for this agent
    GetTranslator() command.Translator
}
```

**Design Rationale**:
- `Start()`: Abstracts CLI startup (different flags per agent)
- `IsAvailable()`: Environment validation (API keys, CLI installed)
- `GetMetadata()`: Descriptive info (name, version, capabilities)
- `GetTranslator()`: Command translation (handles agent-specific commands)

---

### Command Translator Interface

```go
type Translator interface {
    // Rename session/conversation
    RenameSession(ctx context.Context, sessionID, newName string) error

    // Set working directory context
    SetDirectory(ctx context.Context, sessionID, dirPath string) error

    // Run initialization hook (agent-specific)
    RunHook(ctx context.Context, sessionID, hookType string) error
}
```

**Design Rationale**:
- Unified commands that have agent-specific implementations
- Graceful degradation: Return `ErrNotSupported` if agent doesn't support command
- Manifest always updated (fallback behavior)

---

### Agent Registry

```go
type AgentRegistry struct {
    agents map[string]Agent
    mu     sync.RWMutex
}

func (r *AgentRegistry) Register(name string, agent Agent)
func (r *AgentRegistry) Get(name string) (Agent, error)
func (r *AgentRegistry) List() []AgentInfo
```

**Design Rationale**:
- Singleton registry (single source of truth)
- Thread-safe (concurrent access from CLI commands)
- Discoverable (List() for `agm agent list`)

---

### Manifest v3 Schema

```yaml
version: "3.0"
agent: "gemini"  # NEW FIELD (required)
session_id: "..."
tmux_session_name: "my-session"
lifecycle: "active"
context:
  project: "~/projects/myapp"
metadata:
  created_at: "2026-01-15T10:00:00Z"
agent_metadata:  # Renamed from "claude" to be generic
  gemini:
    conversation_id: "xyz-789"
```

**Key Changes**:
- Added `agent` field (required)
- Renamed `claude` section to `agent_metadata.claude`
- Added `agent_metadata.gemini` section
- Increased version to 3.0

**Migration Path**: AGM reads v2, writes v3 on first update

---

## Consequences

### Positive

✅ **Extensibility**: New agents added via new adapter (no core changes)
✅ **Testability**: Mock agents for unit tests, real agents for integration tests
✅ **UX Consistency**: Single binary, unified session list, consistent commands
✅ **Backward Compatibility**: v2 manifests still work, CSM command symlinked
✅ **Future-Proof**: Plugin system possible (custom agents via external adapters)

### Negative

⚠️ **Abstraction Cost**: Upfront design effort for Agent interface
⚠️ **Graceful Degradation**: Some commands won't work on all agents (requires fallback logic)
⚠️ **Testing Complexity**: Must test each adapter + translation layer
⚠️ **Documentation Burden**: Must document agent-specific behavior differences

### Neutral

🔄 **Manifest Migration**: Users must migrate CSM → AGM manifests (wizard provided)
🔄 **Learning Curve**: Users must understand agent differences (docs/comparison table)

---

## Mitigations

**Abstraction Cost**:
- Keep Agent interface minimal (4 methods initially)
- Add methods incrementally as needed (YAGNI principle)

**Graceful Degradation**:
- Clear error messages when command not supported
- Manifest updated as fallback (local state correct even if agent fails)
- Document support matrix (which commands work on which agents)

**Testing Complexity**:
- Mock agent for fast unit tests
- Real agents in CI (integration tests)
- BDD scenarios for user-facing behavior

**Documentation Burden**:
- Auto-generate support matrix from code
- Agent comparison table in docs
- Examples for each agent

---

## Validation

**BDD Scenarios**:
- Create session with `--agent claude`
- Create session with `--agent gemini`
- Rename command works for Claude (slash command)
- Rename command works for Gemini (API call)
- Agent list shows all available agents

**Integration Tests**:
- Start Claude session, verify tmux + manifest
- Start Gemini session, verify tmux + manifest
- Switch between agents, verify context maintained

**User Testing**:
- Survey: "Do you understand how to choose an agent?" (>80% yes)
- Survey: "Is switching between agents easy?" (>4/5 stars)

---

## Related Decisions

- **ADR-002**: Command Translation Layer (depends on this)
- **ADR-003**: Environment Validation (uses Agent.IsAvailable())
- **ADR-005**: Manifest Versioning (v2 → v3 migration)

---

## References

- **Design Pattern**: Adapter Pattern (Gang of Four)
- **Go Interface Design**: Effective Go (https://go.dev/doc/effective_go)
- **Similar Tools**: direnv (multi-shell abstraction), asdf (multi-runtime abstraction)

---

**Implementation Status:** ✅ Complete (Shipped in AGM v3.0)
**Date Completed:** 2026-02-04
