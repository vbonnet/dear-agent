# Engram Integration

AGM integrates with Engram to automatically load project context when creating new sessions.

## Usage

### Basic Usage (Default Query)
```bash
agm new --with-engram my-session
```
Queries Engram with session name `"my-session"` and injects relevant engrams as context.

### Custom Query
```bash
agm new --with-engram="JWT authentication patterns" auth-session
```
Queries Engram with custom query `"JWT authentication patterns"`.

### Without Engram (Default)
```bash
agm new my-session
```
Creates session without Engram integration (zero overhead).

## Installation

### Prerequisites
1. Install Engram:
```bash
git clone https://github.com/vbonnet/engram
cd engram/core && go install ./cmd/engram
```

2. Verify Engram is in PATH:
```bash
which engram
engram retrieve "test" --format json
```

## Configuration

Configure via environment variables:

- `AGM_ENGRAM_PATH`: Custom path to engram binary (default: searches PATH)
- `AGM_ENGRAM_LIMIT`: Maximum number of results (default: 10)
- `AGM_ENGRAM_SCORE_THRESHOLD`: Minimum relevance score 0.0-1.0 (default: 0.7)
- `AGM_ENGRAM_TIMEOUT`: Query timeout in seconds (default: 5)

Example:
```bash
export AGM_ENGRAM_PATH=/custom/path/to/engram
export AGM_ENGRAM_LIMIT=15
export AGM_ENGRAM_SCORE_THRESHOLD=0.8
export AGM_ENGRAM_TIMEOUT=10
```

## How It Works

1. AGM detects `--with-engram` flag
2. Queries Engram CLI: `engram retrieve "<query>" --format json --limit 10`
3. Filters results by relevance score (≥0.7)
4. Formats engrams as XML-style system message
5. Injects system message before first user prompt
6. Stores metadata in session manifest

## System Message Format

```markdown
<system>
The following context from Engram may be relevant:

<engram id="abc12345" score="0.92" tags="authentication,jwt">
JWT Authentication Patterns

Use JWT tokens for stateless authentication. Store in httpOnly cookies
to prevent XSS attacks. Implement token refresh flow for long-lived sessions.
... [truncated]
</engram>

Note: This context was automatically loaded. Use it as reference.
</system>
```

## Troubleshooting

### Error: Engram not found
```
Warning: Engram integration failed: engram binary not found
Continuing session creation without Engram context.

Install Engram:
  git clone https://github.com/vbonnet/engram
  cd engram/core && go install ./cmd/engram

Or set custom path:
  export AGM_ENGRAM_PATH=/path/to/engram
```

**Solution**: Install Engram or set `AGM_ENGRAM_PATH`.

### Error: Query timeout
```
Warning: Engram integration failed: engram query timed out after 5s
Continuing session creation without Engram context.
```

**Solution**: Increase timeout with `AGM_ENGRAM_TIMEOUT=10` or check Engram index health.

### No context loaded
If `--with-engram` is used but no context appears:

1. Verify Engram returns results:
   ```bash
   engram retrieve "your-query" --format json
   ```

2. Check relevance scores (must be ≥0.7 by default):
   ```bash
   engram retrieve "your-query" --format json | jq '.[].score'
   ```

3. Lower score threshold:
   ```bash
   export AGM_ENGRAM_SCORE_THRESHOLD=0.5
   agm new --with-engram my-session
   ```

## Session Metadata

Engram integration stores metadata in session manifest:

```yaml
engram_metadata:
  enabled: true
  query: "implement authentication"
  engram_ids:
    - "abc12345"
    - "def67890"
  loaded_at: 2026-01-18T03:00:00Z
  count: 2
```

View manifest:
```bash
cat ~/.agm/sessions/<session-id>/manifest.yaml
```

## Graceful Degradation

AGM continues session creation even if Engram integration fails:

- **Binary not found**: Warns user, continues without engrams
- **Query timeout**: Logs warning, continues without engrams
- **Parse error**: Logs error, continues without engrams
- **Zero results**: Silent (no engrams found, not an error)

Session creation never blocked by Engram errors.

## Performance

- Typical query time: 120-150ms
- Timeout protection: 5 seconds maximum
- Zero overhead without `--with-engram` flag
- Subprocess overhead acceptable for session creation use case

## Future Enhancements (V2)

- MCP server integration (in-session context updates)
- Go package import (2-5ms queries vs 50-200ms subprocess)
- Automatic query on session resume
- Interactive engram selection UI
