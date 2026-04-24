# ADR-011: Gemini CLI Adapter Strategy

**Status:** Accepted
**Date:** 2026-03-11
**Deciders:** AGM Gemini Parity Swarm
**Related:** ADR-001 (Multi-Agent Architecture), ADR-002 (Command Translation Layer), ADR-004 (Tmux Integration)

---

## Context

Gemini can be integrated into AGM via two distinct approaches:

1. **CLI Adapter**: Run `gemini` command-line tool in tmux (like Claude)
2. **API Adapter**: Use Google AI SDK to call Gemini API directly (like original Gemini translator)

Each approach has different trade-offs for session management, state persistence, and user experience.

### Problem Statement

**User Need**: Developers want unified session management for Gemini that matches Claude's UX (persistent sessions, resume, hooks).

**Technical Constraint**: Gemini provides both CLI and API interfaces, but they operate differently:
- **CLI**: Interactive tool with session files, tmux-compatible, local state
- **API**: REST API with conversation history, cloud-based, programmatic

**Decision Point**: Which integration pattern should AGM use for Gemini support?

### Prior Art

**Claude Integration** (existing):
- Uses tmux-based CLI adapter exclusively
- Session persistence via tmux session + manifest + history.jsonl
- Hooks via tmux send-keys + state detection
- Resume via Claude's native UUID-based session restoration

**Gemini Translator** (Phase 0):
- Used Google AI SDK (API-based approach)
- No tmux integration
- No session persistence
- Command translation for rename/setdir/hooks

---

## Decision

We will implement **Gemini CLI Adapter first**, with API Adapter as future enhancement.

**Phase 1 (Completed 2026-03-11)**: Gemini CLI Adapter
- Run `gemini` CLI in tmux
- Session persistence via tmux + manifest + session store
- Resume via `gemini --resume <uuid>`
- Feature parity with Claude (hooks, commands, state detection)

**Phase 2 (Completed 2026-03-11)**: Command Execution
- CommandRename → `/chat save <name>`
- CommandSetDir → `cd <path>`
- CommandClearHistory → File deletion
- CommandSetSystemPrompt → Metadata storage
- CommandAuthorize → `--include-directories` flag
- CommandRunHook → Context file pattern

**Phase 3 (Future)**: API Adapter (optional alternative)
- Google AI SDK integration
- API-based session management
- Cloud-native (no tmux dependency)
- Better for automation/CI/CD workflows

---

## Alternatives Considered

### Alternative 1: API Adapter Only

**Approach**: Use Google AI SDK exclusively, no tmux integration

**Pros**:
- No tmux dependency
- Cloud-native (works in containers)
- Simple state management (API manages conversation history)
- Better for automation/CI/CD
- Existing translator code can be reused

**Cons**:
- ❌ No feature parity with Claude (no tmux, no hooks, no local state)
- ❌ Different UX (API-based vs CLI-based)
- ❌ Network dependency (requires internet connection)
- ❌ No offline support
- ❌ Can't reuse tmux infrastructure

**Verdict**: Rejected for initial implementation. Doesn't meet feature parity requirement.

---

### Alternative 2: CLI Adapter Only (CHOSEN for Phase 1)

**Approach**: Run `gemini` CLI in tmux, match Claude's architecture

**Pros**:
- ✅ Feature parity with Claude (tmux, hooks, local state, resume)
- ✅ Consistent UX across agents (Claude and Gemini behave the same)
- ✅ Reuses tmux infrastructure (code reuse)
- ✅ Offline support (sessions persist locally)
- ✅ Session restoration via `gemini --resume <uuid>`

**Cons**:
- ⚠️ Requires tmux (same as Claude)
- ⚠️ Less suitable for automation (requires interactive terminal)
- ⚠️ Dual state tracking (Gemini's session files + AGM metadata)

**Verdict**: ACCEPTED. Best path to feature parity and consistent UX.

---

### Alternative 3: Dual Adapters (CHOSEN for Long-Term)

**Approach**: Implement both CLI and API adapters, let users choose

**Pros**:
- ✅ Flexibility: Users choose best adapter for workflow
- ✅ CLI adapter for development (interactive, persistent)
- ✅ API adapter for automation (programmatic, cloud-native)
- ✅ Both implement same Agent interface (abstraction preserved)

**Cons**:
- ⚠️ More code to maintain (two adapters vs one)
- ⚠️ Feature drift risk (CLI and API may diverge)
- ⚠️ Documentation overhead (must explain both approaches)

**Verdict**: ACCEPTED for long-term strategy. CLI adapter first, API adapter later.

---

## Implementation Details

### Gemini CLI Adapter Architecture

**File**: `internal/agent/gemini_cli_adapter.go`

**Key Components**:

```go
type GeminiCLIAdapter struct {
    sessionStore SessionStore  // Stores SessionID → SessionMetadata mapping
}

type SessionMetadata struct {
    TmuxName   string    // Tmux session name
    Title      string    // User-friendly session title
    UUID       string    // Gemini's native session UUID (for --resume)
    WorkingDir string    // Current working directory
    Project    string    // Project identifier
    CreatedAt  time.Time // Session creation timestamp
    SystemPrompt string  // System instructions (stored in AGM metadata)
}
```

**Session Lifecycle**:

1. **CreateSession**:
   - Generate AGM SessionID (UUID)
   - Create tmux session with `gemini --include-directories <dirs>`
   - Wait for Gemini prompt (via `tmux.WaitForProcessReady()`)
   - Extract Gemini UUID from `--list-sessions` output
   - Store SessionID → Metadata mapping

2. **ResumeSession**:
   - Lookup SessionID → Metadata
   - Check if tmux session exists
   - If not, create tmux session and run `gemini --resume <uuid>`
   - Attach to tmux session

3. **TerminateSession**:
   - Send `exit\r` to Gemini via tmux
   - Remove SessionID from session store

**State Detection**:
- Uses tmux scraping (same as Claude)
- Detects Gemini prompt: `^[0-9]+>`
- `WaitForProcessReady()` polls tmux pane for prompt appearance

**Command Translation** (see ADR-002 for details):
- CommandRename → `/chat save <name>` + metadata update
- CommandSetDir → `cd <path>` + metadata update
- CommandClearHistory → Remove history.jsonl file
- CommandSetSystemPrompt → Store in metadata
- CommandAuthorize → No-op (handled at creation)
- CommandRunHook → Create context file

---

### Tmux Integration Pattern

**Reused from Claude Adapter**:

| Operation | Implementation |
|-----------|----------------|
| Session Creation | `tmux.NewSession(name, workingDir)` |
| Process Launch | `tmux.SendCommand(name, "gemini --include-directories ...")` |
| Ready Detection | `tmux.WaitForProcessReady(name, "gemini", 30s)` |
| Message Sending | `tmux.SendCommand(name, message)` |
| Session Attachment | `tmux attach-session -t <name>` (CLI command) |
| Session Termination | `tmux.SendCommand(name, "exit\r")` |

**Benefits of Reuse**:
- No new tmux code required
- Same patterns as Claude (consistency)
- Battle-tested infrastructure

---

### Gemini CLI-Specific Features

**1. UUID-Based Resume**:

Gemini CLI stores sessions with UUIDs:
```bash
$ gemini --list-sessions
0: Wed, Feb 26, 2025, 01:06:06 PM [23a6e871-bb1f-48ec-bdbe-1f6ae90f9686]
1: Wed, Feb 26, 2025, 01:05:57 PM [8c123456-abcd-1234-5678-9012345678ab]
```

AGM extracts UUID at session creation and uses it for resume:
```bash
gemini --resume 23a6e871-bb1f-48ec-bdbe-1f6ae90f9686
```

**2. Directory Authorization**:

Gemini CLI requires directory authorization to access files. AGM pre-authorizes directories via `--include-directories` flag:

```bash
gemini --include-directories '~/project' --include-directories '~/lib'
```

This avoids interactive trust prompts during session creation.

**3. Session Checkpoints**:

Gemini CLI's `/chat save <name>` creates checkpoints:
- Allows renaming sessions
- Creates restore points
- AGM uses this for CommandRename

**4. History Files**:

Gemini CLI stores sessions in project-specific directories:
```
~/.gemini/tmp/<project_hash>/chats/<session_id>/
```

AGM clears history by removing session directory (CommandClearHistory).

---

### API Adapter Architecture (Future)

**File**: `internal/agent/gemini_api_adapter.go` (not yet implemented)

**Key Components**:

```go
type GeminiAPIAdapter struct {
    client *genai.Client  // Google AI SDK client
    store  ConversationStore  // Stores SessionID → ConversationID mapping
}
```

**Session Lifecycle**:

1. **CreateSession**:
   - Generate SessionID
   - Create conversation via API
   - Store SessionID → ConversationID mapping

2. **ResumeSession**:
   - Lookup SessionID → ConversationID
   - Load conversation history from API
   - Ready to accept messages

3. **TerminateSession**:
   - Remove SessionID from store
   - Optionally delete conversation via API

**No Tmux Dependency**:
- All state managed via API
- No terminal multiplexer required
- Better for cloud environments

**Command Translation**:
- CommandRename → API call to update conversation title
- CommandSetDir → Store in conversation metadata
- CommandClearHistory → API call to clear history
- CommandSetSystemPrompt → API call to update system instructions

**Use Cases**:
- CI/CD pipelines (no interactive terminal)
- Containerized environments (no tmux)
- Programmatic automation
- Serverless functions

---

## Consequences

### Positive

✅ **Feature Parity**: Gemini CLI adapter matches Claude's UX (tmux, hooks, resume)
✅ **Code Reuse**: Reuses tmux infrastructure from Claude adapter
✅ **Consistent UX**: All CLI agents (Claude, Gemini) behave the same
✅ **Offline Support**: Sessions persist locally, no network dependency
✅ **Incremental Path**: CLI adapter first, API adapter later (no big-bang rewrite)

### Negative

⚠️ **Tmux Dependency**: CLI adapter requires tmux (same as Claude)
⚠️ **Dual State**: Must maintain consistency between Gemini files and AGM metadata
⚠️ **Less Automation-Friendly**: CLI adapter not ideal for CI/CD (API adapter better)
⚠️ **Future Maintenance**: Two adapters means more code to maintain

### Neutral

🔄 **Adapter Choice**: Users will eventually choose CLI vs API based on workflow
🔄 **Documentation**: Must document when to use each adapter

---

## Mitigations

**Tmux Dependency**:
- Same as Claude (already documented in ADR-004)
- `agm doctor` validates tmux availability
- Installation instructions in docs

**Dual State**:
- Commands update both Gemini state (via tmux) and AGM metadata (via session store)
- Automated tests verify state consistency
- Clear error messages if state diverges

**Automation Friendliness**:
- CLI adapter works for most development workflows
- API adapter planned for CI/CD use cases
- Document which adapter to use for which scenario

**Future Maintenance**:
- Shared Agent interface ensures consistency
- Parity tests validate both adapters
- Feature matrix documents differences

---

## Validation

**BDD Scenarios** (CLI Adapter):
- Create Gemini session → tmux session exists
- Resume Gemini session → loads session by UUID
- Rename Gemini session → `/chat save` succeeds
- Clear history → history.jsonl removed
- Set system prompt → stored in metadata

**Integration Tests**:
- Real Gemini CLI session creation
- UUID extraction from `--list-sessions`
- Directory authorization via `--include-directories`
- Session resume via `gemini --resume <uuid>`

**Parity Tests**:
- All Agent interface methods implemented
- Capabilities match expectations
- Commands execute successfully

---

## Related Decisions

- **ADR-001**: Multi-Agent Architecture (defines Agent interface)
- **ADR-002**: Command Translation Layer (updated with Gemini CLI commands)
- **ADR-004**: Tmux Integration Strategy (CLI adapter reuses patterns)

---

## References

- **Gemini CLI Documentation**: https://ai.google.dev/gemini-api/docs/cli
- **Google AI SDK**: https://github.com/google/generative-ai-go
- **Similar Dual-Adapter Pattern**: kubectl (CLI tool + client-go library)

---

## Updates

### Update 2026-03-11: Phase 1 & 2 Complete

**Status**: ✅ CLI Adapter Implemented

**Completed**:
- GeminiCLIAdapter with full Agent interface implementation
- Session creation with directory authorization
- UUID-based resume via `gemini --resume <uuid>`
- Command execution (Rename, SetDir, ClearHistory, SetSystemPrompt, Authorize, RunHook)
- Tmux integration patterns reused from Claude
- Session store for SessionID → Metadata mapping
- BDD tests for Gemini CLI integration

**Test Coverage**:
- `internal/agent/gemini_cli_adapter_test.go` (unit tests)
- `test/integration/gemini_cli_integration_test.go` (integration tests)
- `test/integration/lifecycle/gemini_hooks_test.go` (hook tests)
- `test/integration/agent_parity_commands_test.go` (parity tests)

**Next Steps**:
- Phase 3: API Adapter implementation (future)
- Enhanced state detection (poll Gemini CLI for status)
- System prompt injection via `/memory` commands

---

**Implementation Status:** ✅ Phase 1 & 2 Complete (CLI Adapter)
**Date Completed:** 2026-03-11
**Next Phase:** API Adapter (TBD)
