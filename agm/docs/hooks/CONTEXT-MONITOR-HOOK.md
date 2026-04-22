# Context Monitor Hook Interface

## Overview

The Context Monitor Hook automatically updates AGM session manifests with real-time context usage data extracted from Claude Code's output. This enables the tmux status line to display accurate context percentages without manual updates.

## Hook Type

**PostToolUse Hook** - Executes after every tool call in Claude Code

## Hook Contract

### Input (Environment Variables)

Claude Code provides these environment variables to PostToolUse hooks:

```bash
CLAUDE_SESSION_ID        # Current Claude session UUID
CLAUDE_TOOL_NAME         # Name of tool that was executed
CLAUDE_TOOL_RESULT       # Tool execution result/output
CLAUDE_WORKING_DIR       # Current working directory
CLAUDE_PROJECT_DIR       # Project directory (if set)
```

### Additional Context (from stdin)

The hook receives a JSON object on stdin with tool execution details:

```json
{
  "session_id": "uuid",
  "tool_name": "Bash",
  "tool_result": "...",
  "token_usage": {
    "input_tokens": 12345,
    "output_tokens": 678,
    "total_tokens": 13023
  },
  "timestamp": "2026-03-15T10:30:00Z"
}
```

### Output (Return Value)

- Exit code 0: Success
- Exit code 1: Non-fatal error (logged, execution continues)
- Exit code 2: Fatal error (execution may halt)

## Token Extraction Strategy

### Primary Source: System Reminders

Claude Code includes token usage in system reminder messages:

```
<system-reminder>Token usage: 12345/200000; 187655 remaining</system-reminder>
```

Pattern: `Token usage: (\d+)/(\d+); (\d+) remaining`

Extracted data:
- Used tokens: Group 1 (12345)
- Total tokens: Group 2 (200000)
- Percentage: (used / total) * 100

### Fallback: Tool Result Scanning

If system reminders aren't present, scan tool results for token indicators:

```
Budget: 12345 tokens used of 200000 (6.2%)
```

### Error Handling

- Missing token data: Skip update (no-op)
- Invalid format: Log warning, skip update
- Calculation errors: Use last known value

## AGM Integration

### Manifest Update

The hook calls `agm session set-context-usage` to update the manifest:

```bash
agm session set-context-usage "$percentage" --session "$session_name"
```

### Session Detection

The hook needs to map `CLAUDE_SESSION_ID` to AGM session name:

1. Check `~/.claude/sessions/$CLAUDE_SESSION_ID/manifest.yaml` for AGM association
2. Look for `agm_session_name` field
3. If not found, skip update (session not AGM-managed)

### Update Frequency

- **Minimum interval**: 10 seconds (avoid excessive manifest writes)
- **Cache**: Store last update timestamp in `/tmp/agm-context-cache-$session_id`
- **Threshold**: Only update if percentage changed by ≥1%

## Implementation Phases

### Phase 1: Basic Hook (Current)

- Parse system reminders for token usage
- Update AGM manifest via CLI command
- No caching (update on every hook call)

### Phase 2: Optimized Hook (Future)

- Add 10-second update interval throttling
- Cache last percentage to avoid redundant updates
- Batch updates (queue changes, flush periodically)

### Phase 3: Native Integration (Future)

- AGM daemon receives context updates directly
- No CLI subprocess overhead
- Real-time updates via IPC

## Hook Installation

### Location

```
~/.claude/hooks/posttool/context-monitor
```

### Registration

Claude Code auto-discovers hooks in:
- `~/.claude/hooks/posttool/`
- Project-specific: `.claude/hooks/posttool/`

### Permissions

```bash
chmod +x ~/.claude/hooks/posttool/context-monitor
```

## Testing Strategy

### Unit Tests

- Token extraction from various formats
- Session ID mapping logic
- Percentage calculation edge cases

### Integration Tests

1. Create AGM session
2. Execute tool calls in Claude Code
3. Verify manifest updates automatically
4. Check status line reflects changes

### Multi-Agent Tests

Test with all supported agent types:
- Claude Code (claude)
- Gemini CLI (gemini)
- ChatGPT CLI (gpt)
- OpenCode (opencode)

## Error Scenarios

| Error | Handling | Example |
|-------|----------|---------|
| No AGM session | Skip silently | Non-AGM Claude session |
| Invalid token format | Log warning, skip | Corrupted system reminder |
| AGM CLI unavailable | Log error, skip | `agm` not in PATH |
| Manifest write failure | Log error, retry once | File permission issue |
| Session not found | Log warning, skip | AGM session deleted |

## Performance Considerations

### Hook Execution Time

- **Target**: <50ms per invocation
- **Measurement**: Log execution time in debug mode
- **Optimization**: Minimize subprocess calls, cache lookups

### Overhead

- PostToolUse hooks run on EVERY tool call
- Fast execution is critical to avoid slowing down Claude Code
- Use Python (fast startup) rather than shell script (subprocess heavy)

## Security Considerations

- Hook runs with user permissions (no privilege escalation)
- Input from Claude Code is trusted (same user session)
- Validate percentage values (0-100 range)
- Sanitize session names (prevent command injection)

## Example Usage

### Scenario: User working in AGM session

1. User executes tool in Claude Code
2. PostToolUse hook triggers
3. Hook extracts token usage: "45000/200000"
4. Calculates percentage: 22.5%
5. Updates AGM manifest: `agm session set-context-usage 22.5`
6. Tmux status line refreshes (next 10-second interval)
7. User sees: `🤖 DONE | 22% | main (+3) | my-session`

### Scenario: User in non-AGM session

1. User executes tool in Claude Code
2. PostToolUse hook triggers
3. Hook checks for AGM association: Not found
4. Hook exits silently (exit code 0)
5. No manifest update

## Future Enhancements

1. **Context Alerts**: Notify user at 70%, 80%, 90% thresholds
2. **Historical Tracking**: Store context usage timeline
3. **Predictive Warnings**: Estimate when context will be exhausted
4. **Multi-Session Aggregation**: Show total context across all sessions
5. **Context Optimization**: Auto-compact when threshold reached

## References

- AGM manifest schema: `internal/manifest/manifest.go`
- Status line implementation: `cmd/agm/status_line.go`
- Claude Code hooks: `~/.claude/hooks/README.md`
- Token usage tracking: Claude Code system reminders
