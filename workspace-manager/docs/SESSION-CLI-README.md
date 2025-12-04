# Session CLI - Claude Session Management

A unified CLI for resuming and managing Claude sessions with automatic recovery, tmux integration, and session discovery.

## Features

- **Easy Resume**: Resume Claude sessions by tmux name, workspace ID, or UUID
- **Automatic Recovery**: Handle CWD deleted bugs and corrupted sessions
- **Session Discovery**: Auto-discover Claude sessions from history.jsonl
- **Cleanup Tools**: Archive stale sessions, find orphaned data
- **Health Checks**: Validate session integrity before resume
- **Tmux Integration**: Automatic tmux session creation and management

## Quick Start

### Installation

```bash
# Clone repository
cd /tmp/engram-research

# Install via symlink (recommended)
mkdir -p ~/.local/bin

# Link the main dispatcher
ln -sf /tmp/engram-research/wayfinder-projects/workspace-design/workspace-management/session \
    ~/.local/bin/session

# Add to PATH if not already there
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc

# Install completions (optional)
# For Bash:
mkdir -p ~/.local/share/bash-completion/completions
cp completions/session.bash ~/.local/share/bash-completion/completions/session

# For Zsh:
mkdir -p ~/.local/share/zsh/site-functions
cp completions/session.zsh ~/.local/share/zsh/site-functions/_session
```

### First-Time Setup

```bash
# Discover existing Claude sessions from history.jsonl
session sync

# This will:
# 1. Parse ~/.claude/history.jsonl
# 2. Match sessions to existing manifests
# 3. Offer to create manifests for orphaned sessions

# List all sessions
session list
```

## Usage

### Resume a Session

Resume by any identifier:

```bash
# By tmux session name
session resume claude-1

# By workspace session ID
session resume github.com-user-repo-main

# By Claude UUID
session resume c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2

# By partial match (if unique)
session resume repo
```

**What happens**:
1. Resolves identifier → finds manifest
2. Performs health checks (directories exist, no corruption)
3. Creates or attaches to tmux session
4. Auto-resumes Claude with correct UUID
5. Updates manifest timestamps

### List Sessions

```bash
# List all sessions
session list

# List only Claude sessions
session list --claude

# List only workspace-only sessions
session list --workspace

# List only active sessions
session list --active

# List only stale sessions
session list --stale
```

**Output**:
```
Claude Sessions

UUID (truncated)   | Workspace ID                | Tmux     | Last Activity
──────────────────────────────────────────────────────────────────────────────
c86ffd41-cbcc      | github.com-user-repo-main   | claude-1 | 2025-12-03 17:30
f7a2b8c9-4d3e      | gitlab.com-org-project-dev  | claude-2 | 2025-12-02 14:22
```

### Discover & Sync Sessions

```bash
# Discover active Claude sessions from history.jsonl (default)
session sync

# Show all sessions (including short/test sessions)
session sync --all

# Filter to sessions active in last N days
session sync --days 90

# With verbose output
session sync --verbose
```

**What happens**:
1. Parses `~/.claude/history.jsonl` for all sessions
2. Deduplicates sessions (keeps most recent activity per UUID)
3. By default, filters to active sessions (>20 messages, >24h duration)
4. Matches to existing manifests by UUID or worktree path
5. Identifies orphaned Claude sessions (no manifest)
6. Offers to create manifests for orphaned sessions
7. Shows progress indicator for each session mapped

**Filtering**:
- **Default**: Shows only active sessions (>20 messages AND >24h duration)
  - Filters out test sessions and one-off experiments
  - Survives vacation gaps (duration-based, not time-based)
  - Typically matches tmux sessions perfectly
- **`--all`**: Shows all sessions (including 1-message tests)
- **`--days N`**: Only show sessions active in last N days

### Cleanup Operations

```bash
# Archive sessions inactive for 30+ days
session cleanup --stale 30

# List archived sessions
session cleanup --archive-list

# Restore an archived session
session cleanup --archive-restore my-session-20251203-120000

# Permanently delete all archives (DESTRUCTIVE)
session cleanup --archive-purge

# Find tmux sessions not tracked in manifests
session cleanup --orphaned-tmux

# Find Claude sessions not tracked in manifests
session cleanup --orphaned-claude
```

## Recovery Scenarios

### CWD Deleted Bug

**Problem**: Worktree directory was deleted (common after machine restart).

**Detection**: Automatic during `session resume`

**Recovery Options**:
1. **Recreate worktree**: If still in `git worktree list`, automatically repairs
2. **Use fallback directory**: Creates `~/sessions/{id}/working` and updates manifest
3. **Archive session**: Moves to `.archive/` for later restoration
4. **Cancel**: Exit without changes

**Example**:
```bash
$ session resume claude-1

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
CWD Deleted Bug Detected
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

The worktree directory was deleted (common after machine restart).

Recovery options:
  1. Recreate worktree (if still in git worktree list)
  2. Use fallback directory (~/sessions/my-session/working)
  3. Archive session and start fresh
  4. Cancel

Select option (1-4): 2

✓ Using fallback directory: /home/user/sessions/my-session/working
✓ Creating tmux session 'claude-1' with Claude
```

### Corrupted Session

**Problem**: Manifest file has invalid data (bad timestamps, wrong paths, etc.)

**Detection**: Automatic during `session resume`

**Recovery Options**:
1. **Attempt automatic repair**: Fixes common issues, creates backup
2. **Archive corrupted session**: Moves to `.archive/`
3. **Cancel**: Exit without changes

**What gets repaired**:
- Missing `session_id` field
- Invalid ISO timestamps
- Wrong Claude session paths
- Non-absolute worktree paths

**Example**:
```bash
$ session resume my-session

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Session Corruption Detected
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Session corruption detected:
  - Invalid created_at timestamp: not-a-date

Recovery options:
  1. Attempt automatic repair
  2. Archive corrupted session
  3. Cancel

Select option (1-3): 1

✓ Corruption repaired, continuing...
✓ Backup saved to: /home/user/sessions/my-session/manifest.yaml.backup.1733264123
```

### Missing Claude Directories

**Problem**: `~/.claude/session-env/{uuid}` is missing or empty.

**Detection**: Automatic during `session resume`

**Recovery Options**:
1. **Archive session**: Claude session is likely deleted, archive manifest
2. **Continue anyway**: Attempt resume (will likely fail)

### Orphaned Sessions

**Problem**: Claude sessions in `history.jsonl` but no manifest exists.

**Detection**: Run `session sync` or `session cleanup --orphaned-claude`

**Recovery**: Automatically offers to create manifests during `session sync`

## Manifest Structure

Sessions are stored in `~/sessions/{session-id}/manifest.yaml`:

```yaml
session_id: github.com-user-repo-main
status: active
created_at: 2025-12-01T09:26:26Z
last_activity: 2025-12-03T17:30:00Z

repository:
  url: https://github.com/user/repo

worktree:
  path: /home/user/worktrees/repo-main
  branch: main

claude:
  session_id: c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2
  session_env_path: /home/user/.claude/session-env/c86ffd41-...
  file_history_path: /home/user/.claude/file-history/c86ffd41-...
  started_at: 2025-12-01T18:04:00Z
  last_activity: 2025-12-03T17:30:00Z

tmux:
  session_name: claude-1
  window_name: main
  created_at: 2025-12-01T09:26:26Z
```

**Sections**:
- **Top-level**: Workspace metadata (session_id, status, timestamps)
- **repository**: Git repository URL
- **worktree**: Working directory path and branch
- **claude**: Claude session integration (optional)
- **tmux**: Tmux session info (optional)

## Environment Variables

```bash
# Override sessions directory (default: ~/sessions)
export SESSIONS_DIR=/custom/path

# Override Claude history location (default: ~/.claude/history.jsonl)
export CLAUDE_HISTORY=/custom/history.jsonl

# Override resume log location (default: ~/sessions/.resume-log)
export RESUME_LOG=/custom/resume.log

# Enable verbose output
export VERBOSE=true
```

## Troubleshooting

### Command not found: session

```bash
# Check if in PATH
which session

# If not, add ~/.local/bin to PATH
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

### "Session not found" even though it exists

```bash
# Run sync to discover sessions
session sync

# Check if manifest exists
ls ~/sessions/*/manifest.yaml

# Verify Claude UUID is in manifest
grep -r "session_id:" ~/sessions/*/manifest.yaml
```

### Resume hangs or fails

```bash
# Check tmux sessions
tmux list-sessions

# Check Claude directories exist
ls ~/.claude/session-env/

# Enable verbose output
session resume --verbose my-session
```

### Corrupted manifest

```bash
# Try automatic repair
session resume my-session
# Select option 1 (auto-repair)

# Or manually edit
vim ~/sessions/my-session/manifest.yaml
```

## Advanced Usage

### Custom Session Creation

Manually create a session manifest:

```bash
mkdir -p ~/sessions/my-custom-session

cat > ~/sessions/my-custom-session/manifest.yaml <<EOF
session_id: my-custom-session
status: active
created_at: $(date -Iseconds)
last_activity: $(date -Iseconds)

repository:
  url: https://github.com/user/repo

worktree:
  path: /path/to/worktree
  branch: main
EOF

# Resume it
session resume my-custom-session
```

### Batch Operations

Archive all stale sessions non-interactively:

```bash
# List stale sessions (30+ days)
session list --stale

# Archive them (will still prompt for each)
session cleanup --stale 30
```

Find all orphaned Claude sessions:

```bash
session cleanup --orphaned-claude

# Then sync to create manifests
session sync
```

### Resume Log Analysis

Track session resume operations:

```bash
# View resume log
cat ~/sessions/.resume-log

# Format: timestamp | session_id | claude_uuid | action | details
# Example:
# 2025-12-03T17:30:00Z | my-session | c86ffd41-... | attached |
```

## Development

### Running Tests

```bash
cd /tmp/engram-research/wayfinder-projects/workspace-design/workspace-management

# Install BATS (if not installed)
git clone https://github.com/bats-core/bats-core.git /tmp/bats
sudo /tmp/bats/install.sh /usr/local

# Run unit tests
bats tests/unit/*.bats

# Run integration tests
bats tests/integration/*.bats

# Run all tests
bats tests/**/*.bats
```

### Code Structure

```
workspace-management/
├── session                    # Main CLI dispatcher
├── commands/                  # Command implementations
│   ├── resume.sh             # Resume command
│   ├── sync.sh               # Sync command
│   ├── list.sh               # List command
│   ├── cleanup.sh            # Cleanup command
│   └── .template.sh          # Command template
├── lib/                       # Shared libraries
│   ├── common-utils.sh       # Logging, validation
│   ├── path-utils.sh         # Path helpers
│   ├── manifest-utils.sh     # YAML read/write
│   ├── claude-discovery.sh   # Parse history.jsonl
│   ├── tmux-utils.sh         # Tmux control
│   └── recovery-utils.sh     # Recovery operations
├── completions/               # Shell completions
│   ├── session.bash
│   └── session.zsh
└── tests/                     # Test suite
    ├── unit/                  # Unit tests
    └── integration/           # Integration tests
```

## Implementation Details

**Built with Wayfinder methodology**:
- D1-D4: Discovery and design phases
- S5-S7: Implementation phases
- Total: ~2,800 lines of code
- 43 tests (unit + integration)
- 5 implementation phases
- ~13-15 hours development time

**Review Conditions Met**:
- ✅ Condition #2: Auto-sync offer on resume failure
- ✅ Condition #3: Claude session directory validation
- ✅ Condition #4: JSON Lines format validation
- ✅ Condition #5: Progress tracking in migrations
- ✅ Condition #6: Resume action logging
- ✅ Condition #8: Corruption detection and repair

## License

MIT License - see LICENSE file for details.

## Support

File issues at: https://github.com/vbonnet/engram-research/issues

Tag with: `workspace-management`, `claude-sessions`, `session-cli`
