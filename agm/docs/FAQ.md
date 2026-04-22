# AGM Frequently Asked Questions

Common questions and answers about AGM (AI/Agent Session Manager).

## Table of Contents

- [General](#general)
- [Installation & Setup](#installation--setup)
- [Session Management](#session-management)
- [Multi-Agent Support](#multi-agent-support)
- [UUID & Association](#uuid--association)
- [Troubleshooting](#troubleshooting)
- [Performance & Optimization](#performance--optimization)
- [Security & Privacy](#security--privacy)
- [Advanced Usage](#advanced-usage)

## General

### What is AGM?

AGM (AI/Agent Session Manager) is a smart session manager for AI agents (Claude, Gemini, GPT) that provides:
- Multi-agent support with unified interface
- Session persistence and resumability
- Automatic UUID detection and association
- Interactive TUI with fuzzy search
- Batch operations for cleanup and organization

**Evolved from:** Agent Session Manager (AGM)

### Why use AGM instead of just using AI agents directly?

**Benefits of AGM:**
- **Session persistence** - Resume conversations weeks/months later
- **Organization** - Manage multiple projects/tasks with separate sessions
- **Multi-agent** - Switch between Claude/Gemini/GPT based on task needs
- **Automation** - Automatic UUID tracking, session recovery, cleanup
- **Searchability** - AI-powered semantic search of conversation history
- **Productivity** - Fuzzy matching, batch operations, interactive TUI

### What's the difference between AGM and AGM?

**AGM (Agent Session Manager):**
- Claude-only support
- Single-agent focus
- Original version

**AGM (AI/Agent Session Manager):**
- Multi-agent support (Claude, Gemini, GPT)
- Command translation abstraction
- Enhanced features (agent routing, environment validation)
- Evolved version (AGM v3 → AGM v3)

**Migration:** See [MIGRATION-CLAUDE-MULTI.md](MIGRATION-CLAUDE-MULTI.md)

### Is AGM free and open source?

**Yes!** AGM is MIT licensed and fully open source.

**Repository:** https://github.com/vbonnet/ai-tools/tree/main/agm

### What are the system requirements?

**Required:**
- Linux or macOS
- tmux (session multiplexer)
- Go 1.24+ (for building from source)

**Optional (for specific agents):**
- Claude CLI (for Claude agent)
- Gemini CLI (for Gemini agent)
- OpenAI CLI (for GPT agent)

## Installation & Setup

### How do I install AGM?

```bash
# Install via Go
go install github.com/vbonnet/ai-tools/agm/cmd/agm@latest

# Verify installation
agm --version

# Enable bash completion (recommended)
cd ~/src/ai-tools/agm
./scripts/setup-completion.sh
```

**Detailed:** See [GETTING-STARTED.md](GETTING-STARTED.md#installation)

### Do I need to configure API keys for all agents?

**No**, only for agents you plan to use.

**Claude:**
```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

**Gemini:**
```bash
export GEMINI_API_KEY="AIza..."
export GOOGLE_GENAI_USE_VERTEXAI=false
```

**GPT:**
```bash
export OPENAI_API_KEY="sk-..."
```

**Environment setup help:**
```bash
agm doctor <agent>  # Validates and guides setup
```

### How do I enable bash completion?

```bash
# Run setup script
cd ~/src/ai-tools/agm
./scripts/setup-completion.sh

# Or manually
cp scripts/agm-completion.bash ~/.agm-completion.bash
echo 'source ~/.agm-completion.bash' >> ~/.bashrc
source ~/.bashrc
```

**Features:**
- Command completion: `agm k<TAB>` → `agm kill`
- Session name completion
- No file fallback

### Where is the configuration file?

**Default location:** `~/.config/agm/config.yaml`

**Create if missing:**
```bash
mkdir -p ~/.config/agm
cat > ~/.config/agm/config.yaml <<EOF
defaults:
  interactive: true
  auto_associate_uuid: true
  confirm_destructive: true

ui:
  theme: "agm"
  picker_height: 15
EOF
```

**Documentation:** See [USER-GUIDE.md](USER-GUIDE.md#configuration)

### How do I update AGM to the latest version?

```bash
# Reinstall via Go
go install github.com/vbonnet/ai-tools/agm/cmd/agm@latest

# Verify new version
agm --version
```

## Session Management

### How do I create a session?

```bash
# Interactive form (prompts for name, agent, project)
agm new

# Quick create with defaults
agm new my-session

# Specify agent and project
agm new --harness gemini-cli research-task --project ~/research
```

**Detailed:** See [CLI-REFERENCE.md](CLI-REFERENCE.md#agm-new)

### How do I resume a session?

```bash
# Interactive picker (if multiple sessions)
agm

# Resume by name
agm resume my-session

# Fuzzy matching
agm my-ses  # Matches "my-session"
```

### What's the difference between "active", "stopped", and "archived"?

**Active:**
- tmux session attached (you're in it)
- Actively working
- Shows in `agm list` by default

**Stopped:**
- tmux session detached or killed
- Not actively working, but not archived
- Shows in `agm list` by default

**Archived:**
- Marked as archived (completed or old)
- Excluded from `agm list` by default
- Can be restored with `agm unarchive`
- Suggested for deletion after 90 days (configurable)

### How do I archive a session?

```bash
# Archive single session
agm archive my-session

# Batch cleanup (interactive)
agm clean
```

**What happens:**
- Session marked as archived
- tmux session killed if running
- Files remain in `~/.claude-sessions/<session-name>/`
- Can be restored with `agm unarchive`

### Can I restore an archived session?

**Yes!**

```bash
# Exact match
agm unarchive my-session

# Pattern matching
agm unarchive *research*
agm unarchive "session-202?"
```

**Detailed:** See [CLI-REFERENCE.md](CLI-REFERENCE.md#agm-unarchive)

### How do I delete a session permanently?

**AGM doesn't have a delete command yet** (safety by design).

**Manual deletion:**
```bash
# Archive first
agm archive my-session

# Then delete directory
rm -rf ~/.claude-sessions/my-session
```

**Batch cleanup suggestion:**
- Archived sessions >90 days are suggested for deletion in `agm clean`

### Can I rename a session?

**Not directly via AGM** (limitation).

**Workaround:**
```bash
# 1. Archive old session
agm archive old-name

# 2. Create new session with new name
agm new new-name

# 3. Manually copy conversation history if needed
```

**Future feature:** Session rename is on the roadmap

### Where are sessions stored?

**Session manifests:** `~/.claude-sessions/<session-name>/manifest.json`

**Agent conversation history:**
- Claude: `~/.claude/history.jsonl`
- Gemini: Remote (Google servers)
- GPT: Remote (OpenAI servers)

**Session environment:** `~/.claude-sessions/<session-name>/session-env/`

## Multi-Agent Support

### Which agents does AGM support?

**Currently supported:**
- **Claude** (Anthropic) - Code, reasoning, debugging
- **Gemini** (Google) - Research, summarization, large context
- **GPT** (OpenAI) - Chat, brainstorming, general tasks

**Future:** Extensible architecture allows adding more agents

### How do I choose which agent to use?

**Quick guide:**

- **Claude** - Code generation, debugging, multi-step reasoning
- **Gemini** - Research, massive context (1M tokens), document analysis
- **GPT** - General chat, quick questions, brainstorming

**Detailed comparison:** See [AGENT-COMPARISON.md](AGENT-COMPARISON.md)

### Can I switch agents for an existing session?

**No**, agent is set during session creation and cannot be changed.

**Workaround:**
1. Archive old session
2. Create new session with different agent
3. Reference old session context in new session

### Can I use multiple agents for the same project?

**Yes!** Create separate sessions with different agents:

```bash
# Research with Gemini
agm new --harness gemini-cli myapp-research --project ~/projects/myapp

# Implementation with Claude
agm new --harness claude-code myapp-code --project ~/projects/myapp

# Quick questions with GPT
agm new --harness codex-cli myapp-questions --project ~/projects/myapp
```

**Pattern:** Use session naming to indicate agent purpose

### What is Command Translation?

**Command Translation** provides unified commands across agents using the `CommandTranslator` abstraction.

**Supported commands:**
- `RenameSession` - Rename agent conversation
- `SetDirectory` - Set working directory context
- `RunHook` - Execute initialization hook

**Implementation:**
- Claude: tmux send-keys (slash commands)
- Gemini: API calls (UpdateConversationTitle)
- GPT: API calls (thread metadata)

**Graceful degradation:** Unsupported commands return `ErrNotSupported`

**Documentation:** See [COMMAND-TRANSLATION-DESIGN.md](COMMAND-TRANSLATION-DESIGN.md)

### What is agent routing?

**Agent routing** (AGENTS.md) automatically selects agents based on session names/keywords.

**Status:** Infrastructure complete, integration pending

**Example (future):**
```yaml
# ~/.config/agm/AGENTS.md
default_agent: claude
preferences:
  - keywords: [research, summarize]
    agent: gemini
  - keywords: [code, debug]
    agent: claude
```

**Then:**
```bash
agm new research-papers      # Auto-selects gemini
agm new debug-api            # Auto-selects claude
```

**Current workaround:** Use explicit `--harness` flag

## UUID & Association

### What is a UUID and why does it matter?

**UUID** (Universally Unique Identifier) links AGM sessions to agent conversations.

**Purpose:**
- Resume conversations across sessions
- Maintain conversation history
- Sync with agent's conversation store

**Agent-specific:**
- Claude: Session ID from `~/.claude/history.jsonl`
- Gemini: Conversation ID from API
- GPT: Thread ID from API

### How does UUID auto-detection work?

**Auto-detection process:**
1. AGM creates session and starts agent
2. Monitors agent output during initialization
3. Extracts UUID from history files or API responses
4. Associates UUID with session manifest

**Confidence levels:**
- **High** (<2.5 min old) → Auto-applied
- **Medium** (2.5-5 min old) → Confirmation prompt
- **Low** (>5 min old) → Listed in suggestions

### What if UUID is not detected automatically?

**Manual association:**

```bash
# Fix specific session
agm fix my-session

# Fix all sessions (high confidence only)
agm fix --all

# Scan all unassociated sessions
agm fix
```

**UUID suggestions:**
1. Auto-detected from history (high confidence)
2. Recent UUIDs from history file
3. Manual entry option

**Detailed:** See [CLI-REFERENCE.md](CLI-REFERENCE.md#agm-fix)

### Can I clear a UUID association?

**Yes:**

```bash
agm fix --clear my-session
```

**Use case:** Wrong UUID was associated, need to re-associate

### What happens if two sessions have the same UUID?

**Problem:** Duplicate UUIDs cause conversation conflicts

**Detection:**
```bash
agm doctor  # Detects duplicate UUIDs
```

**Fix:**
```bash
# Clear duplicate
agm fix --clear session-with-wrong-uuid

# Re-associate correct UUID
agm fix session-with-wrong-uuid
```

## Troubleshooting

### My session disappeared after logout, why?

**Cause:** User lingering not enabled (systemd)

**Fix:**
```bash
# Enable lingering
loginctl enable-linger $USER

# Verify
agm doctor
```

**What is lingering?**
- Allows user sessions to persist after logout
- Required for tmux sessions to survive logout

### Session stuck in "thinking" state, how do I recover?

**Recovery:**

```bash
# Send interrupt + diagnosis prompt
agm session send my-session --prompt "⚠️ Your session was stuck. Please analyze what caused the hang."

# Or reject current operation
agm session reject my-session --reason "Timeout exceeded, moving on"
```

**Prevention:**
- Monitor long-running operations
- Use timeouts for API calls

### UUID not detected, how do I fix it?

**Check history file:**
```bash
cat ~/.claude/history.jsonl | tail -5
```

**If empty:**
- Send a message in Claude
- Wait 30 seconds
- Run `agm fix --all`

**If not empty:**
```bash
# Manual association
agm fix my-session
# (Select UUID from suggestions)
```

### Session health check failed, what do I do?

**Run validation:**
```bash
agm doctor --validate
```

**Common issues:**

**1. Version mismatch:**
```bash
# Auto-fix (safe)
agm doctor --validate --fix
```

**2. Compacted JSONL:**
```bash
# Risky fix (with backup)
agm doctor --validate --fix
# (Requires confirmation)
```

**3. Lock contention:**
```bash
# Force resume
agm resume my-session --force
```

**Detailed:** See [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

### Agent command failed, how do I debug?

**Verbose output:**
```bash
agm resume my-session --verbose
```

**Check agent logs:**
- Claude: `~/.claude/`
- Gemini: Check API logs in GCP Console
- GPT: Check API logs in OpenAI Dashboard

**Environment validation:**
```bash
agm doctor <agent>
```

### How do I report a bug?

**Before reporting:**
1. Run `agm doctor --validate`
2. Check [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
3. Search existing issues

**Report:**
- GitHub Issues: https://github.com/vbonnet/ai-tools/issues
- Include: AGM version, OS, error message, steps to reproduce

## Performance & Optimization

### AGM is slow, how can I optimize it?

**Check health:**
```bash
agm doctor
```

**Optimize config:**
```yaml
# ~/.config/agm/config.yaml
advanced:
  health_check_cache: "10s"    # Increase cache duration
  tmux_timeout: "3s"           # Reduce timeout
  uuid_detection_window: "3m"  # Reduce detection window
```

**Cleanup old sessions:**
```bash
agm clean  # Archive/delete old sessions
```

### How many sessions can I have?

**Practical limit:** ~500 sessions

**Performance impact:**
- Listing: Negligible (<1s for 500 sessions)
- Health checks: ~1s per 100 sessions
- Storage: ~10MB per session (manifest + logs)

**Recommendation:**
- Regular cleanup (weekly `agm clean`)
- Archive completed sessions
- Delete old archived sessions (>90 days)

### How do I speed up session listing?

**Use simple format:**
```bash
# Fastest (names only)
agm list --format=simple

# Fast (JSON, no formatting)
agm list --format=json

# Slower (table with formatting)
agm list  # default
```

**Filter results:**
```bash
# Limit to specific agent
agm list --harness claude-code

# Limit to active sessions
agm list  # default (excludes archived)
```

## Security & Privacy

### Where are my API keys stored?

**AGM does NOT store API keys.**

**User responsibility:**
- Set environment variables (e.g., `ANTHROPIC_API_KEY`)
- Use password managers for secure storage
- Never commit API keys to version control

**Recommended approach:**
```bash
# Use password manager
export GEMINI_API_KEY=$(pass show gemini-api-key)

# Or use direnv per-project
# .envrc
export GEMINI_API_KEY=$(pass show gemini-api-key)
```

### Is my conversation history private?

**Yes**, with caveats:

**Local storage:**
- AGM stores manifests locally (`~/.claude-sessions/`)
- Claude history stored locally (`~/.claude/history.jsonl`)

**Remote storage:**
- Gemini: Conversations stored on Google servers
- GPT: Conversations stored on OpenAI servers

**Access control:**
- Sessions are per-user (not shared across users)
- File permissions: `~/.claude-sessions/` (700, user-only)

**Recommendation:**
- Encrypt disk for sensitive conversations
- Regular cleanup of old sessions
- Be aware of cloud provider data retention policies

### Can other users access my sessions?

**No**, sessions are user-specific.

**Isolation:**
- Session files: `~/.claude-sessions/` (user home directory)
- tmux sessions: Named with user prefix
- File permissions: 700 (user-only)

**Multi-user system:**
- Each user has separate AGM state
- No cross-user session access

### How do I securely share a session with a teammate?

**AGM sessions are not designed for sharing.**

**Workaround:**
1. Export conversation summary
2. Share summary (not session files)
3. Teammate creates their own session

**Example:**
```bash
# Your session
agm resume my-session

# Generate summary
# > "Summarize this conversation in 10 bullet points"

# Share summary with teammate (copy-paste)

# Teammate creates session
agm new their-session
# > "Context: [paste summary]"
```

### Should I commit AGM sessions to version control?

**NO!**

**Reasons:**
- Sessions contain conversation history (potentially sensitive)
- UUIDs are user-specific (not portable)
- Manifests include absolute paths
- Large files (logs, history)

**Gitignore:**
```bash
echo '.claude-sessions/' >> ~/.gitignore
echo '.envrc' >> ~/.gitignore
```

## Advanced Usage

### Can I run AGM in CI/CD?

**Limited support** (AGM is designed for interactive use).

**Non-interactive mode:**
```bash
# Disable colors
agm list --no-color

# JSON output
agm list --format=json

# Health checks
agm doctor --validate --json
```

**Use cases:**
- Health monitoring
- Session inventory
- Cleanup automation

**Not recommended:**
- Creating sessions in CI/CD (requires tmux)
- Interactive operations

### How do I automate session cleanup?

**Weekly cleanup script:**

```bash
#!/bin/bash
# weekly-cleanup.sh

# Archive stopped sessions >30 days
agm list --format=json | jq -r '
  .[] |
  select(.status == "stopped") |
  select(.updated < (now - 2592000)) |
  .name
' | while read session; do
  agm archive "$session" --force
done

echo "Cleanup complete"
```

**Cron job:**
```bash
# Run every Sunday at 2 AM
0 2 * * 0 ~/scripts/weekly-cleanup.sh
```

### Can I use AGM programmatically?

**Yes**, via JSON output:

```bash
# List sessions (JSON)
agm list --format=json

# Health check (JSON)
agm doctor --validate --json

# Parse with jq
SESSION_COUNT=$(agm list --format=json | jq 'length')
echo "Total sessions: $SESSION_COUNT"
```

**Go library:**
- AGM packages can be imported in Go programs
- See `internal/` packages for APIs
- Not yet documented for external use

### How do I backup my sessions?

**Backup manifests:**
```bash
# Backup
tar -czf agm-backup-$(date +%Y%m%d).tar.gz ~/.claude-sessions/

# Restore
tar -xzf agm-backup-20260203.tar.gz -C ~/
```

**Backup agent history:**
```bash
# Claude
cp ~/.claude/history.jsonl ~/.claude/history.jsonl.backup

# Gemini/GPT (stored remotely, no local backup needed)
```

**Recommendation:**
- Weekly backups
- Store backups off-machine (cloud, external drive)
- Test restore procedure periodically

### Can I extend AGM with plugins?

**Not currently supported.**

**Extensibility:**
- AGM has modular architecture (`internal/` packages)
- Agent abstraction allows adding new agents
- Command translator supports new command types

**Future feature:** Plugin system is on the roadmap

### How do I contribute to AGM?

**Welcome!**

**Steps:**
1. Fork repository
2. Read [CONTRIBUTING.md](../CONTRIBUTING.md)
3. Create feature branch
4. Implement feature with tests
5. Submit pull request

**Areas for contribution:**
- New agent integrations
- UI/UX improvements
- Documentation
- Bug fixes
- Test coverage

**Repository:** https://github.com/vbonnet/ai-tools/tree/main/agm

## Getting More Help

### Where can I find more documentation?

**Core documentation:**
- [Getting Started Guide](GETTING-STARTED.md) - Quick onboarding
- [User Guide](USER-GUIDE.md) - Comprehensive usage
- [Examples](EXAMPLES.md) - Real-world scenarios
- [CLI Reference](CLI-REFERENCE.md) - Complete command reference

**Specialized guides:**
- [Agent Comparison](AGENT-COMPARISON.md) - Choosing the right agent
- [Troubleshooting](TROUBLESHOOTING.md) - Common issues
- [Accessibility](ACCESSIBILITY.md) - Screen reader support
- [BDD Catalog](BDD-CATALOG.md) - Living documentation

**For developers:**
- [Contributing](../CONTRIBUTING.md) - Development setup
- [Test Plan](../TEST-PLAN.md) - Testing strategy
- [Command Translation Design](COMMAND-TRANSLATION-DESIGN.md) - Architecture

### How do I get support?

**Self-service:**
1. Check [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
2. Run `agm doctor --validate`
3. Search this FAQ

**Community support:**
- GitHub Issues: https://github.com/vbonnet/ai-tools/issues
- GitHub Discussions: https://github.com/vbonnet/ai-tools/discussions

**Before asking:**
- Search existing issues
- Include AGM version, OS, error message
- Provide steps to reproduce

### How can I stay updated on new features?

**Follow development:**
- GitHub Releases: https://github.com/vbonnet/ai-tools/releases
- Changelog: [CHANGELOG.md](../CHANGELOG.md)
- Repository: Watch for updates

**Roadmap:**
- Agent routing (AGENTS.md integration)
- Session rename command
- Plugin system
- Additional agents

---

**Last updated:** 2026-02-03
**AGM Version:** 3.0
**Maintained by:** Foundation Engineering

**Didn't find your answer?** Open an issue: https://github.com/vbonnet/ai-tools/issues
