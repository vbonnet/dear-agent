# ADR-001: File-Based Session Storage with JSONL History

**Status:** Accepted
**Date:** 2026-02-11
**Deciders:** Gemini Adapter Development Team
**Context:** V1 Implementation

## Context and Problem Statement

The Gemini Adapter needs to store conversation sessions (messages, context, metadata). We must decide between in-memory storage, file-based storage, or database storage for the V1 implementation.

**Key Requirements:**
- Store multiple concurrent sessions
- Persist conversations across process restarts
- Simple implementation
- Human-readable format

**Constraints:**
- Sessions should survive process restarts (unlike GPT adapter V1)
- Must integrate with existing SessionStore interface
- Compatible with export/import feature

## Decision Drivers

- **Persistence:** Users expect conversations to survive restarts
- **Simplicity:** Avoid database complexity
- **Compatibility:** Reuse JSONL format from export/import
- **Debugging:** Human-readable format helps troubleshooting
- **Storage Pattern:** Learn from GPT adapter (in-memory) vs Claude adapter (file-based)

## Considered Options

### Option 1: In-Memory Storage (Like GPT Adapter)
**Implementation:**
```go
type GeminiAdapter struct {
    sessions map[agent.SessionID]*Session
    mu       sync.RWMutex
}
```

**Pros:**
- ✅ Simple implementation
- ✅ O(1) lookups
- ✅ No file I/O overhead

**Cons:**
- ❌ Data lost on process restart
- ❌ Not production-ready
- ❌ Inconsistent with user expectations for API agents

### Option 2: File-Based Storage with JSONL (Chosen)
**Implementation:**
```go
// Session metadata: ~/.agm/sessions.json (shared store)
// Conversation history: ~/.agm/gemini/{session-id}/history.jsonl
type GeminiAdapter struct {
    sessionStore SessionStore // For metadata
    modelName    string
    apiKey       string
}
```

**Pros:**
- ✅ Persistent across restarts
- ✅ Human-readable JSONL format
- ✅ Compatible with export feature (same format)
- ✅ Simple append-only writes
- ✅ No database dependency
- ✅ Aligns with Claude adapter pattern

**Cons:**
- ⚠️ Slower than in-memory (disk I/O)
- ⚠️ File corruption risk (mitigated by append-only)
- ⚠️ More complex than in-memory

### Option 3: Database Storage (SQLite)
**Implementation:**
```go
// SQLite database: sessions table + messages table
CREATE TABLE sessions (id TEXT PRIMARY KEY, ...);
CREATE TABLE messages (id TEXT, session_id TEXT, content TEXT, ...);
```

**Pros:**
- ✅ Persistent across restarts
- ✅ ACID transactions
- ✅ Scalable (indexes, queries)

**Cons:**
- ❌ Heavy dependency
- ❌ Overkill for simple storage
- ❌ More complex testing
- ❌ Schema migration complexity

## Decision Outcome

**Chosen Option:** **Option 2 - File-Based Storage with JSONL**

**Rationale:**
1. **User Expectations:** API agents should persist conversations (unlike CLI agents where tmux provides persistence)
2. **Format Reuse:** JSONL already used for export/import, no new format needed
3. **Simplicity:** File-based simpler than database, more robust than in-memory
4. **Debugging:** Human-readable files help troubleshooting
5. **Consistency:** Aligns with Claude adapter's file-based approach
6. **Production-Ready:** Suitable for real-world usage from V1

**Trade-offs Accepted:**
- Slightly slower than in-memory (acceptable for typical usage)
- File I/O adds complexity (mitigated by append-only pattern)
- Manual directory cleanup needed (sessions preserved after termination)

## Implementation Details

### Storage Layout
```
~/.agm/
├── sessions.json              # Metadata store (all agents)
└── gemini/
    ├── abc-123.../
    │   └── history.jsonl      # Session abc-123 history
    └── def-456.../
        └── history.jsonl      # Session def-456 history
```

### JSONL Format
```jsonl
{"id":"msg-1","role":"user","content":"Hello","timestamp":"2026-02-11T10:00:00Z"}
{"id":"msg-2","role":"assistant","content":"Hi!","timestamp":"2026-02-11T10:00:01Z"}
```

### Append-Only Writes
```go
func (a *GeminiAdapter) appendToHistory(sessionID SessionID, message Message) error {
    historyPath, err := a.getHistoryPath(sessionID)
    if err != nil {
        return err
    }

    file, err := os.OpenFile(historyPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return fmt.Errorf("failed to open history file: %w", err)
    }
    defer file.Close()

    encoder := json.NewEncoder(file)
    if err := encoder.Encode(message); err != nil {
        return fmt.Errorf("failed to write message: %w", err)
    }

    return nil
}
```

### Session Lifecycle
1. **Create:**
   - Generate UUID
   - Create directory: `~/.agm/gemini/{session-id}/`
   - Store metadata in SessionStore
2. **Send Message:**
   - Load full history from JSONL
   - Send to Gemini API with history
   - Append user message to JSONL
   - Append assistant response to JSONL
3. **Terminate:**
   - Delete metadata from SessionStore
   - **Preserve** session directory and history file
4. **Restart:**
   - All sessions in SessionStore are resumable
   - History loaded from JSONL files

## Consequences

### Positive
- ✅ Sessions survive process restarts
- ✅ Human-readable conversation logs
- ✅ Compatible with existing export/import
- ✅ Simple append-only write pattern
- ✅ Production-ready from V1
- ✅ Consistent with Claude adapter pattern

### Negative
- ⚠️ File I/O overhead on every message
- ⚠️ Session directories preserved after termination (manual cleanup needed)
- ⚠️ Full history loaded into memory for each API call

### Neutral
- ℹ️ JSONL parsing required for GetHistory()
- ℹ️ File corruption risk mitigated by append-only writes
- ℹ️ Directory structure visible to users (transparency)

## Validation

### Success Metrics
- [x] Sessions persist across process restarts
- [x] History files are valid JSONL
- [x] Export/Import uses same format
- [x] All tests pass with file-based storage
- [x] No file corruption in testing

### Risks Mitigated
- **File Corruption:** Append-only writes prevent partial line writes
- **Disk Space:** User responsible for cleanup (documented limitation)
- **Performance:** File I/O acceptable for typical conversation length
- **Concurrency:** Single-writer pattern (no concurrent writes to same file)

## Alternative Considered: Hybrid Approach

**Proposal:** In-memory cache + file-based persistence
```go
type GeminiAdapter struct {
    sessionStore SessionStore
    historyCache map[SessionID][]Message // In-memory cache
    mu           sync.RWMutex
}
```

**Why Not Chosen:**
- Added complexity without significant benefit
- Cache invalidation complexity
- V1 goal is simplicity
- Can be added in V2 if performance issues arise

## Migration Path (V2 Enhancements)

### Potential Optimizations
1. **In-Memory Cache:**
   - Cache loaded histories
   - Invalidate on write
   - Reduce file I/O on repeated reads

2. **Partial History Loading:**
   - Load last N messages only
   - Reduce memory footprint for long conversations
   - Requires context window management

3. **Compression:**
   - gzip old history files
   - Trade space for CPU
   - Transparent to user

4. **Database Migration:**
   - If file-based proves insufficient
   - SQLite for sessions with >1000 messages
   - Fallback to file-based for simplicity

## References

- [SPEC.md](SPEC.md) - Session storage requirements
- [ARCHITECTURE.md](ARCHITECTURE.md) - Storage architecture details
- [GPT ADR-001](../../gpt/ADR-001-in-memory-storage.md) - Comparison with GPT approach
- [Agent Interface](../../interface.go) - Storage-agnostic interface design
