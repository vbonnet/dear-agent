# Engram Hooks

This directory contains Claude Code hooks that enhance the engram workflow.

## Available Hooks

### reload-plugin-commands

**Type**: SessionStart hook
**Purpose**: Automatically detects changes in plugin commands and reloads plugins at session start
**Location**: `./swarm/completed/hooks-rewrite/reload-plugin-commands/`
**Installed**: `~/.claude/hooks/session-start/reload-plugin-commands`

**What it does**:
- Monitors plugin command directories for changes (engram, agm)
- Computes hashes using fast path (frontmatter) or slow path (full SHA256)
- Compares with committed hashes to detect changes
- Automatically reloads plugins when changes detected via `claude plugin update`
- Never blocks session start (fail-open design, always exits 0)
- Implements 5-second timeout per plugin reload

**Performance**: ~3ms typical execution time (13x-220x faster than shell version)

**Configuration**: No configuration needed - automatically triggered at session start. Enable debug mode:
```bash
export CLAUDE_PLUGIN_RELOAD_DEBUG=1
```

**Installation**:
```bash
# From hook repository
cd ./swarm/completed/hooks-rewrite/reload-plugin-commands/
make install-claude  # For Claude Code
make install-gemini  # For Gemini CLI (stub)
```

**Benefits**:
- Always see latest plugin commands without manual reload
- Fast detection using frontmatter hashes (~1ms per plugin)
- Falls back to full SHA256 validation if frontmatter missing
- Never blocks session startup (fail-open design)
- 89% test coverage for reliability
- Cross-platform support (Claude Code + Gemini CLI)

**Example Output** (with debug enabled):
```
[reload-plugin-commands] Checking plugin: engram@engram
[reload-plugin-commands] Committed hash: abc123...
[reload-plugin-commands] Current hash: def456...
[reload-plugin-commands] Hash mismatch detected for engram - reloading...
[reload-plugin-commands] Updating marketplace: engram
[reload-plugin-commands] Updating plugin: engram
[reload-plugin-commands] Plugin engram reloaded successfully
[reload-plugin-commands] No changes detected for agm
```

**Documentation**:
- [SPEC.md]: `./swarm/completed/hooks-rewrite/reload-plugin-commands/SPEC.md`
- [README.md]: `./swarm/completed/hooks-rewrite/reload-plugin-commands/README.md`
- [Tests]: 89.0% coverage, 28 unit tests in `internal/reloader/plugins_test.go`

### ecphory-session-hook

**Type**: SessionStart hook
**Purpose**: Retrieves relevant engram memories at session start based on current working directory context
**Location**: `./swarm/completed/hooks-rewrite/ecphory-session-hook/`
**Installed**: `~/.claude/hooks/sessionstart/ecphory-session-hook`

**What it does**:
- Automatically executes when a new Claude Code or Gemini CLI session starts
- Retrieves contextual engram memories using `engram retrieve --auto --format table`
- Displays relevant memories from previous sessions to help users resume work
- Provides diagnostic warnings when engram is not configured or returns zero results
- Implements fail-open behavior (never blocks session startup)
- Enforces 5-second timeout to prevent hanging sessions

**Performance**: <20ms typical execution time (SessionStart target)

**Configuration**: No configuration needed - automatically triggered at session start

**Installation**:
```bash
# From hook repository
cd ./swarm/completed/hooks-rewrite/ecphory-session-hook/
make install-claude  # For Claude Code
make install-gemini  # For Gemini CLI
```

**Benefits**:
- Instant context restoration at session start
- No manual `engram retrieve` commands needed
- Clear diagnostic messages when engram needs configuration
- Never blocks session startup (fail-open design)
- Cross-platform support (Claude Code + Gemini CLI)

**Example Output**:
```
# When successful:
Found 3 engram(s):

1. API Error Handling Patterns
   Type: pattern
   File: patterns/error-handling.ai.md
   ...

# When engram needs configuration:
⚠️  ECPHORY RETRIEVAL FAILED
    Reason: exit status 1
    Context: ~/src/myproject

    Fix:
      1. Run 'engram init' to initialize workspace
      2. Run 'engram doctor' to verify configuration
      3. Check ~/.engram/core symlink exists:
         ls -la ~/.engram/core

    Without engrams loaded, tool usage guidance is unavailable.
```

**Documentation**:
- [SPEC.md]: `./swarm/completed/hooks-rewrite/ecphory-session-hook/SPEC-ecphory-session-hook.md`
- [README.md]: `./swarm/completed/hooks-rewrite/ecphory-session-hook/README.md`
- [VERIFICATION.md]: `./swarm/completed/hooks-rewrite/ecphory-session-hook/VERIFICATION.md`

### posttool-auto-commit-beads.py

**Type**: PostToolUse hook
**Purpose**: Automatically commits changes to `.beads/issues.jsonl` after bead operations

**What it does**:
- Monitors for `bd create`, `bd update`, `bd import` bash commands
- Monitors for `engram:create-bead` skill executions
- Automatically stages and commits `.beads/issues.jsonl` if it has uncommitted changes
- Uses standard commit message format
- Non-blocking - allows original operation even if commit fails

**Configuration**:

Add to your `.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Bash|Skill",
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/repos/engram/main/hooks/posttool-auto-commit-beads.py",
            "description": "Auto-commit beads database changes after bd create/update/import or create-bead"
          }
        ]
      }
    ]
  }
}
```

**Benefits**:
- Never forget to commit bead database changes
- Maintains clean git history with atomic bead operations
- Prevents losing bead data between sessions
- Works across all repos that use beads (engram-research, ai-tools, etc.)

**Example Output**:

```
✅ Auto-committed beads changes to .beads/issues.jsonl
```

### stop-validate-build-test.sh

**Type**: Stop hook
**Purpose**: Validates build and test integrity before session exit

**What it does**:
- Triggers automatically when Claude tries to exit a session
- Detects test command (npm test, pytest, cargo test, go test, make test)
- Detects build command (npm run build, cargo build, go build, make build)
- Runs tests and build with configurable timeout
- Shows loud ❌ FAILURE or ✅ SUCCESS message
- Provides remediation guidance if tests/build fail

**Note**: Stop hooks cannot technically block exit (only PreToolUse hooks can block). This hook provides loud feedback. The always-loaded engram enforces behavioral compliance ("MUST NOT exit with failures").

**Configuration**:

Automatically registered in `engram/main/.claude/settings.json`:

```json
{
  "hooks": {
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/repos/engram/main/hooks/stop-validate-build-test.sh",
            "description": "Validate build/test integrity before session exit"
          }
        ]
      }
    ]
  }
}
```

**Project-specific configuration** (optional):

Create `.build-integrity.yaml` in your project root:

```yaml
# Version
version: 1

# Test command (null = auto-detect)
test_command: "npm test"

# Build command (null = auto-detect, skip if not needed)
build_command: "npm run build"

# Timeout in seconds (default: 300)
timeout: 300

# Enable/disable checks (default: true)
enabled: true

# Watch paths (only validate if these change)
watch_paths:
  - "src/"
  - "lib/"
  - "tests/"

# Exclude patterns (skip validation if only these change)
exclude_patterns:
  - "*.md"
  - "docs/**"
```

**Auto-detection** (no config needed for standard projects):
- npm: Detects `package.json` scripts
- Python: Detects pytest.ini, pyproject.toml, or tests/ directory
- Rust: Detects Cargo.toml
- Go: Detects go.mod or *.go files
- Makefile: Detects test/build targets

**Benefits**:
- Zero tolerance enforcement (100% test pass rate)
- Prevents broken commits from reaching version control
- Behavioral enforcement via always-loaded engram
- Detailed remediation guidance (fix vs delete tests)
- Works with `/check-build` skill for manual validation

**Example Output (Success)**:
```
✅ Build/Test Integrity Check PASSED
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Tests: PASSED ✅
  • Tests completed successfully

Build: PASSED ✅

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ All checks passed. Safe to exit session.
```

**Example Output (Failure)**:
```
❌ Build/Test Integrity Check FAILED
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Tests: FAILED ❌
  • 40 tests passed
  • 2 tests failed
    - test_user_creation: AssertionError (line 42)
    - test_password_hash: Missing salt (line 67)

Build: SKIPPED (tests failed)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

⚠️  CRITICAL: You MUST NOT exit session with failing tests.

Next steps:
1. Fix failing tests
2. Run /check-build to re-validate
3. Try exiting again

Remediation guidance:
• test_user_creation: Brittle test? Fix assertions
• test_password_hash: Real bug? Fix implementation

See always-loaded engram for detailed remediation workflow.
```

**Integration**:
- Works with always-loaded engram: `engrams/references/claude-code/build-test-integrity.ai.md`
- Works with `/check-build` skill for manual validation
- Shared validation library: `hooks/lib/validate_build_test.sh`

### pretool-worktree-enforcer

**Type**: PreToolUse hook (Go binary)
**Purpose**: Enforces safe write destinations — blocks writes to golden-reference repos, redirects
writes targeting main-repo paths to the active worktree, and blocks writes to paths outside any git
repository.
**Location**: `hooks/cmd/pretool-worktree-enforcer/`
**Installed**: `~/.claude/hooks/pretool-worktree-enforcer`

**What it does** (in order):

1. **Pass-through for non-write tools** — Read, Bash, Grep, Glob: exits 0 immediately.
2. **Golden-reference enforcement** — Blocks Write/Edit/MultiEdit to paths inside configured
   `workspace_roots` (e.g. `.`) with exit code 2.
3. **Non-repo write blocking** — Blocks writes to any path that is not inside a git repository and
   not in the explicit allow-list below. This prevents agents from accumulating files in untracked
   locations (e.g. `the git history` outside git).
4. **Worktree redirection** — If the target path is inside a git worktree (and not the main repo),
   emits a JSON redirect so Claude Code writes to the correct worktree path.

**Allow-list for non-repo paths:**

| Prefix | Rationale |
|--------|-----------|
| `/tmp/` | Temporary scripts and test fixtures |
| `~/.csm/` | CSM/astrocyte runtime state |
| `~/.claude/` | Claude Code settings and hook binaries |
| `~/.wayfinder/` | Wayfinder template cache |
| `~/.config/` | XDG config (chezmoi, golden-ref) |
| `~/.local/` | XDG local data (chezmoi source) |
| `~/.agm/` | AGM session state |

**Exit Codes:**

| Code | Meaning |
|------|---------|
| 0 | Allow (pass-through or worktree in correct location) |
| 1 | Error (JSON parse failure or internal error) |
| 2 | Blocked (golden-ref violation or non-repo write) |

**Documentation**: See `cmd/pretool-worktree-enforcer/IMPLEMENTATION_STATUS.md` for full
implementation history.

---

### sessionstart-guardian

**Type**: SessionStart hook (Go binary)
**Purpose**: Pre-flight check that validates all configured hooks exist before session starts
**Location**: `hooks/cmd/sessionstart-guardian/`
**Installed**: `~/.claude/hooks/sessionstart-guardian`

**What it does**:
- Reads `~/.claude/settings.json` and enabled plugin `hooks.json` files
- Validates all referenced hook commands exist on disk
- Detects extension mismatches (e.g., config says `.py` but binary exists without extension)
- Blocks session start (exit 2) when missing hooks would cause hangs (SessionStart hooks with >10s timeout)
- Warns (exit 0) for non-critical issues like missing PostToolUse hooks
- Pure filesystem stat checks — no network calls, no subprocess execution

**Performance**: <50ms typical execution time

**Exit Codes**:
- 0: Healthy or warnings only (non-blocking)
- 2: Critical issues found (blocks session to prevent hang)

**Example Output** (critical):
```
BLOCKED: Hook pre-flight check failed
  • Missing: ~/bin/ecphory-session-hook.sh (SessionStart, 30s timeout)
Fix: engram doctor --auto-fix
```

**Documentation**:
- [SPEC.md](cmd/sessionstart-guardian/SPEC.md)

## Debugging Hooks

All Go-based PreToolUse hooks support debug logging to help troubleshoot issues.

### Log Files

Hooks write debug logs to `~/.claude/hooks/logs/<hook-name>.log`:

- `pretool-bash-blocker.log` - Bash command validation decisions
- `pretool-worktree-enforcer.log` - Worktree path redirection (if enabled)
- `pretool-beads-protection.log` - Beads file protection (if enabled)

### Example: Debugging pretool-bash-blocker

**Monitor hook in real-time:**
```bash
tail -f ~/.claude/hooks/logs/pretool-bash-blocker.log
```

**Check recent denials:**
```bash
grep "DENIED" ~/.claude/hooks/logs/pretool-bash-blocker.log | tail -10
```

**View specific command validation:**
```bash
grep "Parsed command: cd /tmp" ~/.claude/hooks/logs/pretool-bash-blocker.log
```

### Log Format

```
[2026-03-09 05:26:31] === Hook invoked ===
[2026-03-09 05:26:31] Raw input: {"tool_input": {"command": "cd /tmp && ls"}}
[2026-03-09 05:26:31] Parsed command: cd /tmp && ls
[2026-03-09 05:26:31] VALIDATOR: Pattern #1 MATCHED: cd command
[2026-03-09 05:26:31] DENIED: cd command - Use absolute paths instead
```

### Troubleshooting

**Empty log file?**
- Hook not being invoked (check `~/.claude/settings.json`)
- Hook not installed (run `hooks/deploy.sh`)

**"Parsed command:" shows empty string?**
- JSON format mismatch (check if hook needs update)
- Check raw input line for actual JSON received

**Log missing?**
- Check directory permissions: `ls -ld ~/.claude/hooks/logs`
- Hook uses fail-open design: continues working even without logging

**Hook blocking commands unexpectedly?**
- Check log for which pattern matched
- Verify command against patterns in `hooks/internal/validator/validator.go`
- Consider if command can be rewritten (e.g., `cd /path && git status` → `git -C /path status`)

### JSON Input Format

As of v2.0.0, hooks expect Claude Code's format:

```json
{
  "tool_input": {
    "command": "git status"
  },
  "hook_event_name": "PreToolUse",
  "tool_name": "Bash"
}
```

**Not** the old format:
```json
{
  "parameters": {
    "command": "git status"
  }
}
```

If you see empty commands in logs, the hook may need updating to parse the correct JSON structure.

## Installation

These hooks are automatically available when using engram as a Claude Code plugin via the engram-research workspace or any project that includes engram as a submodule.

For standalone usage, reference the hook path in your project's `.claude/settings.json` as shown above.

## Development

When creating new hooks:

1. Name them descriptively as Go binaries: `pretool-<purpose>` or `posttool-<purpose>` under `cmd/`
2. Build with: `make -C hooks build-macos`
3. Add clear comments explaining purpose and behavior
4. Make them non-blocking - exit with code 0 even on errors
5. Provide helpful output to stderr
6. Document in this README

## Testing

To test hooks without running through Claude Code:

```bash
# Create test input JSON
echo '{"name": "Bash", "parameters": {"command": "bd create"}}' | python hooks/posttool-auto-commit-beads.py

# Check exit code
echo $?  # Should be 0
```
