# ADR-001: Normalize User Input When Searching History for /rename Commands

**Status**: Accepted
**Date**: 2026-02-17
**Authors**: AGM Team
**Related Issues**: UUID discovery failure with trailing whitespace

## Context

AGM's UUID discovery system searches `~/.claude/history.jsonl` for `/rename` commands to find the Claude session UUID associated with a given session name. The discovery flow is:

1. User runs: `agm session associate my-session`
2. AGM searches history for: `/rename my-session`
3. If found, extracts the `sessionId` (Claude UUID) from that entry

The search compares the `display` field from history.jsonl against the expected command string.

### The Bug

Claude Code writes the **exact user input** to history.jsonl, including trailing whitespace. When users type commands with trailing spaces (common behavior), the search fails:

**User typed:**
```
/rename my-session
                   ↑ trailing space
```

**History.jsonl records:**
```json
{
  "display": "/rename my-session ",
  "sessionId": "abc-123"
}
```

**AGM searches for:**
```
"/rename my-session"
                   ↑ no trailing space
```

**Result**: Exact string match fails → UUID not found → association fails

### Real-World Impact

This bug was discovered when `/agm:agm-assoc agm-new-broken` failed with:
```
❌ UUID discovery failed for 'agm-new-broken':
  Level 2a (rename): no rename found for: agm-new-broken
```

Even though the rename existed in history with a trailing space.

## Decision

**Normalize the `display` field with `strings.TrimSpace()` before comparison.**

### Implementation

**Before:**
```go
renameCmd := "/rename " + sessionName
for _, entry := range entries {
    if entry.Display == renameCmd {  // ❌ Exact match fails with whitespace
        // ...
    }
}
```

**After:**
```go
renameCmd := "/rename " + sessionName
for _, entry := range entries {
    if strings.TrimSpace(entry.Display) == renameCmd {  // ✅ Normalized match
        // ...
    }
}
```

## Rationale

1. **User behavior**: Trailing whitespace is common when typing commands
2. **Source of truth**: Claude Code faithfully records exact input (correct behavior)
3. **Search robustness**: AGM should handle variations in user input (our responsibility)
4. **No side effects**: TrimSpace only affects search, doesn't modify history file
5. **Performance**: Negligible (runs once per history entry during search)

## Consequences

### Positive
- ✅ UUID discovery works with trailing/leading whitespace
- ✅ More forgiving user experience
- ✅ Matches shell behavior (ignores leading/trailing spaces)
- ✅ No breaking changes (only makes search more permissive)

### Negative
- None identified

### Test Coverage
Added regression test in `discovery_test.go`:
```go
{
    name:        "trailing whitespace in display field - should match",
    sessionName: "trailing-space-session",
    wantUUID:    "44444444-4444-4444-4444-444444444444",
    wantErr:     false,
}
```

Test data includes entry with trailing space:
```go
{
    SessionID: "44444444-4444-4444-4444-444444444444",
    Display:   "/rename trailing-space-session ", // Trailing space
}
```

## Alternatives Considered

### Option 1: Exact Match Only
- Require users to type commands without trailing spaces
- **Rejected**: Poor UX, doesn't match shell conventions

### Option 2: Fuzzy Match
- Use Levenshtein distance to match similar strings
- **Rejected**: Over-engineered for whitespace handling, risk of false positives

### Option 3: Normalize on Write
- Modify Claude Code to trim whitespace before writing to history
- **Rejected**: Changes behavior in Claude Code, affects other tools that read history

### Option 4: Regex Match
- Use regex to match with optional whitespace: `/rename\s+session-name\s*`
- **Rejected**: More complex than needed for simple whitespace handling

## Related Changes

- Commit: 038d1be "fix(agm): handle trailing whitespace in /rename commands"
- File: `internal/uuid/discovery.go:63`
- Test: `internal/uuid/discovery_test.go:107-113`

## References

- Discovery implementation: `internal/uuid/discovery.go`
- Test coverage: `internal/uuid/discovery_test.go`
- History format: Claude Code stores exact user input in `display` field
