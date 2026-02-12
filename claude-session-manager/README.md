# AI/Agent Session Manager (AGM)

Smart session management for AI agents (Claude, Gemini, GPT) with interactive TUI, multi-agent support, and automatic session tracking.

## Multi-Agent Quick Start

```bash
# Create session with specific agent
agm new --agent claude my-coding-session   # Claude: code, reasoning
agm new --agent gemini research-task       # Gemini: research, 1M context
agm new --agent gpt chat-session           # GPT: chat, brainstorming

# Resume any session (agent auto-detected)
agm resume my-coding-session

# List all sessions (shows agents)
agm list
```

## Choosing an Agent

Not sure which agent to use?

- **Claude** (Anthropic): Best for code, long context (200K), multi-step reasoning
- **Gemini** (Google): Best for research, summarization, massive context (1M tokens)
- **GPT** (OpenAI): Best for chat, brainstorming, general Q&A

**Detailed comparison**: See [docs/AGENT-COMPARISON.md](docs/AGENT-COMPARISON.md) for:
- Feature comparison table (context windows, strengths, limitations)
- Use case guide (when to use each agent)
- Quick decision tree (choose agent in <2 minutes)
- Command translator support levels

**New to AGM?** See [docs/MIGRATION-CLAUDE-MULTI.md](docs/MIGRATION-CLAUDE-MULTI.md) if transitioning from Claude-only sessions.

### Agent Routing with AGENTS.md

**Status: Infrastructure Complete, Integration Pending**

Automate agent selection based on session names using AGENTS.md configuration files.

**Current state:**
- ✅ `internal/agents` package implemented (YAML parsing, keyword matching, multi-path detection)
- ⚠️ Integration with `agm new` pending (requires agent selection support in AGM core)
- ℹ️ Manual agent selection works: `agm new --agent <agent> <session-name>`

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

**Workaround:** Use explicit `--agent` flag until integration complete:
```bash
agm new creative-project --agent gemini    # Explicit agent selection (works now)
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
- **[Agent Comparison](docs/AGENT-COMPARISON.md)** - Choose the right agent (Claude/Gemini/GPT)

**🔧 Technical Documentation:**
- **[Architecture Overview](docs/ARCHITECTURE.md)** - Complete system architecture and design
- **[API Reference](docs/API-REFERENCE.md)** - Developer API for Go packages and interfaces
- **[BDD Catalog](docs/BDD-CATALOG.md)** - Living documentation (8 feature files, 20+ scenarios)

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
- **Batch operations** - Multi-select cleanup for archival/deletion
- **Pattern-based restore** - Glob patterns for archived session recovery (`agm unarchive *[REDACTED_EMPLOYER]*`)
- **AI-powered search** - Semantic search using Google Vertex AI (`agm search "OAuth work"`)

### 🔌 Command Translation (Multi-Agent)

AGM provides a unified command interface across different AI agents using the `CommandTranslator` abstraction. This allows generic operations (rename session, set directory, run hooks) to work across Claude, Gemini, and future agents.

**Supported Commands:**
- **RenameSession**: Rename agent session/conversation
- **SetDirectory**: Set working directory context
- **RunHook**: Execute initialization hook (agent-dependent)

**Example Usage:**
```go
import "github.com/vbonnet/ai-tools/claude-session-manager/internal/command"

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

# Archive management (NEW!)
agm unarchive *pattern*         # Restore archived sessions by pattern
agm search "semantic query"     # AI-powered semantic search
```

## Installation

```bash
go install github.com/vbonnet/ai-tools/claude-session-manager/cmd/agm@latest
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
- Optional purpose/description
- Auto-creates tmux session + starts Claude
- **Sequenced initialization** - Sends `/rename` to generate UUID, then `/agm:assoc` (via tmux control mode)
- Auto-associates UUID via history detection
- Reliable UUID capture with 95%+ success rate

### `agm list [flags]`

List sessions with rich formatting:

```bash
agm list                 # Active/stopped sessions (table format)
agm list --all           # Include archived
agm list --archived      # Only archived
agm list --format=json   # Machine-readable output
```

**Output formats:**
- `table` (default) - Formatted table with status, project, updated time
- `json` - Machine-readable JSON
- `simple` - Simple name list

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

### `agm unarchive <pattern>`

Restore archived sessions using glob patterns with interactive selection:

```bash
agm unarchive my-session        # Exact match - auto-restore
agm unarchive *[REDACTED_EMPLOYER]*          # Pattern match - show picker if multiple
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

### `agm send <session-name> [flags]`

Send a message/prompt to a running AGM session, interrupting any active thinking state:

```bash
# Send inline prompt
agm send my-session --prompt "Please review the code"

# Send prompt from file (for large multi-line prompts)
agm send my-session --prompt-file /path/to/prompt.txt
```

**Features:**
- **Auto-interrupt**: Sends ESC to interrupt thinking before sending prompt
- **Literal mode**: Uses tmux `-l` flag to prevent special character interpretation
- **Reliable execution**: Prompt is executed as command, not queued as "pasted text"
- **Large prompts**: Supports up to 10KB prompt files

**Use cases:**
- Automated recovery of stuck sessions (used by astrocyte daemon)
- Sending diagnosis prompts to investigate hangs
- Batch message delivery to multiple sessions

**Important:** Session must be running (active tmux session). This command executes the prompt immediately, bypassing any thinking/processing state.

**Flags:**
- `--prompt <text>` - Prompt text to send
- `--prompt-file <path>` - File containing prompt to send (max 10KB)

**Example:**
```bash
# Send diagnosis request to stuck session
agm send gemini-research --prompt "⚠️ Your session was stuck. Please analyze what caused the hang and file an incident report."

# Send multi-line prompt from file
agm send my-session --prompt-file ~/templates/code-review-prompt.txt
```

### `agm reject <session-name> [flags]`

Reject a permission prompt with a custom reason (automates the Down → Tab → paste → Enter flow):

```bash
# Reject with inline reason
agm reject my-session --reason "Use Read tool instead of cat"

# Reject with violation prompt from file
agm reject my-session --reason-file ~/prompts/VIOLATION-PROMPTS.md
```

**Features:**
- **Automated navigation**: Navigates to "No" option using arrow keys
- **Custom reasoning**: Adds rejection reason as additional instructions
- **Smart extraction**: Extracts "## Standard Prompt (Recommended)" from markdown files
- **Literal mode**: Uses tmux `-l` flag for reliable text transmission

**Use cases:**
- Rejecting bash commands that violate tool usage guidelines
- Providing feedback on why a permission was denied
- Automated enforcement of coding standards

**Important:** Session must be showing a permission prompt with a "No" option. This command assumes "No" is the second option (requires one Down keypress).

**Flags:**
- `--reason <text>` - Rejection reason to send
- `--reason-file <path>` - File containing rejection reason (max 10KB)

**Example:**
```bash
# Reject tool usage violation
agm reject my-session --reason-file ~/src/ws/oss/tool-usage-analysis/prompts/VIOLATION-PROMPTS.md

# Reject with custom feedback
agm reject my-session --reason "Please use absolute paths and separate tool calls. Read the bash tool guidance at ~/docs/bash-rules.md"
```

**Workflow executed:**
1. Send Down key to navigate to "No" option
2. Send Tab key to add additional instructions
3. Send rejection reason text in literal mode
4. Send Enter to submit

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

## Configuration

Create `~/.config/agm/config.yaml`:

```yaml
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
├── manifest/       # Session manifest (v2 schema)
└── session/        # Session status computation
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

**Agent not available**:
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
