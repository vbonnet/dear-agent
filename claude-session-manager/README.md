# Claude Session Manager (CSM)

Smart session management for Claude AI with interactive TUI, fuzzy matching, and automatic UUID detection.

## Features

### 🎯 Smart Session Management
- **Interactive picker** - Beautiful TUI for session selection
- **Fuzzy matching** - Typo-tolerant session names ("my-ses" → "my-session")
- **Auto UUID detection** - Hybrid detection from `~/.claude/history.jsonl`
- **Batch operations** - Multi-select cleanup for archival/deletion
- **Pattern-based restore** - Glob patterns for archived session recovery (`csm unarchive *[REDACTED_EMPLOYER]*`)
- **AI-powered search** - Semantic search using Google Vertex AI (`csm search "OAuth work"`)

### 🚀 Quick Start

```bash
# Smart resume/create (no args needed!)
csm                    # Shows picker if multiple sessions, creates if none

# Named session (with fuzzy matching)
csm my-session         # Exact match → resume
csm my-ses            # Fuzzy match → "did you mean?"
csm new-name          # No match → offer to create

# Explicit commands
csm new               # Interactive form for new session
csm list              # List all sessions with status
csm clean             # Batch cleanup (archive/delete)
csm fix               # Fix UUID associations

# Archive management (NEW!)
csm unarchive *pattern*         # Restore archived sessions by pattern
csm search "semantic query"     # AI-powered semantic search
```

## Installation

```bash
go install github.com/vbonnet/ai-tools/claude-session-manager/cmd/csm@latest
```

### Bash Completion (Recommended)

Enable tab completion for command and session names:

```bash
# Run the setup script (recommended)
./scripts/setup-completion.sh

# Or manually install
cp scripts/csm-completion.bash ~/.csm-completion.bash
echo 'source ~/.csm-completion.bash' >> ~/.bashrc
source ~/.csm-completion.bash
```

**Features:**
- Command completion: `csm k<TAB>` → `csm kill`
- Session name completion: `csm kill <TAB>` → shows active sessions
- No file fallback: Only shows valid CSM commands/sessions (not random files)

**Note:** This uses a custom completion script that prevents bash from falling back to file/directory completion, which is a common issue with Cobra's default completion.

## Commands

### Primary Command: `csm [session-name]`

Smart behavior based on context:

**No session name provided:**
- Multiple sessions exist → Shows interactive picker
- No sessions exist → Prompts to create new session

**Session name provided:**
- Exact match → Resumes that session
- Fuzzy matches found → "Did you mean" prompt
- No match → Offers to create new session

### `csm new [session-name]`

Create new session with interactive form:
- Session name validation (alphanumeric, hyphens, underscores)
- Project directory selection
- Optional purpose/description
- Auto-creates tmux session + starts Claude
- **Sequenced initialization** - Sends `/rename` to generate UUID, then `/csm-assoc` (via tmux control mode)
- Auto-associates UUID via history detection
- Reliable UUID capture with 95%+ success rate

### `csm list [flags]`

List sessions with rich formatting:

```bash
csm list                 # Active/stopped sessions (table format)
csm list --all           # Include archived
csm list --archived      # Only archived
csm list --format=json   # Machine-readable output
```

**Output formats:**
- `table` (default) - Formatted table with status, project, updated time
- `json` - Machine-readable JSON
- `simple` - Simple name list

### `csm clean`

Interactive batch cleanup with smart suggestions:

- **Stopped sessions >30 days** - Suggested for archival
- **Archived sessions >90 days** - Suggested for deletion
- Multi-select interface with confirmation
- Thresholds customizable in `~/.config/csm/config.yaml`

### `csm fix [session-name]`

Manual UUID association management:

```bash
csm fix                  # Scan all unassociated sessions
csm fix my-session       # Fix specific session with suggestions
csm fix --all            # Auto-fix all (high confidence only)
csm fix --clear my-sess  # Remove UUID association
```

**UUID Suggestions:**
1. Auto-detected from history (high confidence)
2. Recent UUIDs from `~/.claude/history.jsonl`
3. Manual entry option

### `csm doctor [flags]`

Health check and validation for CSM and Claude sessions:

```bash
csm doctor                    # Structural checks only
csm doctor --validate         # Structural + functional testing
csm doctor --validate --fix   # Test and auto-fix issues
csm doctor --validate --json  # JSON output for scripting
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
  Fix: csm doctor --validate --fix

✓ System is healthy (or ⚠ Some issues found - see recommendations above)
```

### `csm archive <session-name>`

Archive a session (marks as archived, keeps manifest).

### `csm unarchive <pattern>`

Restore archived sessions using glob patterns with interactive selection:

```bash
csm unarchive my-session        # Exact match - auto-restore
csm unarchive *[REDACTED_EMPLOYER]*          # Pattern match - show picker if multiple
csm unarchive "session-202?"    # Wildcard year
csm unarchive "*"               # All archived - interactive selection
```

**Features:**
- Glob pattern support (`*`, `?`, `[abc]`)
- Auto-restore if single match found
- Interactive selection menu for multiple matches
- Searches both in-place archived sessions and `.archive-old-format/`

### `csm search <query>`

Find archived sessions using AI-powered semantic search:

```bash
csm search "that conversation about Composio"
csm search "OAuth integration with MCP"
csm search "last week's debugging session"
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

## Accessibility

CSM supports WCAG AA accessibility standards through global flags and environment variables:

### Disable Colors

For users who cannot distinguish colors or need plain text output:

```bash
# Using flags (recommended)
csm list --no-color
csm doctor --no-color

# Using environment variable (legacy)
NO_COLOR=1 csm list
```

The `--no-color` flag:
- Disables all ANSI color codes
- Works in CI/CD environments
- Applies to all subcommands (persistent flag)

### Screen Reader Support

For users using screen readers or assistive technology:

```bash
# Using flags (recommended)
csm doctor --screen-reader
csm list --screen-reader

# Using environment variable (legacy)
CSM_SCREEN_READER=1 csm doctor
```

The `--screen-reader` flag:
- Converts Unicode symbols to text labels (`✓` → `[SUCCESS]`, `❌` → `[ERROR]`, `⚠` → `[WARNING]`)
- Ensures all information is available as text
- Works with popular screen readers (NVDA, JAWS, VoiceOver)

### Combine Both Flags

```bash
csm doctor --no-color --screen-reader
```

### Automatic Accessibility Detection

CSM automatically detects non-TTY environments (CI/CD, pipes) and disables colors. Flags provide explicit control when needed.

**Documentation:** See `docs/UX_PATTERNS.md` for complete accessibility guidelines.

## Configuration

Create `~/.config/csm/config.yaml`:

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

CSM uses a hybrid approach for UUID detection:

### Automatic Detection
1. Reads `~/.claude/history.jsonl` for recent Claude sessions
2. Matches by project directory
3. Confidence levels:
   - **High** (< 2.5 min old) - Auto-applied
   - **Medium** (2.5-5 min old) - Manual confirmation
   - **Low** (> 5 min old) - Listed in suggestions

### Manual Association
Use `csm fix` to manually associate UUIDs:
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
│   ├── init_sequence.go   # Sequenced /rename → /csm-assoc initialization
│   ├── socket.go          # Unix socket management (/tmp/csm.sock)
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

### Testing with csm test

For integration testing and debugging CSM features in isolated environments, use the `csm test` subcommands:

```bash
# Create isolated test session (separate from production sessions)
csm test create my-test

# Send commands to test session
csm test send my-test "csm associate --create my-project"

# Capture output for verification
csm test capture my-test --lines 50

# Cleanup when done
csm test cleanup my-test
```

**Common testing patterns:**

```bash
# Test CSM session lifecycle
csm test create lifecycle-test
csm test send lifecycle-test "csm new test-session --project ~/projects/test"
csm test capture lifecycle-test
csm test cleanup lifecycle-test

# Test with JSON output (for automation)
csm test create api-test --json
csm test send api-test "csm list" --json
csm test cleanup api-test --json

# Interactive debugging
csm test create debug-session
# ... manually send commands as needed ...
csm test cleanup debug-session
```

**Test isolation:** Test sessions use `/tmp/csm-test-*` directories and `csm-test-*` tmux sessions, completely isolated from production CSM state (`~/.claude-sessions/`).

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
go build ./cmd/csm
```

## Troubleshooting

### UUID not detected

Check if `~/.claude/history.jsonl` exists and has entries:

```bash
cat ~/.claude/history.jsonl | tail -5
```

If empty, send a message in Claude to populate history, then run:

```bash
csm fix --all
```

### Session not appearing in picker

Verify manifest exists:

```bash
ls ~/sessions/session-*/manifest.yaml
```

Check session status:

```bash
csm list --all
```

### Fuzzy matching not working

Ensure similarity is ≥60%:
- "tset" matches "test" (75%)
- "myses" matches "my-session" (70%)
- "xyz" doesn't match "abc" (0%)

Adjust threshold in future versions via config.

## Migration from v1

CSM v2 reads v1 manifests automatically. No migration needed.

V1 → V2 field mapping:
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
