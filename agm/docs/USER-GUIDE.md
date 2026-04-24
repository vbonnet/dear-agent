# AGM User Guide

Comprehensive guide to using AGM (AI/Agent Session Manager) for managing AI agent sessions.

## Table of Contents

- [Introduction](#introduction)
- [Core Concepts](#core-concepts)
- [Session Lifecycle](#session-lifecycle)
- [Multi-Agent Support](#multi-agent-support)
- [Interactive Features](#interactive-features)
- [Advanced Usage](#advanced-usage)
- [Configuration](#configuration)
- [Best Practices](#best-practices)

## Introduction

AGM (AI/Agent Session Manager) provides smart session management for multiple AI agents with:

- **Unified interface** - Same commands work across Claude, Gemini, and GPT
- **Session persistence** - Resume conversations weeks or months later
- **Automatic tracking** - Auto-detects and associates agent UUIDs
- **Fuzzy search** - Find sessions even with typos
- **Batch operations** - Archive, restore, and cleanup multiple sessions

**Philosophy:** AGM handles session lifecycle so you can focus on your work with AI agents.

## Core Concepts

### Sessions

A **session** is a managed tmux session with an AI agent that includes:

- **Name** - Unique identifier (e.g., `coding-session`, `research-task`)
- **Agent** - Claude, Gemini, or GPT
- **Project** - Working directory context
- **UUID** - Agent-specific conversation identifier
- **Status** - active, stopped, or archived
- **Metadata** - Tags, description, created/updated timestamps

### Session Manifest

Each session has a manifest file (`~/.claude-sessions/<session-name>/manifest.json`) storing:

```json
{
  "version": 2,
  "session": {
    "name": "my-session",
    "agent": "claude"
  },
  "context": {
    "project": "~/projects/demo",
    "tags": ["work", "urgent"]
  },
  "agents": {
    "claude": {
      "uuid": "abc123..."
    }
  },
  "lifecycle": "active",
  "timestamps": {
    "created": "2026-02-01T10:00:00Z",
    "updated": "2026-02-03T15:30:00Z"
  }
}
```

### Session States

```
┌─────────┐
│ created │ ──► New session, tmux running
└─────────┘
     │
     ▼
┌─────────┐
│ active  │ ──► tmux session attached (you're in it)
└─────────┘
     │
     ▼
┌─────────┐
│ stopped │ ──► tmux session detached or killed
└─────────┘
     │
     ▼
┌──────────┐
│ archived │ ──► Marked as archived, can be restored
└──────────┘
```

### UUID Association

**UUID** (Universally Unique Identifier) links AGM sessions to agent conversations:

- **Claude:** Session ID from `~/.claude/history.jsonl`
- **Gemini:** Conversation ID from Gemini API
- **GPT:** Thread ID from OpenAI API

**Auto-detection:**
- AGM monitors agent output during session creation
- Extracts UUID from history files or API responses
- Associates UUID with session manifest
- Confidence levels: High (auto-apply), Medium (confirm), Low (suggest)

**Manual association:** Use `agm fix` if auto-detection fails

## Session Lifecycle

### Creating Sessions

#### Interactive Creation

```bash
# Interactive form prompts for:
# - Session name
# - Agent (claude/gemini/gpt)
# - Project directory
# - Optional description
agm new
```

#### Quick Creation

```bash
# Create with defaults
agm new my-session

# Specify agent
agm new --harness gemini-cli research-task

# Specify project directory
agm new coding-session --project ~/projects/myapp

# Add tags
agm new work-task --tags work,urgent

# Full specification
agm new my-session \
  --harness claude-code \
  --project ~/projects/demo \
  --tags code,review \
  --description "Code review session"
```

#### Creation Process

1. **Validation** - Name uniqueness, directory exists
2. **Manifest creation** - Generate session manifest
3. **tmux session** - Create tmux session with agent
4. **UUID detection** - Auto-detect and associate UUID
5. **Initialization** - Run agent-specific hooks
6. **Attachment** - Attach to session (or detached mode)

**Flags:**
- `--detached` - Create but don't attach
- `--no-uuid` - Skip UUID auto-detection

### Resuming Sessions

#### Smart Resume (No Arguments)

```bash
# Shows interactive picker if multiple sessions
# Creates new session if none exist
agm
```

**Behavior:**
- 0 sessions → Prompt to create
- 1 session → Auto-resume
- 2+ sessions → Interactive picker

#### Resume by Name

```bash
# Exact match
agm resume my-session

# Fuzzy matching (typo-tolerant)
agm my-ses          # Matches "my-session"
agm resrch          # Matches "research-task"
```

**Fuzzy matching:**
- Levenshtein distance algorithm
- 0.6 similarity threshold
- "Did you mean?" prompt for close matches

#### Interactive Picker

```bash
# Show all sessions
agm list

# Interactive selection with arrow keys
# Press Enter to resume
```

**Picker features:**
- Fuzzy search (type to filter)
- Color-coded status
- Project path display
- Last updated timestamps

### Listing Sessions

#### Basic Listing

```bash
# Active and stopped sessions (default)
agm list

# Include archived sessions
agm list --all

# Only archived sessions
agm list --archived
```

#### Output Formats

```bash
# Table format (default, human-readable)
agm list

# JSON format (machine-readable)
agm list --format=json

# Simple list (session names only)
agm list --format=simple
```

**Table output:**
```
NAME              STATUS    AGENT   PROJECT                 UPDATED
coding-session    active    claude  ~/projects/myapp       2 minutes ago
research-task     stopped   gemini  ~/research             1 hour ago
old-session       archived  claude  ~/projects/legacy      30 days ago
```

**JSON output:**
```json
[
  {
    "name": "coding-session",
    "status": "active",
    "agent": "claude",
    "project": "~/projects/myapp",
    "updated": "2026-02-03T15:30:00Z"
  }
]
```

### Archiving Sessions

#### Archive Single Session

```bash
# Archive by name
agm archive my-session

# Archive with confirmation prompt
agm archive my-session
# ⚠ Archive session 'my-session'? (y/n):
```

**What happens:**
- Session marked as archived (manifest updated)
- tmux session killed if running
- Files remain in `~/.claude-sessions/<session-name>/`
- Can be restored with `agm unarchive`

#### Batch Cleanup

```bash
# Interactive multi-select cleanup
agm clean
```

**Smart suggestions:**
- **Stopped >30 days** → Suggested for archival
- **Archived >90 days** → Suggested for deletion
- Color-coded recommendations
- Multi-select interface
- Confirmation before action

**Customizable thresholds:**
```yaml
# ~/.config/agm/config.yaml
defaults:
  cleanup_threshold_days: 30    # stopped → archive
  archive_threshold_days: 90    # archived → delete
```

### Restoring Sessions

#### Restore by Pattern

```bash
# Exact match - auto-restore
agm unarchive my-session

# Pattern match - interactive picker if multiple
agm unarchive *acme*          # Matches all with "acme"
agm unarchive "session-202?"    # Wildcard year
agm unarchive "*"               # All archived (picker)
```

**Glob patterns:**
- `*` - Match any characters
- `?` - Match single character
- `[abc]` - Match any of a, b, c
- `[0-9]` - Match any digit

#### Restore Workflow

1. **Pattern matching** - Find archived sessions
2. **Selection** - Auto-restore if 1 match, picker if multiple
3. **Manifest update** - Mark as active
4. **Ready to resume** - Use `agm resume <session-name>`

### Searching Sessions

#### Semantic Search (AI-Powered)

```bash
# Find by conversation content
agm search "that discussion about OAuth"
agm search "debugging the database connection"
agm search "last week's code review"
```

**Features:**
- Powered by Google Vertex AI (Claude Haiku)
- Searches `~/.claude/history.jsonl` conversation history
- Interactive selection for multiple results
- Auto-restores selected session
- Results cached for 5 minutes
- Rate limited: 10 searches/minute

**Authentication required:**
```bash
# Configure Google Cloud
gcloud auth application-default login
export GOOGLE_CLOUD_PROJECT=your-project-id
```

**Flags:**
- `--max-results <N>` - Limit results (default: 10)

#### Pattern-Based Search

```bash
# List and filter
agm list | grep research
agm list --all | grep 2026-01

# Unarchive with patterns
agm unarchive *research*
```

## Multi-Agent Support

### Agent Selection

#### Create with Specific Agent

```bash
# Claude (default)
agm new --harness claude-code coding-session

# Gemini
agm new --harness gemini-cli research-task

# GPT
agm new --harness codex-cli chat-session
```

#### Agent Auto-Detection

**Current:** Manual selection required (`--harness` flag)

**Future (AGENTS.md routing):**
```yaml
# ~/.config/agm/AGENTS.md
default_agent: claude
preferences:
  - keywords: [research, summarize, analyze]
    agent: gemini
  - keywords: [code, debug, refactor]
    agent: claude
  - keywords: [chat, brainstorm, quick]
    agent: gpt
```

**Then:**
```bash
agm new research-papers        # Auto-selects gemini
agm new debug-api             # Auto-selects claude
agm new quick-question        # Auto-selects gpt
```

**Status:** Infrastructure complete, integration pending

### Agent Comparison

| Feature | Claude | Gemini | GPT |
|---------|--------|--------|-----|
| Context Window | 200K tokens | 1M tokens | 128K tokens |
| Best For | Code, reasoning | Research, summary | Chat, general |
| Speed | Moderate | Fast | Fast |
| Reasoning Depth | Excellent | Moderate | Good |

**Detailed:** See [AGENT-COMPARISON.md](AGENT-COMPARISON.md)

### Command Translation

AGM provides unified commands across agents using `CommandTranslator`:

**Supported commands:**
- `RenameSession` - Rename agent conversation
- `SetDirectory` - Set working directory context
- `RunHook` - Execute initialization hook

**Implementation:**
- **Claude:** tmux send-keys (slash commands like `/rename`)
- **Gemini:** API calls (UpdateConversationTitle, UpdateMetadata)
- **GPT:** API calls (thread metadata updates)

**Graceful degradation:** Unsupported commands return `ErrNotSupported`

**Documentation:** See [COMMAND-TRANSLATION-DESIGN.md](COMMAND-TRANSLATION-DESIGN.md)

### Switching Agents

```bash
# Use different agents for different tasks
agm new --harness claude-code implement-feature
agm new --harness gemini-cli research-approach
agm new --harness codex-cli brainstorm-ideas

# Resume any session (agent auto-detected from manifest)
agm resume implement-feature   # Resumes with Claude
agm resume research-approach   # Resumes with Gemini
```

**Agent persistence:** Stored in session manifest, survives resume

## Interactive Features

### Session Picker

**Activation:**
- Run `agm` with no arguments
- Multiple sessions exist

**Features:**
- Fuzzy search (type to filter)
- Arrow keys to navigate
- Color-coded status (green=active, yellow=stopped, gray=archived)
- Project path display
- Last updated timestamps
- Cursor symbol + bold text for selection

**Accessibility:**
- `--no-color` flag disables colors
- `--screen-reader` flag converts symbols to text labels
- High-contrast theme (WCAG AA compliant)

### Forms & Prompts

#### New Session Form

**Fields:**
1. **Session name** - Alphanumeric, hyphens, underscores
2. **Agent** - claude/gemini/gpt
3. **Project directory** - Browse or type path
4. **Description** - Optional purpose

**Validation:**
- Name uniqueness
- Directory existence
- Agent availability

#### Confirmation Dialogs

**Used for:**
- Destructive operations (delete, archive)
- Risky actions (JSONL reorder)
- Multi-step workflows

**Example:**
```
⚠ Archive session 'my-session'?

This will:
- Mark session as archived
- Kill tmux session if running
- Can be restored with 'agm unarchive'

Continue? (y/n):
```

### Batch Operations

#### Multi-Select Cleanup

```bash
agm clean
```

**Interface:**
- Checkbox list of sessions
- Space to toggle selection
- Enter to confirm
- ESC to cancel

**Smart grouping:**
1. **Archive candidates** (stopped >30 days)
2. **Delete candidates** (archived >90 days)
3. **Other sessions**

**Confirmation:**
```
Selected actions:
- Archive: 3 sessions
- Delete: 2 sessions

Proceed? (y/n):
```

## Advanced Usage

### UUID Management

#### Auto-Detection

**Automatic during creation:**
```bash
# AGM monitors agent output and auto-detects UUID
agm new my-session
# ✓ Session created
# ✓ UUID detected: abc123...
```

**Confidence levels:**
- **High** (<2.5 min old) → Auto-applied
- **Medium** (2.5-5 min old) → Confirmation prompt
- **Low** (>5 min old) → Listed in suggestions

#### Manual Association

```bash
# Scan all unassociated sessions
agm fix

# Fix specific session
agm fix my-session

# Auto-fix all (high confidence only)
agm fix --all

# Clear UUID association
agm fix --clear my-session
```

**UUID suggestions:**
1. Auto-detected from history (high confidence)
2. Recent UUIDs from `~/.claude/history.jsonl`
3. Manual entry option

**Suggestion display:**
```
UUID suggestions for 'my-session':

1. abc123... (high confidence)
   Project: ~/projects/demo
   Timestamp: 2 minutes ago

2. def456... (medium confidence)
   Project: ~/projects/demo
   Timestamp: 5 minutes ago

3. Manual entry

Select UUID (1-3):
```

### Sending Messages

#### Send Prompt to Session

```bash
# Send inline prompt
agm session send my-session --prompt "Please review the code"

# Send from file
agm session send my-session --prompt-file ~/prompts/review.txt
```

**Features:**
- Auto-interrupts thinking state (sends ESC first)
- Literal mode (tmux `-l` flag)
- Supports up to 10KB prompt files
- Executes immediately (not queued)

**Use cases:**
- Automated recovery of stuck sessions
- Diagnosis prompts for hangs
- Batch message delivery

**Requirements:**
- Session must be running (active tmux session)
- Prompt executed immediately

#### Reject Permission Prompt

```bash
# Reject with inline reason
agm session reject my-session --reason "Use Read tool instead of cat"

# Reject with violation prompt from file
agm session reject my-session --reason-file ~/prompts/VIOLATION.md
```

**Workflow:**
1. Navigate to "No" option (Down arrow)
2. Add additional instructions (Tab)
3. Send rejection reason (literal mode)
4. Submit (Enter)

**Features:**
- Automated navigation
- Custom reasoning
- Smart extraction (extracts "## Standard Prompt" from markdown)
- Literal mode for reliable transmission

**Use cases:**
- Rejecting tool usage violations
- Providing feedback on permission denials
- Automated enforcement of coding standards

### Health Checks

The `doctor` command provides two modes of health checking:

1. **Quick Health Check** (default): Fast structural checks (~1-5 seconds)
2. **Deep Validation** (`--validate`): Thorough functional testing (~5-30 seconds per session)

#### When to Use Each Mode

**Use Quick Health Check (`agm doctor`) when:**
- Running daily health checks
- Quick system overview needed
- Checking for configuration issues
- Verifying installation status
- Performance matters (fast exit)

**Use Deep Validation (`agm doctor --validate`) when:**
- Debugging session resume failures
- Preparing for production deployment
- Running automated CI/CD tests
- Need to verify session resumability
- Auto-fixing issues with `--apply-fixes`

#### Quick Health Check (Default)

```bash
# Fast structural checks only
agm doctor
```

**Checks performed:**
- Claude installation (history.jsonl exists)
- tmux installation and socket status
- User lingering (session persistence)
- Duplicate session directories
- Duplicate Claude UUIDs
- Sessions with empty/missing UUIDs
- Session health (manifest validity)

**Performance:** ~1-5 seconds total

**Example output:**
```
=== AGM Health Check ===

✓ Claude history found
✓ tmux installed: tmux 3.3a
✓ tmux socket active: /tmp/tmux-1000/default
✓ User lingering enabled
✓ Found 224 session manifests

--- Session Health ---
✓ All sessions healthy

✓ System is healthy
```

#### Deep Validation (Functional Testing)

```bash
# Structural + functional tests
agm doctor --validate

# Test and auto-fix issues
agm doctor --validate --apply-fixes

# JSON output for scripting
agm doctor --validate --json
```

**Performance:** ~5-30 seconds per session (depends on number of sessions)

**Functional tests:**
- Session resumability (creates test tmux session)
- Resume error classification (6 error types)
- Auto-fix strategies for common issues

**Resume error types:**
1. Empty session-env directory
2. Version mismatch (Claude CLI version changed)
3. Compacted JSONL (summaries not at end)
4. Missing JSONL file
5. CWD mismatch (working directory changed)
6. Lock contention (session locked)

**Auto-fix strategies:**
- **Safe:** Version mismatch (updates manifest)
- **Risky:** JSONL reorder (with backup, requires confirmation)

#### Agent-Specific Validation

```bash
# Validate Gemini environment
agm doctor gemini

# Generate .envrc template
agm doctor gemini --generate-envrc

# Interactive setup wizard
agm doctor gemini --fix
```

**Validation checks:**
- Required environment variables
- Command availability
- Conflict detection (Vertex AI vs API key)
- Environment source analysis

**Example output:**
```
╔══════════════════════════════════════╗
║ Gemini Environment Validation        ║
╚══════════════════════════════════════╝

Command Checks:
  ✓ gemini CLI installed

Environment Variables:
  ❌ GEMINI_API_KEY not set
     Required for Gemini API authentication

  ❌ GOOGLE_GENAI_USE_VERTEXAI=true
     Should be false for API key mode

Conflicts Detected:
  ⚠️  GOOGLE_CLOUD_PROJECT set
     Can be ignored for API key mode

Recommended Fixes:
  1. Use direnv (per-project config)
  2. Use ~/.bashrc (global config)
  3. Per-session export (temporary)

Run 'agm doctor gemini --fix' for interactive setup.
```

**Documentation:** See [agm-environment-management-spec.md](agm-environment-management-spec.md)

### Testing & Development

#### Test Commands

```bash
# Create isolated test session
agm test create my-test

# Send commands to test session
agm test send my-test "agm new test-session"

# Capture output
agm test capture my-test --lines 50

# Cleanup
agm test cleanup my-test
```

**Test isolation:**
- Uses `/tmp/agm-test-*` directories
- Separate tmux sessions (`agm-test-*`)
- Doesn't affect production state

**Common patterns:**
```bash
# Test session lifecycle
agm test create lifecycle-test
agm test send lifecycle-test "agm list"
agm test capture lifecycle-test
agm test cleanup lifecycle-test

# JSON output for automation
agm test create api-test --json
agm test send api-test "agm list" --json
agm test cleanup api-test --json
```

## Configuration

### Configuration File

**Location:** `~/.config/agm/config.yaml`

```yaml
defaults:
  interactive: true                # Enable interactive prompts
  auto_associate_uuid: true        # Auto-detect UUIDs
  confirm_destructive: true        # Confirm before delete/archive
  cleanup_threshold_days: 30       # Stopped → archive
  archive_threshold_days: 90       # Archived → delete

ui:
  theme: "agm"                     # UI theme (agm, dracula, catppuccin)
  picker_height: 15                # Session picker height
  show_project_paths: true         # Show full paths
  show_tags: true                  # Show session tags
  fuzzy_search: true               # Enable fuzzy matching

advanced:
  tmux_timeout: "5s"               # Tmux command timeout
  health_check_cache: "5s"         # Health check cache duration
  lock_timeout: "30s"              # Lock acquisition timeout
  uuid_detection_window: "5m"      # UUID detection time window
```

### Environment Variables

#### Agent API Keys

```bash
# Claude
export ANTHROPIC_API_KEY="sk-ant-..."

# Gemini
export GEMINI_API_KEY="AIza..."
export GOOGLE_GENAI_USE_VERTEXAI=false

# GPT
export OPENAI_API_KEY="sk-..."
```

#### AGM Behavior

```bash
# Disable colors (accessibility)
export NO_COLOR=1
# Or use flag: agm list --no-color

# Screen reader support
export AGM_SCREEN_READER=1
# Or use flag: agm list --screen-reader

# Google Cloud (for semantic search)
export GOOGLE_CLOUD_PROJECT=your-project-id
export GOOGLE_CLOUD_LOCATION=us-central1
```

### Themes

**Available themes:**
- `agm` - High-contrast for dark terminals (default, WCAG AA)
- `agm-light` - High-contrast for light terminals
- `dracula` - Dracula color scheme
- `catppuccin` - Catppuccin color scheme
- `charm` - Charm library default
- `base` - Minimal styling

**Configuration:**
```yaml
ui:
  theme: "agm"
```

**Accessibility:**
- `agm` and `agm-light` are WCAG AA compliant (4.5:1 contrast)
- Selection indicated by color + cursor + bold text
- Semantic colors (green=success, red=error, yellow=warning)

**Documentation:** See [ACCESSIBILITY.md](ACCESSIBILITY.md)

## Best Practices

### Naming Conventions

**Good names:**
- `coding-session` - Clear, descriptive
- `research-ai-papers` - Specific purpose
- `debug-api-issue` - Action-oriented
- `project-alpha-dev` - Project context

**Avoid:**
- `session1`, `test`, `temp` - Too generic
- `asdf`, `foo`, `bar` - Non-descriptive
- `my-super-long-session-name-that-is-hard-to-type` - Too long

**Conventions:**
- Use lowercase
- Use hyphens (not underscores or spaces)
- Be specific but concise
- Include project or purpose

### Session Organization

#### Use Tags

```bash
# Create with tags
agm new work-task --tags work,urgent,backend

# List by tags (future)
agm list --tag work
```

**Tag categories:**
- **Type:** code, research, chat, debug
- **Priority:** urgent, normal, low
- **Project:** project-alpha, project-beta
- **Status:** wip, review, blocked

#### Use Project Context

```bash
# Always specify project directory
agm new coding-session --project ~/projects/myapp

# Or navigate first
cd ~/projects/myapp
agm new coding-session
```

**Benefits:**
- Agent has correct working directory
- Easy to identify session purpose
- Better session organization

#### Regular Cleanup

```bash
# Weekly cleanup
agm clean

# Or automatic with thresholds
# ~/.config/agm/config.yaml
defaults:
  cleanup_threshold_days: 7   # Aggressive cleanup
  archive_threshold_days: 30  # Quick deletion
```

### Multi-Agent Workflows

#### Specialized Sessions

```bash
# Design phase: Use GPT for brainstorming
agm new design-phase --harness codex-cli

# Research phase: Use Gemini for large context
agm new research-phase --harness gemini-cli

# Implementation: Use Claude for code
agm new implementation --harness claude-code
```

#### Agent Handoff

**Pattern:** Research → Design → Implement

```bash
# 1. Research with Gemini
agm new research-microservices --harness gemini-cli
# ... research multiple papers ...
agm archive research-microservices

# 2. Design with GPT
agm new design-architecture --harness codex-cli
# ... brainstorm architecture ...
agm archive design-architecture

# 3. Implement with Claude
agm new implement-service --harness claude-code
# ... write code ...
```

### UUID Management

#### Proactive Association

```bash
# After creating session, verify UUID
agm list --format=json | jq '.[] | select(.name=="my-session") | .uuid'

# If missing, fix immediately
agm fix my-session
```

#### Periodic Audits

```bash
# Check for missing UUIDs
agm doctor

# Fix all high-confidence associations
agm fix --all
```

### Backup & Recovery

#### Session Manifests

**Location:** `~/.claude-sessions/<session-name>/manifest.json`

**Backup:**
```bash
# Backup all manifests
tar -czf agm-sessions-backup-$(date +%Y%m%d).tar.gz ~/.claude-sessions/

# Restore
tar -xzf agm-sessions-backup-20260203.tar.gz -C ~/
```

#### Agent History

**Claude:**
```bash
# Backup history
cp ~/.claude/history.jsonl ~/.claude/history.jsonl.backup

# Restore
cp ~/.claude/history.jsonl.backup ~/.claude/history.jsonl
```

**Gemini/GPT:** Stored remotely, no local backup needed

### Performance Optimization

#### Cache Configuration

```yaml
advanced:
  health_check_cache: "5s"      # Cache health checks
  uuid_detection_window: "5m"   # Detection time window
```

**Tradeoffs:**
- Longer cache → Faster, but stale data
- Shorter cache → Fresher data, but slower

#### Batch Operations

```bash
# Instead of multiple individual commands
agm archive session1
agm archive session2
agm archive session3

# Use batch cleanup
agm clean
# (multi-select session1, session2, session3)
```

### Security

#### API Keys

**Never commit:**
```bash
# Add to .gitignore
echo '.envrc' >> .gitignore
echo '.env' >> .gitignore
```

**Use password managers:**
```bash
# With pass
export GEMINI_API_KEY=$(pass show gemini-api-key)

# With vault
export GEMINI_API_KEY=$(vault kv get -field=key secret/gemini)
```

**Environment-specific:**
```bash
# Per-project .envrc (with direnv)
# ~/projects/myapp/.envrc
export GEMINI_API_KEY=$(pass show gemini-dev)

# ~/projects/production/.envrc
export GEMINI_API_KEY=$(pass show gemini-prod)
```

#### Session Data

**Contains sensitive info:**
- Conversation history
- Project paths
- API keys (if accidentally pasted)

**Protection:**
```bash
# Restrict permissions
chmod 700 ~/.claude-sessions/

# Regular cleanup
agm clean  # Archive old sessions
```

## Next Steps

- **Examples:** See [EXAMPLES.md](EXAMPLES.md) for real-world scenarios
- **CLI Reference:** See [CLI-REFERENCE.md](CLI-REFERENCE.md) for complete command documentation
- **FAQ:** See [FAQ.md](FAQ.md) for common questions
- **Troubleshooting:** See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for issue resolution

---

**Last updated:** 2026-02-03
**AGM Version:** 3.0
**Maintained by:** Foundation Engineering
