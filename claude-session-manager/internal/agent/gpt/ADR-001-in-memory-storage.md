# ADR-001: In-Memory Session Storage for V1

**Status:** Accepted
**Date:** 2026-02-11
**Deciders:** GPT Adapter Development Team
**Context:** V1 Implementation

## Context and Problem Statement

The GPT Adapter needs to store conversation sessions (messages, context, metadata). We must decide between in-memory storage, file-based storage, or database storage for the V1 implementation.

**Key Requirements:**
- Store multiple concurrent sessions
- Thread-safe access
- Fast read/write performance
- Simple implementation (V1 goal)

**Constraints:**
- Limited development time for V1
- Must ship working adapter quickly
- Can iterate in V2

## Decision Drivers

- **Development Speed:** V1 prioritizes shipping over feature completeness
- **Simplicity:** Reduce complexity for initial implementation
- **Performance:** In-memory access is faster than disk I/O
- **Testing:** In-memory storage is easier to test (no file cleanup)
- **Interface Compliance:** Must implement all `agent.Agent` methods regardless of storage

## Considered Options

### Option 1: In-Memory Storage (Map)
**Implementation:**
```go
type Adapter struct {
    sessions map[agent.SessionID]*Session
    mu       sync.RWMutex
}
```

**Pros:**
- ✅ Simple implementation (50 lines of code)
- ✅ O(1) lookups
- ✅ No file I/O overhead
- ✅ No serialization/deserialization complexity
- ✅ Easy to test (no file cleanup)
- ✅ Thread-safe with simple mutex

**Cons:**
- ❌ Data lost on process restart
- ❌ Memory usage grows with conversation history
- ❌ No persistence across restarts
- ❌ Not production-ready for long-running services

### Option 2: File-Based Storage (JSONL)
**Implementation:**
```go
// Each session: ~/.csm/sessions/gpt/{session-id}.jsonl
func (a *Adapter) saveSession(sessionID) {
    file, _ := os.Create(sessionPath(sessionID))
    for _, msg := range session.Messages {
        json.Marshal(msg, file)
    }
}
```

**Pros:**
- ✅ Persistent across restarts
- ✅ No memory limitations
- ✅ Human-readable format (JSONL)
- ✅ Compatible with export/import feature

**Cons:**
- ❌ Complex implementation (file locking, atomic writes)
- ❌ Slower performance (disk I/O on every message)
- ❌ File corruption risk (power loss, partial writes)
- ❌ Cross-platform path handling complexity
- ❌ File cleanup/garbage collection needed
- ⚠️ Estimated 200+ lines of additional code

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
- ✅ Scalable (indexes, query optimization)
- ✅ Standard SQL interface

**Cons:**
- ❌ Heavy dependency (SQLite driver)
- ❌ Schema migration complexity
- ❌ Overkill for simple key-value storage
- ❌ More complex testing (database fixtures)
- ⚠️ Estimated 300+ lines of additional code

## Decision Outcome

**Chosen Option:** **Option 1 - In-Memory Storage**

**Rationale:**
1. **V1 Goal Alignment:** Ship working adapter quickly, iterate later
2. **Simplicity:** 50 lines vs. 200+ lines (4x less code)
3. **Sufficient for Testing:** Development and integration testing work fine
4. **Interface Compliance:** Implements all 12 `agent.Agent` methods regardless of persistence
5. **Export/Import Workaround:** Users can export sessions to JSONL for persistence if needed
6. **Clear V2 Path:** File-based storage planned for V2

**Trade-offs Accepted:**
- Session data lost on restart (acceptable for V1 dev/test usage)
- Memory growth with conversation length (acceptable for short-lived sessions)
- Not production-ready (V1 is for testing parity with Claude/Gemini adapters)

## Implementation Details

### Session Storage
```go
type Adapter struct {
    client   *openai.Client
    sessions map[agent.SessionID]*Session // In-memory only
    mu       sync.RWMutex                // Thread safety
    model    string
}
```

### Thread Safety
```go
// Read operations
a.mu.RLock()
session, exists := a.sessions[sessionID]
a.mu.RUnlock()

// Write operations
a.mu.Lock()
a.sessions[sessionID] = newSession
a.mu.Unlock()
```

### Session Lifecycle
1. **Create:** `sessions[id] = &Session{...}` (in map)
2. **Update:** Append to `session.Messages` slice
3. **Read:** Copy messages slice (prevent external modification)
4. **Delete:** `delete(sessions, id)` (remove from map)
5. **Restart:** All sessions lost (documented limitation)

## Consequences

### Positive
- ✅ V1 shipped in 3 days instead of 2 weeks
- ✅ All 12 Agent methods implemented and tested
- ✅ Thread-safe with simple mutex design
- ✅ >90% test coverage achieved
- ✅ No file I/O bugs or edge cases

### Negative
- ❌ Sessions lost on restart (documented in README)
- ❌ Users must export important conversations manually
- ❌ Not suitable for production deployments (V1 limitation)

### Neutral
- ⚠️ V2 will require refactoring for file-based storage
- ⚠️ Export/Import feature provides manual persistence workaround

## Validation

### Success Metrics
- [x] All tests pass with in-memory storage
- [x] Thread safety validated with `go test -race`
- [x] Export/Import provides manual persistence
- [x] Documentation clearly states V1 limitation

### Risks Mitigated
- **Memory Leaks:** Sessions explicitly deleted via `TerminateSession()`
- **Concurrent Access:** Protected by `sync.RWMutex`
- **Data Loss:** Documented limitation, export feature available

## Migration Path (V2)

### File-Based Storage Implementation
```go
type Adapter struct {
    // V1: In-memory
    sessions map[agent.SessionID]*Session

    // V2: File-based (new field)
    storageDir string // ~/.csm/sessions/gpt/
}

func (a *Adapter) loadSessions() error {
    // Load all .jsonl files from storageDir
    files, _ := os.ReadDir(a.storageDir)
    for _, file := range files {
        session := parseJSONL(file)
        a.sessions[session.ID] = session
    }
}

func (a *Adapter) saveSession(sessionID) error {
    // Write session.Messages to {storageDir}/{sessionID}.jsonl
    file, _ := os.Create(filepath.Join(a.storageDir, sessionID+".jsonl"))
    defer file.Close()
    for _, msg := range a.sessions[sessionID].Messages {
        json.NewEncoder(file).Encode(msg)
    }
}
```

### Backward Compatibility
- V2 can migrate V1 sessions via export/import
- JSONL format already supported in V1
- No breaking changes to Agent interface

## References

- [SPEC.md](SPEC.md) - V1 limitations documented
- [README.md](README.md) - User-facing limitation notice
- [ADR-002](ADR-002-exponential-backoff.md) - Error handling strategy
- [Agent Interface](../interface.go) - Storage-agnostic interface design

## Notes

- This ADR focuses on **V1 implementation speed** over feature completeness
- Persistence deferred to V2 based on user feedback
- Export/Import feature provides acceptable workaround for critical data
- Decision revisited in V2 planning (Q2 2026)
