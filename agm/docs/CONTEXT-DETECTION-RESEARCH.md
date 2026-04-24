# Context Detection Research - Claude Code Sessions

## Research Summary

Investigation into extracting context usage from Claude Code session state files.

**Date**: 2026-03-15 (updated 2026-03-23)
**Goal**: Determine if context usage can be automatically detected from session files
**Status**: Complete - Implemented and verified in production

---

## Findings

### 1. Claude Code Session Structure

Claude Code stores session data in `~/.claude/projects/{project-path-hash}/`:
- `{session-uuid}.jsonl` - Full conversation history with tool calls, results, and usage
- `{session-uuid}/subagents/` - Sub-agent conversation logs

The `{project-path-hash}` is derived from the project working directory path
(e.g., `~/src` becomes `-home-user-src`).

### 2. Token Usage Availability

**Stored in every assistant message** as structured JSON `usage` field:
```json
{
  "type": "assistant",
  "message": {
    "model": "claude-sonnet-4-5-20250929",
    "usage": {
      "input_tokens": 12,
      "cache_creation_input_tokens": 4014,
      "cache_read_input_tokens": 23244,
      "output_tokens": 1
    }
  }
}
```

**Key insight**: `input_tokens + cache_creation_input_tokens + cache_read_input_tokens`
gives the total input context size for that API call. The last assistant message reflects
the current conversation size. Combined with model-specific context window sizes (200K
for Claude 3.5/4.x models), this yields an accurate context usage percentage.

### 3. Context Detection Strategies

#### Strategy A: Parse conversation JSONL (IMPLEMENTED - Phase 7+)
**Approach**: Read last 20KB of `{uuid}.jsonl`, parse structured JSON `usage` fields

**Implementation** (current):
```go
func extractUsageFromJSONL(line string) *manifest.ContextUsage {
    // Parse JSON entry, extract assistant message usage
    totalInput := msg.Usage.InputTokens +
                  msg.Usage.CacheCreationInputTokens +
                  msg.Usage.CacheReadInputTokens
    percentage := float64(totalInput) / float64(contextWindow) * 100.0
}
```

**Pros**:
- Works without any hooks (fully automatic)
- Accurate (uses actual API token counts)
- Efficient (tail-read last 20KB, <50ms)

**Cons**:
- Shows last request's usage (slightly stale between requests)
- Requires knowing model context window sizes

#### Strategy B: Use AGM manifest (PREFERRED when hook active)
**Approach**: Read context from AGM manifest (updated by PostToolUse hook)

**Implementation**:
```go
if m.ContextUsage != nil && m.ContextUsage.PercentageUsed >= 0 {
    return m.ContextUsage, nil
}
```

#### Strategy C: Hook-based capture (Phase 6)
**Approach**: PostToolUse hook captures token usage and updates manifest

**Status**: Implemented but depends on hook installation

---

## Implementation Details

### Four-Tier Fallback Chain

```go
func DetectContextFromManifestOrLog(m *manifest.Manifest) (*manifest.ContextUsage, error) {
    // Tier 1: Manifest (updated by PostToolUse hook)
    if m.ContextUsage != nil && m.ContextUsage.PercentageUsed >= 0 {
        return m.ContextUsage, nil
    }

    // Tier 2: statusLine file (/tmp/agm-context/{session_id}.json)
    // The statusLine API is now the recommended primary source for context data.
    // These files are written by the statusLine subsystem and contain structured
    // JSON with session state, context usage, and other metadata.
    if m.SessionID != "" {
        if usage, err := DetectContextFromStatusLine(m.SessionID); err == nil {
            return usage, nil
        }
    }

    // Tier 3: Parse conversation log (automatic fallback)
    if m.Claude.UUID != "" {
        if usage, err := DetectContextFromConversationLog(m.Claude.UUID); err == nil {
            return usage, nil
        }
    }

    // Tier 4: Unavailable
    return nil, fmt.Errorf("context usage unavailable")
}
```

### Conversation Log Path Resolution

Claude Code conversation logs are found via glob:
```
~/.claude/projects/*/{session-uuid}.jsonl
```

Legacy fallback paths:
```
~/.claude/projects/{session-uuid}/conversation.jsonl
~/.claude/sessions/{session-uuid}/conversation.jsonl
```

### Model Context Windows

| Model Prefix | Context Window |
|-------------|----------------|
| claude-opus-4 | 200,000 |
| claude-sonnet-4 | 200,000 |
| claude-haiku-4 | 200,000 |
| claude-3-5 | 200,000 |
| claude-3-opus | 200,000 |
| claude-3-sonnet | 200,000 |
| claude-3-haiku | 200,000 |
| Unknown claude-* | 200,000 (default) |

### Caching

- Cache parsed context for 30 seconds per session
- Invalidate when file modification time changes (detects new responses)
- Cache valid only if BOTH: age < 30s AND file unchanged

---

## Verification Results (2026-03-23)

Tested with live AGM sessions on `/tmp/agm.sock`:

| Session | Context % | Color | Status |
|---------|-----------|-------|--------|
| agm-tmux-status-line | 94% | orange | WORKING |
| audit-suite | 100% | red | WORKING |
| auto-research-checkup | 66% | green | WORKING |

All sessions correctly show context usage in the tmux status line.

---

## Files Modified

- `internal/session/context_detector.go` - Core detection logic
- `internal/session/context_detector_test.go` - Unit tests
- `internal/session/status_line_collector.go` - Integration with status line
- `cmd/agm/install_tmux_status.go` - tmux socket installation
- `cmd/agm/status_line.go` - CLI command

---

## Security Considerations

- Conversation logs may contain sensitive data
- Only read from user's own sessions (no privilege escalation)
- File paths validated via `filepath.Glob` (no directory traversal)
- No network access required
