# Engram MCP Server API Documentation

Complete API reference for the Engram MCP Server (Tasks 3.2 + 3.5).

## Overview

The Engram MCP Server provides programmatic access to Engram's memory retrieval system, beads management, plugin registry, and Wayfinder workflow tracking through the Model Context Protocol (MCP).

**Server Info**:
- **Name**: `engram-mcp-server`
- **Version**: 1.1.0 (Task 3.2 = 1.0, Task 3.5 = 1.1)
- **Protocol**: MCP 0.1.0 (JSON-RPC over stdio)
- **Performance Target**: <100ms per tool invocation

## Tools

### 1. engram_retrieve (Task 3.2 + Task 3.5 Enhanced)

Retrieve relevant engrams using advanced semantic search with embedding-based similarity ranking.

**Enhancements (Task 3.5)**:
- Embedding-based semantic search (sentence-transformers)
- Top-k retrieval with relevance scoring
- Cached embeddings for performance (<50ms target)

#### Request Schema

```typescript
{
  name: "engram.retrieve",
  arguments: {
    query: string,      // Required: Search query
    tag?: string,       // Optional: Filter by tag (e.g., "go", "python")
    limit?: number      // Optional: Max results (default: 5)
  }
}
```

#### Response

Returns text content containing retrieved engrams.

```typescript
{
  content: [
    {
      type: "text",
      text: string    // Retrieved engram content
    }
  ]
}
```

#### Examples

**Basic retrieval:**
```json
{
  "name": "engram.retrieve",
  "arguments": {
    "query": "error handling patterns"
  }
}
```

**Filtered by tag:**
```json
{
  "name": "engram.retrieve",
  "arguments": {
    "query": "error handling",
    "tag": "go",
    "limit": 3
  }
}
```

#### CLI Mapping

Internally calls:
```bash
engram retrieve "<query>" [--tag <tag>] [--limit <limit>]
```

#### Error Handling

- Returns error message if engram CLI fails
- Returns "No results found" if query matches nothing
- Timeout: 30 seconds

---

### 2. engram.plugins.list

List all installed engram plugins with their metadata.

#### Request Schema

```typescript
{
  name: "engram.plugins.list",
  arguments: {}
}
```

#### Response

Returns formatted text listing plugins.

```typescript
{
  content: [
    {
      type: "text",
      text: string    // Formatted plugin list
    }
  ]
}
```

**Format:**
```
Available Engram Plugins:

**plugin-name** (plugin-type)
  Plugin description
  Location: core|user

**another-plugin** (connector)
  Another description
  Location: user
```

#### Examples

```json
{
  "name": "engram.plugins.list",
  "arguments": {}
}
```

#### Implementation Details

Scans directories:
- `~/.engram/core/plugins/`
- `~/.engram/user/plugins/`

For each plugin directory with `plugin.yaml`:
- Parses name, type, description
- Tracks location (core vs user)

#### Error Handling

- Returns "No plugins found" if no plugins exist
- Skips malformed plugin.yaml files
- Continues if directories are not readable

---

### 3. wayfinder.phase.status

Get current Wayfinder phase status for a project directory.

#### Request Schema

```typescript
{
  name: "wayfinder.phase.status",
  arguments: {
    project: string    // Required: Project directory path
  }
}
```

#### Response

Returns JSON with phase information.

```typescript
{
  content: [
    {
      type: "text",
      text: string    // JSON-formatted phase status
    }
  ]
}
```

**JSON Structure:**
```json
{
  "project": "/absolute/path/to/project",
  "phase": "S6: Design",
  "progress": "60%",
  "status": "In Progress"
}
```

#### Examples

```json
{
  "name": "wayfinder.phase.status",
  "arguments": {
    "project": "the git history"
  }
}
```

**Relative path support:**
```json
{
  "name": "wayfinder.phase.status",
  "arguments": {
    "project": "./my-project"
  }
}
```

#### Implementation Details

Reads `WAYFINDER-STATUS.md` from project directory:

Expected format:
```markdown
Current Phase: **S6: Design**
Progress: 60%
Status: In Progress
```

Parses with regex:
- `Current Phase: **<phase>**`
- `Progress: <progress>`
- `Status: <status>`

#### Error Handling

- Returns error if `WAYFINDER-STATUS.md` not found
- Returns "Unknown" for unparseable fields
- Resolves relative paths to absolute

---

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ENGRAM_ROOT` | Engram workspace root | `~/.engram` |
| `ENGRAM_CLI` | Path to engram CLI | `engram` |

### Claude Code Registration

Add to `~/.claude/settings.json`:

```json
{
  "mcpServers": {
    "engram": {
      "command": "node",
      "args": [
        "engram/mcp-server/dist/index.js"
      ],
      "env": {
        "ENGRAM_ROOT": "~/.engram",
        "ENGRAM_CLI": "/usr/local/bin/engram"
      }
    }
  }
}
```

---

## Performance

| Operation | Typical Latency | Max Latency |
|-----------|----------------|-------------|
| engram.retrieve | <1s | 30s (timeout) |
| engram.plugins.list | <100ms | N/A |
| wayfinder.phase.status | <50ms | N/A |

**Buffer Limits:**
- Output buffer: 10MB
- Execution timeout: 30s

---

## Error Response Format

All tools return errors in this format:

```typescript
{
  content: [
    {
      type: "text",
      text: "Error: <error message>"
    }
  ],
  isError: true
}
```

---

## Version History

### 0.1.0 (2026-02-19)

Initial release:
- ✅ engram.retrieve tool
- ✅ engram.plugins.list tool
- ✅ wayfinder.phase.status tool
- ✅ Basic error handling
- ✅ CLI integration
- ✅ MCP stdio transport

---

## Future Enhancements

Potential additions (not in 0.1.0):

- [ ] engram.store - Store new engrams
- [ ] engram.plugins.install - Install plugins
- [ ] wayfinder.phase.complete - Mark phase complete
- [ ] beads.* tools - Beads integration
- [ ] Caching layer - Reduce CLI calls
- [ ] Health check endpoint
- [ ] Metrics/telemetry

---

## Support

**Issues:** https://github.com/vbonnet/engram/issues
**Documentation:** https://github.com/vbonnet/engram
**License:** Apache 2.0
