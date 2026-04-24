# AGM Command Reference

Complete reference for AGM (AI/Agent Session Manager) CLI commands.

**Version**: 3.0
**Updated**: 2026-02-03

---

## Table of Contents

- [Global Flags](#global-flags)
- [Session Management](#session-management)
  - [agm (default)](#agm-default)
  - [agm new](#agm-new)
  - [agm resume](#agm-resume)
  - [agm sessions resume-all](#agm-sessions-resume-all)
  - [agm list](#agm-list)
  - [agm kill](#agm-kill)
- [Agent Management](#agent-management)
  - [agm agent list](#agm-agent-list)
- [Workspace Management](#workspace-management)
  - [agm workspace list](#agm-workspace-list)
  - [agm workspace show](#agm-workspace-show)
  - [agm workspace new](#agm-workspace-new)
  - [agm workspace del](#agm-workspace-del)
- [Workflow Management](#workflow-management)
  - [agm workflow list](#agm-workflow-list)
- [Session Lifecycle](#session-lifecycle)
  - [agm archive](#agm-archive)
  - [agm unarchive](#agm-unarchive)
  - [agm clean](#agm-clean)
- [Session Communication](#session-communication)
  - [agm send](#agm-send)
  - [agm reject](#agm-reject)
- [UUID Management](#uuid-management)
  - [agm fix](#agm-fix)
  - [agm associate](#agm-associate)
  - [agm get-uuid](#agm-get-uuid)
  - [agm get-session-name](#agm-get-session-name)
- [System Health](#system-health)
  - [agm doctor](#agm-doctor)
- [Advanced Features](#advanced-features)
  - [agm search](#agm-search)
  - [agm backup](#agm-backup)
  - [agm sync](#agm-sync)
  - [agm logs](#agm-logs)
  - [agm unlock](#agm-unlock)
  - [agm migrate](#agm-migrate)
- [Testing](#testing)
  - [agm test](#agm-test)
- [Utilities](#utilities)
  - [agm version](#agm-version)

---

## Global Flags

These flags work with all commands:

```bash
-C, --directory <path>       # Working directory (default: current directory)
    --config <file>          # Config file (default: ~/.config/agm/config.yaml)
    --sessions-dir <dir>     # Sessions directory (default: ~/sessions)
    --log-level <level>      # Log level: debug, info, warn, error
    --debug                  # Enable debug logging (env: AGM_DEBUG)
    --timeout <duration>     # Tmux command timeout (overrides config)
    --skip-health-check      # Skip health check
    --no-color               # Disable colored output (WCAG AA compliance)
    --screen-reader          # Use text symbols instead of Unicode
```

### Examples

```bash
# Run command in specific directory
agm -C ~/projects/myapp list

# Enable debug output
agm --debug new my-session

# Disable colors for CI/CD
agm --no-color doctor

# Screen reader friendly output
agm --screen-reader --no-color list
```

---

## Session Management

### agm (default)

Smart session resume or create with context-aware behavior.

**Usage**: `agm [session-name]`

**Behavior**:
- **No arguments**: Shows interactive picker if sessions exist, prompts to create if none
- **With session name**: Resumes if exists, offers fuzzy matches, or creates new

**Examples**:

```bash
# Smart picker or create
agm

# Resume or create specific session
agm my-project

# Fuzzy matching (typo-tolerant)
agm my-proj   # Suggests "my-project"
```

---

### agm new

Create a new session with tmux integration.

**Usage**: `agm new [session-name]`

**Flags**:
- `--detached` - Create without attaching (useful inside tmux)
- `--harness <name>` - CLI harness to use (claude-code, gemini-cli, codex-cli, opencode-cli)
- `--workspace <name>` - Workspace to use (auto for detection, or explicit name)
- `--workflow <name>` - Workflow mode (deep-research, code-review, etc.)
- `--project-id <id>` - Project identifier
- `--prompt <text>` - Initial prompt to send
- `--prompt-file <file>` - File containing initial prompt

**Behavior**:
- Outside tmux + no name: Prompts for name, creates tmux + agent
- Outside tmux + name: Creates tmux session with that name
- Inside tmux + no name: Uses current tmux name, starts agent
- Inside tmux + matching name: Uses current tmux, starts agent
- Inside tmux + different name: Error unless --detached

**Examples**:

```bash
# Create new session (interactive)
agm new

# Create with specific name
agm new my-coding-session

# Create with specific agent
agm new --harness gemini-cli research-task
agm new --harness claude-code code-review
agm new --harness codex-cli brainstorm-ideas

# Create with workflow
agm new --harness gemini-cli --workflow deep-research url-analysis

# Create detached (from within tmux)
agm new other-session --detached

# Create with initial prompt
agm new task --prompt "Review the authentication code"
agm new research --prompt-file ~/prompts/research-template.txt

# Create with workspace detection
agm new --workspace=auto my-session      # Auto-detect from current directory
agm new --workspace=oss coding-task      # Explicitly set OSS workspace
agm new --workspace=acme acme-app-work     # Explicitly set Acme Corp workspace
```

**What it does**:
1. Creates or uses existing tmux session
2. Starts specified AI agent CLI
3. Detects workspace from project directory (or uses --workspace flag)
4. Creates manifest linking tmux session to agent session
5. Auto-detects and associates UUID
6. Sends initial prompt if provided

**Workspace Detection**:
- Reads workspace paths from `~/.agm/config.yaml`
- Matches project directory against workspace paths
- Falls back to interactive selection if ambiguous
- Stores workspace in session manifest's `Context.Workspace` field

See [workspace-detection.md](workspace-detection.md) for detailed algorithm.

---

### agm resume

Resume an existing session.

**Usage**: `agm resume <session-name>`

**Examples**:

```bash
# Resume by name
agm resume my-project

# Resume with fuzzy matching
agm resume my-pro  # Suggests matches
```

**What it does**:
1. Validates session exists and is healthy
2. Attaches to tmux session
3. Restores agent context (if available)

---

### agm sessions resume-all

Resume all stopped sessions in batch.

**Usage**: `agm sessions resume-all [flags]`

**Flags**:
- `--detached` - Resume without attaching (default: true)
- `--include-archived` - Also resume archived sessions
- `--workspace-filter <name>` - Only resume sessions in specific workspace
- `--dry-run` - Preview without executing
- `--continue-on-error` - Continue if some sessions fail (default: true)

**Examples**:

```bash
# Resume all stopped sessions
agm sessions resume-all

# Resume only sessions in specific workspace
agm sessions resume-all --workspace-filter=alpha

# Preview what would be resumed (dry-run)
agm sessions resume-all --dry-run

# Resume including archived sessions
agm sessions resume-all --include-archived
```

**What it does**:
1. Lists all manifests and filters to stopped sessions
2. Computes session status in batch (efficient single tmux query)
3. Resumes sessions sequentially with 500ms delays (prevents tmux overload)
4. Displays progress indicators (spinner + progress bar)
5. Collects errors and shows summary report
6. Writes `.agm/resume-timestamp` for orchestrator coordination (ADR-010)

**Use Cases**:
- **Post-reboot recovery**: Restore all sessions after machine restart
- **Workspace isolation**: Resume only sessions in specific workspace
- **Batch operations**: Resume 20+ sessions efficiently

**Performance**:
- 20 sessions: ~12 seconds
- 50 sessions: ~30 seconds
- Batch status computation: O(1) tmux calls vs O(n) for individual checks

**See Also**:
- [ADR-010: Orchestrator Resume Detection](../docs/adr/ADR-010-orchestrator-resume-detection.md) - Integration with orchestrator v2 for post-resume restart prompts
- `agm admin enable-auto-resume` - Enable automatic boot-time resume (future)

---

### agm list

List sessions with status information.

**Usage**: `agm list [flags]`

**Flags**:
- `--all` - Include archived sessions
- `--json` - Output as JSON
- `--all-workspaces` - Show sessions from all workspaces
- `--workspace-filter <name>` - Filter sessions by specific workspace

**Session Status**:
- `active` - Tmux session is running
- `stopped` - Tmux session not running
- `archived` - Session marked as archived

**Examples**:

```bash
# List active/stopped sessions
agm list

# List all sessions including archived
agm list --all

# JSON output for scripting
agm list --json

# Show sessions from all workspaces
agm list --all-workspaces

# Filter sessions by workspace
agm list --workspace-filter oss
agm list --workspace-filter acme
```

**Cross-Workspace Discovery**:

AGM automatically discovers sessions across all workspaces when:
- `--all-workspaces` flag is set
- `--workspace-filter <name>` is specified
- Running from outside workspace directories (e.g., `~/src`)

Discovery checks both `.agm/sessions` (new location) and `sessions` (legacy location) for backward compatibility.
```

**Output Format**:

```
NAME              STATUS    AGENT    WORKSPACE  PROJECT                    UPDATED
my-coding-task    active    claude   oss        ~/projects/webapp          2h ago
research-urls     stopped   gemini   acme     ~/research                 1d ago
old-session       archived  claude   -          ~/old-project              30d ago
```

**Columns**:
- **NAME**: Session name
- **STATUS**: Session lifecycle status (active, stopped, archived)
- **AGENT**: AI agent used (claude, gemini, gpt)
- **WORKSPACE**: Workspace name (from session manifest, "-" if empty)
- **PROJECT**: Project directory path
- **UPDATED**: Time since last update

---

### agm kill

Kill a running session.

**Usage**: `agm kill <session-name>`

**Examples**:

```bash
# Kill specific session
agm kill my-session

# Kill with confirmation
agm kill old-task
```

**What it does**:
1. Terminates tmux session
2. Updates session status to stopped
3. Preserves manifest for later resume

---

## Agent Management

### agm agent list

List available AI agents with availability status.

**Usage**: `agm agent list [flags]`

**Flags**:
- `--json` - Output as JSON for scripting

**Availability Checks**:
- `claude`: Requires ANTHROPIC_API_KEY
- `gemini`: Requires GEMINI_API_KEY
- `gpt`: Requires OPENAI_API_KEY

**Examples**:

```bash
# List agents (table format)
agm agent list

# JSON output
agm agent list --json
```

**Output**:

```
AGENT    AVAILABLE  CONTEXT    STRENGTHS
claude   yes        200K       Code, reasoning, long context
gemini   yes        1M         Research, summarization, massive context
gpt      no         128K       Chat, brainstorming, general Q&A
```

**See Also**: [Agent Comparison Guide](AGENT-COMPARISON.md)

---

## Workspace Management

Workspace commands help organize sessions by context (e.g., `oss` vs `acme`).

### agm workspace list

List all configured workspaces from `~/.agm/config.yaml`.

**Usage**: `agm workspace list`

**Examples**:

```bash
# List all workspaces
agm workspace list
```

**Output**:

```
NAME     PATH                           SESSIONS
oss      ~/projects/myworkspace                   42
acme   ~/src/ws/acme                18
```

**Columns**:
- **NAME**: Workspace name
- **PATH**: Workspace directory path
- **SESSIONS**: Number of sessions in workspace

---

### agm workspace show

Show detailed information about a specific workspace.

**Usage**: `agm workspace show <name>`

**Examples**:

```bash
# Show workspace details
agm workspace show oss
agm workspace show acme
```

**Output**:

```
Workspace: oss
Path: ~/projects/myworkspace
Sessions: 42

Sessions in workspace:
- my-coding-task (active)
- research-urls (stopped)
- old-task (archived)
...
```

**What it shows**:
- Workspace name and path
- Number of sessions
- List of all sessions in workspace with status

---

### agm workspace new

Create a new workspace with interactive configuration.

**Usage**: `agm workspace new <name>`

**Examples**:

```bash
# Create workspace with interactive prompts
agm workspace new personal
agm workspace new client-project
```

**Interactive Prompts**:
1. Workspace path (e.g., `~/src/ws/personal`)
2. Validation and confirmation

**What it does**:
1. Prompts for workspace path
2. Validates path exists
3. Updates `~/.agm/config.yaml` with new workspace
4. Creates atomic backup of config before update

**Configuration Format** (in `~/.agm/config.yaml`):

```yaml
workspaces:
  - name: oss
    path: ~/projects/myworkspace
  - name: acme
    path: ~/src/ws/acme
  - name: personal
    path: ~/src/ws/personal
```

---

### agm workspace del

Delete a workspace from configuration (sessions remain).

**Usage**: `agm workspace del <name>`

**Examples**:

```bash
# Delete workspace (with confirmation)
agm workspace del old-workspace
agm workspace del archived-project
```

**What it does**:
1. Prompts for confirmation
2. Removes workspace from `~/.agm/config.yaml`
3. **Sessions remain** - only config is updated
4. Creates atomic backup before deletion

**Important**: This only removes the workspace from configuration. Sessions in that workspace are NOT deleted and remain accessible via `agm list --all`.

---

## Workflow Management

### agm workflow list

List available workflows and their agent compatibility.

**Usage**: `agm workflow list [flags]`

**Flags**:
- `--harness <name>` - Filter by harness compatibility

**Examples**:

```bash
# List all workflows
agm workflow list

# List workflows for specific agent
agm workflow list --harness=gemini-cli
agm workflow list --harness=claude-code
```

**Available Workflows**:
- `deep-research` - Research URLs and synthesize insights
- `code-review` - Analyze code changes and provide feedback
- `architect` - Design system architectures

**See Also**: Workflow documentation (coming soon)

---

## Session Lifecycle

### agm archive

Archive a session (marks as archived, keeps all data).

**Usage**: `agm archive [--async] <session-name>`

**Flags**:

| Flag | Description |
|------|-------------|
| `--async` | Archive an active session asynchronously (required for active sessions, not valid for stopped sessions) |
| `--all` | Archive all inactive sessions |
| `--older-than` | Filter by age (e.g. `30d`, `7d`, `1w`) |
| `--dry-run` | Preview without executing |

**Session state determines behavior**:

- **Stopped sessions**: Archive directly without confirmation. Do NOT use `--async`.
- **Active sessions**: MUST use `--async`. Spawns a background reaper for graceful shutdown.

**Error cases**:
- Active session without `--async`: `session is active; use --async to archive an active session`
- Stopped session with `--async`: `--async should only be used for active sessions; omit --async for stopped sessions`

**Examples**:

```bash
# Archive a stopped session (no confirmation prompt)
agm session archive old-project

# Archive an active session (--async required)
agm session archive --async active-session

# Archive all inactive sessions older than 30 days (preview)
agm session archive --all --older-than=30d --dry-run

# Archive all inactive sessions older than 30 days
agm session archive --all --older-than=30d
```

**What it does**:
1. Validates session exists
2. Checks whether session is active in tmux
3. Enforces `--async` mutual exclusivity with session state
4. Updates lifecycle status to "archived"
5. Preserves all session data
6. Hides from default `agm session list` output

---

### agm unarchive

Restore archived sessions using pattern matching.

**Usage**: `agm unarchive <pattern>`

**Pattern Support**:
- `*` - Match any characters
- `?` - Match single character
- `[abc]` - Match character set

**Examples**:

```bash
# Exact match - auto-restore
agm unarchive my-session

# Pattern match - show picker if multiple
agm unarchive *acme*
agm unarchive "session-202?"    # Wildcard year
agm unarchive "*"               # All archived - interactive selection
```

**Search Locations**:
- In-place archived sessions
- `.archive-old-format/` directory

---

### agm clean

Interactive batch cleanup with smart suggestions.

**Usage**: `agm clean`

**Smart Suggestions**:
- Stopped sessions > 30 days: Suggested for archival
- Archived sessions > 90 days: Suggested for deletion

**Examples**:

```bash
# Interactive cleanup
agm clean
```

**Features**:
- Multi-select interface
- Confirmation before destructive actions
- Customizable thresholds in config

**Configuration**:

```yaml
# ~/.config/agm/config.yaml
defaults:
  cleanup_threshold_days: 30    # Stopped → archive
  archive_threshold_days: 90    # Archived → delete
```

---

## Session Communication

### agm session send

Send message/prompt to running session, interrupting active thinking.

**Usage**: `agm session send <session-name> [flags]`

**Flags**:
- `--prompt <text>` - Prompt text to send
- `--prompt-file <path>` - File containing prompt (max 10KB)

**Examples**:

```bash
# Send inline prompt
agm session send my-session --prompt "Please review the code"

# Send from file (large prompts)
agm session send my-session --prompt-file ~/prompts/diagnosis.txt

# Send multi-line prompt
agm session send research --prompt "Analyze the following:
1. Authentication flow
2. Error handling
3. Security concerns"

# Interrupt and redirect stuck session
agm session send my-session --prompt "Stop and list all files in current directory"

# Send code review request
agm session send code-review --prompt "Review src/auth/login.py for security issues"

# Send research task
agm session send research-task --prompt-file ~/tasks/api-analysis.md
```

**Features**:
- Auto-interrupt: Sends ESC to stop thinking
- Literal mode: Prevents special character interpretation
- Reliable execution: Prompt runs as command, not pasted text
- Large prompts: Supports up to 10KB files

**Use Cases**:
- Automated recovery of stuck sessions
- Sending diagnosis prompts
- Batch message delivery
- Automated code review requests
- Research task automation
- CI/CD integration for AI-assisted analysis

**Requirements**: Session must be running (active tmux session)

**Tips**:
- Use `--prompt-file` for complex, multi-line prompts
- Keep prompts under 10KB for reliability
- Verify session is active with `agm list` before sending

---

### agm session reject

Reject permission prompt with custom reason.

**Usage**: `agm session reject <session-name> [flags]`

**Flags**:
- `--reason <text>` - Rejection reason
- `--reason-file <path>` - File containing reason (max 10KB)

**Examples**:

```bash
# Reject with inline reason
agm session reject my-session --reason "Use Read tool instead of cat"

# Reject with violation prompt from file
agm session reject my-session --reason-file ~/prompts/VIOLATION-PROMPTS.md

# Reject with detailed feedback
agm session reject task --reason "Please use absolute paths and separate tool calls"

# Reject with coding standards
agm session reject code-session --reason "Use Edit tool, not sed command via Bash"

# Reject with security guidance
agm session reject review-task --reason "Do not read .env files. Request user to provide required values."

# Reject with process guidance
agm session reject research --reason "Create separate Read tool calls instead of using cat. One file per call."
```

**What it does**:
1. Navigates to "No" option (Down key)
2. Adds additional instructions (Tab key)
3. Sends rejection reason (literal mode)
4. Submits (Enter key)

**Features**:
- Automated navigation
- Smart extraction: Extracts "## Standard Prompt (Recommended)" from markdown
- Literal mode: Reliable text transmission

**Use Cases**:
- Rejecting tool usage violations
- Providing feedback on permission denials
- Automated enforcement of coding standards

**Requirements**: Session must show permission prompt with "No" option

---

## UUID Management

### agm fix

Manual UUID association management.

**Usage**: `agm fix [session-name] [flags]`

**Flags**:
- `--all` - Auto-fix all sessions (high confidence only)
- `--clear <session>` - Remove UUID association

**Examples**:

```bash
# Scan all unassociated sessions
agm fix

# Fix specific session (with suggestions)
agm fix my-session

# Auto-fix all high-confidence matches
agm fix --all

# Remove UUID association
agm fix --clear my-session
```

**UUID Suggestion Sources**:
1. Auto-detected from history (high confidence)
2. Recent UUIDs from `~/.claude/history.jsonl`
3. Manual entry option

**Confidence Levels**:
- **High** (< 2.5 min old): Auto-applied
- **Medium** (2.5-5 min): Manual confirmation
- **Low** (> 5 min): Listed in suggestions

---

### agm session associate

Associate a AGM session with the current Claude session UUID.

**Usage**: `agm session associate <session-name> [flags]`

**Flags**:
- `--uuid <uuid>` - Specify Claude UUID explicitly (instead of auto-detection)
- `--create` - Create new manifest if session doesn't exist
- `-C, --directory <path>` - Working directory for new session

**Examples**:

```bash
# Associate current Claude session with AGM session "my-project"
agm session associate my-project

# Create new session if it doesn't exist
agm session associate my-project --create

# Specify directory for new session
agm session associate my-project --create -C ~/projects/myapp

# Use specific Claude UUID instead of auto-detection
agm session associate my-project --uuid c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2
```

**Use Cases**:
- Associate existing AGM session with current Claude UUID
- Create new AGM session from within Claude
- Reconnect session after UUID changes
- Debug session association issues

---

### agm get-uuid

Get Claude UUID for a session.

**Usage**: `agm get-uuid <session-name>`

**Examples**:

```bash
# Get UUID
agm get-uuid my-session

# Use in scripts
UUID=$(agm get-uuid my-session)
echo "Session UUID: $UUID"
```

---

### agm get-session-name

Get session name from UUID.

**Usage**: `agm get-session-name <uuid>`

**Examples**:

```bash
# Get session name
agm get-session-name abc123-def456-...

# Use in scripts
SESSION=$(agm get-session-name $UUID)
agm resume $SESSION
```

---

## System Health

### agm doctor

Health check and validation for AGM and agent sessions.

**Usage**: `agm doctor [flags]`

**Modes**:

1. **Quick Health Check** (default): Fast structural checks (~1-5 seconds)
2. **Deep Validation** (`--validate`): Thorough functional testing (~5-30 seconds per session)

**When to Use Each Mode**:

- **Use default mode** for daily health checks, quick overviews, and when performance matters
- **Use `--validate` mode** for debugging resume failures, production readiness checks, and automated testing

**Flags**:
- `--validate` - Enable deep validation (structural + functional testing)
- `--apply-fixes` - Auto-fix issues (requires --validate)
- `--json` - JSON output for scripting
- `--test` - Check test sessions in ~/sessions-test/ instead of production

**Structural Checks** (always performed):
- Agent installation (history files, binaries)
- tmux installation and socket status
- User lingering (session persistence after logout)
- Duplicate session directories
- Duplicate agent UUIDs
- Sessions with empty/missing UUIDs
- Session health (manifest validity, directory structure)

**Functional Validation** (--validate flag only):
- Tests actual session resumability
- Classifies 6 resume error types:
  - Empty session-env directory
  - Version mismatch (agent CLI version changed)
  - Compacted JSONL (summaries not at end)
  - Missing JSONL file
  - CWD mismatch (working directory changed)
  - Lock contention (session locked by process)

**Auto-Fix Strategies** (--apply-fixes flag):
- Safe: Version mismatch (updates session-env manifest)
- Risky: JSONL reorder (with backup/restore, requires confirmation)

**Examples**:

```bash
# Quick health check (structural only - fast)
agm doctor

# Deep validation (structural + functional - slower)
agm doctor --validate

# Test and auto-fix issues
agm doctor --validate --apply-fixes

# JSON output for scripting
agm doctor --validate --json

# Check test sessions instead of production
agm doctor --test
```

**Output Example**:

```
=== AGM Health Check ===

✓ Claude history found
✓ tmux installed: tmux 3.3a
✓ tmux socket active: /tmp/tmux-1000/default
✓ User lingering enabled (sessions persist after logout)
✓ Found 224 session manifests

--- Checking session health ---
⚠ Unhealthy session: my-broken-session
  Issue: JSONL file compacted (summaries not at end)
  Fix: agm doctor --validate --apply-fixes

✓ System is healthy
```

---

## Advanced Features

### agm search

AI-powered semantic search for archived sessions.

**Usage**: `agm search <query> [flags]`

**Flags**:
- `--max-results <N>` - Maximum results (default: 10)

**Examples**:

```bash
# Semantic search
agm search "that conversation about Composio"
agm search "OAuth integration with MCP"
agm search "last week's debugging session"

# Limit results
agm search "API design" --max-results 5
```

**Features**:
- Powered by Google Vertex AI (Claude Haiku)
- Searches conversation history (`~/.claude/history.jsonl`)
- Interactive selection for multiple results
- Auto-restores selected session
- Results cached for 5 minutes
- Rate limited: 10 searches/minute

**Authentication**:

```bash
# Configure Google Cloud credentials
gcloud auth application-default login

# Set project
export GOOGLE_CLOUD_PROJECT=your-project-id
# OR
gcloud config set project your-project-id
```

---

### agm backup

Backup and restore session manifests.

**Usage**: `agm backup <subcommand>`

**Subcommands**:
- `list <identifier>` - List available backups for a session
- `restore <identifier> <backup-number>` - Restore from specific backup

**Identifier Types**:
- Session UUID (full or partial): `c4eb298c`
- Tmux session name: `claude-1`
- Project path pattern: `workspace-design`

**Examples**:

```bash
# List backups for a session
agm backup list c4eb298c              # By UUID prefix
agm backup list claude-1              # By tmux name
agm backup list workspace-design      # By project path

# Restore specific backup
agm backup restore c4eb298c 3         # Restore backup #3 by UUID
agm backup restore claude-1 2         # Restore backup #2 by tmux name
```

**What it does**:
1. Lists numbered backups (`.manifest.1`, `.manifest.2`, etc.)
2. Shows full path to each backup file
3. Restores selected backup with confirmation
4. Creates safety backup of current manifest before restoration

**Backup Location**: Backups stored in `.backups/` subdirectory within session directory

---

### agm sync

Synchronize sessions across machines.

**Usage**: `agm sync [flags]`

**Examples**:

```bash
# Sync sessions
agm sync
```

**Note**: Synchronization implementation varies by environment.

---

### agm logs

Session log management and analysis.

**Usage**: `agm logs <subcommand>`

**Subcommands**:
- `clean` - Remove old message log files
- `stats` - Show log statistics
- `thread <message-id>` - Show conversation thread for a message
- `query` - Search message logs

**Log Storage**: `~/.agm/logs/messages/` as daily JSONL files (format: `YYYY-MM-DD.jsonl`)

**Examples**:

```bash
# Clean logs older than 90 days (default)
agm logs clean

# Clean logs older than 30 days
agm logs clean --older-than 30

# Show log statistics
agm logs stats

# Show conversation thread for a message
agm logs thread 1738612345678-sender-001

# Query logs by sender
agm logs query --sender astrocyte

# Query logs by date
agm logs query --since 2026-02-01

# Combine filters
agm logs query --sender agm-send --since 2026-02-03
```

**Flags**:
- `clean`:
  - `--older-than <days>` - Delete logs older than N days (default: 90)
- `query`:
  - `--sender <name>` - Filter by sender name
  - `--since <date>` - Filter by date (YYYY-MM-DD format)

**Statistics Display**:
- Total log files
- Total messages logged
- Date range (oldest to newest)
- Disk usage (formatted: MB, GB, etc.)
- Log directory location

**Use Cases**:
- Audit message history
- Debug communication issues
- Track session activity
- Clean up old logs to free disk space
- Search for specific messages or senders

---

### agm unlock

Unlock a locked session.

**Usage**: `agm unlock <session-name>`

**Examples**:

```bash
# Unlock session
agm unlock my-session
```

**Use Cases**:
- Session locked by crashed process
- Stale lock file
- Force unlock for recovery

**Warning**: Only use if you're certain the session is not actually in use.

---

### agm migrate

Migrate sessions to unified storage structure.

**Usage**: `agm migrate --to-unified-storage [flags]`

**Flags**:
- `--dry-run` - Preview changes without modifying files
- `--force` - Overwrite existing destinations
- `--workspace <name>` - Migrate only specified workspace (e.g., 'oss')

**What it does**:
1. Discovers sessions across all workspaces (`~/src/ws/*/sessions/`)
2. Moves manifests and conversations to `~/src/sessions/{session-name}/`
3. Converts conversation formats (HTML → JSONL)
4. Creates audit log of all operations
5. Preserves old directories for 30 days (rollback safety)

**Examples**:

```bash
# Preview migration (no changes)
agm migrate --to-unified-storage --dry-run

# Migrate all sessions
agm migrate --to-unified-storage

# Migrate only 'oss' workspace sessions
agm migrate --to-unified-storage --workspace=oss

# Force overwrite of existing destinations
agm migrate --to-unified-storage --force
```

**Migration Report**:
- Succeeded: Number of successfully migrated sessions
- Skipped: Already migrated (use --force to overwrite)
- Failed: Sessions with errors (detailed error messages)

**Use Cases**:
- Consolidate sessions from multiple workspace directories
- Transition to unified storage layout
- Clean up fragmented session storage
- Standardize session organization

**Safety Features**:
- Dry-run mode for previewing changes
- Automatic backup of existing manifests
- Detailed error reporting
- 30-day preservation of old directories

---

## Testing

### agm test

Testing utilities for AGM development and debugging.

**Usage**: `agm test <subcommand>`

**Subcommands**:
- `create <name>` - Create isolated test session with Claude started
- `send <name> <command>` - Send commands to test session
- `capture <name>` - Capture output from test session
- `cleanup <name>` - Cleanup test sessions

**Test Isolation**:
- Uses `/tmp/agm-test-*` directories for state
- Uses `agm-test-*` tmux sessions
- Completely isolated from production (`~/.claude-sessions/`)
- Clean environment for testing AGM functionality

**Examples**:

```bash
# Create test session
agm test create my-test

# Send commands to test session
agm test send my-test "agm associate --create test-project"
agm test send my-test "agm list"

# Capture output from test session
agm test capture my-test
agm test capture my-test --lines 50

# Cleanup test session
agm test cleanup my-test
```

**Use Cases**:
- Test AGM functionality without affecting production sessions
- Automate testing workflows
- Debug session lifecycle
- Validate command behavior
- Integration testing

**Common Testing Patterns**:

```bash
# Test session lifecycle
agm test create lifecycle-test
agm test send lifecycle-test "agm new test-session --project ~/projects/test"
agm test capture lifecycle-test
agm test cleanup lifecycle-test

# Test session association
agm test create assoc-test
agm test send assoc-test "agm associate --create my-project"
agm test send assoc-test "agm associate --status"
agm test capture assoc-test
agm test cleanup assoc-test

# JSON output for automation
agm test create api-test --json
agm test send api-test "agm list" --json
agm test cleanup api-test --json
```

**Best Practices**:
- Always cleanup test sessions after use
- Use descriptive test session names
- Isolate tests to prevent interference
- Use `--json` flag for automation scripts

---

## Utilities

### agm version

Show AGM version and binary location.

**Usage**: `agm version`

**Examples**:

```bash
# Show version
agm version

# Output:
# agm 3.0.0 (/usr/local/bin/agm)
```

---

## Environment Variables

AGM respects these environment variables:

```bash
# Debugging
AGM_DEBUG=true              # Enable debug logging (same as --debug)

# Accessibility
NO_COLOR=1                  # Disable colors (legacy, use --no-color flag)
AGM_SCREEN_READER=1        # Screen reader mode (legacy, use --screen-reader flag)

# Agent API Keys
ANTHROPIC_API_KEY=...      # Claude API key
GEMINI_API_KEY=...         # Gemini API key
OPENAI_API_KEY=...         # GPT API key

# Google Cloud (for search)
GOOGLE_CLOUD_PROJECT=...   # GCP project ID
GOOGLE_APPLICATION_CREDENTIALS=...  # Service account key
```

---

## Configuration File

AGM uses `~/.config/agm/config.yaml` for configuration.

**Example Configuration**:

```yaml
defaults:
  interactive: true              # Enable interactive prompts
  auto_associate_uuid: true      # Auto-detect UUIDs
  confirm_destructive: true      # Confirm before delete/archive
  cleanup_threshold_days: 30     # Stopped → archive threshold
  archive_threshold_days: 90     # Archived → delete threshold

ui:
  theme: "agm"                   # UI theme (agm, agm-light, dracula, catppuccin)
  picker_height: 15              # Session picker height
  show_project_paths: true       # Show full project paths
  show_tags: true                # Show session tags
  fuzzy_search: true             # Enable fuzzy matching

advanced:
  tmux_timeout: "5s"             # Tmux command timeout
  health_check_cache: "5s"       # Health check cache duration
  lock_timeout: "30s"            # Lock acquisition timeout
  uuid_detection_window: "5m"    # UUID detection time window
```

**Available Themes**:
- `agm` - High-contrast for dark terminals (default, WCAG AA compliant)
- `agm-light` - High-contrast for light terminals
- `dracula` - Dracula color scheme
- `catppuccin` - Catppuccin color scheme
- `charm` - Charm Bracelet theme
- `base` - Minimal theme

---

## Exit Codes

AGM uses standard exit codes:

- `0` - Success
- `1` - General error
- `2` - Misuse of command (invalid arguments)
- `3` - Session not found
- `4` - Lock acquisition failed
- `130` - Interrupted by user (Ctrl+C)

**Example Usage**:

```bash
#!/bin/bash
agm resume my-session
if [ $? -eq 0 ]; then
    echo "Session resumed successfully"
else
    echo "Failed to resume session"
fi
```

---

## Common Workflows

### Create and Resume Workflow

```bash
# Create new session with agent
agm new --harness gemini-cli research-project

# Work in session...
# Exit session (Ctrl+D or exit)

# Resume later
agm resume research-project
# OR use fuzzy matching
agm research
```

### Multi-Agent Workflow

```bash
# Create sessions with different agents for different tasks
agm new --harness claude-code code-task
agm new --harness gemini-cli research-task
agm new --harness codex-cli brainstorm-task

# List all sessions
agm list

# Switch between sessions
agm resume code-task
agm resume research-task
```

### Code Review Workflow

```bash
# Create session for code review
agm new --harness claude-code code-review-auth-refactor

# Send code for review
agm session send code-review-auth-refactor --prompt "Review the authentication refactor in src/auth/"

# Resume to see results
agm resume code-review-auth-refactor

# Archive when done
agm archive code-review-auth-refactor
```

### Research and Documentation Workflow

```bash
# Create research session with Gemini (1M context)
agm new --harness gemini-cli --workflow deep-research api-research

# Send URLs for research
agm session send api-research --prompt "Analyze these API design patterns: https://..."

# Resume to review findings
agm resume api-research

# Search later if archived
agm search "API design patterns"
```

### Cleanup Workflow

```bash
# List all sessions
agm list --all

# Archive completed sessions
agm archive old-task

# Interactive cleanup (recommended)
agm clean

# Or manual cleanup
agm archive task1
agm archive task2
agm unarchive task1  # Restore if needed
```

### Debugging Workflow

```bash
# Quick health check (fast)
agm doctor

# Deep validation (thorough)
agm doctor --validate

# Auto-fix issues
agm doctor --validate --apply-fixes

# Fix UUID associations
agm fix my-session
agm fix --all

# Unlock stuck session
agm unlock my-session

# View logs for troubleshooting
agm logs stats
agm logs query --sender agm-send --since 2026-02-01
```

### Search and Restore Workflow

```bash
# Search for archived session by semantic meaning
agm search "OAuth integration"
agm search "that debugging session about timeouts"

# Or use pattern matching
agm unarchive *oauth*
agm unarchive "session-2026*"

# Resume restored session
agm resume oauth-task
```

### Backup and Recovery Workflow

```bash
# List backups for a session
agm backup list my-session

# Restore from backup if needed
agm backup restore my-session 3

# Verify restored state
agm resume my-session
```

### Migration Workflow

```bash
# Preview migration from workspace directories
agm migrate --to-unified-storage --dry-run

# Migrate specific workspace
agm migrate --to-unified-storage --workspace=oss

# Full migration
agm migrate --to-unified-storage

# Verify migration
agm list --all
```

### Automated Session Management

```bash
# Create session with initial prompt
agm new task --harness claude-code --prompt "Review security vulnerabilities"

# Send follow-up commands
agm session send task --prompt-file ~/prompts/security-checklist.txt

# Reject permission with guidance
agm session reject task --reason "Use Read tool instead of cat command"

# Kill session when done
agm kill task
```

---

## Tips and Best Practices

### Naming Conventions

```bash
# Use descriptive names with hyphens
agm new feature-auth-refactor
agm new bug-fix-login-timeout
agm new research-api-design

# Include context
agm new acme-acme-app-oauth
agm new personal-blog-rewrite
```

### Agent Selection

```bash
# Claude: Best for code and reasoning
agm new --harness claude-code code-review-auth

# Gemini: Best for research and long context
agm new --harness gemini-cli research-competitors

# GPT: Best for brainstorming
agm new --harness codex-cli brainstorm-features
```

**See**: [Agent Comparison Guide](AGENT-COMPARISON.md) for detailed guidance.

### Session Organization

```bash
# Use consistent directory structure
cd ~/projects/myapp
agm new myapp-feature-x

cd ~/research/topic
agm new research-topic

# Review sessions by directory
cd ~/projects/myapp
agm list  # Shows sessions in current directory
```

### Accessibility

```bash
# For screen reader users
alias agm='agm --no-color --screen-reader'

# For CI/CD environments
agm --no-color list --json

# For high-contrast needs
# Set theme in ~/.config/agm/config.yaml
ui:
  theme: "agm"  # or "agm-light" for light terminals
```

### Performance

```bash
# Skip health checks for faster commands
agm --skip-health-check list

# Use JSON output for scripting
agm list --json | jq '.[] | select(.status == "active")'

# Cache configuration for repeated commands
export AGM_CONFIG=~/.config/agm/config.yaml
```

---

## Troubleshooting

### Common Issues

**UUID not detected**:
```bash
# Check history file
cat ~/.claude/history.jsonl | tail -5

# If empty, send message in Claude, then:
agm fix --all
```

**Harness not available**:
```bash
# Check which agents are configured
agm agent list

# Set up API keys
export ANTHROPIC_API_KEY=your-key
export GEMINI_API_KEY=your-key
export OPENAI_API_KEY=your-key
```

**Session not appearing**:
```bash
# Include archived sessions
agm list --all

# Check sessions directory
ls -la ~/sessions/
```

**Stuck session**:
```bash
# Unlock session
agm unlock my-session

# Or kill and restart
agm kill my-session
agm resume my-session
```

**See Also**: [Troubleshooting Guide](TROUBLESHOOTING.md) for detailed solutions.

---

## Further Reading

- [Agent Comparison Guide](AGENT-COMPARISON.md) - Choose the right agent
- [BDD Scenario Catalog](BDD-CATALOG.md) - Living documentation
- [Troubleshooting Guide](TROUBLESHOOTING.md) - Common issues and solutions
- [Migration Guide](MIGRATION-CLAUDE-MULTI.md) - Transitioning to multi-agent
- [Accessibility Guide](ACCESSIBILITY.md) - WCAG compliance details

---

## Getting Help

```bash
# General help
agm --help

# Command-specific help
agm new --help
agm doctor --help
agm list --help

# Show version
agm version
```

**Community**:
- GitHub Issues: Report bugs and request features
- Documentation: Complete guides in `docs/` directory

---

**Last Updated**: 2026-02-04
**AGM Version**: 3.0
