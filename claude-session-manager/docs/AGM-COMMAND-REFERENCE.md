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
  - [agm list](#agm-list)
  - [agm kill](#agm-kill)
- [Agent Management](#agent-management)
  - [agm agent list](#agm-agent-list)
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
- [Testing](#testing)
  - [agm test](#agm-test)
- [Utilities](#utilities)
  - [agm version](#agm-version)

---

## Global Flags

These flags work with all commands:

```bash
-C, --directory <path>       # Working directory (default: current directory)
    --config <file>          # Config file (default: ~/.config/csm/config.yaml)
    --sessions-dir <dir>     # Sessions directory (default: ~/sessions)
    --log-level <level>      # Log level: debug, info, warn, error
    --debug                  # Enable debug logging (env: CSM_DEBUG)
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
- `--agent <name>` - AI agent to use (claude, gemini, gpt)
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
agm new --agent gemini research-task
agm new --agent claude code-review
agm new --agent gpt brainstorm-ideas

# Create with workflow
agm new --agent gemini --workflow deep-research url-analysis

# Create detached (from within tmux)
agm new other-session --detached

# Create with initial prompt
agm new task --prompt "Review the authentication code"
agm new research --prompt-file ~/prompts/research-template.txt
```

**What it does**:
1. Creates or uses existing tmux session
2. Starts specified AI agent CLI
3. Creates manifest linking tmux session to agent session
4. Auto-detects and associates UUID
5. Sends initial prompt if provided

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

### agm list

List sessions with status information.

**Usage**: `agm list [flags]`

**Flags**:
- `--all` - Include archived sessions
- `--json` - Output as JSON

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
```

**Output Format**:

```
NAME              STATUS    AGENT    PROJECT                    UPDATED
my-coding-task    active    claude   ~/projects/webapp          2h ago
research-urls     stopped   gemini   ~/research                 1d ago
old-session       archived  claude   ~/old-project              30d ago
```

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

## Workflow Management

### agm workflow list

List available workflows and their agent compatibility.

**Usage**: `agm workflow list [flags]`

**Flags**:
- `--agent <name>` - Filter by agent compatibility

**Examples**:

```bash
# List all workflows
agm workflow list

# List workflows for specific agent
agm workflow list --agent=gemini
agm workflow list --agent=claude
```

**Available Workflows**:
- `deep-research` - Research URLs and synthesize insights
- `code-review` - Analyze code changes and provide feedback
- `architect` - Design system architectures

**See Also**: Workflow documentation (coming soon)

---

## Session Lifecycle

### agm archive

Archive a session (marks as archived, keeps manifest).

**Usage**: `agm archive <session-name>`

**Examples**:

```bash
# Archive single session
agm archive old-project

# Archive stopped session
agm archive completed-task
```

**What it does**:
1. Validates session exists
2. Updates lifecycle status to "archived"
3. Preserves all session data
4. Hides from default `agm list` output

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
agm unarchive *[REDACTED_EMPLOYER]*
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
# ~/.config/csm/config.yaml
defaults:
  cleanup_threshold_days: 30    # Stopped → archive
  archive_threshold_days: 90    # Archived → delete
```

---

## Session Communication

### agm send

Send message/prompt to running session, interrupting active thinking.

**Usage**: `agm send <session-name> [flags]`

**Flags**:
- `--prompt <text>` - Prompt text to send
- `--prompt-file <path>` - File containing prompt (max 10KB)

**Examples**:

```bash
# Send inline prompt
agm send my-session --prompt "Please review the code"

# Send from file (large prompts)
agm send my-session --prompt-file ~/prompts/diagnosis.txt

# Send multi-line prompt
agm send research --prompt "Analyze the following:
1. Authentication flow
2. Error handling
3. Security concerns"
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

**Requirements**: Session must be running (active tmux session)

---

### agm reject

Reject permission prompt with custom reason.

**Usage**: `agm reject <session-name> [flags]`

**Flags**:
- `--reason <text>` - Rejection reason
- `--reason-file <path>` - File containing reason (max 10KB)

**Examples**:

```bash
# Reject with inline reason
agm reject my-session --reason "Use Read tool instead of cat"

# Reject with violation prompt
agm reject my-session --reason-file ~/prompts/VIOLATION-PROMPTS.md

# Reject with detailed feedback
agm reject task --reason "Please use absolute paths and separate tool calls"
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

### agm associate

Create session association from within running session.

**Usage**: `agm associate [flags]`

**Flags**:
- `--create <session-name>` - Create new session association
- `--list` - List all sessions
- `--status` - Show current session status

**Examples**:

```bash
# From within Claude session
agm associate --create my-project

# List sessions (verify association)
agm associate --list

# Check current status
agm associate --status
```

**Use Cases**:
- Manual session creation from within agent
- Verifying session associations
- Debugging association issues

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

**Flags**:
- `--validate` - Structural + functional testing
- `--fix` - Auto-fix issues (with --validate)
- `--json` - JSON output for scripting

**Structural Checks**:
- Agent installation (history files, binaries)
- tmux installation and socket status
- User lingering (session persistence after logout)
- Duplicate session directories
- Duplicate agent UUIDs
- Sessions with empty/missing UUIDs
- Session health (manifest validity, directory structure)

**Functional Validation** (--validate flag):
- Tests actual session resumability
- Classifies 6 resume error types:
  - Empty session-env directory
  - Version mismatch (agent CLI version changed)
  - Compacted JSONL (summaries not at end)
  - Missing JSONL file
  - CWD mismatch (working directory changed)
  - Lock contention (session locked by process)

**Auto-Fix Strategies** (--fix flag):
- Safe: Version mismatch (updates session-env manifest)
- Risky: JSONL reorder (with backup/restore, requires confirmation)

**Examples**:

```bash
# Structural checks only
agm doctor

# Structural + functional testing
agm doctor --validate

# Test and auto-fix issues
agm doctor --validate --fix

# JSON output for scripting
agm doctor --validate --json
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
  Fix: agm doctor --validate --fix

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
- `list` - List available backups
- `restore <backup-id>` - Restore from backup

**Examples**:

```bash
# List backups
agm backup list

# Restore specific backup
agm backup restore 2026-02-03-1430
```

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
- `clean` - Clean old logs
- `stats` - Show log statistics
- `thread <session>` - Show session thread
- `query <pattern>` - Query logs

**Examples**:

```bash
# Clean old logs
agm logs clean

# Show statistics
agm logs stats

# Show session thread
agm logs thread my-session

# Query logs
agm logs query "error"
```

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

## Testing

### agm test

Testing utilities for AGM development and debugging.

**Usage**: `agm test <subcommand>`

**Subcommands**:
- `create <name>` - Create isolated test session
- `send <name> <command>` - Send commands to test session
- `capture <name>` - Capture output from test session
- `cleanup <name>` - Cleanup test session

**Examples**:

```bash
# Create test session
agm test create my-test

# Send commands
agm test send my-test "agm associate --create test-project"

# Capture output
agm test capture my-test --lines 50

# Cleanup
agm test cleanup my-test
```

**Test Isolation**:
- Uses `/tmp/csm-test-*` directories
- Uses `csm-test-*` tmux sessions
- Completely isolated from production (`~/.claude-sessions/`)

**Common Patterns**:

```bash
# Test session lifecycle
agm test create lifecycle-test
agm test send lifecycle-test "agm new test-session --project ~/projects/test"
agm test capture lifecycle-test
agm test cleanup lifecycle-test

# JSON output for automation
agm test create api-test --json
agm test send api-test "agm list" --json
agm test cleanup api-test --json
```

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
CSM_DEBUG=true              # Enable debug logging (same as --debug)

# Accessibility
NO_COLOR=1                  # Disable colors (legacy, use --no-color flag)
CSM_SCREEN_READER=1        # Screen reader mode (legacy, use --screen-reader flag)

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

AGM uses `~/.config/csm/config.yaml` for configuration.

**Example Configuration**:

```yaml
defaults:
  interactive: true              # Enable interactive prompts
  auto_associate_uuid: true      # Auto-detect UUIDs
  confirm_destructive: true      # Confirm before delete/archive
  cleanup_threshold_days: 30     # Stopped → archive threshold
  archive_threshold_days: 90     # Archived → delete threshold

ui:
  theme: "csm"                   # UI theme (csm, csm-light, dracula, catppuccin)
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
- `csm` - High-contrast for dark terminals (default, WCAG AA compliant)
- `csm-light` - High-contrast for light terminals
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
agm new --agent gemini research-project

# Work in session...
# Exit session (Ctrl+D or exit)

# Resume later
agm resume research-project
# OR use fuzzy matching
agm research
```

### Multi-Agent Workflow

```bash
# Create sessions with different agents
agm new --agent claude code-task
agm new --agent gemini research-task
agm new --agent gpt brainstorm-task

# List all sessions
agm list

# Switch between sessions
agm resume code-task
agm resume research-task
```

### Cleanup Workflow

```bash
# List all sessions
agm list --all

# Archive completed sessions
agm archive old-task

# Interactive cleanup
agm clean

# Or manual cleanup
agm archive task1
agm archive task2
agm unarchive task1  # Restore if needed
```

### Debugging Workflow

```bash
# Check system health
agm doctor

# Detailed validation
agm doctor --validate

# Auto-fix issues
agm doctor --validate --fix

# Fix UUID associations
agm fix my-session
agm fix --all

# Unlock stuck session
agm unlock my-session
```

### Search and Restore Workflow

```bash
# Search for archived session
agm search "OAuth integration"

# Or use pattern matching
agm unarchive *oauth*

# Resume restored session
agm resume oauth-task
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
agm new [REDACTED_EMPLOYER]-vida-oauth
agm new personal-blog-rewrite
```

### Agent Selection

```bash
# Claude: Best for code and reasoning
agm new --agent claude code-review-auth

# Gemini: Best for research and long context
agm new --agent gemini research-competitors

# GPT: Best for brainstorming
agm new --agent gpt brainstorm-features
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
# Set theme in ~/.config/csm/config.yaml
ui:
  theme: "csm"  # or "csm-light" for light terminals
```

### Performance

```bash
# Skip health checks for faster commands
agm --skip-health-check list

# Use JSON output for scripting
agm list --json | jq '.[] | select(.status == "active")'

# Cache configuration for repeated commands
export CSM_CONFIG=~/.config/csm/config.yaml
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

**Agent not available**:
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

**Last Updated**: 2026-02-03
**AGM Version**: 3.0
