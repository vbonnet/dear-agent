# CSM - Claude Session Manager

Manage Claude AI sessions with tmux integration, session discovery, and health monitoring.

## Features

- **Session Management**: Create, resume, and archive Claude sessions
- **Tmux Integration**: Automatic tmux session creation and attachment
- **Session Discovery**: Auto-import orphaned sessions from Claude history
- **Health Monitoring**: Check session status and tmux health
- **Lock Management**: Safe concurrent command execution with stale lock detection

## Installation

```bash
make build
make install
```

This installs `csm` to `~/go/bin/csm` and `~/.local/bin/csm`.

## Quick Start

```bash
# Create a new session
csm new my-project

# List all sessions
csm list

# Resume a session
csm resume my-project

# Archive a session
csm archive my-project
```

## Commands

### Session Management

#### `csm new [session-name]`
Create a new Claude session with tmux integration.

```bash
csm new                    # Prompt for name or use current tmux session
csm new my-project         # Create session named "my-project"
```

**Behavior:**
- Outside tmux: Creates new tmux session, starts Claude, attaches
- Inside tmux: Uses current session name, starts Claude

#### `csm resume [identifier]`
Resume a Claude session by UUID, tmux name, or project path.

```bash
csm resume c4eb298c        # By UUID prefix
csm resume my-project      # By tmux session name
csm resume workspace       # By project path pattern
```

#### `csm list [--all]`
List Claude session manifests.

```bash
csm list           # Show non-archived sessions
csm list --all     # Show all sessions including archived
```

Displays session status based on tmux state:
- `active`: tmux session is running
- `stopped`: tmux session not running
- `archived`: session marked as archived

#### `csm archive <session-id>`
Archive a Claude session.

```bash
csm archive my-project
```

### Sync and Discovery

#### `csm sync`
Discover and sync Claude sessions from history.

Scans `~/.claude/history.jsonl` for orphaned sessions (sessions without manifests) and offers to import them.

#### `csm associate [session-name]`
Associate current session with Claude UUID.

Links a CSM session manifest to the Claude conversation UUID. Useful after starting a new Claude session.

### Diagnostics

#### `csm doctor [--fix]`
Check system health and configuration.

```bash
csm doctor        # Check for issues
csm doctor --fix  # Check and attempt auto-fixes
```

Checks for:
- Claude history file exists
- tmux installation
- Duplicate session directories
- Duplicate Claude UUIDs
- Session health (worktree exists, etc.)

#### `csm unlock [--force]`
Remove stale lock files.

```bash
csm unlock         # Check and remove stale locks
csm unlock --force # Force remove (even if process running)
```

This command:
- Checks if lock is held by a running process
- Removes lock if process has exited (stale)
- Warns if attempting to remove active lock
- Use `--force` only if certain the process is dead

**When to use:**
- You see "Another csm command is currently running" error
- A csm process crashed and left a stale lock
- Lock file exists but process is gone

**Don't use:**
- Instead of waiting for command to finish
- When another csm is actually running

#### `csm version`
Print version information.

### Backup

#### `csm backup list`
List available manifest backups.

#### `csm backup create <session-id>`
Create manual backup of session manifest.

#### `csm backup restore <session-id> <backup-number>`
Restore session from backup.

## Configuration

Config file: `~/.config/csm/config.yaml`

```yaml
sessions_dir: ~/sessions      # Where session manifests are stored
log_level: info                # Logging level

lock:
  enabled: true                # Enable file locking
  path: /tmp/csm-<UID>/csm.lock

timeout:
  enabled: true
  tmux_commands: 5s            # Timeout for tmux commands

health_check:
  enabled: true
  cache_duration: 10s          # Cache health check results
  probe_timeout: 2s            # Timeout for health probes
```

## Lock Behavior

CSM uses file locking to prevent race conditions when creating tmux sessions or modifying manifests.

### Commands That Need Locks
- `new` - Creates tmux session, starts Claude
- `resume` - Starts Claude, modifies manifests
- `associate` - Modifies manifests
- `sync` - Modifies manifests
- `archive` - Modifies manifests

### Commands That Don't Need Locks (Read-Only)
- `version` - Show version
- `list` - List sessions
- `doctor` - Health checks
- `unlock` - Remove locks (must work even when locked!)
- `backup` - Backup operations

### Lock Release

Locks are released **before** tmux attachment, so you can:
- Stay attached to a tmux session for hours
- Run other csm commands concurrently
- Multiple users can manage different sessions

### Troubleshooting Locks

If you see "Another csm command is currently running":

1. **Wait**: Another csm command is likely running
2. **Check**: Run `csm unlock` to see lock status
3. **Clean up**: If process crashed, `csm unlock` removes stale locks
4. **Force**: Use `csm unlock --force` only if certain process is dead

## Session Manifest

Each session has a manifest stored at `~/sessions/session-<id>/manifest.yaml`:

```yaml
schema_version: "2.0"
session_id: "session-my-project"
name: "my-project"
created_at: "2025-01-15T10:30:00Z"
updated_at: "2025-01-15T11:45:00Z"
lifecycle: ""  # "" = active, "archived" = archived

context:
  project: "/home/user/my-project"
  purpose: "Building feature X"
  tags: ["backend", "api"]
  notes: "Working on authentication"

tmux:
  session_name: "my-project"

claude:
  uuid: "c4eb298c-1234-5678-abcd-ef0123456789"
```

## Development

### Running Tests

```bash
make test                    # Run all tests
make test-coverage           # Run tests with coverage
go test -v ./...             # Run specific package tests
```

### Building

```bash
make build                   # Build binary to bin/csm
make install                 # Install to ~/go/bin and ~/.local/bin
```

## Troubleshooting

### "Another csm command is currently running"

Run `csm unlock` to check for stale locks. If a csm process crashed, the lock will be automatically removed.

### Session not found

Run `csm sync` to discover and import orphaned sessions from Claude history.

### Tmux session errors

Run `csm doctor` to check tmux health and session status.

### Lock won't release

Use `csm unlock --force` to forcefully remove the lock. Only use this if you're certain no csm process is running:

```bash
ps aux | grep csm           # Check for running csm processes
csm unlock --force          # Force remove lock
```

## Architecture

- **Lock Management**: `internal/lock/` - File-based locking with PID validation
- **Tmux Integration**: `internal/tmux/` - Tmux command execution and health checks
- **Session Discovery**: `internal/claude/` - Parse Claude history for orphaned sessions
- **Manifest Management**: `internal/manifest/` - V2 manifest format with validation
- **Health Checking**: Cached health checks with configurable timeouts

## License

MIT
