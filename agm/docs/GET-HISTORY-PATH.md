# AGM Conversation History Retrieval

Complete guide to retrieving conversation history file paths across all CLI harnesses.

## Overview

The `agm session get-history-path` command returns file paths to conversation logs for AGM sessions. It supports all major CLI harnesses (Claude Code, Gemini CLI, OpenCode, Codex) and automatically constructs correct paths based on the harness type.

**Key Features**:
- Auto-detects harness type from session metadata
- Constructs paths using harness-specific algorithms
- Supports JSON output for scripting
- Verifies file existence with `--verify` flag
- Works with current session or named sessions

## Quick Start

```bash
# Get history for current session (auto-detect)
agm session get-history-path

# Get history for specific session
agm session get-history-path my-session

# Get JSON output
agm session get-history-path --json

# Verify files exist
agm session get-history-path --verify
```

## Architecture

### Harness vs Model Distinction

**Critical concept**: The CLI harness determines storage location, NOT the LLM model.

- **Claude Code harness** → `~/.claude/projects/` (even when using Gemini via MCP)
- **Gemini CLI harness** → `~/.gemini/tmp/` (regardless of model)
- **OpenCode harness** → `~/.local/share/opencode/` (model-agnostic, supports Claude/Gemini/GPT)
- **Codex harness** → `~/.codex/sessions/` (regardless of model)

AGM's `agent` field tracks the harness (despite the misleading name).

### Path Construction Algorithms

Each harness uses different storage patterns:

#### Claude Code
- **Algorithm**: Dash-substitution encoding of working directory
- **Path**: `~/.claude/projects/<encoded-cwd>/<uuid>.jsonl`
- **Encoding**: Replace all non-alphanumeric chars with `-`
- **Example**: `~/src` → `-home-user-src`

#### Gemini CLI
- **Algorithm**: SHA256 hash of working directory
- **Path**: `~/.gemini/tmp/<hash>/chats/`
- **Hash**: First 16 chars of SHA256 hex digest
- **Example**: `~/src` → `a1b2c3d4e5f6g7h8`

#### OpenCode
- **Algorithm**: UUID-based directory structure
- **Path**: `${OPENCODE_DATA_DIR}/storage/message/<uuid>/`
- **Default**: `~/.local/share/opencode/storage/message/<uuid>/`
- **Customization**: Set `OPENCODE_DATA_DIR` environment variable

#### Codex
- **Algorithm**: Date extraction from UUID
- **Path**: `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`
- **Pattern**: UUID contains date in rollout format
- **Example**: `rollout-20260318-abc123` → `2026/03/18/`

## Command Reference

### Synopsis

```
agm session get-history-path [session-name] [flags]
```

### Arguments

| Argument | Description | Required |
|----------|-------------|----------|
| `session-name` | Session name, ID, or tmux session name | No (auto-detects) |

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--json` | Output in JSON format | `false` |
| `--verify` | Check if files exist on filesystem | `false` |
| `--help` | Show command help | - |

### Global Flags

Inherits all `agm` global flags (see `agm --help`).

## Output Formats

### Human-Readable (default)

```
Session: my-session
Agent:   claude
UUID:    54790b4a-5342-4a60-a25f-5b260e319b5a

Conversation History:
  ~/.claude/projects/-home-user-src/54790b4a.jsonl
  ~/.claude/projects/-home-user-src/sessions-index.json

Metadata:
  Working Directory: ~/src
  Encoding Method:   dash-substitution
```

### JSON Format (`--json`)

```json
{
  "session_name": "my-session",
  "session_id": "1b819dd2-d41e-47b3-9dae-d7d5188abed9",
  "agent": "claude",
  "uuid": "54790b4a-5342-4a60-a25f-5b260e319b5a",
  "paths": [
    "~/.claude/projects/-home-user-src/54790b4a.jsonl",
    "~/.claude/projects/-home-user-src/sessions-index.json"
  ],
  "exists": true,
  "metadata": {
    "encoded_directory": "-home-user-src",
    "encoding_method": "dash-substitution",
    "harness": "claude",
    "working_directory": "~/src"
  }
}
```

### JSON Schema

```json
{
  "session_name": "string (optional)",
  "session_id": "string (optional)",
  "agent": "string (claude|gemini|opencode|codex)",
  "uuid": "string (RFC 4122 format)",
  "paths": ["string"],
  "exists": "boolean",
  "metadata": {
    "working_directory": "string (optional)",
    "encoding_method": "string (optional)",
    "harness": "string"
  },
  "error": {
    "code": "string",
    "message": "string",
    "suggestion": "string (optional)"
  }
}
```

## Usage Examples

### Basic Usage

```bash
# Current session (auto-detect from tmux)
agm session get-history-path

# Specific session by name
agm session get-history-path my-project

# By tmux session name
agm session get-history-path claude-my-project

# By session ID
agm session get-history-path 1b819dd2-d41e-47b3-9dae-d7d5188abed9
```

### Scripting with JSON

```bash
# Extract first history file path
HISTORY_PATH=$(agm session get-history-path --json | jq -r '.paths[0]')

# Check if files exist
EXISTS=$(agm session get-history-path --json | jq -r '.exists')

# Get harness type
HARNESS=$(agm session get-history-path --json | jq -r '.agent')

# Verify paths before processing
if agm session get-history-path --verify --json | jq -e '.exists' > /dev/null; then
  echo "History files exist"
else
  echo "History files missing"
fi
```

### Integration with Other Tools

```bash
# Read conversation history
HISTORY=$(agm session get-history-path --json | jq -r '.paths[0]')
cat "$HISTORY"

# Count messages
HISTORY=$(agm session get-history-path --json | jq -r '.paths[0]')
grep -c '"type":"message"' "$HISTORY"

# Backup conversation
HISTORY=$(agm session get-history-path --json | jq -r '.paths[0]')
cp "$HISTORY" ~/backups/

# Search conversation content
HISTORY=$(agm session get-history-path --json | jq -r '.paths[0]')
jq -r '.content' "$HISTORY" | grep "search term"
```

## Error Handling

### Common Errors

#### Session Not Found

```
Error: failed to discover UUID for session 'nonexistent': UUID discovery failed
```

**Cause**: Session doesn't exist in AGM database or history
**Solution**: Check session name with `agm session list`

#### Working Directory Missing

```json
{
  "error": {
    "code": "WORKING_DIR_REQUIRED",
    "message": "working directory required for Claude Code paths",
    "suggestion": "Ensure session manifest has context.project field"
  }
}
```

**Cause**: Session metadata missing working directory
**Solution**: Update session manifest or re-create session

#### Unknown Agent Type

```json
{
  "error": {
    "code": "UNKNOWN_AGENT",
    "message": "unknown agent type: custom",
    "suggestion": "Supported agents: claude, gemini, opencode, codex"
  }
}
```

**Cause**: Unsupported harness type
**Solution**: Use one of the supported harnesses

### Error Codes

| Code | Description | Severity |
|------|-------------|----------|
| `WORKING_DIR_REQUIRED` | Working directory missing from metadata | Error |
| `UNKNOWN_AGENT` | Unsupported harness type | Error |
| `UUID_REQUIRED` | UUID missing from request | Error |
| `AGENT_REQUIRED` | Agent/harness type missing | Error |
| `UUID_PARSE_ERROR` | Invalid UUID format for Codex | Warning |

## Multi-Harness Support

### Using with Different Harnesses

The command works transparently across all harnesses:

```bash
# Claude Code session
agm session get-history-path claude-project
# Returns: ~/.claude/projects/-home-user-workspace/uuid.jsonl

# Gemini CLI session
agm session get-history-path gemini-project
# Returns: ~/.gemini/tmp/a1b2c3d4/chats/

# OpenCode session
agm session get-history-path opencode-project
# Returns: ~/.local/share/opencode/storage/message/uuid/

# Codex session
agm session get-history-path codex-project
# Returns: ~/.codex/sessions/2026/03/18/rollout-*.jsonl
```

### Environment Variables

#### OpenCode Custom Data Directory

```bash
# Set custom OpenCode storage location
export OPENCODE_DATA_DIR=/custom/path
agm session get-history-path opencode-session
# Returns: /custom/path/storage/message/uuid/
```

## Troubleshooting

### Files Don't Exist

```bash
# Check if paths are correct but files are missing
agm session get-history-path --verify --json
```

If `exists: false`, check:
1. Session is active (not archived too early)
2. Harness has written conversation logs
3. Storage directory permissions are correct

### Wrong Paths Returned

Verify session metadata:
```bash
# Check agent field
agm session list --format json | jq '.[] | {name, agent, uuid}'

# Verify working directory
agm session list --format json | jq '.[] | {name, project: .context.project}'
```

### Permission Denied

Ensure you have read access to harness storage directories:
```bash
ls -la ~/.claude/projects/
ls -la ~/.gemini/tmp/
ls -la ~/.local/share/opencode/storage/
ls -la ~/.codex/sessions/
```

## Performance Considerations

- **Database Query**: O(1) session lookup by ID
- **Path Construction**: O(1) for all harnesses except Codex
- **File Verification**: O(n) where n = number of paths (with `--verify`)
- **No I/O without `--verify`**: Paths constructed algorithmically

## Related Commands

| Command | Description |
|---------|-------------|
| `agm session list` | List all AGM sessions |
| `agm session get-uuid` | Get UUID for a session |
| `agm session associate` | Associate current session with AGM |
| `agm get-session-name` | Get AGM session name for context |

## Development

### Adding New Harness Support

To add support for a new harness:

1. Add agent type to manifest schema
2. Implement path construction in `internal/history/paths.go`:
   ```go
   func getNewHarnessPaths(uuid, workingDir string, verify bool) (*HistoryLocation, error) {
       // Implement path construction algorithm
   }
   ```
3. Add case to `GetHistoryPaths()` switch
4. Write unit tests in `paths_test.go`
5. Update documentation

### Testing

```bash
# Unit tests
go test ./internal/history/... -v

# Coverage
go test -coverprofile=coverage.out ./internal/history/...
go tool cover -html=coverage.out

# Race detector
go test -race ./internal/history/...

# Integration tests
go test ./test/integration/history_paths_test.go -v
```

## FAQ

### Q: Does this work with archived sessions?
**A**: Yes, as long as the session metadata (agent, UUID, working directory) is in the database.

### Q: Can I use this to find conversation logs from other tools?
**A**: Yes! Works with any harness that AGM tracks (Claude Code, Gemini CLI, OpenCode, Codex).

### Q: Why does `agent` field show "claude" for OpenCode sessions?
**A**: This is a known misnaming. The field tracks the **harness** not the model. OpenCode sessions will show `agent: "opencode"`.

### Q: What if my working directory moved?
**A**: Update the session manifest's `context.project` field to the new directory, then run the command again.

### Q: Can I get history for sessions not in AGM?
**A**: No, the session must be registered with AGM (have a manifest and UUID).

## Support

### Reporting Issues
Include in your report:
- AGM version: `agm version`
- Command run: Full command with flags
- Error output: Complete error message
- Session info: `agm session list | grep <session>`
- Harness type: Value of `agent` field

### Getting Help
1. Check this documentation
2. Review error codes table above
3. File issue at https://github.com/vbonnet/ai-tools/issues
4. Open a GitHub issue

---

**Last Updated**: 2026-03-18
**Version**: 1.0.0
**Command**: `agm session get-history-path`
