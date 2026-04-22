# AGM CLI Reference

Complete command-line reference for AGM (AI/Agent Session Manager).

## Table of Contents

- [Global Flags](#global-flags)
- [Command Overview](#command-overview)
- [Session Management](#session-management)
- [Search & Discovery](#search--discovery)
- [Health & Diagnostics](#health--diagnostics)
- [Advanced Commands](#advanced-commands)
- [Testing Commands](#testing-commands)
- [Exit Codes](#exit-codes)

## Global Flags

Available for all commands:

```bash
--config string       Config file (default: ~/.config/agm/config.yaml)
--no-color           Disable colored output
--screen-reader      Screen reader mode (convert symbols to text)
--verbose            Verbose output
--help, -h           Show help
--version, -v        Show version
```

**Examples:**
```bash
# Use custom config
agm list --config ~/my-config.yaml

# Disable colors for CI/CD
agm doctor --no-color

# Screen reader support
agm list --screen-reader

# Verbose output for debugging
agm resume my-session --verbose
```

## Command Overview

| Command | Description | Aliases |
|---------|-------------|---------|
| `agm [session]` | Smart resume/create | (default) |
| `agm new` | Create new session | `create`, `n` |
| `agm resume` | Resume session | `attach`, `r` |
| `agm list` | List sessions | `ls`, `l` |
| `agm archive` | Archive session | `arc` |
| `agm unarchive` | Restore archived session | `restore` |
| `agm clean` | Batch cleanup | `cleanup` |
| `agm search` | AI-powered search | `find` |
| `agm fix` | Fix UUID associations | `associate` |
| `agm doctor` | Health check | `health`, `validate` |
| `agm send` | Send message to session | |
| `agm reject` | Reject permission prompt | |
| `agm test` | Testing commands | |

## Session Management

### agm [session-name]

Smart default command with context-aware behavior.

**Usage:**
```bash
agm [session-name] [flags]
```

**Behavior:**

**No session name provided:**
- 0 sessions exist → Prompt to create new session
- 1 session exists → Auto-resume that session
- 2+ sessions exist → Show interactive picker

**Session name provided:**
- Exact match → Resume that session
- Fuzzy matches found → "Did you mean?" prompt
- No match → Offer to create new session

**Examples:**
```bash
# Interactive picker
agm

# Resume by name
agm my-session

# Fuzzy matching
agm my-ses            # Matches "my-session"

# Create if doesn't exist
agm new-session       # Prompts to create
```

**Flags:**
```bash
--force              Skip confirmation prompts
```

---

### agm new

Create a new session.

**Usage:**
```bash
agm new [session-name] [flags]
```

**Flags:**
```bash
--harness string         Harness to use (claude-code|gemini-cli|codex-cli|opencode-cli) (default: claude-code)
--model string           Model to use (e.g., sonnet, opus, 2.5-flash, 5.4). If omitted, uses harness default.
                         claude-code: sonnet (default), opus, haiku, opusplan
                         gemini-cli: 2.5-flash (default), 2.5-pro, 3.1-pro, 3-flash
                         codex-cli: 5.4 (default), 5.4-mini, 5.3-codex
                         opencode-cli: requires selection (no default)
--project string         Project directory (default: current directory)
--tags strings           Tags (comma-separated)
--description string     Session description
--detached               Create but don't attach
--no-uuid                Skip UUID auto-detection
--force                  Skip confirmation prompts
```

**Examples:**
```bash
# Interactive form (prompts for all fields)
agm new

# Quick create with defaults
agm new my-session

# Specify harness
agm new --harness gemini-cli research-task

# Specify harness and model
agm new coding-session --harness claude-code --model opus
agm new research --harness gemini-cli --model 2.5-pro

# Full specification
agm new coding-session \
  --harness claude-code \
  --project ~/projects/myapp \
  --tags code,review,urgent \
  --description "Code review for PR #123"

# Create without attaching
agm new background-task --detached

# Skip UUID auto-detection
agm new test-session --no-uuid
```

**Interactive form fields:**
1. Session name (required)
2. Harness selection (claude-code/gemini-cli/codex-cli/opencode-cli)
3. Project directory (browse or type)
4. Description (optional)
5. Tags (optional)

**Validation:**
- Name uniqueness (must not exist)
- Name format (alphanumeric, hyphens, underscores)
- Directory existence (if specified)
- Harness availability (API key configured)

**Exit codes:**
- `0` - Session created successfully
- `1` - Validation error (name exists, invalid format)
- `2` - Harness not available
- `3` - Directory not found

---

### agm resume

Resume an existing session.

**Usage:**
```bash
agm resume <session-name> [flags]
```

**Flags:**
```bash
--force              Force resume even if session is locked
```

**Examples:**
```bash
# Resume by name
agm resume my-session

# Force resume (bypass lock)
agm resume my-session --force
```

**Behavior:**
- Checks if session exists
- Verifies tmux session is available
- Attaches to tmux session
- Restores agent context

**Errors:**
- Session not found → Suggests fuzzy matches
- Session locked → Shows lock owner, use `--force` to override
- tmux session missing → Offers to recreate

---

### agm list

List sessions with optional filtering.

**Usage:**
```bash
agm list [flags]
```

**Flags:**
```bash
--all                    Include archived sessions
--archived               Only archived sessions
--format string          Output format (table|json|simple) (default: table)
--tag string             Filter by tag
--harness string         Filter by harness (claude-code|gemini-cli|codex-cli|opencode-cli)
--project string         Filter by project directory
```

**Examples:**
```bash
# Active and stopped sessions (default)
agm list

# Include archived
agm list --all

# Only archived
agm list --archived

# JSON output
agm list --format=json

# Simple list (names only)
agm list --format=simple

# Filter by tag
agm list --tag work

# Filter by agent
agm list --harness claude-code

# Filter by project
agm list --project ~/projects/myapp

# Combine filters
agm list --harness gemini-cli --tag research
```

**Output formats:**

**Table (default):**
```
NAME              STATUS    AGENT   PROJECT                 UPDATED
coding-session    active    claude  ~/projects/myapp       2 minutes ago
research-task     stopped   gemini  ~/research             1 hour ago
old-session       archived  claude  ~/projects/legacy      30 days ago
```

**JSON:**
```json
[
  {
    "name": "coding-session",
    "status": "active",
    "agent": "claude",
    "project": "~/projects/myapp",
    "uuid": "abc123...",
    "tags": ["code", "review"],
    "created": "2026-02-01T10:00:00Z",
    "updated": "2026-02-03T15:30:00Z"
  }
]
```

**Simple:**
```
coding-session
research-task
old-session
```

---

### agm archive

Archive a session.

**Usage:**
```bash
agm archive <session-name> [flags]
```

**Flags:**
```bash
--force              Skip confirmation prompt
```

**Examples:**
```bash
# Archive with confirmation
agm archive my-session

# Archive without confirmation
agm archive my-session --force
```

**What happens:**
1. Confirmation prompt (unless `--force`)
2. Kill tmux session if running
3. Update manifest (mark as archived)
4. Files remain in `~/.claude-sessions/<session-name>/`

**Reverse operation:** Use `agm unarchive` to restore

---

### agm unarchive

Restore archived session(s) using glob patterns.

**Usage:**
```bash
agm unarchive <pattern> [flags]
```

**Flags:**
```bash
--force              Skip confirmation prompt
```

**Examples:**
```bash
# Exact match - auto-restore
agm unarchive my-session

# Pattern match - interactive picker if multiple
agm unarchive *acme*

# Wildcard year
agm unarchive "session-202?"

# All archived (interactive selection)
agm unarchive "*"

# Restore without confirmation
agm unarchive my-session --force
```

**Glob patterns:**
- `*` - Match any characters
- `?` - Match single character
- `[abc]` - Match any of a, b, c
- `[0-9]` - Match any digit
- `[!0-9]` - Match any non-digit

**Behavior:**
- 1 match → Auto-restore
- Multiple matches → Interactive picker
- 0 matches → Error message

**What happens:**
1. Pattern matching against archived sessions
2. Selection (auto or interactive)
3. Update manifest (mark as active)
4. Ready to resume with `agm resume`

---

### agm clean

Interactive batch cleanup.

**Usage:**
```bash
agm clean [flags]
```

**Flags:**
```bash
--dry-run            Show what would be cleaned without doing it
--force              Skip confirmation prompts
```

**Examples:**
```bash
# Interactive cleanup
agm clean

# Dry run (preview)
agm clean --dry-run

# Auto-cleanup without confirmation
agm clean --force
```

**Smart suggestions:**

**Archive candidates (stopped >30 days):**
- Color: Yellow
- Action: Archive
- Threshold: Configurable in config.yaml

**Delete candidates (archived >90 days):**
- Color: Red
- Action: Delete
- Threshold: Configurable in config.yaml

**Interface:**
- Multi-select checkbox list
- Space to toggle selection
- Arrow keys to navigate
- Enter to confirm
- ESC to cancel

**Confirmation:**
```
Selected actions:
- Archive: 3 sessions (session1, session2, session3)
- Delete: 2 sessions (old1, old2)

Total space freed: ~150 MB

Proceed? (y/n):
```

## Search & Discovery

### agm search

AI-powered semantic search of conversation history.

**Usage:**
```bash
agm search <query> [flags]
```

**Flags:**
```bash
--max-results int    Maximum results (default: 10)
```

**Examples:**
```bash
# Semantic search
agm search "that conversation about OAuth"

# Find by topic
agm search "debugging database connection"

# Find by timeframe
agm search "last week's code review"

# Limit results
agm search "API implementation" --max-results 5
```

**How it works:**
1. Sends query to Google Vertex AI (Claude Haiku)
2. Searches `~/.claude/history.jsonl` conversation history
3. Ranks results by relevance
4. Interactive selection for multiple results
5. Auto-restores selected session

**Requirements:**
- Google Cloud credentials configured
- `GOOGLE_CLOUD_PROJECT` environment variable set
- Vertex AI API enabled

**Authentication:**
```bash
# Configure credentials
gcloud auth application-default login

# Set project
export GOOGLE_CLOUD_PROJECT=your-project-id
```

**Performance:**
- Results cached for 5 minutes
- Rate limited: 10 searches/minute

**Exit codes:**
- `0` - Search completed, session selected
- `1` - No results found
- `2` - Authentication error
- `3` - API error

---

### agm fix

Fix UUID associations.

**Usage:**
```bash
agm fix [session-name] [flags]
```

**Flags:**
```bash
--all                Auto-fix all unassociated sessions (high confidence only)
--clear              Clear UUID association for session
```

**Examples:**
```bash
# Scan all unassociated sessions
agm fix

# Fix specific session
agm fix my-session

# Auto-fix all high-confidence associations
agm fix --all

# Clear UUID association
agm fix --clear my-session
```

**Behavior:**

**No session specified:**
- Scans all sessions
- Lists unassociated sessions
- Prompts to fix each

**Session specified:**
- Shows UUID suggestions for that session
- Ranked by confidence (high/medium/low)
- Displays context (project, timestamp)
- Allows manual entry

**UUID suggestions:**
1. Auto-detected from history (high confidence)
2. Recent UUIDs from `~/.claude/history.jsonl`
3. Manual entry option

**Example interaction:**
```
UUID suggestions for 'my-session':

1. abc123... (high confidence)
   Project: ~/projects/demo
   Timestamp: 2 minutes ago
   Reason: Recent conversation in same directory

2. def456... (medium confidence)
   Project: ~/projects/demo
   Timestamp: 5 minutes ago
   Reason: Same project, older timestamp

3. Manual entry

Select UUID (1-3): 1

✓ UUID associated: abc123...
```

**Auto-fix behavior (`--all`):**
- Only applies high-confidence associations
- Skips medium/low confidence
- Shows summary of changes

## Health & Diagnostics

### agm doctor

Health check and validation.

**Usage:**
```bash
agm doctor [agent] [flags]
```

**Flags:**
```bash
--validate           Run functional tests (creates test sessions)
--fix                Auto-fix issues (interactive confirmation for risky fixes)
--json               JSON output for scripting
--generate-envrc     Generate .envrc template (agent-specific)
--generate-bashrc    Generate ~/.bashrc snippet (agent-specific)
```

**Examples:**
```bash
# Structural health check
agm doctor

# Functional validation
agm doctor --validate

# Validate and fix
agm doctor --validate --fix

# JSON output
agm doctor --validate --json

# Agent-specific validation
agm doctor gemini

# Generate environment config
agm doctor gemini --generate-envrc
agm doctor gemini --generate-bashrc

# Interactive setup
agm doctor gemini --fix
```

**Structural checks:**
- ✅ Claude installation (history.jsonl exists)
- ✅ tmux installation and version
- ✅ tmux socket status
- ✅ User lingering (session persistence)
- ⚠️ Duplicate session directories
- ⚠️ Duplicate Claude UUIDs
- ⚠️ Sessions with empty/missing UUIDs
- ⚠️ Session health (manifest validity)

**Functional validation (`--validate`):**
- Creates test tmux session
- Attempts resume operation
- Classifies resume errors (6 types)
- Reports resumability status

**Resume error types:**
1. Empty session-env directory
2. Version mismatch (Claude CLI version changed)
3. Compacted JSONL (summaries not at end)
4. Missing JSONL file
5. CWD mismatch (working directory changed)
6. Lock contention (session locked)

**Auto-fix strategies (`--fix`):**
- **Safe:** Version mismatch (updates manifest)
- **Risky:** JSONL reorder (with backup, requires confirmation)

**Output example:**
```
=== AGM Health Check ===

✓ Claude history found
✓ tmux installed: tmux 3.3a
✓ tmux socket active: /tmp/tmux-1000/default
✓ User lingering enabled
✓ Found 224 session manifests

--- Session Health ---
⚠ Unhealthy session: my-broken-session
  Issue: JSONL file compacted (summaries not at end)
  Fix: agm doctor --validate --fix

Summary:
- Healthy sessions: 223
- Unhealthy sessions: 1

⚠ Some issues found - see recommendations above
```

**Agent-specific validation (e.g., `agm doctor gemini`):**

**Checks:**
- ✅ gemini CLI installed
- ✅ GEMINI_API_KEY set
- ✅ GOOGLE_GENAI_USE_VERTEXAI=false
- ⚠️ Conflicts detected (Vertex AI vars)

**Output example:**
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

╔══════════════════════════════════════╗
║ Recommended Fixes                     ║
╚══════════════════════════════════════╝

Option 1: Use direnv (recommended)
  1. agm doctor gemini --generate-envrc > .envrc
  2. Edit .envrc and add API key
  3. direnv allow

Option 2: Use ~/.bashrc (global)
  1. agm doctor gemini --generate-bashrc >> ~/.bashrc
  2. Edit ~/.bashrc and add API key
  3. source ~/.bashrc

Option 3: Per-session export
  export GEMINI_API_KEY="your-key"
  export GOOGLE_GENAI_USE_VERTEXAI=false

Run 'agm doctor gemini --fix' for interactive setup.
```

**Template generation:**

**`.envrc` (direnv):**
```bash
agm doctor gemini --generate-envrc > .envrc
```

**Output:**
```bash
# Gemini API Configuration
# WARNING: Add .envrc to .gitignore!

# Required: Gemini API key
export GEMINI_API_KEY="REPLACE_WITH_YOUR_API_KEY"

# Required: Disable Vertex AI mode
export GOOGLE_GENAI_USE_VERTEXAI=false

# Optional: Override Cloud Workstation defaults
unset GOOGLE_CLOUD_PROJECT
unset GOOGLE_CLOUD_LOCATION

# Security best practice: Load from password manager
# Example with pass:
#   export GEMINI_API_KEY=$(pass show gemini-api-key)
```

**`~/.bashrc` snippet:**
```bash
agm doctor gemini --generate-bashrc >> ~/.bashrc
```

**Documentation:** See [agm-environment-management-spec.md](agm-environment-management-spec.md)

## Advanced Commands

### agm session send

Send message/prompt to a running session.

**Usage:**
```bash
agm session send <session-name> [flags]
```

**Flags:**
```bash
--prompt string          Prompt text to send
--prompt-file string     File containing prompt (max 10KB)
```

**Examples:**
```bash
# Send inline prompt
agm session send my-session --prompt "Please review the code"

# Send from file
agm session send my-session --prompt-file ~/prompts/review.txt

# Diagnosis prompt
agm session send stuck-session --prompt "⚠️ Your session was stuck. Analyze what caused the hang."
```

**Features:**
- Auto-interrupts thinking state (sends ESC first)
- Literal mode (tmux `-l` flag for reliable transmission)
- Executes immediately (not queued)
- Supports up to 10KB prompt files

**Use cases:**
- Automated recovery of stuck sessions
- Sending diagnosis prompts
- Batch message delivery

**Requirements:**
- Session must be running (active tmux session)
- Valid session name

**Exit codes:**
- `0` - Message sent successfully
- `1` - Session not found
- `2` - Session not running
- `3` - Prompt file too large or not found

---

### agm session reject

Reject permission prompt with custom reason.

**Usage:**
```bash
agm session reject <session-name> [flags]
```

**Flags:**
```bash
--reason string          Rejection reason
--reason-file string     File containing reason (max 10KB)
```

**Examples:**
```bash
# Reject with inline reason
agm session reject my-session --reason "Use Read tool instead of cat"

# Reject with violation prompt from file
agm session reject my-session --reason-file ~/prompts/VIOLATION.md
```

**Workflow executed:**
1. Send Down key (navigate to "No" option)
2. Send Tab key (add additional instructions)
3. Send rejection reason (literal mode)
4. Send Enter (submit)

**Features:**
- Automated navigation to "No" option
- Custom reasoning as additional instructions
- Smart extraction (extracts "## Standard Prompt" from markdown)
- Literal mode for reliable transmission

**Use cases:**
- Rejecting tool usage violations
- Providing feedback on permission denials
- Automated enforcement of coding standards

**Requirements:**
- Session must show permission prompt
- "No" option must be second option (one Down keypress)

**Example reason file:**
```markdown
# Tool Usage Violation

## Standard Prompt (Recommended)

You attempted to use `cat` to read a file. This violates tool usage guidelines.

**Correct approach:** Use the Read tool instead.

Please review ~/docs/tool-usage.md for complete guidelines.
```

## Testing Commands

### agm test

Testing and debugging commands.

**Subcommands:**
- `agm test create` - Create isolated test session
- `agm test send` - Send command to test session
- `agm test capture` - Capture test session output
- `agm test cleanup` - Cleanup test session

---

#### agm test create

Create isolated test session.

**Usage:**
```bash
agm test create <session-name> [flags]
```

**Flags:**
```bash
--json               JSON output
```

**Examples:**
```bash
# Create test session
agm test create my-test

# JSON output
agm test create my-test --json
```

**Isolation:**
- Uses `/tmp/agm-test-*` directories
- Separate tmux sessions (`agm-test-*`)
- No impact on production state

---

#### agm test send

Send command to test session.

**Usage:**
```bash
agm test send <session-name> <command> [flags]
```

**Flags:**
```bash
--json               JSON output
```

**Examples:**
```bash
# Send command
agm test send my-test "agm list"

# Send complex command
agm test send my-test "agm new test-session --harness claude-code"
```

---

#### agm test capture

Capture test session output.

**Usage:**
```bash
agm test capture <session-name> [flags]
```

**Flags:**
```bash
--lines int          Number of lines to capture (default: 100)
--json               JSON output
```

**Examples:**
```bash
# Capture last 100 lines
agm test capture my-test

# Capture last 50 lines
agm test capture my-test --lines 50
```

---

#### agm test cleanup

Cleanup test session.

**Usage:**
```bash
agm test cleanup <session-name> [flags]
```

**Flags:**
```bash
--json               JSON output
```

**Examples:**
```bash
# Cleanup test session
agm test cleanup my-test

# JSON output
agm test cleanup my-test --json
```

## Exit Codes

Standard exit codes used by AGM:

| Code | Meaning | Example |
|------|---------|---------|
| `0` | Success | Command completed successfully |
| `1` | General error | Session not found, validation failed |
| `2` | Configuration error | Harness not available, API key missing |
| `3` | Resource error | File not found, directory doesn't exist |
| `4` | Lock error | Session locked by another process |
| `5` | Network error | API request failed, timeout |

**Examples:**
```bash
# Check exit code
agm resume my-session
echo $?  # 0 if successful, non-zero if error

# Use in scripts
if agm doctor --validate; then
  echo "System healthy"
else
  echo "Health check failed: $?"
  exit 1
fi

# Error handling
agm resume my-session || {
  echo "Failed to resume: $?"
  agm new my-session
}
```

## Environment Variables

AGM respects these environment variables:

```bash
# Agent API keys
ANTHROPIC_API_KEY         # Claude API key
GEMINI_API_KEY           # Gemini API key
OPENAI_API_KEY           # GPT API key

# Gemini configuration
GOOGLE_GENAI_USE_VERTEXAI    # true/false (should be false for API key mode)

# Google Cloud (for semantic search)
GOOGLE_CLOUD_PROJECT         # GCP project ID
GOOGLE_CLOUD_LOCATION        # GCP region (e.g., us-central1)

# Accessibility
NO_COLOR                     # Disable colors (legacy, use --no-color flag)
AGM_SCREEN_READER           # Screen reader mode (legacy, use --screen-reader flag)

# Advanced
AGM_CONFIG                   # Custom config file path
AGM_DEBUG                    # Enable debug output
```

## Shell Completion

Enable tab completion for faster command entry:

```bash
# Install completion script
cd ~/src/ai-tools/agm
./scripts/setup-completion.sh

# Or manually
cp scripts/agm-completion.bash ~/.agm-completion.bash
echo 'source ~/.agm-completion.bash' >> ~/.bashrc
source ~/.bashrc
```

**Features:**
- Command completion: `agm k<TAB>` → `agm kill`
- Session name completion: `agm resume <TAB>` → shows sessions
- No file fallback (only valid commands/sessions)

## Next Steps

- **Getting Started:** See [GETTING-STARTED.md](GETTING-STARTED.md) for quick start
- **User Guide:** See [USER-GUIDE.md](USER-GUIDE.md) for comprehensive usage
- **Examples:** See [EXAMPLES.md](EXAMPLES.md) for real-world scenarios
- **FAQ:** See [FAQ.md](FAQ.md) for common questions

---

**Last updated:** 2026-02-03
**AGM Version:** 3.0
**Maintained by:** Foundation Engineering
