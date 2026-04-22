# AGM Agentic API Reference

## Overview

AGM exposes three API surfaces, all backed by a shared operations layer (`internal/ops/`):

| Surface | Consumer | Entry Point |
|---------|----------|-------------|
| **CLI** | Humans, shell scripts | `agm session list --output json` |
| **MCP** | Claude, LLM tool-use | MCP server (JSON-RPC) |
| **Skills** | Claude Code agents | Skill files with frontmatter |

Every operation flows through `ops.OpContext`, ensuring identical behavior,
error handling, and output formatting regardless of surface.

## Error Code Catalog

All errors use stable codes that agents can match on programmatically.

| Code | Status | Type | Title | When | Suggestion |
|------|--------|------|-------|------|------------|
| AGM-001 | 404 | `session/not_found` | Session not found | Identifier matches no session | `agm session list` or `--all` |
| AGM-002 | 409 | `session/archived` | Session is archived | Mutating an archived session | Unarchive first, or create new |
| AGM-003 | 503 | `tmux/not_running` | Tmux not running | No tmux server detected | Start tmux, check socket |
| AGM-004 | 503 | `dolt/unavailable` | Dolt unavailable | Dolt server not reachable | `agm admin dolt-status` |
| AGM-005 | 400 | `input/invalid` | Invalid input | Bad field value or format | Check value, use `--schema` |
| AGM-006 | 403 | `permission/denied` | Permission denied | Insufficient permissions | Check file/socket permissions |
| AGM-007 | 409 | `session/exists` | Session exists | Creating duplicate session | Use different name or resume |
| AGM-008 | 503 | `harness/unavailable` | Harness unavailable | AI harness not responding | Check harness process |
| AGM-009 | 404 | `workspace/not_found` | Workspace not found | Workspace detection failed | Set `--workspace` explicitly |
| AGM-010 | 404 | `uuid/not_associated` | UUID not associated | UUID not linked to session | `agm admin fix-uuid` |
| AGM-011 | 500 | `storage/error` | Storage error | Storage read/write failed | `agm admin doctor` |
| AGM-100 | 200 | `dry_run` | Dry run | `--dry-run` flag is set | Remove flag to execute |

## RFC 7807 Error Format

All errors are returned as RFC 7807 Problem Details objects:

```json
{
  "status": 404,
  "type": "session/not_found",
  "code": "AGM-001",
  "title": "Session not found",
  "detail": "No session matches identifier \"my-session\".",
  "instance": "session/get",
  "suggestions": [
    "Run `agm session list` to see available sessions.",
    "Check if the session was archived: `agm session list --all`.",
    "Use a session name, UUID, or UUID prefix as the identifier."
  ],
  "parameters": {
    "identifier": "my-session"
  }
}
```

**Key fields for agents:**
- `code` -- stable, never changes; safe for programmatic matching
- `suggestions` -- actionable next steps the agent should try
- `parameters` -- echoes back the input that caused the error

## Output Formats

### `--output text` (default)

Human-readable tables and prose. Best for interactive terminal use.

```
$ agm session list
NAME         STATUS    BACKEND   UPDATED
my-project   active    claude    2m ago
research     stopped   gemini    1h ago
```

### `--output json` / `-o json`

Machine-readable JSON. Use this from agents, scripts, and MCP.

```
$ agm session list -o json
[{"name":"my-project","status":"active","backend":"claude",...}]
```

When `--output json` is set, errors go to stderr as RFC 7807 JSON.
Non-interactive mode is automatically enabled.

## Field Masks

Use `--fields` to request only specific fields, reducing token consumption:

```
$ agm session list -o json --fields name,status
[{"name":"my-project","status":"active"},{"name":"research","status":"stopped"}]
```

```
$ agm session get my-project -o json --fields name,uuid,status
{"name":"my-project","status":"active","uuid":"abc-123"}
```

**How it works:** `ops.ApplyFieldMask()` marshals the result to a map, then
filters to only the requested keys. Unknown fields are silently ignored.

**When to use:** Always use `--fields` from agents. A full session object
contains 20+ fields; most operations need only 2-3.

## Dry Run

Mutation commands support `--dry-run` to preview changes without executing:

```
$ agm session archive my-project --dry-run -o json
{
  "status": 200,
  "type": "dry_run",
  "code": "AGM-100",
  "title": "Dry run",
  "detail": "Would archive session \"my-project\".",
  "suggestions": ["Remove --dry-run flag to execute."]
}
```

Dry-run returns an AGM-100 `OpError` with status 200. Agents can parse
the `detail` field to confirm the intended action before re-running
without the flag.

## Progressive Disclosure (Skills)

Skills use a 3-layer documentation model to minimize token overhead:

### Layer 1: Frontmatter (always loaded)
```yaml
---
name: agm-session-list
description: List AGM sessions with optional filters
arguments:
  - name: status
    description: Filter by status (active, stopped, archived)
    required: false
---
```

### Layer 2: Skill body (loaded on invocation)
Concise usage instructions and examples. Kept under 50 lines.

### Layer 3: `--help` output (on demand)
Full CLI help text, only fetched when the agent needs detailed flag info.

This layered approach means an agent scanning available tools pays ~5 tokens
per skill (frontmatter only), not 500+ tokens for full documentation.

## OpContext

Every operation receives an `OpContext` with shared configuration:

```go
type OpContext struct {
    Storage    dolt.Storage
    Tmux       session.TmuxInterface
    Config     *config.Config
    DryRun     bool       // --dry-run flag
    Fields     []string   // field mask
    OutputMode string     // "json" or "text"
}
```

CLI, MCP, and Skills each construct an `OpContext` and pass it to the
same `ops.*` functions. This guarantees that `agm session list`,
the MCP `session/list` tool, and the `agm-session-list` skill all
return identical results with identical error handling.
