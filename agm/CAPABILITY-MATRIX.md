# AI Agent Capability Matrix

## Introduction

This document compares behavioral differences between Claude, Gemini, and GPT mock adapters in the agm BDD test suite. These mock adapters simulate agent behavior deterministically for testing purposes without requiring real API calls.

The matrix documents which features are supported by each agent, behavioral differences in response patterns, and edge cases that may require special handling.

---

## Core Features

| Feature | Claude | Gemini | GPT | Notes | Example |
|---------|--------|--------|-----|-------|---------|
| Create Session | ✅ | ✅ | ✅ | All agents support session creation with UUID generation | `adapter.CreateSession(ctx, req)` → Session with unique ID |
| Send Message | ✅ | ✅ | ✅ | Appends user and assistant messages to history | `adapter.SendMessage(ctx, req)` → Response with content |
| Get History | ✅ | ✅ | ✅ | Returns full message array (user + assistant messages) | `adapter.GetHistory(ctx, sessionID)` → []Message |
| Get Session | ✅ | ✅ | ✅ | Retrieves session metadata (ID, agent, state, timestamps) | `adapter.GetSession(ctx, sessionID)` → Session |

---

## State Management

| Feature | Claude | Gemini | GPT | Notes | Example |
|---------|--------|--------|-----|-------|---------|
| Pause Session | ✅ | ✅ | ✅ | Transitions active → paused | `adapter.PauseSession(ctx, sessionID)` → State: paused |
| Resume Session | ✅ | ✅ | ✅ | Transitions paused → active; fails on archived | `adapter.ResumeSession(ctx, sessionID)` → State: active |
| Archive Session | ✅ | ✅ | ✅ | Terminal state, cannot be resumed | `adapter.ArchiveSession(ctx, sessionID)` → State: archived |
| State Persistence | ✅ | ✅ | ✅ | Agent field persists across pause/resume/archive | After pause/resume, session.Agent unchanged |

---

## Response Patterns

| Feature | Claude | Gemini | GPT | Notes | Example |
|---------|--------|--------|-----|-------|---------|
| Name Recall | ✅ | ✅ | ✅ | Extracts and remembers names from "my name is X" | `"My name is Alice"` → `"Nice to meet you!"` |
| Name Retrieval | ✅ | ✅ | ✅ | Recalls name from history when asked | `"What is my name?"` → `"Your name is Alice."` |
| Default Echo | ✅ | ✅ | ✅ | Prefix differs by agent | Claude: `"Claude received: hello"`<br>Gemini: `"Gemini received: hello"`<br>GPT: `"GPT received: hello"` |
| Verbosity (Explain) | ❌ | ❌ | ✅ | GPT provides multi-line structured breakdown for "explain" keyword | `"explain sessions"` → 3-line response with numbered list |
| Error Detail | ⚠️ Basic | ⚠️ Basic | ✅ Enhanced | GPT includes actionable guidance in error messages | GPT: `"session X not found. Verify with 'agm session list'."` |
| First Message Recall | ❌ | ❌ | ✅ | GPT can reference first message when prompted with "recall first" | `"recall first"` → Response includes "message 1" |

---

## Edge Cases

| Edge Case | Agents Affected | Reproduction | Expected Behavior | BDD Scenario |
|-----------|-----------------|--------------|-------------------|--------------|
| Large message history (25+ messages) | Claude, Gemini, GPT | Create session, send 25 sequential messages, send 1 more, check history count | All 52 messages (25 user + 25 assistant + 1 user + 1 assistant) preserved in history array | `conversation_persistence.feature:54-67` |
| Invalid session ID | Claude, Gemini, GPT | Call `SendMessage` with non-existent session ID | Error: `"session {id} not found"` (GPT adds: `"Verify with 'agm session list'."`) | `session_lifecycle.feature:49-59` |
| Concurrent session isolation | Claude, Gemini, GPT | Create 2 sessions with same agent, send different messages to each | Each session history contains only its own messages (no leakage) | `session_lifecycle.feature:61-75` |
| Archived session messaging | Claude, Gemini, GPT | Archive session, attempt to send message | Error: `"session {id} is archived"` (GPT adds: `"Use 'agm session resume' to reactivate."`) | Existing scenarios (implicit in archive tests) |
| Multi-turn context retention | Claude, Gemini, GPT | Send 5 messages building context (color, age, location, job, question) | Final response correctly references information from earlier messages | `conversation_persistence.feature:37-52` |

---

## Performance Characteristics

| Characteristic | Claude | Gemini | GPT | Notes |
|----------------|--------|--------|-----|-------|
| Context Window | Simulated: 200k tokens | Simulated: 1M tokens | Simulated: 128k tokens | Mock adapters don't enforce limits; values represent real agent capabilities |
| Response Latency | Simulated: instant | Simulated: instant | Simulated: instant | Real agents: 500ms-5s depending on load |
| Thread Safety | ✅ `sync.RWMutex` | ✅ `sync.RWMutex` | ✅ `sync.RWMutex` | All mocks use read/write locks for concurrent access |
| Determinism | ✅ Fully deterministic | ✅ Fully deterministic | ✅ Fully deterministic | Same input always produces same output (no randomness) |

---

## Usage Recommendations

### When to Use Claude
- Standard conversational interactions
- Basic context retention
- Fast iteration (simple echo responses)

### When to Use Gemini
- Testing behavior consistency with Claude
- Validating cross-agent compatibility
- Custom string manipulation patterns (Gemini uses custom case-insensitive matching)

### When to Use GPT
- Testing verbose/detailed responses (explain pattern)
- Validating enhanced error messages
- Testing first message recall functionality
- Demonstrating agent-specific behavioral differences

---

## Testing Notes

### BDD Test Pass Rates (Expected)
- **Claude**: 100% (all scenarios pass)
- **Gemini**: ≥90% (minor differences in string matching may cause 1-2 failures)
- **GPT**: ≥90% (unique patterns may cause failures in tests expecting exact Claude behavior)

### Known Differences
1. **Response Prefixes**: Each agent uses different prefix (`"Claude received:"` vs `"Gemini received:"` vs `"GPT received:"`)
2. **Error Messages**: GPT provides more detailed error messages with actionable guidance
3. **Explain Pattern**: Only GPT supports verbose multi-line responses for "explain" keyword
4. **First Message Recall**: Only GPT supports "recall first" pattern

### Adding New Agents
To add a new agent mock (e.g., Claude-3.5):
1. Create `{agent}.go` implementing `Adapter` interface
2. Add to `testenv.Environment` struct
3. Update `GetAdapter()` switch statement
4. Add `{agent}` row to all Scenario Outline Examples tables
5. Document differences in this capability matrix

---

## References

- **Mock Adapters**: `main/agm/test/bdd/internal/adapters/mock/`
- **BDD Scenarios**: `main/agm/test/bdd/features/`
- **Test Execution**: `go test ./test/bdd/... -v`

---

**Last Updated**: 2026-01-18
**Maintained By**: agm development team
