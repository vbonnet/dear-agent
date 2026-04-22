# ADR-001: Multi-Agent Architecture

**Status:** Accepted
**Date:** 2026-01-15
**Deciders:** Foundation Engineering Team
**Related:** AGM to AGM rename decision

---

## Context

Agent Session Manager (AGM) was originally designed to manage Claude CLI sessions exclusively. As AI landscape evolved, users expressed need to manage multiple AI agents (Gemini, GPT) with same tooling. Three architectural approaches were considered for multi-agent support.

### Problem Statement

**User Need**: Developers want unified session management across multiple AI providers without learning separate CLIs for each agent.

**Business Driver**: As AI agents proliferate, providing multi-agent support increases AGM's value proposition and differentiates from single-agent tools.

**Technical Constraint**: Must maintain backward compatibility with existing AGM sessions while enabling extensibility for new agents.

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
✅ **Backward Compatibility**: v2 manifests still work, AGM command symlinked
✅ **Future-Proof**: Plugin system possible (custom agents via external adapters)

### Negative

⚠️ **Abstraction Cost**: Upfront design effort for Agent interface
⚠️ **Graceful Degradation**: Some commands won't work on all agents (requires fallback logic)
⚠️ **Testing Complexity**: Must test each adapter + translation layer
⚠️ **Documentation Burden**: Must document agent-specific behavior differences

### Neutral

🔄 **Manifest Migration**: Users must migrate AGM → AGM manifests (wizard provided)
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
- Create session with `--harness claude-code`
- Create session with `--harness gemini-cli`
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

## Updates

### Update 2026-03-11: Four-Harness Production Deployment

**Status**: Production (4 harnesses: Claude, Gemini, Codex, OpenCode)

#### Expanded Agent Interface

The Agent interface has grown from 4 methods to **11 required methods**:

```go
type Agent interface {
    // Session Lifecycle (3 methods)
    CreateSession(ctx SessionContext) (SessionID, error)
    ResumeSession(sessionID SessionID) error
    TerminateSession(sessionID SessionID) error

    // Session State (1 method)
    GetSessionStatus(sessionID SessionID) (SessionStatus, error)

    // Communication (2 methods)
    SendMessage(sessionID SessionID, message string) error
    GetHistory(sessionID SessionID) ([]Message, error)

    // Data Exchange (2 methods)
    ExportConversation(sessionID SessionID, format ExportFormat) ([]byte, error)
    ImportConversation(sessionID SessionID, data []byte, format ImportFormat) error

    // Capabilities (1 method)
    Capabilities() AgentCapabilities

    // Commands (1 method)
    ExecuteCommand(sessionID SessionID, command Command) error

    // Metadata (1 method - was 3 originally)
    Name() string
    Version() string
}
```

**Rationale**: Expanded interface to support full session lifecycle management, data portability, and capability discovery.

#### Four Harness Implementations

| Harness | Status | Parity Score | Key Architecture |
|---------|--------|--------------|------------------|
| **Claude** | ✅ Production | 100% (baseline) | UUID-based resume, hook state detection |
| **Gemini** | ✅ Production | ~92% | API-based, polling state detection |
| **Codex** | ✅ Production | ~93% | API-based, OpenAI SDK |
| **OpenCode** | ✅ Production | ~95% | Server-based, SSE state detection |

#### State Detection Strategies

Multi-harness implementation revealed **three distinct state detection strategies**:

1. **Hook-based + Fallback** (Claude):
   - Primary: Hook scripts update manifest state field
   - Fallback: Tmux pane scraping for prompt detection
   - Latency: <100ms (hooks), ~200ms (scraping)
   - Reliability: 95%

2. **API Polling** (Gemini, Codex):
   - Query provider API for conversation state
   - Latency: 500ms-2s (network dependent)
   - Reliability: 90%

3. **SSE Push** (OpenCode):
   - Server pushes state change events via Server-Sent Events
   - Latency: <50ms
   - Reliability: 98%
   - Infrastructure: `internal/monitor/opencode/` (88.1% test coverage)

**Design Decision**: Allow harness-specific state detection rather than forcing unified approach.

**Rationale**:
- Each provider has optimal detection method (hooks for local, SSE for server, API for cloud)
- Unified approach would degrade to "lowest common denominator" (API polling)
- Abstraction would be leaky (providers evolve independently)
- Transparency benefits users (docs explain tradeoffs)

#### Parity Testing Infrastructure

To enforce interface compliance across all harnesses, we implemented comprehensive parity testing:

**Test Coverage**:
- 50+ parameterized test scenarios
- Each scenario runs for ALL harnesses (200+ test executions total)
- Test files:
  - `test/integration/agent_parity_integration_test.go` (6 test tables)
  - `test/integration/agent_parity_session_management_test.go` (14 tests)
  - `test/integration/agent_parity_messaging_test.go` (8 tests)
  - `test/integration/agent_parity_data_exchange_test.go` (8 tests)
  - `test/integration/agent_parity_capabilities_test.go` (11 tests)
  - `test/integration/agent_parity_commands_test.go` (7 tests)

**Test Automation**:
- Script: `test/scripts/add_harness_to_parity_tests.py`
- Automates adding new harnesses to all parity test scenarios
- Reduces test update time from hours to minutes

**Parity Score Calculation**:
```
Parity Score = (Supported Features / Total Features) × 100%
```

Where "Total Features" is defined by Claude baseline (100%).

#### Sequential Merge Strategy

**Decision**: New harnesses merge sequentially (not simultaneously)

**Order**: OpenCode → Codex → Gemini (future harnesses follow pattern)

**Rationale**:
1. First merge establishes test automation pattern
2. Subsequent merges use automation (faster, fewer errors)
3. Minimizes merge conflicts (one at a time)
4. Each merge provides learning for next

**Coordination**: Multi-session AGM communication used to coordinate three parallel implementations.

#### Documentation Strategy

**Decision**: Update existing docs (not create new harness-specific docs)

**Approach**:
- `docs/ARCHITECTURE.md`: Added "Agent Interface Specification" section (harness-agnostic)
- Harness comparison matrix (capabilities, differences, tradeoffs)
- Harness-specific subsections where divergence exists
- ADR updates documenting multi-harness decisions

**Rationale**:
- Less duplication (DRY principle)
- Builds on existing documentation
- Easier to maintain single source of truth
- Clearer for users (all harness info in one place)

#### Lessons Learned

**What Worked Well**:
- ✅ Agent interface is robust (all 4 harnesses fit cleanly)
- ✅ Adapter pattern scales (adding harnesses is formulaic)
- ✅ Parity tests enforce consistency (caught edge cases early)
- ✅ Test automation saves time (5x efficiency gain documented)
- ✅ Sequential merge minimizes conflicts

**What We'd Do Differently**:
- ⚠️ Create test automation script earlier (not after first harness)
- ⚠️ Standardize worktree naming from start (avoid retroactive renames)
- ⚠️ Document state detection strategies sooner (avoid duplicate exploration)

**Future Considerations**:
- 🔮 Plugin system for external harnesses (user-defined adapters)
- 🔮 Auto-generate capability matrix from code (reduce doc drift)
- 🔮 Harness-specific optimization flags (e.g., `--sse-timeout` for OpenCode)

#### Updated Consequences

**Additional Positive**:
- ✅ **Diverse state detection**: Each harness uses optimal strategy (hooks, API, SSE)
- ✅ **High parity scores**: All harnesses >90% feature parity
- ✅ **Production proven**: Four harnesses in production use
- ✅ **Scalable testing**: Test automation makes adding harnesses fast

**Additional Negative**:
- ⚠️ **State detection complexity**: Three different strategies to maintain
- ⚠️ **Documentation overhead**: Must document harness differences clearly
- ⚠️ **Test maintenance**: 200+ test executions (50+ scenarios × 4 harnesses)

#### Gemini CLI vs API Adapter Decision

**Context**: Gemini can be integrated two ways:
1. **CLI Adapter**: Run `gemini` CLI in tmux (like Claude)
2. **API Adapter**: Use Google AI SDK to call Gemini API directly

**Decision**: Implement **both** adapters, starting with CLI adapter for parity with Claude.

**Rationale**:
- **CLI Adapter** (GeminiCLIAdapter):
  - Provides feature parity with Claude (tmux integration, resume, hooks)
  - Reuses tmux infrastructure (session persistence, WaitForProcessReady)
  - Supports `gemini --resume <uuid>` for session restoration
  - Enables local session management without API dependencies
  - Better for offline/airgapped environments

- **API Adapter** (GeminiAPIAdapter - future):
  - Direct API access (no tmux dependency)
  - Better for programmatic use (automation, CI/CD)
  - Simpler state management (no tmux scraping)
  - Cloud-native (works in containers without tmux)

**Implementation Priority**:
1. Phase 1: CLI adapter (completed 2026-03-11)
2. Phase 2: Command execution via tmux SendCommand (completed 2026-03-11)
3. Phase 3: API adapter (future - when API-based workflows needed)

**Key Architectural Insights**:
- Both adapters implement the same Agent interface
- CLI adapter uses tmux patterns from Claude (code reuse)
- API adapter would use HTTP client patterns from Gemini translator
- Users can choose adapter based on use case (CLI for development, API for automation)

**Coexistence Pattern**:
```go
// User chooses adapter via --agent-mode flag
agm new --harness gemini-cli --agent-mode cli    # Uses GeminiCLIAdapter
agm new --harness gemini-cli --agent-mode api    # Uses GeminiAPIAdapter (future)
```

**Trade-offs**:
- ✅ Flexibility: Users choose best adapter for their workflow
- ✅ Incremental: CLI adapter provides immediate value
- ⚠️ Complexity: Two code paths to maintain (mitigated by shared Agent interface)
- ⚠️ Feature drift: CLI and API adapters may have different capabilities (documented in support matrix)

#### Related ADRs

- **ADR-002**: Command Translation Layer (updated for 4 harnesses + Gemini CLI commands)
- **ADR-003**: State Detection Strategies (proposed - would formalize decisions above)
- **ADR-004**: Parity Testing Framework (proposed - would document test infrastructure)
- **ADR-011**: Gemini CLI Adapter Strategy (proposed - would document CLI vs API adapter decision in depth)

---

**Implementation Status:** ✅ Complete (Shipped in AGM v3.0)
**Date Completed:** 2026-02-04
**Updated:** 2026-03-11 (four-harness production deployment + Gemini CLI adapter)
