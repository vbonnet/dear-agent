# ADR-002: Command Translation Layer

**Status:** Accepted
**Date:** 2026-01-18
**Deciders:** Foundation Engineering Team
**Related:** ADR-001 (Multi-Agent Architecture)

---

## Context

Different AI agents expose different command interfaces:
- **Claude**: Slash commands (`/rename`, `/agm-assoc`) sent via tmux
- **Gemini**: API calls (UpdateConversationTitle, UpdateMetadata)
- **GPT**: (Future) Unknown interface

Users expect consistent behavior across agents. Command like "rename session" should work regardless of underlying agent.

### Problem Statement

**User Need**: Single command to rename session, regardless of agent provider

**Example**:
```bash
agm session rename my-session new-name
```

Should work for:
- Claude session (sends `/rename new-name` via tmux)
- Gemini session (calls `UpdateConversationTitle` API)
- GPT session (TBD implementation)

**Technical Challenge**: How to abstract agent-specific commands without leaking implementation details?

---

## Decision

We will implement **Command Translation Layer** using Strategy pattern, with graceful degradation for unsupported commands.

**Architecture**:
1. **CommandTranslator Interface**: Define common commands (Rename, SetDirectory, RunHook)
2. **Per-Agent Translators**: Implement agent-specific translation (ClaudeTranslator, GeminiTranslator)
3. **Graceful Degradation**: Return `ErrNotSupported` if agent doesn't support command
4. **Manifest Fallback**: Update manifest locally even if agent command fails

---

## Alternatives Considered

### Alternative 1: No Abstraction (Agent-Specific Commands)

**Approach**: Different commands per agent (`agm claude rename`, `agm gemini rename`)

**Pros**:
- No abstraction complexity
- Clear which commands work on which agents
- Easy to implement

**Cons**:
- Poor UX (user must remember agent-specific commands)
- Violates DRY (duplicate command logic)
- Difficult to switch agents mid-workflow

**Verdict**: Rejected. UX suffers, doesn't scale.

---

### Alternative 2: Best-Effort Emulation

**Approach**: AGM emulates unsupported commands locally (e.g., fake rename by updating tmux session name)

**Pros**:
- Consistent UX (all commands work on all agents)
- Simple user mental model

**Cons**:
- Misleading (agent doesn't actually know about rename)
- Creates state divergence (AGM thinks renamed, agent doesn't)
- Complex failure modes (what if agent rejects later?)

**Verdict**: Rejected. Violates "fail fast, fail clear" principle.

---

### Alternative 3: Command Translation with Graceful Degradation (CHOSEN)

**Approach**: Translate commands when possible, return `ErrNotSupported` otherwise, always update manifest

**Pros**:
- Honest UX (clear when command unsupported)
- Manifest always correct (local state maintained)
- Extensible (new commands added incrementally)
- Fails gracefully (warn user, don't crash)

**Cons**:
- Some commands won't work on all agents
- Requires documentation of support matrix

**Verdict**: ACCEPTED. Best balance of honesty and usability.

---

## Implementation Details

### CommandTranslator Interface

```go
type Translator interface {
    // Rename session/conversation
    // Returns ErrNotSupported if agent doesn't support renaming
    RenameSession(ctx context.Context, sessionID, newName string) error

    // Set working directory context
    // Returns ErrNotSupported if agent doesn't support directory context
    SetDirectory(ctx context.Context, sessionID, dirPath string) error

    // Run initialization hook (agent-specific behavior)
    // Returns ErrNotSupported if agent doesn't support hooks
    RunHook(ctx context.Context, sessionID, hookType string) error
}

// Sentinel error for unsupported commands
var ErrNotSupported = errors.New("command not supported by this agent")
```

**Design Rationale**:
- Explicit `ErrNotSupported` error (not generic error)
- Context parameter for cancellation/timeout
- Session ID (not manifest) to avoid coupling

---

### Claude Translator Implementation

```go
type ClaudeTranslator struct {
    tmux *tmux.Client
}

func (t *ClaudeTranslator) RenameSession(ctx context.Context, sessionID, newName string) error {
    // Send slash command via tmux
    cmd := fmt.Sprintf("/rename %s", newName)
    return t.tmux.SendKeys(sessionID, cmd)
}

func (t *ClaudeTranslator) SetDirectory(ctx context.Context, sessionID, dirPath string) error {
    // Send association command
    cmd := fmt.Sprintf("/agm-assoc %s", dirPath)
    return t.tmux.SendKeys(sessionID, cmd)
}

func (t *ClaudeTranslator) RunHook(ctx context.Context, sessionID, hookType string) error {
    // Send hook script via tmux
    hookScript := getHookScript(hookType)
    return t.tmux.SendKeys(sessionID, hookScript)
}
```

**Implementation Notes**:
- Uses tmux SendKeys (synchronous)
- Slash commands are Claude-specific
- Hooks are arbitrary bash scripts

---

### Gemini Translator Implementation

```go
type GeminiTranslator struct {
    client *genai.Client
}

func (t *GeminiTranslator) RenameSession(ctx context.Context, sessionID, newName string) error {
    // Call Google AI API
    conversation, err := t.client.GetConversation(ctx, sessionID)
    if err != nil {
        return err
    }

    return t.client.UpdateConversationTitle(ctx, conversation.ID, newName)
}

func (t *GeminiTranslator) SetDirectory(ctx context.Context, sessionID, dirPath string) error {
    // Gemini doesn't have directory context concept
    // Store in metadata as custom field
    conversation, err := t.client.GetConversation(ctx, sessionID)
    if err != nil {
        return err
    }

    metadata := conversation.Metadata
    metadata["working_directory"] = dirPath
    return t.client.UpdateMetadata(ctx, conversation.ID, metadata)
}

func (t *GeminiTranslator) RunHook(ctx context.Context, sessionID, hookType string) error {
    // Gemini doesn't support arbitrary script execution
    return ErrNotSupported
}
```

**Implementation Notes**:
- Uses Google AI SDK (async API calls)
- Rename maps to UpdateConversationTitle
- SetDirectory uses custom metadata field
- RunHook not supported (returns ErrNotSupported)

---

### Caller Pattern (with Fallback)

```go
func renameSession(manifest *Manifest, newName string) error {
    // Get translator for this agent
    agent, _ := agentRegistry.Get(manifest.Agent)
    translator := agent.GetTranslator()

    // Try to translate command
    err := translator.RenameSession(ctx, manifest.SessionID, newName)

    if errors.Is(err, command.ErrNotSupported) {
        // Graceful degradation: warn user, update manifest only
        fmt.Fprintf(os.Stderr, "Warning: %s agent doesn't support rename, updating manifest only\n", manifest.Agent)
        manifest.TmuxSessionName = newName
        return manifestWriter.Write(manifest)
    }

    if err != nil {
        return fmt.Errorf("rename failed: %w", err)
    }

    // Success: update manifest
    manifest.TmuxSessionName = newName
    return manifestWriter.Write(manifest)
}
```

**Pattern Explanation**:
1. Get translator for agent
2. Try to execute command
3. If `ErrNotSupported`: Warn, update manifest, succeed
4. If other error: Fail fast with clear message
5. If success: Update manifest, succeed

**Key Insight**: Manifest always updated, even if agent doesn't support command. This ensures AGM's local state is correct.

---

## Command Support Matrix

| Command | Claude | Gemini CLI | Gemini API | GPT (Future) |
|---------|--------|------------|------------|--------------|
| RenameSession | ✅ Slash command | ✅ `/chat save` | ✅ API call | 🔜 Planned |
| SetDirectory | ✅ Slash command | ✅ `cd` command | ⚠️ Metadata only | 🔜 Planned |
| ClearHistory | ✅ Slash command | ✅ File deletion | ✅ API call | 🔜 Planned |
| SetSystemPrompt | ✅ Slash command | ✅ Metadata store | ✅ API call | 🔜 Planned |
| RunHook | ✅ tmux send | ✅ Hook context file | ❌ Not supported | 🔜 Planned |
| AuthorizeDirectory | ✅ Trust prompt | ✅ `--include-directories` | N/A | 🔜 Planned |

**Legend**:
- ✅ Fully supported (native agent feature)
- ⚠️ Partial support (workaround implementation)
- ❌ Not supported (returns ErrNotSupported)
- 🔜 Planned (future implementation)

**New Commands** (added in Gemini CLI integration):
- **CommandSetDir**: Changes working directory for session
- **CommandClearHistory**: Removes conversation history
- **CommandSetSystemPrompt**: Sets/updates system instructions
- **CommandAuthorize**: Pre-authorizes directories for agent access

---

## Consequences

### Positive

✅ **Consistent UX**: Same command works across agents (when supported)
✅ **Graceful Degradation**: Clear warning when command unsupported, doesn't crash
✅ **Manifest Integrity**: Local state always correct, even if agent fails
✅ **Extensibility**: New commands added without breaking existing code
✅ **Testability**: Mock translator for unit tests

### Negative

⚠️ **Partial Feature Parity**: Not all commands work on all agents
⚠️ **User Confusion**: Users may not understand why command failed for one agent but worked for another
⚠️ **Documentation Burden**: Must maintain support matrix

### Neutral

🔄 **API Complexity**: Each agent may require different API clients (Google AI, OpenAI, etc.)
🔄 **Error Handling**: Must distinguish unsupported vs error

---

## Mitigations

**Partial Feature Parity**:
- Document support matrix clearly
- `agm agent info <agent>` shows capabilities
- Clear warning messages when degrading

**User Confusion**:
- Explain "why" in error messages (e.g., "Gemini doesn't support hooks because it's API-based, not CLI-based")
- Link to docs in error output

**Documentation Burden**:
- Auto-generate support matrix from code
- Unit tests validate matrix is up-to-date

---

## Validation

**BDD Scenarios**:
- Rename Claude session → sends slash command
- Rename Gemini session → calls API
- Run hook on Claude session → succeeds
- Run hook on Gemini session → warns, updates manifest only

**Unit Tests**:
- Mock translator returns ErrNotSupported → caller warns, updates manifest
- Mock translator returns error → caller fails with error
- Mock translator succeeds → caller updates manifest

**Integration Tests**:
- Real Claude session rename → verify tmux sent command
- Real Gemini session rename → verify API called

---

## Related Decisions

- **ADR-001**: Multi-Agent Architecture (defines Agent interface)
- **ADR-003**: Environment Validation (validates before calling translator)
- **ADR-005**: Manifest Versioning (manifest always updated)

---

### Gemini CLI Translator Implementation

**Added:** 2026-03-11 (Gemini CLI integration Phase 1 & 2)

```go
type GeminiCLIAdapter struct {
    sessionStore SessionStore
}

func (a *GeminiCLIAdapter) ExecuteCommand(cmd Command) error {
    sessionID, _ := getStringParam(cmd.Params, "session_id")
    metadata, err := a.sessionStore.Get(SessionID(sessionID))
    if err != nil {
        return fmt.Errorf("session not found: %w", err)
    }

    switch cmd.Type {
    case CommandRename:
        // Use Gemini CLI's /chat save command
        newName, _ := getStringParam(cmd.Params, "name")
        if err := tmux.SendCommand(metadata.TmuxName, fmt.Sprintf("/chat save %s\r", newName)); err != nil {
            return err
        }
        // Update AGM metadata (dual tracking)
        metadata.Title = newName
        return a.sessionStore.Set(SessionID(sessionID), metadata)

    case CommandSetDir:
        // Send cd command to tmux session
        newPath, _ := getStringParam(cmd.Params, "path")
        if err := tmux.SendCommand(metadata.TmuxName, fmt.Sprintf("cd %s\r", newPath)); err != nil {
            return err
        }
        // Update AGM metadata
        metadata.WorkingDir = newPath
        return a.sessionStore.Set(SessionID(sessionID), metadata)

    case CommandClearHistory:
        // Remove Gemini history file
        historyPath := filepath.Join(os.Getenv("HOME"), ".gemini", "sessions", metadata.TmuxName, "history.jsonl")
        return os.Remove(historyPath) // Ignore if doesn't exist

    case CommandSetSystemPrompt:
        // Store in AGM metadata (Gemini CLI doesn't have runtime system prompt update)
        prompt, _ := getStringParam(cmd.Params, "prompt")
        metadata.SystemPrompt = prompt
        return a.sessionStore.Set(SessionID(sessionID), metadata)

    case CommandAuthorize:
        // Gemini CLI uses --include-directories at session creation
        // Runtime authorization not supported (no-op)
        return nil

    case CommandRunHook:
        // Execute hook via context file pattern
        hookName, _ := getStringParam(cmd.Params, "hook_name")
        return a.executeHook(SessionID(sessionID), metadata.TmuxName, hookName)
    }
}
```

**Implementation Notes**:

1. **CommandRename**:
   - Uses Gemini CLI's `/chat save <name>` command
   - Dual tracking: Updates both Gemini checkpoint and AGM metadata
   - Ensures consistency between Gemini's state and AGM's view

2. **CommandSetDir**:
   - Sends `cd <path>` to tmux session (shell command, not Gemini CLI command)
   - Updates AGM metadata to track current working directory
   - Future messages inherit new directory context

3. **CommandClearHistory**:
   - Removes `~/.gemini/sessions/<session>/history.jsonl` file
   - Gemini CLI detects missing history and starts fresh conversation
   - Non-destructive: Original session UUID still exists for resume

4. **CommandSetSystemPrompt**:
   - Stores in AGM metadata (Gemini CLI doesn't support runtime system prompt updates)
   - Future enhancement: Inject via `/memory set` or similar Gemini CLI command
   - Workaround: Set via `--system-instruction` at session creation

5. **CommandAuthorize**:
   - No-op for Gemini CLI (directories authorized via `--include-directories` flag at creation)
   - Runtime authorization not supported by Gemini CLI
   - Trust prompts handled at session creation time

6. **CommandRunHook**:
   - Creates hook context file in `~/.agm/gemini-hooks/<sessionID>-<hookName>.json`
   - External hook scripts can read context file and execute custom logic
   - Non-blocking: Hook failures logged but don't fail operation

**CLI-Specific Translation Challenges**:

| Challenge | Solution |
|-----------|----------|
| **Directory Authorization** | Pre-authorize via `--include-directories` at session creation |
| **System Prompt Updates** | Store in metadata (no runtime update in Gemini CLI) |
| **History Clearing** | Direct file manipulation (no CLI command for clear) |
| **Session Renaming** | Use `/chat save` to create checkpoint with new name |
| **Hook Execution** | Context file pattern (external scripts read JSON) |

**Tmux Integration Pattern**:

Gemini CLI adapter reuses tmux patterns from Claude adapter:
- `tmux.SendCommand()` for delivering commands
- `tmux.WaitForProcessReady()` for prompt detection
- `tmux.HasSession()` for session existence checks
- Session-to-tmux 1:1 mapping (consistent with Claude)

**Dual State Tracking**:

Gemini CLI maintains state in two places:
1. **Gemini's State**: Sessions in `~/.gemini/tmp/<project_hash>/chats/`
2. **AGM's State**: Metadata in `~/.agm/sessions.json`

Commands must update **both** states to maintain consistency.

---

## Future Extensions

**v3.1+**:
- Add `GetConversation()` to fetch agent-side state
- Add `SyncState()` to reconcile AGM ↔ agent state divergence
- Add `GetCapabilities()` for runtime feature detection
- Gemini API adapter (alternative to CLI adapter)

**v4.0+**:
- Plugin system: Custom translators for custom agents
- Auto-discovery: Detect agent capabilities without hardcoding
- Runtime system prompt updates for Gemini CLI (via `/memory` commands)

---

## References

- **Design Pattern**: Strategy Pattern (Gang of Four)
- **Similar Systems**:
  - Database drivers (sql.Driver interface, per-DB implementations)
  - Cloud provider SDKs (AWS, GCP, Azure abstraction layers)
- **Go Error Handling**: https://go.dev/blog/error-handling-and-go

---

**Implementation Status:** ✅ Complete (Shipped in AGM v3.0)
**Date Completed:** 2026-02-04
**Updated:** 2026-03-11 (Gemini CLI command implementations)
