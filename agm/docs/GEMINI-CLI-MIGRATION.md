# Gemini CLI Migration Guide

**Guide for users migrating to or from Gemini CLI within AGM**

**Last Updated**: 2026-03-11
**AGM Version**: 2.0+
**Gemini Parity Score**: 94/100

---

## Overview

AGM achieves 94/100 feature parity between Claude Code and Gemini CLI adapters, enabling seamless cross-agent workflows. This guide covers migration patterns, compatibility considerations, and best practices.

---

## Quick Start: Switching Agents

### From Claude to Gemini

**Scenario**: You're working in a Claude session but need Gemini's larger context window (1M tokens vs 200K).

```bash
# Export current Claude session
agm export my-claude-session --format jsonl > conversation.jsonl

# Create new Gemini session
agm new --harness gemini-cli my-gemini-session

# Import conversation history
agm import my-gemini-session conversation.jsonl

# Resume work in Gemini
agm attach my-gemini-session
```

**What's Preserved**:
- ✅ Full conversation history
- ✅ Message timestamps
- ✅ User/assistant roles
- ✅ Working directory context

**What's NOT Preserved**:
- ❌ Session-specific metadata (UUID, tmux configuration)
- ❌ Real-time state (you start fresh in Gemini)

### From Gemini to Claude

**Scenario**: You need Claude's superior reasoning for a complex problem.

```bash
# Export Gemini session
agm export research-session --format jsonl > gemini-data.jsonl

# Create Claude session
agm new --harness claude-code problem-solving

# Import and continue
agm import problem-solving gemini-data.jsonl
agm attach problem-solving
```

---

## Feature Parity Matrix

### Fully Compatible Features (100% Parity)

These work identically across Claude and Gemini:

| Feature | Claude | Gemini | Notes |
|---------|--------|--------|-------|
| Session Creation | ✅ | ✅ | `agm new --harness {harness}` |
| Session Resume | ✅ | ✅ | `agm resume <session>` |
| Message Sending | ✅ | ✅ | `agm send <session> "message"` |
| History Retrieval | ✅ | ✅ | `agm history <session>` |
| JSONL Export | ✅ | ✅ | Full conversation export |
| Markdown Export | ✅ | ✅ | Human-readable format |
| Session Termination | ✅ | ✅ | `agm terminate <session>` |
| Directory Authorization | ✅ | ✅ | `--add-dir` flag |
| Command Execution | ✅ | ✅ | CommandSetDir, CommandRename, etc. |

### Partial Parity Features (90-95%)

Minor differences in implementation:

| Feature | Claude | Gemini | Delta |
|---------|--------|--------|-------|
| UUID Persistence | ✅ Full | ✅ Extraction-based | Gemini extracts UUID from CLI output |
| HTML Export | ✅ Native | ❌ Not supported | Use JSONL or Markdown for Gemini |
| Real-time Streaming | ✅ Native | ⚠️ Buffered | Tmux buffering affects streaming UX |

### Architecture Differences

| Aspect | Claude | Gemini |
|--------|--------|--------|
| Backend | Claude Code CLI | Gemini CLI (`gemini`) |
| Tmux Integration | Direct attach | Session wrapping |
| API Key | `ANTHROPIC_API_KEY` | `GEMINI_API_KEY` |
| Context Window | 200K tokens | 1M tokens |
| Resume Mechanism | UUID + `--resume` | UUID extraction + `--resume` |

---

## Migration Patterns

### Pattern 1: Research → Implementation

**Use Case**: Start research in Gemini (large context), move to Claude for implementation.

```bash
# Phase 1: Research in Gemini
agm new --harness gemini-cli research --add-dir ~/docs,~/papers
# (Process 100s of documents, 500K+ tokens)

# Phase 2: Export findings
agm export research --format markdown > findings.md

# Phase 3: Implement in Claude
agm new --harness claude-code implementation --add-dir ~/code
agm send implementation "Based on findings.md, implement..."
```

**Why**: Gemini's 1M token window handles research, Claude's reasoning excels at implementation.

### Pattern 2: Debugging → Context Expansion

**Use Case**: Hit Claude's 200K context limit during debugging.

```bash
# Phase 1: Initial debugging in Claude
agm new --harness claude-code debug-session
# (Reach 200K token limit)

# Phase 2: Migrate to Gemini for more context
agm export debug-session --format jsonl > debug.jsonl
agm new --harness gemini-cli debug-expanded
agm import debug-expanded debug.jsonl
# (Continue with up to 1M tokens)
```

**Why**: Gemini's larger context accommodates massive log files or codebases.

### Pattern 3: Team Collaboration

**Use Case**: Multiple team members working on same task with different agents.

```bash
# Engineer A (Claude user)
agm export team-task --format jsonl > team-task.jsonl

# Share file via git/slack/etc.

# Engineer B (Gemini user)
agm new --harness gemini-cli team-task-continue
agm import team-task-continue team-task.jsonl
# (Pick up where Engineer A left off)
```

**Why**: JSONL format provides agent-agnostic history interchange.

---

## Compatibility Considerations

### JSONL Format

**Fully Compatible** - Use for all cross-agent migrations:

```json
{"role": "user", "content": "What is 2+2?", "timestamp": "2026-03-11T10:00:00Z"}
{"role": "assistant", "content": "2 + 2 = 4", "timestamp": "2026-03-11T10:00:05Z"}
```

Both Claude and Gemini adapters read/write this format identically.

### Markdown Format

**Compatible with caveats**:
- ✅ Human-readable export works for both
- ⚠️ Import from Markdown not fully tested (use JSONL for safety)

### HTML Format

**Gemini Limitation**:
- ❌ Gemini CLI does not support HTML export
- ✅ Workaround: Use `agm export --format markdown` or `--format jsonl`

### Directory Authorization

**Identical Behavior**:

```bash
# Works the same for both agents
agm new --harness claude-code project --add-dir ~/code,~/docs,~/data
agm new --harness gemini-cli project --add-dir ~/code,~/docs,~/data
```

Both agents pre-authorize directories to avoid interactive trust prompts.

### Resume Behavior

**Slightly Different but Compatible**:

| Agent | Resume Command | UUID Source |
|-------|---------------|-------------|
| Claude | `claude --resume <uuid>` | Native session tracking |
| Gemini | `gemini --resume <uuid>` | Extracted from `--list-sessions` |

**Impact**: None for end users - AGM abstracts the difference.

---

## Best Practices

### When to Migrate

✅ **DO migrate when:**
- You hit context window limits (Claude 200K → Gemini 1M)
- You need different agent strengths (reasoning vs context)
- Collaborating across teams with different agent preferences

❌ **DON'T migrate when:**
- You're mid-task with complex state (finish first, then migrate)
- You need HTML export (stay with Claude)
- Migration would lose critical context

### Migration Workflow

1. **Export at clean stopping points** (end of conversation turn, not mid-reasoning)
2. **Always use JSONL format** for maximum compatibility
3. **Test import immediately** after export to verify data integrity
4. **Include context in new session** - add directories, files, etc.
5. **Summarize previous work** in first message after import

### Preserving Context

```bash
# Good: Export with full context
agm export old-session --format jsonl --include-metadata > full-context.jsonl

# Better: Add context summary when importing
agm import new-session full-context.jsonl
agm send new-session "Previous session context: [summary of key points]"
```

### Testing Migrations

Always test with non-critical sessions first:

```bash
# Create test session
agm new --harness claude-code test-migration
agm send test-migration "Test message 1"
agm send test-migration "Test message 2"

# Export
agm export test-migration --format jsonl > test.jsonl

# Import to different agent
agm new --harness gemini-cli test-migration-target
agm import test-migration-target test.jsonl

# Verify
agm history test-migration-target
# (Should show both test messages)
```

---

## Troubleshooting

### Import Fails with "Invalid Format"

**Cause**: Non-JSONL file or corrupted export

**Fix**:
```bash
# Verify file format
head test.jsonl
# Should show valid JSON lines, not HTML/Markdown

# Re-export with explicit format
agm export session --format jsonl > session.jsonl
```

### Missing Messages After Import

**Cause**: Partial export or import cutoff

**Fix**:
```bash
# Check export completeness
wc -l session.jsonl
# Compare with original session message count

# Re-export if needed
agm export session --format jsonl --full > session-full.jsonl
```

### UUID Not Persisting After Resume

**Cause**: Gemini CLI output parsing issue (rare)

**Impact**: Low - session resumes with "latest" instead of specific UUID

**Fix**: None needed - graceful degradation works

### Directory Authorization Lost

**Cause**: Directories not re-authorized in new session

**Fix**:
```bash
# Re-authorize when creating new session
agm new --harness gemini-cli new-session --add-dir ~/dir1,~/dir2
```

---

## Advanced: Automated Migration Scripts

### Batch Migration

Migrate multiple sessions from Claude to Gemini:

```bash
#!/bin/bash
# migrate-claude-to-gemini.sh

for session in $(agm list --harness claude-code --format plain); do
  echo "Migrating $session..."
  agm export "$session" --format jsonl > "/tmp/${session}.jsonl"
  agm new --harness gemini-cli "${session}-gemini"
  agm import "${session}-gemini" "/tmp/${session}.jsonl"
  echo "✓ Migrated $session → ${session}-gemini"
done
```

### Conditional Migration

Migrate only if context exceeds threshold:

```bash
#!/bin/bash
# migrate-if-large.sh

SESSION=$1
THRESHOLD=150000  # 150K tokens

TOKEN_COUNT=$(agm history "$SESSION" --format jsonl | wc -c)

if [ "$TOKEN_COUNT" -gt "$THRESHOLD" ]; then
  echo "Session $SESSION exceeds 150K tokens, migrating to Gemini..."
  agm export "$SESSION" --format jsonl > "/tmp/${SESSION}.jsonl"
  agm new --harness gemini-cli "${SESSION}-large"
  agm import "${SESSION}-large" "/tmp/${SESSION}.jsonl"
  echo "✓ Migrated to ${SESSION}-large"
else
  echo "Session $SESSION within limits, no migration needed"
fi
```

---

## FAQ

**Q: Can I run both agents simultaneously?**
A: Yes! AGM supports multiple concurrent sessions across different agents:
```bash
agm new --harness claude-code coding-task
agm new --harness gemini-cli research-task
# Both run independently
```

**Q: Will migrating lose my conversation history?**
A: No - JSONL export/import preserves full conversation history with timestamps and roles.

**Q: What happens to UUID after migration?**
A: New session gets new UUID. Old UUID is not migrated (each agent manages UUIDs independently).

**Q: Can I go back and forth between agents?**
A: Yes, but each migration creates a new session. Prefer doing focused work in one agent, then migrating at clean stopping points.

**Q: Does migration affect billing?**
A: No - you're only billed for actual API calls to each agent. Exporting/importing is local metadata manipulation.

---

## See Also

- [docs/AGENT-COMPARISON.md](./AGENT-COMPARISON.md) - When to use each agent
- [docs/agents/gemini-cli.md](./agents/gemini-cli.md) - Gemini CLI user guide
- [docs/gemini-parity-analysis.md](./gemini-parity-analysis.md) - Detailed parity breakdown
- [SPEC.md](../SPEC.md) - Technical specification

---

**Migration Guide Version**: 1.0
**Last Updated**: 2026-03-11
**Parity Score**: 94/100
