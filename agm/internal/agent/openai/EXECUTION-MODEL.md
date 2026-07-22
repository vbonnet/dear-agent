# OpenAI Execution Model Decision

**Status**: Implemented legacy compatibility path
**Date**: 2026-02-24
**Last reviewed**: 2026-07-21
**Task**: 1.5 - Determine Execution Model

---

## Decision

**Chosen Model**: **API-based execution** (not tmux-based CLI)

---

## Rationale

Based on Phase 0 research (see `OPENAI-EXECUTION-MODEL.md`):

### Why This Adapter Remains API-Only

1. **Separate execution contracts**: AGM now supports OpenAI Codex as the
   `codex-cli` harness. This `openai` adapter remains the legacy Chat
   Completions compatibility path, with local JSONL history and no tmux pane.

2. **API is Primary Interface**:
   - Stable, documented, feature-rich
   - Supports all models (GPT-4.1, o3, o4-mini, etc.)
   - Native streaming support via Server-Sent Events
   - Simpler implementation (no tmux orchestration)

3. **Cross-Platform Compatibility**:
   - Works on Windows, macOS, Linux without tmux dependency
   - No shell process management complexity
   - Better for containerized environments

4. **Follows Existing Patterns**:
   - Reuses existing GPT evaluation client patterns
   - Consistent with OpenAI ecosystem best practices

### Why NOT Tmux-Based

1. **No CLI to orchestrate**: Would require building custom REPL
2. **Added complexity**: Tmux session management, process lifecycle
3. **Platform limitations**: Windows tmux support limited
4. **Maintenance burden**: Custom CLI + API client maintenance

### Codex CLI Separation

Codex CLI is implemented as the separate `codex-cli` harness. It owns an
interactive process, tmux composer readiness, and provider-native conversation
identity. The `openai` harness owns direct API calls and local conversation
history. Callers must not infer one execution model from the other.

---

## Implementation Architecture

### Core Components

```
OpenAIAdapter
├── Client (internal/agent/openai/client.go)
│   ├── Chat Completions API
│   ├── Error handling (auth, rate limits, API errors)
│   └── Azure OpenAI support
├── SessionManager (internal/agent/openai/session_manager.go)
│   ├── Conversation history (JSONL storage)
│   ├── Metadata (title, model, working directory)
│   └── Session persistence (~/.agm/openai-sessions/)
└── Agent Interface Implementation
    ├── CreateSession → Generate UUID + initialize storage
    ├── SendMessage → API call with conversation context
    ├── GetHistory → Load from local storage
    └── ExecuteCommand → Synthetic command translation
```

### Streaming Capability

The SDK and model capability helper can identify streaming support:
- Server-Sent Events (SSE) for real-time responses
- `stream: true` parameter in API requests
- Delta chunks processed as they arrive
- Supports partial response updates

The adapter's current `SendMessage` transaction uses complete responses;
streaming delivery is not yet implemented.

### Synthetic Hooks

Since API-based execution has no shell access, hooks are **synthetic**:
- `SessionStart`: Triggered when session created via CreateSession()
- `SessionEnd`: Triggered when session archived/deleted
- `MessageSent`: Triggered after successful API response
- Hooks execute in AGM process context (not OpenAI subprocess)

**Note**: Real hooks (like Claude's shell hooks) not possible with API-only model.

---

## Execution Flow

### Session Creation
```
1. User: agm session new openai
2. AGM: Calls OpenAIAdapter.CreateSession()
3. Adapter: Generates UUID session ID
4. SessionManager: Creates ~/.agm/openai-sessions/{uuid}/
5. SessionManager: Initializes metadata.json
6. Hook: Fires SessionStart synthetic hook
7. Return: Session ready for messages
```

### Message Send
```
1. User types message in tmux/terminal
2. AGM: Calls OpenAIAdapter.SendMessage(sessionID, message)
3. Adapter: Acquires the context-aware store-scoped stable session lock
4. SessionManager: Reloads completed conversation history from JSONL
5. Client: Calls OpenAI API with completed history plus the new user message under a finite deadline
6. Client: Receives the complete response
7. SessionManager: Atomically commits the user and assistant messages as one turn
8. Hook: Fires MessageSent synthetic hook
9. Return: Display response to user

If completion fails, is canceled, or times out, neither provisional message is
persisted and the store lock is released. AGM CLI sends also hold a longer,
provider-appropriate stable session-ID mutation lock across a locked lifecycle
reload, reconstruction, readiness, bounded completion, and the completed-turn
commit. Archive uses the same boundary, so it cannot race a stale pre-lock
lifecycle snapshot against paid provider work.
```

### Session Resumption
```
1. User: agm session resume {session-id}
2. Adapter: Calls SessionManager.GetSession(sessionID)
3. SessionManager: Loads metadata.json
4. SessionManager: Loads messages.jsonl (on-demand)
5. Return: Full conversation context restored
```

---

## Comparison: Tmux vs API Execution

| Aspect | Tmux-Based (Claude/Gemini) | API-Based (OpenAI) |
|--------|----------------------------|-------------------|
| **Session Isolation** | tmux sessions | In-memory + file storage |
| **Message Delivery** | `tmux send-keys` | HTTP POST to API |
| **Resume** | Attach to tmux | Load from ~/.agm/openai-sessions/ |
| **Process Management** | Lifecycle via tmux | Stateless API calls |
| **Hooks** | Shell hooks in subprocess | Synthetic hooks in AGM process |
| **Working Directory** | tmux pane CWD | Metadata-based context injection |
| **Streaming** | Native CLI | SDK-capable; adapter delivery is currently complete-response |
| **Cross-Platform** | Unix-only (limited Windows) | Windows/macOS/Linux |
| **Complexity** | High (tmux + process mgmt) | Low (HTTP client) |

---

## Limitations and Trade-offs

### Limitations
1. **No Real Hooks**: Cannot execute shell commands in OpenAI context
   - Mitigation: Synthetic hooks in AGM process for most use cases
2. **No Working Directory**: API has no concept of CWD
   - Mitigation: Store in metadata, inject via system messages if needed
3. **Stateless**: Each API call independent (no persistent process)
   - Mitigation: Local conversation history storage maintains state

### Trade-offs
1. **Simplicity vs Features**: API-only is simpler but lacks CLI hooks
2. **Storage**: Must manage conversation history client-side
3. **Context Window**: Full history sent each API call (cost consideration)
   - Mitigation: Implement context pruning if needed (Phase 3)

---

## Future Enhancements

### Phase 2+
- [ ] Implement streaming responses (CreateChatCompletionStream)
- [ ] Add context window management (pruning old messages)
- [ ] Support Responses API (built-in tools, web search)
- [ ] Implement Conversations API integration (server-side persistence)
- [x] Keep Codex CLI isolated in the separate `codex-cli` harness

### Optional
- Custom REPL (if user requests interactive mode)
- Token usage tracking and optimization
- Conversation export (Markdown, HTML, JSONL)

---

## Acceptance Criteria (Task 1.5)

- [x] Execution model decided: API-based
- [x] Decision documented (this file)
- [x] Implementation architecture defined
- [x] Session creation/resumption flow documented
- [x] Streaming approach identified (go-openai SDK)
- [x] Hooks model: Synthetic hooks documented
- [x] Limitations and trade-offs documented

---

## References

- Phase 0: OPENAI-EXECUTION-MODEL.md (swarm project)
- Phase 0: OPENAI-API-CAPABILITIES.md (API features audit)
- Phase 0: GEMINI-PATTERNS-ANALYSIS.md (comparison patterns)
- Implementation: internal/agent/openai/client.go
- Implementation: internal/agent/openai/session_manager.go
- SDK: github.com/sashabaranov/go-openai

---

**Conclusion**: API-based execution model chosen for OpenAI adapter. Implementation complete in Phase 1. Streaming and advanced features to be added in Phase 2+.
