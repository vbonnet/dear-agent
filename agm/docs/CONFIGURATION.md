# AGM Configuration Guide

Complete reference for configuring AGM (AI Agent Session Manager).

## Table of Contents

- [Overview](#overview)
- [Configuration Precedence](#configuration-precedence)
- [Environment Variables](#environment-variables)
- [Config File](#config-file)
- [Per-Session Configuration](#per-session-configuration)
- [Backend Configuration](#backend-configuration)
- [Database Configuration](#database-configuration)
- [Event Bus Configuration](#event-bus-configuration)
- [Engram Integration](#engram-integration)
- [Agent Selection Configuration](#agent-selection-configuration)
- [MCP Server Configuration](#mcp-server-configuration)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## Overview

AGM can be configured through multiple mechanisms with clear precedence rules. Configuration covers:

- **Session behavior**: Defaults, thresholds, auto-detection
- **UI preferences**: Themes, colors, accessibility
- **Backend selection**: tmux (default) or Temporal workflows
- **Database**: SQLite storage, FTS5 search indexing
- **Event bus**: WebSocket server for real-time updates
- **Integrations**: Engram semantic memory, MCP servers
- **Advanced settings**: Timeouts, locks, health checks

## Configuration Precedence

Configuration is loaded in the following order (later sources override earlier ones):

1. **Hardcoded defaults** (in source code)
2. **Config file** (`~/.config/agm/config.yaml`)
3. **Environment variables** (e.g., `AGM_LOG_LEVEL`)
4. **Command-line flags** (e.g., `--debug`)

Example:
```bash
# Config file sets log_level: info
# Environment variable overrides it
export AGM_LOG_LEVEL=debug

# Command-line flag takes final precedence
agm list --log-level warn
```

## Environment Variables

### Core Settings

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `AGM_CONFIG` | string | `~/.config/agm/config.yaml` | Custom config file path |
| `AGM_SESSIONS_DIR` | string | `~/.config/agm/sessions` | Directory for session manifests |
| `AGM_LOG_LEVEL` | string | `info` | Logging level: `debug`, `info`, `warn`, `error` |
| `AGM_LOG_FILE` | string | _(empty)_ | Path to log file (if set, logs to file instead of stderr) |
| `AGM_DEBUG` | bool | `false` | Enable debug mode (`true`, `1`, or `false`) |
| `AGM_STATE_DIR` | string | `~/.agm` | State directory for locks and readiness files |

### AI Agent API Keys

These environment variables are required for using different AI agents:

| Variable | Type | Required For | Description |
|----------|------|--------------|-------------|
| `ANTHROPIC_API_KEY` | string | Claude | Anthropic API key for Claude sessions |
| `GOOGLE_API_KEY` | string | Gemini | Google API key for Gemini sessions |
| `GEMINI_API_KEY` | string | Gemini | Alternative to `GOOGLE_API_KEY` |
| `OPENAI_API_KEY` | string | GPT | OpenAI API key for GPT sessions |
| `GOOGLE_CLOUD_PROJECT` | string | Gemini (Vertex) | GCP project ID for Vertex AI |
| `GCP_PROJECT` | string | Gemini (Vertex) | Alternative to `GOOGLE_CLOUD_PROJECT` |
| `CLOUD_ML_REGION` | string | Gemini (Vertex) | GCP region for Vertex AI |
| `GOOGLE_APPLICATION_CREDENTIALS` | string | Gemini (Vertex) | Path to GCP service account JSON |
| `CLAUDECODE` | bool | Claude Code | Indicates running in Claude Code environment |

**Security Note:** Never store API keys in `config.yaml`. Use environment variables or secure secret management.

**Setup Example:**
```bash
# For Claude sessions
export ANTHROPIC_API_KEY=sk-ant-...

# For Gemini sessions (API)
export GOOGLE_API_KEY=AIza...

# For Gemini sessions (Vertex AI)
export GOOGLE_CLOUD_PROJECT=my-project
export GOOGLE_APPLICATION_CREDENTIALS=~/gcp-key.json
export CLOUD_ML_REGION=us-central1

# For GPT sessions
export OPENAI_API_KEY=sk-...
```

### Backend Selection

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `AGM_SESSION_BACKEND` | string | `tmux` | Session backend: `tmux` or `temporal` |

### Tmux Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `AGM_TMUX_SOCKET` | string | _(auto)_ | Custom tmux socket path (for test isolation) |

### Event Bus (WebSocket)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `AGM_EVENTBUS_PORT` | int | `8080` | WebSocket server port for event bus |
| `AGM_EVENTBUS_MAX_CLIENTS` | int | `100` | Maximum concurrent WebSocket clients |

### Engram Integration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `AGM_ENGRAM_PATH` | string | `engram` | Path to engram binary |
| `AGM_ENGRAM_LIMIT` | int | `10` | Maximum search results |
| `AGM_ENGRAM_SCORE_THRESHOLD` | float | `0.7` | Minimum relevance score (0.0-1.0) |
| `AGM_ENGRAM_TIMEOUT` | int | `5` | Query timeout in seconds |

### MCP Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `AGM_MCP_SERVERS` | string | _(empty)_ | MCP servers in format: `name1=url1,name2=url2` |

### Accessibility

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `AGM_SCREEN_READER` | bool | `false` | Enable screen reader mode (text symbols instead of Unicode) |
| `NO_COLOR` | bool | `false` | Disable all color output (standard env var) |

### Testing & Development

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `AGM_MCP_DEBUG` | bool | `false` | Enable MCP server debug logging |
| `AGM_TEST_TMUX` | bool | `false` | Enable tmux tests in CI (normally skipped) |

## Config File

### Location

**Default path:** `~/.config/agm/config.yaml`

**Custom path via flag:**
```bash
agm list --config ~/custom/config.yaml
```

**Custom path via environment variable:**
```bash
export AGM_CONFIG=~/custom/config.yaml
agm list
```

### Full Configuration Example

```yaml
# ~/.config/agm/config.yaml

# === Default Behavior ===
defaults:
  interactive: true                # Enable interactive prompts
  auto_associate_uuid: true        # Auto-detect Claude UUIDs
  confirm_destructive: true        # Confirm before delete/archive
  cleanup_threshold_days: 30       # Stopped → archive threshold (days)
  archive_threshold_days: 90       # Archived → delete threshold (days)

# === UI Preferences ===
ui:
  theme: "agm"                     # Theme: agm, agm-light, dracula, catppuccin, charm, base
  picker_height: 15                # Session picker height (lines)
  show_project_paths: true         # Show full project paths in lists
  show_tags: true                  # Show session tags
  fuzzy_search: true               # Enable fuzzy matching in pickers
  no_color: false                  # Disable colored output (accessibility)
  screen_reader: false             # Use text symbols instead of Unicode

# === Advanced Settings ===
advanced:
  tmux_timeout: "5s"               # Tmux command timeout
  health_check_cache: "5s"         # Health check cache duration
  lock_timeout: "30s"              # Lock acquisition timeout
  uuid_detection_window: "5m"      # UUID detection time window

# === Core Configuration ===
sessions_dir: "~/.config/agm/sessions"  # Session manifests directory
log_level: "info"                       # Logging: debug, info, warn, error
log_file: ""                            # Log file path (empty = stderr)

# === Resilience Features ===
timeout:
  enabled: true                    # Enable timeout protection
  tmux_commands: "5s"              # Tmux command timeout

lock:
  enabled: true                    # Enable file-based locking
  path: "/tmp/agm-{UID}/agm.lock"  # Lock file path ({UID} auto-replaced)

health_check:
  enabled: true                    # Enable health checks
  cache_duration: "5s"             # Cache duration
  probe_timeout: "2s"              # Probe timeout
```

### Minimal Configuration Example

```yaml
# ~/.config/agm/config.yaml
# Minimal config - rely on defaults

defaults:
  interactive: true
  cleanup_threshold_days: 60      # Longer retention

ui:
  theme: "agm"
  picker_height: 20               # Taller picker
```

### Theme Examples

#### High Contrast Dark (Default)
```yaml
ui:
  theme: "agm"
```
- WCAG AA compliant contrast ratios (4.5:1 minimum)
- Optimized for dark terminals
- Semantic colors: green=success, red=error, yellow=warning

#### High Contrast Light
```yaml
ui:
  theme: "agm-light"
```
- WCAG AA compliant for light terminals
- High contrast on white/light backgrounds

#### Popular Community Themes
```yaml
ui:
  theme: "dracula"     # Dracula color scheme
  # theme: "catppuccin" # Catppuccin color scheme
  # theme: "charm"      # Charm Bracelet colors
  # theme: "base"       # Minimal base16 colors
```

### Accessibility Configuration

```yaml
# Screen reader optimized
ui:
  no_color: true          # Disable colors
  screen_reader: true     # Use text symbols (>, +, -, *) instead of Unicode
  theme: "agm"            # High contrast theme (color-blind safe)
```

Or via environment variables:
```bash
export NO_COLOR=1
export AGM_SCREEN_READER=1
```

## Per-Session Configuration

Session-specific settings are stored in the session manifest: `~/.config/agm/sessions/{session-name}/manifest.yaml`

### Session Manifest Structure

```yaml
schema_version: "2.0"
session_id: "s-abc123"
name: "my-project"
created_at: 2026-02-15T10:30:00Z
updated_at: 2026-02-15T15:45:00Z
lifecycle: ""                    # "" (active/stopped) or "archived"

# Context metadata
context:
  project: "~/projects/my-app"
  purpose: "Implement authentication"
  tags:
    - feature-dev
    - auth
  notes: "Working on OAuth integration"

# Claude session
claude:
  uuid: "01234567-89ab-cdef-0123-456789abcdef"

# Tmux session
tmux:
  session_name: "my-project"

# AI agent selection
agent: "claude"                  # claude, gemini, or gpt

# Engram integration (optional)
engram_metadata:
  enabled: true
  query: "authentication patterns"
  engram_ids:
    - "engram-001"
    - "engram-002"
  loaded_at: 2026-02-15T10:30:00Z
  count: 2
```

### Overriding Settings Per Session

While global settings come from `config.yaml`, per-session behavior is controlled by:

1. **Session metadata** (tags, purpose, notes)
2. **Agent selection** (claude vs gemini vs gpt)
3. **Engram integration** (enabled per session)
4. **Lifecycle state** (active, archived)

Example: Create a session with specific metadata
```bash
agm new my-session \
  --harness gemini-cli \
  --project ~/code/my-app \
  --purpose "Refactor authentication module" \
  --tags feature,security
```

## Backend Configuration

AGM supports multiple session backends via the `AGM_SESSION_BACKEND` environment variable.

### Tmux Backend (Default)

```bash
# Use tmux backend (default)
export AGM_SESSION_BACKEND=tmux
agm new my-session
```

Configuration:
- No additional config needed
- Automatically manages tmux sessions
- Works locally and via SSH

Custom tmux socket (for testing):
```bash
export AGM_TMUX_SOCKET=/tmp/test-tmux.sock
```

### Temporal Backend (Experimental)

```bash
# Use Temporal workflow backend
export AGM_SESSION_BACKEND=temporal
agm new my-session
```

**Note:** Temporal backend is a stub implementation. Full Temporal integration is planned for future releases.

When implemented, it will support:
- Distributed session management
- Workflow-based session lifecycle
- Cloud-native deployments
- Multi-node coordination

## Database Configuration

AGM uses SQLite with FTS5 (Full-Text Search) for session storage and search.

### Database Location

**Default path:** `~/.config/agm/agm.db`

The database is automatically created on first use with the following schema:
- **sessions** table: Session metadata
- **session_fts** table: FTS5 index for full-text search
- **hierarchy** tables: Parent-child session relationships

### SQLite Features

#### FTS5 Full-Text Search
```bash
# Search across all session content
agm search "authentication implementation"

# Searches through:
# - Session names
# - Project paths
# - Purpose descriptions
# - Tags
# - Notes
# - Claude conversation history
```

#### Search Configuration

Search behavior is controlled by the `SearchOptions` in code:

```go
type SearchOptions struct {
    Limit  int    // Max results (default: 50)
    Offset int    // Pagination offset
    Filter Filter // Additional constraints
}

type Filter struct {
    Lifecycle      string    // "", "archived"
    Agent          string    // "claude", "gemini", "gpt"
    CreatedAfter   time.Time
    CreatedBefore  time.Time
    ParentSession  string    // Parent session ID
    HasEscalations bool
}
```

#### Database Maintenance

The database is automatically maintained, but you can perform operations:

```bash
# Migrate YAML manifests to SQLite
agm migrate

# Sync manifests to database
agm sync

# Cleanup old sessions
agm clean
```

### Database Customization

While the database path is not currently configurable via environment variable, you can:

1. **Use custom sessions directory:**
   ```bash
   export AGM_SESSIONS_DIR=~/custom/sessions
   ```
   Database will be at `~/custom/sessions/../agm.db`

2. **Backup database:**
   ```bash
   cp ~/.config/agm/agm.db ~/.config/agm/agm.db.backup
   ```

3. **Reset database:**
   ```bash
   rm ~/.config/agm/agm.db
   agm sync  # Rebuild from YAML manifests
   ```

## Event Bus Configuration

AGM includes a WebSocket-based event bus for real-time session updates.

### WebSocket Server

The event bus enables:
- Real-time session status updates
- Multi-client synchronization
- TUI (Terminal UI) live updates
- External integrations

### Configuration

```bash
# Set WebSocket port
export AGM_EVENTBUS_PORT=9090

# Set max concurrent clients
export AGM_EVENTBUS_MAX_CLIENTS=50
```

### Defaults

| Setting | Default | Description |
|---------|---------|-------------|
| Port | `8080` | WebSocket server port |
| Max Clients | `100` | Maximum concurrent connections |
| Broadcast Buffer | `256` | Event queue size |

### Event Types

The event bus broadcasts:
- `session.created`
- `session.started`
- `session.stopped`
- `session.archived`
- `session.deleted`
- `session.resumed`

### Connecting Clients

WebSocket endpoint: `ws://localhost:8080/events`

Example client (JavaScript):
```javascript
const ws = new WebSocket('ws://localhost:8080/events');

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Event:', data.type, data.session);
};
```

## Engram Integration

Engram provides semantic memory and context retrieval for AI sessions.

### Configuration

```bash
# Path to engram binary
export AGM_ENGRAM_PATH=/usr/local/bin/engram

# Search result limit
export AGM_ENGRAM_LIMIT=20

# Minimum relevance score (0.0-1.0)
export AGM_ENGRAM_SCORE_THRESHOLD=0.8

# Query timeout (seconds)
export AGM_ENGRAM_TIMEOUT=10
```

### Config File

```yaml
# Via config.yaml (future enhancement)
engram:
  enabled: true
  binary_path: "/usr/local/bin/engram"
  limit: 20
  score_threshold: 0.8
  timeout: 10s
```

### Per-Session Engram

Enable Engram for specific sessions:

```bash
# Create session with Engram context
agm new my-session --engram-query "authentication patterns"

# Query is stored in manifest:
# engram_metadata:
#   enabled: true
#   query: "authentication patterns"
#   engram_ids: [...]
```

## Agent Selection Configuration

AGM supports multiple AI agents (Claude, Gemini, OpenAI/GPT) with automatic routing based on keywords.

### AGENTS.md File

Agent preferences are configured via `AGENTS.md` (YAML format with markdown extension).

**Detection order (first found wins):**
1. `./AGENTS.md` (local project)
2. `~/.config/agm/AGENTS.md` (global config)
3. No file → Use default (Claude)

### AGENTS.md Schema

```yaml
# ~/.config/agm/AGENTS.md

schema_version: "1.0"
default_agent: "claude"        # Fallback agent: claude, gemini, openai, or gpt

# Keyword-based routing rules
preferences:
  - keywords:
      - "gemini"
      - "google"
      - "vertex"
    agent: "gemini"

  - keywords:
      - "gpt"
      - "openai"
      - "chatgpt"
    agent: "openai"            # Use OpenAI for GPT models

  - keywords:
      - "research"
      - "data"
      - "analyze"
    agent: "openai"            # Use OpenAI for research/data tasks
```

### How It Works

When creating a session, AGM checks the session name against keywords:

```bash
# Uses Gemini (matches "gemini" keyword)
agm new gemini-test-session

# Uses Claude (matches "research" keyword in preferences)
agm new research-project

# Uses default (Claude) - no keyword match
agm new my-app
```

**Precedence:**
1. Explicit `--harness` flag (highest priority)
2. Keyword match in `AGENTS.md`
3. `default_agent` in `AGENTS.md`
4. System default (Claude)

### Validation

AGM validates `AGENTS.md` on load:
- Invalid YAML → Warning + use defaults
- Missing `default_agent` → Use Claude
- Empty keywords → Skip preference
- Invalid preferences → Warn + skip

## MCP Server Configuration

MCP (Model Context Protocol) servers provide additional tools and context to AI agents.

### Configuration Methods

#### 1. Environment Variable

```bash
export AGM_MCP_SERVERS="google-docs=http://localhost:3000,github=http://localhost:3001"
```

Format: `name1=url1,name2=url2`

#### 2. YAML File

Create `~/.config/agm/mcp.yaml`:

```yaml
mcp_servers:
  - name: google-docs
    url: http://localhost:3000
    type: mcp
  - name: github
    url: http://localhost:3001
    type: mcp
  - name: filesystem
    url: http://localhost:3002
    type: mcp
```

### AGM MCP Server Configuration

The bundled `agm-mcp-server` can be configured at `~/.config/agm/mcp-server.yaml`:

```yaml
mcp_server:
  enabled: true
  transport: stdio                 # stdio (default) or http
  auto_register: true              # Auto-register with Claude Code
  claude_config_path: ~/.config/claude/mcp_servers.json
  sessions_dir: ~/.config/agm/sessions

  # Available tools
  tools:
    - agm_list_sessions
    - agm_search_sessions
    - agm_get_session_metadata
```

## Best Practices

### Production Environment

```yaml
# Production config
defaults:
  interactive: false               # Non-interactive for automation
  confirm_destructive: true        # Always confirm destructive ops
  cleanup_threshold_days: 90       # Longer retention
  archive_threshold_days: 180

advanced:
  tmux_timeout: "10s"              # Longer timeout for slow systems
  lock_timeout: "60s"              # Longer lock timeout

log_level: "warn"                  # Less verbose logging
log_file: "/var/log/agm/agm.log"  # Log to file
```

### Development Environment

```yaml
# Development config
defaults:
  interactive: true
  cleanup_threshold_days: 7        # Shorter retention for testing

ui:
  theme: "dracula"                 # Personal preference
  picker_height: 25                # More visible

advanced:
  tmux_timeout: "30s"              # Generous timeout for debugging

log_level: "debug"                 # Verbose logging
```

### Security Considerations

1. **File Permissions**
   ```bash
   chmod 600 ~/.config/agm/config.yaml
   chmod 700 ~/.config/agm/sessions
   ```

2. **Secrets Management**
   - Never store API keys in `config.yaml`
   - Use environment variables or secure vaults
   - Git-ignore `~/.config/agm/`

3. **Multi-User Systems**
   ```yaml
   # Use user-specific directories
   sessions_dir: "/home/${USER}/.config/agm/sessions"

   lock:
     path: "/tmp/agm-${UID}/agm.lock"  # UID auto-replaced
   ```

### Performance Tuning

1. **Database Optimization**
   ```bash
   # Periodic vacuum
   sqlite3 ~/.config/agm/agm.db "VACUUM;"

   # Rebuild FTS5 index
   agm sync --force
   ```

2. **Event Bus Tuning**
   ```bash
   # Reduce max clients for resource-constrained systems
   export AGM_EVENTBUS_MAX_CLIENTS=25
   ```

3. **Engram Performance**
   ```bash
   # Lower timeout for faster responses (may miss results)
   export AGM_ENGRAM_TIMEOUT=3

   # Reduce result limit
   export AGM_ENGRAM_LIMIT=5
   ```

### High Availability

For multi-node deployments (future):

```yaml
# Temporal backend configuration (planned)
backend:
  type: temporal
  temporal:
    host: "temporal.example.com:7233"
    namespace: "agm-production"
    task_queue: "agm-sessions"
```

## Troubleshooting

### Config Not Loading

**Problem:** Changes to `config.yaml` not taking effect

**Solutions:**
1. Verify file location:
   ```bash
   ls -la ~/.config/agm/config.yaml
   ```

2. Check YAML syntax:
   ```bash
   # Validate YAML
   python3 -c "import yaml; yaml.safe_load(open('~/.config/agm/config.yaml'))"
   ```

3. Check precedence (env vars override config file):
   ```bash
   env | grep AGM_
   ```

### Permission Errors

**Problem:** Cannot create sessions or write config

**Solutions:**
```bash
# Fix permissions
chmod 700 ~/.config/agm
chmod 600 ~/.config/agm/config.yaml
chmod 700 ~/.config/agm/sessions

# Check ownership
ls -la ~/.config/agm
```

### Tmux Socket Issues

**Problem:** Cannot connect to tmux sessions

**Solutions:**
```bash
# Check socket path
echo $AGM_TMUX_SOCKET

# Verify tmux is running
tmux list-sessions

# Reset socket
unset AGM_TMUX_SOCKET
agm list
```

### Database Corruption

**Problem:** SQLite database errors

**Solutions:**
```bash
# Check integrity
sqlite3 ~/.config/agm/agm.db "PRAGMA integrity_check;"

# Rebuild from YAML manifests
mv ~/.config/agm/agm.db ~/.config/agm/agm.db.old
agm sync

# If successful, remove old database
rm ~/.config/agm/agm.db.old
```

### Event Bus Connection Issues

**Problem:** Cannot connect to WebSocket server

**Solutions:**
```bash
# Check if port is in use
lsof -i :8080

# Try different port
export AGM_EVENTBUS_PORT=9090
agm list

# Test connection
curl http://localhost:8080/health
```

### Engram Integration Failures

**Problem:** Engram queries timing out or failing

**Solutions:**
```bash
# Verify engram is installed
which engram
engram --version

# Test engram directly
engram search "test query"

# Increase timeout
export AGM_ENGRAM_TIMEOUT=15

# Check binary path
export AGM_ENGRAM_PATH=$(which engram)
```

### Debug Logging

Enable comprehensive debug logging:

```bash
# Via environment variable
export AGM_DEBUG=1
export AGM_LOG_LEVEL=debug

# Via config file
# log_level: "debug"

# Run command with debug output
agm list 2>&1 | tee agm-debug.log
```

### Reset to Defaults

If all else fails, reset configuration:

```bash
# Backup current config
mv ~/.config/agm/config.yaml ~/.config/agm/config.yaml.backup

# AGM will use defaults
agm list

# Restore if needed
mv ~/.config/agm/config.yaml.backup ~/.config/agm/config.yaml
```

## Configuration Schema Reference

### Type Specifications

| Type | Format | Example |
|------|--------|---------|
| `string` | Text value | `"agm"` |
| `bool` | Boolean | `true`, `false` |
| `int` | Integer | `15`, `100` |
| `float` | Decimal | `0.7`, `0.85` |
| `duration` | Time string | `"5s"`, `"30m"`, `"1h"` |
| `path` | File/directory path | `"~/.config/agm"`, `"/tmp/lock"` |

### Required vs Optional Fields

All fields in `config.yaml` are **optional** - defaults will be used for missing values.

**Recommended minimal config:**
```yaml
ui:
  theme: "agm"

defaults:
  cleanup_threshold_days: 30
```

### Validation

AGM validates configuration on startup:
- Invalid YAML syntax → Falls back to defaults
- Invalid values → Warning logged, defaults used
- Missing file → Defaults used (no error)

## Related Documentation

- [USER-GUIDE.md](USER-GUIDE.md) - General usage guide
- [CLI-REFERENCE.md](CLI-REFERENCE.md) - Command-line reference
- [ACCESSIBILITY.md](ACCESSIBILITY.md) - Accessibility features
- [ARCHITECTURE.md](ARCHITECTURE.md) - System architecture
- [FAQ.md](FAQ.md) - Frequently asked questions

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-02-15 | Initial comprehensive configuration documentation |

---

**Need help?** File an issue at: https://github.com/vbonnet/ai-tools/issues
