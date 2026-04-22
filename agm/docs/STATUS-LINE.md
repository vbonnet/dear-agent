# AGM StatusLine Capture System

## Overview

Claude Code (CC) supports a `statusLine` command in `settings.json` that
receives full session JSON on stdin after every assistant message. AGM uses
this to capture exact cost, model, context window, and rate limit data in
real-time.

## Data Flow

```
CC assistant message completes
        |
        v
CC pipes session JSON to statusLine command's stdin
        |
        v
agm-statusline-capture (Go binary)
  1. Reads JSON from stdin
  2. Extracts session_id
  3. Writes full JSON to /tmp/agm-context/{session_id}.json
  4. Outputs prompt text to stdout (for terminal display)
        |
        v
/tmp/agm-context/{session_id}.json
        |
        v
agm status-line (tmux status bar renderer)
  Reads the JSON file via ReadStatusLineFile()
  Displays cost, model, context %, rate limits
```

## Configuration

In `~/.claude/settings.json`, set the `statusLine` command:

```json
{
  "statusLine": "agm-statusline-capture"
}
```

The binary must be in `$PATH`. CC will invoke it after every assistant
message, piping session JSON to stdin and displaying stdout in the
terminal status area.

## Session JSON Schema (from CC)

CC pipes the following JSON to stdin:

```json
{
  "session_id": "abc123...",
  "transcript_path": "/path/to/transcript.jsonl",
  "cwd": "~/src",
  "session_name": "my-session",
  "model": {
    "id": "claude-opus-4-6",
    "display_name": "Opus 4.6 (1M context)"
  },
  "cost": {
    "total_cost_usd": 64.09,
    "total_duration_ms": 13471590
  },
  "context_window": {
    "total_input_tokens": 311334,
    "context_window_size": 1000000,
    "used_percentage": 17,
    "remaining_percentage": 83
  },
  "version": "2.1.81"
}
```

## Available Data Fields

| Field | Description |
|-------|-------------|
| `session_id` | CC session UUID |
| `transcript_path` | Path to the session's JSONL transcript |
| `cwd` | Current working directory |
| `session_name` | Human-readable session name |
| `model.id` | Model API identifier (e.g., `claude-opus-4-6`) |
| `model.display_name` | Human-readable model name |
| `cost.total_cost_usd` | Exact cumulative session cost in USD |
| `cost.total_duration_ms` | Total session duration in milliseconds |
| `context_window.total_input_tokens` | Tokens used in the context window |
| `context_window.context_window_size` | Maximum context window size |
| `context_window.used_percentage` | Percentage of context window used |
| `context_window.remaining_percentage` | Percentage of context window remaining |
| `version` | CC version string |

## Cost Accuracy

AGM has two sources of cost data:

| Source | Accuracy | When Used |
|--------|----------|-----------|
| StatusLine `total_cost_usd` | **Exact** (CC-calculated) | Interactive sessions (statusLine fires) |
| Token-based estimate (`estimateCostFromUsage`) | Approximate | Non-interactive sessions (`-p` mode) where statusLine does not fire |

The priority order for cost display is:

1. **StatusLine file cost** (exact, from CC) -- preferred
2. **Manifest `LastKnownCost`** (cached from previous statusline read)
3. **Estimated cost from conversation log** (token-count-based fallback)

For interactive sessions, the statusline capture runs after every assistant
message, so cost data is always fresh and exact.
