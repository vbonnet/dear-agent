# AGM Quick Reference

One-page cheat sheet for AGM (AI/Agent Session Manager) CLI.

---

## Essential Commands

```bash
# Create new session
agm new my-session

# Create with specific agent
agm new --harness gemini-cli research-task
agm new --harness claude-code code-review

# Resume session
agm resume my-session
agm my-session              # Shorthand

# List sessions
agm list                    # Active/stopped only
agm list --all              # Include archived

# Kill session
agm kill my-session
```

---

## Agent Selection

```bash
# List available agents
agm agent list

# Create with agent
agm new --harness claude-code   code-task      # Code, reasoning
agm new --harness gemini-cli   research       # Research, 1M context
agm new --harness codex-cli      brainstorm     # Chat, ideas
```

**Quick Decision**:
- Code/reasoning → `claude`
- Research/summarization → `gemini`
- Brainstorming/chat → `gpt`

---

## Session Lifecycle

```bash
# Archive old session
agm archive old-project

# Restore archived session
agm unarchive old-project
agm unarchive *acme*      # Pattern matching

# Interactive cleanup
agm clean

# Search archived sessions (AI)
agm search "OAuth work"
```

---

## Session Communication

```bash
# Send message to running session
agm session send my-session --prompt "Review the code"
agm session send my-session --prompt-file ~/prompts/task.txt

# Reject permission with reason
agm session reject my-session --reason "Use Read tool instead"
```

---

## Health & Debugging

```bash
# System health check
agm doctor
agm doctor --validate       # Functional testing
agm doctor --validate --fix # Auto-fix issues

# Fix UUID associations
agm fix                     # Scan all
agm fix my-session         # Specific session
agm fix --all              # Auto-fix all

# Unlock stuck session
agm unlock my-session
```

---

## Advanced Features

```bash
# Workflows
agm workflow list
agm new --harness gemini-cli --workflow deep-research url-task

# Backup/Restore
agm backup list my-session
agm backup restore my-session 3

# Logs
agm logs stats
agm logs clean --older-than 30
agm logs query --sender astrocyte --since 2026-02-01

# Migration
agm migrate --to-unified-storage --dry-run
agm migrate --to-unified-storage --workspace=oss
```

---

## Global Flags

```bash
-C <dir>        # Working directory
--debug         # Debug logging
--no-color      # Disable colors
--screen-reader # Accessibility mode
--json          # JSON output (where supported)
```

---

## Common Patterns

### Quick Session
```bash
agm new task && agm attach task
```

### Multi-Agent Workflow
```bash
agm new --harness claude-code code-review
agm new --harness gemini-cli research
agm list  # See all sessions
```

### Cleanup Old Sessions
```bash
agm list --all              # Review all
agm clean                   # Interactive cleanup
```

### Find Old Work
```bash
agm search "OAuth integration"
agm unarchive *oauth*
```

---

## Configuration

**Location**: `~/.config/agm/config.yaml`

```yaml
defaults:
  interactive: true
  auto_associate_uuid: true
  cleanup_threshold_days: 30

ui:
  theme: "agm"              # agm, agm-light, dracula
  fuzzy_search: true

advanced:
  tmux_timeout: "5s"
```

---

## Environment Variables

```bash
# API Keys
export ANTHROPIC_API_KEY=...  # Claude
export GEMINI_API_KEY=...     # Gemini
export OPENAI_API_KEY=...     # GPT

# Google Cloud (for search)
export GOOGLE_CLOUD_PROJECT=...

# Debug
export AGM_DEBUG=true
```

---

## Status Indicators

- `active` - Session running in tmux
- `stopped` - Session not running
- `archived` - Session archived (hidden by default)

---

## Getting Help

```bash
agm --help              # General help
agm new --help          # Command help
agm version             # Show version
```

**Documentation**:
- Full reference: [AGM-COMMAND-REFERENCE.md](AGM-COMMAND-REFERENCE.md)
- Agent comparison: [AGENT-COMPARISON.md](AGENT-COMPARISON.md)
- Troubleshooting: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

---

## Troubleshooting Quick Fixes

```bash
# UUID not detected
agm fix --all

# Session won't resume
agm doctor --validate --fix

# Harness not available
agm agent list
export ANTHROPIC_API_KEY=...

# Stuck session
agm unlock my-session
agm kill my-session
```

---

**Version**: 3.0 | **Updated**: 2026-02-04
