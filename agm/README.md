# AI/Agent Session Manager (AGM)

Smart session management for AI agents (Claude, Gemini, Codex) with interactive TUI, multi-agent support, and automatic session tracking.

## Architecture

### C4 Component Diagram

![AGM Component Diagram](diagrams/rendered/c4-component-agm.png)

**Component Architecture** showing the multi-CLI session coordination system:

- **CommandTranslator**: Translates AGM commands to CLI-specific syntax
- **AdapterRegistry**: Manages adapters for Claude, Gemini, Codex, and OpenCode
- **Dolt Storage**: Persists session metadata in versioned SQL database (Git-like commits)
- **CoordinationDaemon**: Background process for session lifecycle management
- **MessageQueue**: Inter-session communication and event routing
- **SessionManager**: Central orchestration of session operations

**Diagram Source**: `diagrams/c4-component-agm.d2`

## Multi-Agent Quick Start

```bash
# Create session with specific harness
agm new --harness claude-code my-coding-session   # Claude Code: code, reasoning
agm new --harness gemini-cli research-task        # Gemini CLI: research, 1M context
agm new --harness codex-cli chat-session          # Codex CLI: OpenAI API (GPT-4, GPT-3.5)
agm new --harness opencode-cli dev-session        # OpenCode CLI: native SSE monitoring

# Resume any session (agent auto-detected)
agm resume my-coding-session

# List all sessions (shows agents)
agm list
```

**Session Naming:** Use alphanumeric, dashes, and underscores only. Avoid dots (`.`), colons (`:`), and spaces - they cause lookup failures. See [Session Naming Guide](docs/SESSION-NAMING-GUIDE.md).

## Test Sessions

Create isolated test sessions that won't clutter your production workspace:

```bash
# Isolated test session (recommended for experiments)
agm new --test quick-experiment

# Test workspace session (for multi-day test projects)
agm new test-project --workspace=test

# Production session with "test" in name (non-interactive/script override)
agm new auth-testing-work --allow-test-name

# OR interactively: omit the flag and select option 3 when prompted
agm new auth-testing-work
# > Use --test flag (required for test scenarios)
#   Cancel and rename to non-test name
# > Create anyway (production session, human override)  ← select this
```

**Key Features:**
- `--test` flag creates sessions in `~/sessions-test/` (isolated from production)
- Automatic pattern detection warns when session names contain "test"
- Interactive prompt offers 3 choices: use `--test`, cancel/rename, or force production creation
- `--allow-test-name` flag provides the same production override for scripts/non-interactive use
- Cleanup command removes orphaned test sessions safely

**Learn More:** See [Test Session Guide](docs/TEST-SESSION-GUIDE.md) for:
- Best practices and examples
- Test pattern detection explained
- Migration guide for existing test sessions
- Troubleshooting common issues

## Choosing a Harness

Not sure which agent to use?

- **Claude** (Anthropic): Best for code, long context (200K), multi-step reasoning
- **Gemini** (Google): Best for research, summarization, massive context (1M tokens)
- **Codex** (OpenAI API): Best for GPT-4 access, API-based workflows, Azure OpenAI - [Setup Guide](docs/agents/codex.md)
- **OpenCode** (Open Source): Native SSE monitoring, real-time state detection

**Detailed comparison**: See [docs/AGENT-COMPARISON.md](docs/AGENT-COMPARISON.md) for:
- Feature comparison table (context windows, strengths, limitations)
- Use case guide (when to use each agent)
- Quick decision tree (choose agent in <2 minutes)
- Command translator support levels

**New to AGM?** See [docs/MIGRATION-CLAUDE-MULTI.md](docs/MIGRATION-CLAUDE-MULTI.md) if transitioning from Claude-only sessions.

### Harness Routing with AGENTS.md

**Status: Infrastructure Complete, Integration Pending**

Automate agent selection based on session names using AGENTS.md configuration files.

**Current state:**
- ✅ `internal/agents` package implemented (YAML parsing, keyword matching, multi-path detection)
- ⚠️ Integration with `agm new` pending (requires agent selection support in AGM core)
- ℹ️ Manual harness selection works: `agm new --harness <harness> <session-name>`

**Example future `AGENTS.md` (not yet active)**:
```yaml
default_agent: claude
preferences:
  - keywords: [creative, design, brainstorm]
    agent: gemini
  - keywords: [code, debug, refactor]
    agent: claude
```

**Once integrated** (targeted for future release):
```bash
agm new creative-project      # Would auto-select gemini (matches "creative")
agm new code-refactor         # Would auto-select claude (matches "code")
agm new random-task           # Would use claude (default, no keyword match)
```

**Workaround:** Use explicit `--harness` flag until integration complete:
```bash
agm new creative-project --harness gemini-cli    # Explicit harness selection (works now)
```

See `docs/AGENTS.md.example` for full configuration spec. Integration tracked in project roadmap.

---

## Documentation

**🚀 Start Here:**
- **[Documentation Index](docs/INDEX.md)** - Complete navigation hub with learning paths
- **[Quick Reference](docs/AGM-QUICK-REFERENCE.md)** - One-page cheat sheet with essential commands
- **[Getting Started](docs/GETTING-STARTED.md)** - Installation and first steps (10 minutes)

**📚 Core Guides:**
- **[Command Reference](docs/AGM-COMMAND-REFERENCE.md)** - Complete CLI reference with all commands and examples
- **[User Guide](docs/USER-GUIDE.md)** - Comprehensive usage guide and workflows
- **[Examples](docs/EXAMPLES.md)** - 30+ real-world scenarios across 7 categories
- **[Agent Comparison](docs/AGENT-COMPARISON.md)** - Choose the right agent for your use case
- **[Session Naming Guide](docs/SESSION-NAMING-GUIDE.md)** - Safe session names and character rules

**🔧 Technical Documentation:**
- **[Architecture Overview](docs/ARCHITECTURE.md)** - Complete system architecture and design
- **[API Reference](docs/API-REFERENCE.md)** - Developer API for Go packages and interfaces
- **[BDD Catalog](docs/BDD-CATALOG.md)** - Living documentation (8 feature files, 20+ scenarios)
- **[Dolt Storage](internal/dolt/README.md)** - Production Dolt database backend (Git-like versioned SQL)
- **[Dolt Setup](internal/dolt/SETUP.md)** - Dolt server setup and testing instructions

**🔄 Migration & Troubleshooting:**
- **[Migration Guide](docs/AGM-MIGRATION-GUIDE.md)** - Version migration (validation, rollback)
- **[Troubleshooting](docs/TROUBLESHOOTING.md)** - Common issues and solutions
- **[FAQ](docs/FAQ.md)** - Frequently asked questions

**♿ Accessibility:**
- **[Accessibility Guide](docs/ACCESSIBILITY.md)** - WCAG AA compliance, screen readers, high contrast

**For detailed documentation navigation**, see **[Documentation Index](docs/INDEX.md)** with:
- Quick navigation by role (Users, Developers, Contributors)
- Documentation by topic (Installation, Usage, Architecture, API)
- 5 learning paths (10 minutes to 3 hours)
- Complete documentation list (32 files)

---

## Features

### 🎯 Smart Session Management
- **Interactive picker** - Beautiful TUI for session selection
- **Fuzzy matching** - Typo-tolerant session names ("my-ses" → "my-session")
- **Auto UUID detection** - Hybrid detection from `~/.claude/history.jsonl`
- **Batch operations** - Multi-select cleanup for archival/deletion, bulk session resume (`agm sessions resume-all`)
- **Pattern-based restore** - Glob patterns for archived session recovery (`agm unarchive *acme*`)
- **AI-powered search** - Semantic search using Google Vertex AI (`agm search "OAuth work"`)
- **Git auto-commit** - Automatic git commits for manifest changes (create, archive, associate, etc.)
- **Post-reboot recovery** - Resume all stopped sessions after machine restart with orchestrator coordination

### 🔌 Command Translation (Multi-Agent)

AGM provides a unified command interface across different AI agents using the `CommandTranslator` abstraction. This allows generic operations (rename session, set directory, run hooks) to work across Claude, Gemini, and future agents.

**Supported Commands:**
- **RenameSession**: Rename agent session/conversation
- **SetDirectory**: Set working directory context
- **RunHook**: Execute initialization hook (agent-dependent)

**Example Usage:**
```go
import "github.com/vbonnet/dear-agent/agm/internal/command"

// Create translator (Gemini example)
client := gemini.NewClient(apiKey)
translator := command.NewGeminiTranslator(client)

// Execute command with timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err := translator.RenameSession(ctx, sessionID, "new-name")
if errors.Is(err, command.ErrNotSupported) {
    // Command not supported - graceful degradation
} else if err != nil {
    // Handle error
}
```

**Supported Agents:**
- **Claude**: Commands sent via tmux (slash commands like `/rename`)
- **Gemini**: Commands sent via API calls (UpdateConversationTitle, UpdateMetadata)

See `internal/command/` package documentation for implementation details.

### 🚀 Quick Start

```bash
# Smart resume/create (no args needed!)
agm                    # Shows picker if multiple sessions, creates if none

# Named session (with fuzzy matching)
agm my-session         # Exact match → resume
agm my-ses            # Fuzzy match → "did you mean?"
agm new-name          # No match → offer to create

# Explicit commands
agm new               # Interactive form for new session
agm list              # List all sessions with status
agm clean             # Batch cleanup (archive/delete)
agm fix               # Fix UUID associations

# Bulk operations
agm sessions resume-all                    # Resume all stopped sessions
agm sessions resume-all --workspace alpha  # Resume sessions in workspace
agm sessions resume-all --dry-run          # Preview without executing

# Archive management
agm unarchive *pattern*         # Restore archived sessions by pattern
agm search "semantic query"     # AI-powered semantic search
```

## Installation

```bash
go install github.com/vbonnet/dear-agent/agm/cmd/agm@latest
```

### Bash Completion (Recommended)

Enable tab completion for command and session names:

```bash
# Add to ~/.bashrc (or run manually for current shell)
if command -v agm &> /dev/null; then
    source <(agm completion bash)
fi

# Reload shell
source ~/.bashrc
```

**For zsh users:**
```bash
# Add to ~/.zshrc
if command -v agm &> /dev/null; then
    source <(agm completion zsh)
fi
```

**Features:**
- Command completion: `agm k<TAB>` → `agm kill`
- Session name completion: `agm kill <TAB>` → shows active sessions
- Flag completion: `agm --<TAB>` → shows available flags
- Dynamic: Generated from the binary (always up-to-date)

## Commands

### Primary Command: `agm [session-name]`

Smart behavior based on context:

**No session name provided:**
- Multiple sessions exist → Shows interactive picker
- No sessions exist → Prompts to create new session

**Session name provided:**
- Exact match → Resumes that session
- Fuzzy matches found → "Did you mean" prompt
- No match → Offers to create new session

### `agm new [session-name]`

Create new session with interactive form:
- Session name validation (alphanumeric, hyphens, underscores)
- Project directory selection
- **Workspace detection** - Auto-detects workspace from project directory or allows interactive selection
- Optional purpose/description
- Auto-creates tmux session + starts Claude
- **Sequenced initialization** - Sends `/rename` to generate UUID, then `/agm:assoc` (via tmux control mode)
- Auto-associates UUID via history detection
- Reliable UUID capture with 95%+ success rate

**Workspace Flags**:
```bash
# Auto-detect workspace from current directory
agm new --workspace=auto my-session

# Explicitly set workspace
agm new --workspace=oss my-session
agm new --workspace=acme my-session

# Interactive selection if ambiguous
agm new my-session    # Prompts if multiple workspaces match
```

### `agm list [flags]`

List sessions with rich formatting:

```bash
agm list                 # Active/stopped sessions (table format)
agm list --all           # Include archived
agm list --archived      # Only archived
agm list --output json   # Machine-readable output
```

**Output formats:**
- `table` (default) - Formatted table with status, project, workspace, updated time
- `json` - Machine-readable JSON
- `simple` - Simple name list

**Note**: The workspace column shows which workspace each session belongs to (based on project directory path)

### `agm clean`

Interactive batch cleanup with smart suggestions:

- **Stopped sessions >30 days** - Suggested for archival
- **Archived sessions >90 days** - Suggested for deletion
- Multi-select interface with confirmation
- Thresholds customizable in `~/.config/agm/config.yaml`

### `agm fix [session-name]`

Manual UUID association management:

```bash
agm fix                  # Scan all unassociated sessions
agm fix my-session       # Fix specific session with suggestions
agm fix --all            # Auto-fix all (high confidence only)
agm fix --clear my-sess  # Remove UUID association
```

**UUID Suggestions:**
1. Auto-detected from history (high confidence)
2. Recent UUIDs from `~/.claude/history.jsonl`
3. Manual entry option

### `agm doctor [flags]`

Health check and validation for AGM and Claude sessions:

```bash
agm doctor                    # Structural checks only
agm doctor --validate         # Structural + functional testing
agm doctor --validate --fix   # Test and auto-fix issues
agm doctor --validate --json  # JSON output for scripting
```

**Structural checks:**
- Claude installation (history.jsonl exists)
- tmux installation and socket status
- User lingering (session persistence after logout)
- Duplicate session directories (old vs new format)
- Duplicate Claude UUIDs across sessions
- Sessions with empty/missing UUIDs
- Session health (manifest validity, directory structure)

**Functional validation (--validate flag):**
- Tests actual session resumability (creates test tmux session, attempts resume)
- Classifies 6 resume error types:
  - Empty session-env directory
  - Version mismatch (Claude CLI version changed)
  - Compacted JSONL (conversation summaries not at end)
  - Missing JSONL file
  - CWD mismatch (working directory changed)
  - Lock contention (session locked by another process)
- Auto-fix strategies (--fix flag):
  - Safe: Version mismatch (updates session-env manifest)
  - Risky: JSONL reorder (with backup/restore, requires confirmation)
- Output formats: Text (human-readable) or JSON (--json for scripting)

**Example output:**
```
=== Claude Session Manager Health Check ===

✓ Claude history found
✓ tmux installed: tmux 3.3a
✓ tmux socket active: /tmp/tmux-1000/default
✓ User lingering enabled (sessions persist after logout)
✓ Found 224 session manifests

--- Checking session health ---
⚠ Unhealthy session: my-broken-session
  Issue: JSONL file compacted (summaries not at end)
  Fix: agm doctor --validate --fix

✓ System is healthy (or ⚠ Some issues found - see recommendations above)
```

### `agm archive <session-name>`

Archive a session (marks as archived, keeps manifest).

### `agm session get-history-path [session-name] [flags]`

Retrieve conversation history file paths for AGM sessions across all harnesses:

```bash
# Get history for current session (auto-detect)
agm session get-history-path

# Get history for specific session
agm session get-history-path my-session

# JSON output for scripting
agm session get-history-path --json

# Verify files exist
agm session get-history-path --verify
```

**Features:**
- Works across all CLI harnesses (Claude Code, Gemini CLI, OpenCode, Codex)
- Auto-detects harness type and constructs correct paths
- JSON output for automation and scripting
- File existence verification with `--verify` flag
- Supports current session or named sessions

**Output (JSON):**
```json
{
  "agent": "claude",
  "uuid": "54790b4a-5342-4a60-a25f-5b260e319b5a",
  "paths": [
    "~/.claude/projects/-home-user-src/54790b4a.jsonl",
    "~/.claude/projects/-home-user-src/sessions-index.json"
  ],
  "exists": true,
  "metadata": {
    "working_directory": "~/src",
    "encoding_method": "dash-substitution"
  }
}
```

**See also:**
- Full documentation: [docs/GET-HISTORY-PATH.md](docs/GET-HISTORY-PATH.md)
- Multi-harness skill: `/agm:get-history` (via engram plugin marketplace)

### `agm unarchive <pattern>`

Restore archived sessions using glob patterns with interactive selection:

```bash
agm unarchive my-session        # Exact match - auto-restore
agm unarchive *acme*          # Pattern match - show picker if multiple
agm unarchive "session-202?"    # Wildcard year
agm unarchive "*"               # All archived - interactive selection
```

**Features:**
- Glob pattern support (`*`, `?`, `[abc]`)
- Auto-restore if single match found
- Interactive selection menu for multiple matches
- Searches both in-place archived sessions and `.archive-old-format/`

### `agm search <query>`

Find archived sessions using AI-powered semantic search:

```bash
agm search "that conversation about Composio"
agm search "OAuth integration with MCP"
agm search "last week's debugging session"
```

**Features:**
- Semantic search powered by Google Vertex AI (Claude Haiku)
- Searches conversation history (`~/.claude/history.jsonl`)
- Interactive selection for multiple results
- Auto-restores selected session
- Results cached for 5 minutes
- Rate limited: 10 searches/minute

**Authentication:**
```bash
# Configure Google Cloud credentials
gcloud auth application-default login

# Set project (if not set)
export GOOGLE_CLOUD_PROJECT=your-project-id
# OR
gcloud config set project your-project-id
```

**Flags:**
- `--max-results <N>` - Maximum results to return (default: 10)

## Communication Commands

AGM provides a unified `agm send` command group for all session communication operations.

### `agm send msg <recipient> [flags]`

Send messages to one or more AGM sessions with multi-recipient support and parallel delivery.

**Single recipient:**
```bash
# Send inline prompt
agm send msg my-session --prompt "Please review the code"

# Send prompt from file (for large multi-line prompts)
agm send msg my-session --prompt-file /path/to/prompt.txt

# Backward compatible (still works)
agm session send my-session --prompt "Please review the code"
```

**Multi-recipient (2.5x faster with parallel delivery):**
```bash
# Comma-separated list
agm send msg session1,session2,session3 --prompt "Status check"

# Using --to flag (more readable)
agm send msg --to backend,frontend,api --prompt "Deploy notification"

# Glob pattern expansion
agm send msg "*research*" --prompt "Update on findings"

# Workspace filtering
agm send msg --workspace oss --prompt "OSS update available"
```

**Features:**
- **Multi-recipient**: Send to multiple sessions simultaneously
- **Parallel delivery**: Worker pool with max 5 concurrent deliveries (2.5x speedup)
- **Flexible targeting**: Comma-separated lists, glob patterns, workspace filtering
- **Auto-interrupt**: Sends ESC to interrupt thinking before sending prompt
- **Literal mode**: Uses tmux `-l` flag to prevent special character interpretation
- **Per-recipient error isolation**: One failure doesn't block others
- **Color-coded reporting**: Success/failure status for each recipient
- **Rate limiting**: Per-sender (not per-recipient), 10 messages/minute

**Flags:**
- `--prompt <text>` - Prompt text to send
- `--prompt-file <path>` - File containing prompt to send (max 10KB)
- `--to <recipients>` - Explicit recipient list (alternative to positional arg)
- `--workspace <name>` - Filter sessions by workspace

**Examples:**
```bash
# Single recipient
agm send msg my-session --prompt "Ready for review"

# Broadcast to multiple sessions
agm send msg session1,session2,session3 --prompt "Please pause work"

# Pattern-based broadcast
agm send msg "test-*" --prompt "Run verification tests"

# Workspace-scoped broadcast
agm send msg --workspace acme --prompt "Deploy complete"

# Multi-line from file
agm send msg backend --prompt-file ~/templates/deploy-checklist.txt
```

### `agm send reject <session-name> [flags]`

Reject a permission prompt with a custom reason (automates the Down → Tab → paste → Enter flow).

```bash
# Reject with inline reason
agm send reject my-session --reason "Use Read tool instead of cat"

# Reject with violation prompt from file
agm send reject my-session --reason-file ~/prompts/VIOLATION-PROMPTS.md

# Backward compatible (still works)
agm session reject my-session --reason "Use Read tool instead of cat"
```

**Features:**
- **Automated navigation**: Navigates to "No" option using arrow keys
- **Custom reasoning**: Adds rejection reason as additional instructions
- **Smart extraction**: Extracts "## Standard Prompt (Recommended)" from markdown files
- **Literal mode**: Uses tmux `-l` flag for reliable text transmission

**Flags:**
- `--reason <text>` - Rejection reason to send
- `--reason-file <path>` - File containing rejection reason (max 10KB)

**Workflow executed:**
1. Send Down key to navigate to "No" option
2. Send Tab key to add additional instructions
3. Send rejection reason text in literal mode
4. Send Enter to submit

### `agm send approve <session-name> [flags]`

Approve a permission prompt with optional reason (automates the Tab → paste → Enter flow).

```bash
# Simple approval (no reason)
agm send approve my-session

# Approval with reason
agm send approve my-session --reason "LGTM, approved"

# Approval with auto-continue
agm send approve my-session --auto-continue

# Approval with reason from file
agm send approve my-session --reason-file ~/prompts/APPROVAL-TEMPLATE.md
```

**Features:**
- **Automated approval**: Navigates to "Yes" option (usually default, no navigation needed)
- **Optional reasoning**: Add approval reason as additional instructions
- **Auto-continue**: Automatically continue after approval (bypasses "Continue" prompt)
- **Smart extraction**: Extracts "## Standard Prompt (Recommended)" from markdown files
- **Literal mode**: Uses tmux `-l` flag for reliable text transmission

**Flags:**
- `--reason <text>` - Approval reason to send (optional)
- `--reason-file <path>` - File containing approval reason (max 10KB)
- `--auto-continue` - Automatically continue after approval

**Workflow executed:**
1. Detect prompt type (2-option or 3-option)
2. Navigate to "Yes" option if needed (usually already selected)
3. If `--reason` provided: Send Tab → reason text → Enter
4. If `--auto-continue`: Send additional Enter to bypass "Continue" prompt
5. Otherwise: Send Enter to approve

**Examples:**
```bash
# Quick approval
agm send approve my-session

# Approval with context
agm send approve my-session --reason "File changes reviewed and approved"

# Automated workflow approval
agm send approve deployment-session --reason "All checks passed" --auto-continue
```

## Session Output Capture

### `agm capture <session-name> [flags]`

Capture tmux pane output from AGM sessions in multiple formats.

```bash
# Capture visible content (default)
agm capture my-session

# Capture with line limit
agm capture my-session --lines 50

# Capture full scrollback history
agm capture my-session --history

# Capture last N lines (tail mode)
agm capture my-session --tail 20

# JSON output
agm capture my-session --json

# YAML output
agm capture my-session --yaml

# Filter output with regex
agm capture my-session --filter "ERROR|WARN"
```

**Features:**
- **Multiple modes**: Visible content, full history, tail
- **Output formats**: Text (default), JSON, YAML
- **Regex filtering**: Filter captured lines by pattern
- **Structured output**: JSON/YAML includes metadata (timestamp, line count)

**Flags:**
- `--lines <N>` - Limit output to N lines (default: all visible)
- `--history` - Capture full scrollback history
- `--tail <N>` - Capture last N lines only
- `--json` - Output in JSON format
- `--yaml` - Output in YAML format
- `--filter <regex>` - Filter lines matching regex pattern

**Output Formats:**

Text (default):
```
Line 1 of output
Line 2 of output
...
```

JSON:
```json
{
  "session": "my-session",
  "lines": ["Line 1", "Line 2"],
  "timestamp": "2026-03-15T10:30:00Z",
  "count": 2
}
```

YAML:
```yaml
session: my-session
lines:
  - Line 1
  - Line 2
timestamp: 2026-03-15T10:30:00Z
count: 2
```

**Use Cases:**
- Debugging: Capture error output for analysis
- Monitoring: Extract specific log patterns
- Automation: Parse structured output in scripts
- Multi-session coordination: Capture responses from multiple sessions

## Claude Code Skills

AGM provides Claude Code skills for common workflow automation. Skills are installed to `~/.claude/skills/agm/` and invoked using slash commands.

### Installation

```bash
# Install AGM skills
cd ~/path/to/agm
make install-skills
```

This copies skill scripts to `~/.claude/skills/agm/` and makes them executable.

### Available Skills

#### `/agm:new` - Smart Session Creation

Create a new AGM session with auto-generated name and optional agent/project selection.

```bash
# Create with auto-generated name
/agm:new

# Create with specific name
/agm:new my-session

# Create with specific agent
/agm:new research-task --harness gemini-cli

# Create with project path
/agm:new coding-session --project ~/src/myapp
```

**Features:**
- Auto-generates session names if not provided (format: `agm-YYYYMMDD-HHMMSS`)
- Harness selection via `--harness` flag (claude-code, gemini-cli, codex-cli, opencode-cli)
- Project context via `--project` flag
- Returns session name for chaining with other commands

#### `/agm:send` - Message Sending with Response Capture

Send messages to AGM sessions with optional response capture.

```bash
# Send message
/agm:send my-session --prompt "What's the current status?"

# Send and capture response
/agm:send my-session --prompt "Run tests" --capture-response
```

**Features:**
- Sends prompt to specified session
- Optional `--capture-response` flag to capture and return output
- Useful for multi-agent coordination workflows
- Returns captured response for further processing

**Flags:**
- `--prompt <text>` - Message to send (required)
- `--capture-response` - Capture and return session output

#### `/agm:status` - Session Health Monitoring

Check session status with optional watch mode for continuous monitoring.

```bash
# Check single session status
/agm:status my-session

# Check all sessions
/agm:status --all

# Watch mode (continuous monitoring)
/agm:status my-session --watch
```

**Features:**
- Shows session state (READY, THINKING, PERMISSION_PROMPT, OFFLINE)
- Optional `--all` flag to show all sessions
- Optional `--watch` flag for continuous monitoring (refreshes every 2s)
- Color-coded status indicators

**Flags:**
- `--all` - Show status for all sessions
- `--watch` - Continuous monitoring mode

#### `/agm:resume` - Intelligent Session Resume

Resume AGM sessions with fuzzy matching and last-active support.

```bash
# Resume specific session
/agm:resume my-session

# Fuzzy matching
/agm:resume --fuzzy my-ses

# Resume last active session
/agm:resume --last
```

**Features:**
- Fuzzy matching for typo-tolerant session names
- `--last` flag to resume most recently active session
- Interactive picker if multiple matches found
- Validates session exists before attempting resume

**Flags:**
- `--fuzzy` - Enable fuzzy matching for session names
- `--last` - Resume last active session

### Skill Usage Examples

**Multi-agent workflow coordination:**
```bash
# Create research session
/agm:new research --harness gemini-cli

# Send research request
/agm:send research --prompt "Analyze latest AI papers on reinforcement learning"

# Monitor status
/agm:status research --watch

# Capture results
/agm:send research --prompt "Summarize findings" --capture-response
```

**Automated testing workflow:**
```bash
# Check all test sessions
/agm:status --all

# Send test command to all test sessions
(Requires iterating over session list and using /agm:send for each)

# Resume last test session
/agm:resume --last
```

**For detailed skill documentation**, see `skills/README.md` in the repository.

## Workspace Management

AGM supports workspace-aware session management, allowing you to organize sessions by workspace context (e.g., `oss` vs `acme`).

### `agm workspace list`

List all configured workspaces from `~/.agm/config.yaml`.

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

### `agm workspace show <name>`

Show detailed information about a specific workspace.

```bash
# Show workspace details
agm workspace show oss

# Output includes:
# - Workspace name and path
# - Number of sessions
# - List of sessions in workspace
```

### `agm workspace new <name>`

Create a new workspace with interactive configuration.

```bash
# Create workspace with prompts
agm workspace new personal

# Interactive prompts for:
# - Workspace path
# - Validation
```

### `agm workspace del <name>`

Delete a workspace from configuration (sessions remain).

```bash
# Delete workspace (with confirmation)
agm workspace del old-workspace

# Note: Only removes workspace from config
# Sessions in workspace are NOT deleted
```

### Cross-Workspace Session Discovery

AGM can discover and filter sessions across all workspaces:

```bash
# Show sessions from all workspaces
agm session list --all-workspaces

# Filter sessions by specific workspace
agm session list --workspace-filter oss
agm session list --workspace-filter acme

# Automatic cross-workspace discovery (when outside any workspace)
cd ~/src
agm session list    # Shows sessions from all workspaces
```

**Behavior**:
- `--all-workspaces`: Explicitly list sessions from all configured workspaces
- `--workspace-filter <name>`: Show only sessions from specified workspace
- **Auto-discovery**: When running outside workspace directories (e.g., `~/src`), AGM automatically discovers sessions from all workspaces

**Directory Discovery**:
- Checks both `.agm/sessions` (new workspace-aware location)
- Checks `sessions` (legacy location for backward compatibility)
- Pattern: `~/src/ws/*/sessions/*/manifest.yaml`

### Workspace Detection in `agm new`

AGM automatically detects the workspace based on your project directory:

```bash
# Auto-detect workspace from current directory
agm new --workspace=auto my-session

# Explicitly set workspace
agm new --workspace=oss my-session
agm new --workspace=acme my-session

# Interactive selection if ambiguous
agm new my-session    # Prompts if multiple workspaces match
```

**How it works:**
- Reads workspace paths from `~/.agm/config.yaml`
- Matches current directory against workspace paths
- Falls back to interactive selection if ambiguous
- Stores workspace in session manifest

See [docs/workspace-detection.md](docs/workspace-detection.md) for detailed workspace detection algorithm.

## Accessibility

AGM supports WCAG AA accessibility standards through global flags and environment variables:

### Disable Colors

For users who cannot distinguish colors or need plain text output:

```bash
# Using flags (recommended)
agm list --no-color
agm doctor --no-color

# Using environment variable (legacy)
NO_COLOR=1 agm list
```

The `--no-color` flag:
- Disables all ANSI color codes
- Works in CI/CD environments
- Applies to all subcommands (persistent flag)

### Screen Reader Support

For users using screen readers or assistive technology:

```bash
# Using flags (recommended)
agm doctor --screen-reader
agm list --screen-reader

# Using environment variable (legacy)
AGM_SCREEN_READER=1 agm doctor
```

The `--screen-reader` flag:
- Converts Unicode symbols to text labels (`✓` → `[SUCCESS]`, `❌` → `[ERROR]`, `⚠` → `[WARNING]`)
- Ensures all information is available as text
- Works with popular screen readers (NVDA, JAWS, VoiceOver)

### Combine Both Flags

```bash
agm doctor --no-color --screen-reader
```

### High-Contrast Themes

AGM includes high-contrast themes optimized for accessibility:

```yaml
# ~/.config/agm/config.yaml
ui:
  theme: "agm"        # High-contrast for dark terminals (default)
  # theme: "agm-light" # High-contrast for light terminals
```

The `agm` theme provides:
- WCAG AA compliant contrast ratios (4.5:1 minimum)
- Selection indicated by color + cursor symbol + bold text
- Semantic color consistency (green=success, red=error, yellow=warning)

### Automatic Accessibility Detection

AGM automatically detects non-TTY environments (CI/CD, pipes) and disables colors. Flags provide explicit control when needed.

**Documentation:** See `docs/ACCESSIBILITY.md` for complete WCAG compliance details and contrast ratios.

## Storage Backend

### Current: YAML Manifests (Active)

AGM currently uses YAML manifest files for session metadata storage:

- **Location**: `~/.agm/sessions/*/manifest.yaml` (or workspace-specific: `~/src/ws/<workspace>/.agm/sessions/`)
- **Format**: YAML files containing session metadata (name, project, workspace, agent, timestamps)
- **Conversation History**: Stored in `~/.claude/history.jsonl` (managed by Claude CLI)

**Benefits:**
- Simple, human-readable format
- Easy to inspect and debug
- Git-friendly for version control
- No database server required

**Limitations:**
- Limited query capabilities
- No built-in versioning/history
- Manual synchronization across tools

### Dolt Storage (Partially Integrated)

AGM uses Dolt database backend for session storage with **partial CLI integration** as of March 2026.

**Why Dolt?**
- **Git-like versioning**: Every database change is a commit with full history
- **Workspace isolation**: Separate databases per workspace (OSS vs Acme)
- **Corruption prevention**: Atomic transactions prevent data corruption
- **Advanced queries**: SQL interface for complex session analytics

**Architecture:**
- Per-workspace Dolt instances: `~/src/ws/<workspace>/.dolt/dolt-db`
- Dolt server runs on per-workspace ports (OSS: 3307, Acme: 3308)
- Tables: `agm_sessions`, `agm_messages`, `agm_tool_calls`, `agm_session_tags`

**Current Status:**
- ✅ Dolt adapter implemented (`internal/dolt/`)
- ✅ Migration system with 7 migrations
- ✅ Migration tool built and executed (`cmd/agm-migrate-dolt/`)
- ✅ **Data migrated**: 40/40 sessions migrated to Dolt (March 2026, zero data loss)
- ✅ **Partially integrated**: `agm session list` uses Dolt backend (see `cmd/agm/list_dolt.go`)
- ⚠️ **Other commands still use YAML**: `new`, `resume`, `archive`, `delete` (pending future migration)
- ℹ️ Legacy YAML backend available via `agm session list-yaml` (deprecated)
- ℹ️ Documented in `internal/dolt/README.md` and `internal/dolt/SETUP.md`

**Setup Instructions** (for testing/development):

1. **Start Dolt server:**
   ```bash
   cd ./.dolt
   dolt sql-server --config=server.yaml &
   ```

2. **Set environment variables:**
   ```bash
   export WORKSPACE=oss
   export DOLT_HOST=127.0.0.1
   export DOLT_PORT=3307
   # Do NOT set DOLT_DATABASE - it breaks workspace isolation
   # The adapter defaults to using workspace name as database name
   ```

3. **Verify connectivity:**
   ```bash
   dolt sql -q "SELECT 1"
   ```

**Migration** (when ready):

The migration process will migrate YAML manifests to Dolt storage:
- Session metadata → `agm_sessions` table
- Conversation history → `agm_messages` table
- Tool usage tracking → `agm_tool_calls` table

See `internal/dolt/README.md` for detailed migration instructions and architecture documentation.

**Integration Timeline:**

Completed (March 2026):
1. ✅ Built and executed migration tool (`cmd/agm-migrate-dolt/`)
2. ✅ Migrated all sessions to Dolt (40/40, zero data loss)
3. ✅ Integrated `agm session list` command to use Dolt backend

Future work (pending):
4. Migrate remaining commands to Dolt: `new`, `resume`, `archive`, `delete`
5. Add dual-write mode (write to both YAML and Dolt during transition)
6. Deprecate YAML backend entirely
7. Add storage backend selection to configuration

## Configuration

Create `~/.config/agm/config.yaml`:

```yaml
# Centralized storage support (optional, opt-in)
storage:
  mode: dotfile                    # Mode: "dotfile" (default) or "centralized"
  workspace: ""                    # Workspace name or path (for centralized mode)
  relative_path: .agm              # Path within workspace (default: .agm)

defaults:
  interactive: true                # Enable interactive prompts
  auto_associate_uuid: true        # Auto-detect UUIDs
  confirm_destructive: true        # Confirm before delete/archive
  cleanup_threshold_days: 30       # Stopped → archive threshold
  archive_threshold_days: 90       # Archived → delete threshold

ui:
  theme: "dracula"                 # UI theme (dracula, catppuccin, charm, base)
  picker_height: 15                # Session picker height
  show_project_paths: true         # Show full project paths
  show_tags: true                  # Show session tags
  fuzzy_search: true               # Enable fuzzy matching

advanced:
  tmux_timeout: "5s"               # Tmux command timeout
  health_check_cache: "5s"         # Health check cache duration
  lock_timeout: "30s"              # Lock acquisition timeout
  uuid_detection_window: "5m"      # UUID detection time window
```

### Centralized Storage (Optional)

AGM supports storing session data in a git-tracked workspace instead of dotfiles:

**Enable centralized mode**:
```yaml
# ~/.config/agm/config.yaml
storage:
  mode: centralized
  workspace: engram-research       # Auto-detects location
  relative_path: .agm
```

**Benefits**:
- **Portable**: Clone repo = all session data
- **Git-tracked**: Full history and backups
- **Organized**: All component data in one place
- **Discoverable**: Cross-component queries

**How it works**:
1. AGM creates symlink: `~/.agm` → `.agm/`
2. All reads/writes go through symlink to centralized location
3. Data automatically git-tracked

See [CENTRALIZED-STORAGE.md](CENTRALIZED-STORAGE.md) for complete documentation.

## UUID Auto-Detection

AGM uses a hybrid approach for UUID detection:

### Automatic Detection
1. Reads `~/.claude/history.jsonl` for recent Claude sessions
2. Matches by project directory
3. Confidence levels:
   - **High** (< 2.5 min old) - Auto-applied
   - **Medium** (2.5-5 min old) - Manual confirmation
   - **Low** (> 5 min old) - Listed in suggestions

### Manual Association
Use `agm fix` to manually associate UUIDs:
- Shows ranked suggestions from history
- Displays context (directory, timestamp, confidence)
- Allows manual UUID entry
- Validates against history

## Architecture

### Module Structure

```
internal/
├── fuzzy/          # Levenshtein distance matching
├── ui/             # Interactive TUI components (Huh)
│   ├── picker.go   # Session picker
│   ├── forms.go    # Multi-step forms
│   ├── confirm.go  # Confirmation dialogs
│   └── cleanup.go  # Multi-select cleanup
├── tmux/           # Tmux integration
│   ├── tmux.go            # Core tmux operations
│   ├── control.go         # Control mode (-C) for programmatic control
│   ├── output_watcher.go  # Output stream monitoring (octal escape handling)
│   ├── init_sequence.go   # Sequenced /rename → /agm:assoc initialization
│   ├── socket.go          # Unix socket management (/tmp/agm.sock)
│   ├── linger.go          # Systemd lingering support
│   └── health.go          # Health checks
├── history/        # ~/.claude/history.jsonl parser
├── detection/      # Hybrid UUID auto-detection
├── fix/            # Manual UUID association
├── manifest/       # Session manifest (YAML schema, current storage)
├── session/        # Session status computation
└── dolt/           # Dolt storage adapter (in development, not yet integrated)
    ├── adapter.go         # Core Dolt adapter implementation
    ├── migrations.go      # Migration system
    ├── sessions.go        # Session CRUD operations
    ├── messages.go        # Message storage
    ├── tool_calls.go      # Tool usage tracking
    └── migrations/        # SQL migration files (001-005)
```

### Design Principles

1. **Smart defaults** - Minimal input required, intelligent behavior
2. **Interactive when helpful** - TUI for multi-option scenarios
3. **Fuzzy matching** - Typo-tolerant (0.6 similarity threshold)
4. **Batch operations** - Multi-select for cleanup tasks
5. **Confidence-based auto-detection** - Only auto-apply high-confidence UUIDs

## Development

### Running Tests

```bash
# All tests
go test ./...

# With coverage
go test -cover ./internal/fuzzy ./internal/ui ./internal/history ./internal/detection ./internal/fix

# Specific module
go test -v ./internal/fuzzy

# Dolt storage tests (requires Dolt server on port 3307)
DOLT_TEST_INTEGRATION=1 WORKSPACE=test DOLT_PORT=3307 go test -v ./internal/dolt
```

### Testing with agm test

For integration testing and debugging AGM features in isolated environments, use the `agm test` subcommands:

```bash
# Create isolated test session (separate from production sessions)
agm test create my-test

# Send commands to test session
agm test send my-test "agm associate --create my-project"

# Capture output for verification
agm test capture my-test --lines 50

# Cleanup when done
agm test cleanup my-test
```

**Common testing patterns:**

```bash
# Test AGM session lifecycle
agm test create lifecycle-test
agm test send lifecycle-test "agm new test-session --project ~/projects/test"
agm test capture lifecycle-test
agm test cleanup lifecycle-test

# Test with JSON output (for automation)
agm test create api-test --json
agm test send api-test "agm list" --json
agm test cleanup api-test --json

# Interactive debugging
agm test create debug-session
# ... manually send commands as needed ...
agm test cleanup debug-session
```

**Test isolation:** Test sessions use `/tmp/agm-test-*` directories and `agm-test-*` tmux sessions, completely isolated from production AGM state (`~/.claude-sessions/`).

### Test Coverage

- `fuzzy`: 95.2% (Levenshtein matching)
- `history`: 88.5% (JSONL parsing)
- `fix`: 89.4% (UUID association)
- `detection`: 68.2% (Auto-detection logic)
- `tmux/output_watcher`: 100% (13 tests - octal escape handling, pattern matching)
- `tmux/init_sequence`: 100% (12 tests - sequenced initialization, ready-file detection)
- `ui`: 10.2% (Interactive components, TTY required)

### Building

```bash
go build ./cmd/agm
```

## Documentation

### Migration Guides

- **[v2→v3 Manifest Migration](docs/MIGRATION-V2-V3.md)** - Upgrading from v2 to v3 manifest schema
- **[Claude→Multi-Agent Migration](docs/MIGRATION-CLAUDE-TO-MULTI-AGENT.md)** - Conceptual shift from single-agent to multi-agent workflows
- **[Dolt Storage Migration](internal/dolt/README.md)** - Future migration from YAML to Dolt storage (not yet available)

### Guides and References

- **[Agent Comparison Matrix](docs/AGENT-COMPARISON.md)** - When to use Claude vs Gemini vs GPT
- **[Usage Scenarios](docs/SCENARIOS.md)** - Real-world examples and BDD scenarios
- **[Troubleshooting Guide](docs/TROUBLESHOOTING.md)** - Common issues and solutions

## Troubleshooting

Common issues:

**UUID not detected**:
```bash
cat ~/.claude/history.jsonl | tail -5
# If empty, send message in Claude, then: agm fix --all
```

**Harness not available**:
```bash
agm agent list  # Check which agents are configured
# See docs/TROUBLESHOOTING.md for API key setup
```

**Session not appearing**:
```bash
agm list --all  # Include archived sessions
```

**For detailed troubleshooting**: See [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md)

## Migration from v1/v2

AGM v3 reads v2 manifests automatically and migrates on first resume.

For manual migration or v2→v3 details, see [docs/MIGRATION-V2-V3.md](docs/MIGRATION-V2-V3.md).

V1 → V2 field mapping (legacy):
- `Worktree.Path` → `Context.Project`
- `Status` → `Lifecycle` ("archived" only, others computed)
- `Claude.SessionID` → `Claude.UUID`

## Contributing

1. Follow existing test patterns (table-driven tests)
2. Maintain >80% coverage for new modules
3. Use Huh library for interactive components
4. Document public functions and types

## License

MIT

## Credits

Built with:
- [Huh](https://github.com/charmbracelet/huh) - Interactive TUI forms
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [agnivade/levenshtein](https://github.com/agnivade/levenshtein) - Fuzzy matching
