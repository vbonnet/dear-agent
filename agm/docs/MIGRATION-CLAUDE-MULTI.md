# Agent Session Manager → Multi-Agent AGM Migration

Transitioning from Claude-only sessions to multi-agent AGM (Agent Grid Manager).

---

## What Changed

### Conceptual Shift

**Before (AGM - Agent Session Manager):**
- Single agent (Claude only)
- Command: `csm`
- Hardcoded to Claude/Anthropic

**After (AGM - Agent Grid Manager):**
- Multi-agent (Claude, Gemini, GPT)
- Command: `csm` or `agm` (both work)
- Harness selection via `--harness` flag

**Evolution:** AGM evolved from Agent Session Manager. The name changed to reflect multi-agent support, but existing AGM users can continue using familiar workflows.

---

## Command Changes

### Command Compatibility

**Good news:** `csm` commands still work! AGM maintains backward compatibility.

```bash
# Both commands work identically:
csm new my-session --harness claude-code
agm new my-session --harness claude-code

# Both list sessions:
csm list
agm list

# Both resume sessions:
csm resume my-session
agm resume my-session
```

**Recommendation:** Transition to `agm` for clarity, but `csm` alias remains supported.

---

### New: Agent Selection

**AGM (old - Claude only):**
```bash
# Implicitly used Claude
csm new my-session
```

**AGM (new - multi-agent):**
```bash
# Explicitly choose agent
agm new my-session --harness claude-code   # Claude (code, reasoning)
agm new my-session --harness gemini-cli   # Gemini (research, 1M context)
agm new my-session --harness codex-cli      # GPT (chat, brainstorming)
```

**Default behavior:** If you omit `--harness`, AGM uses Claude Code (for AGM compatibility).

---

## Migration Checklist

### For Existing AGM Users

- [ ] **No action required** - Existing sessions continue working
- [ ] **Optional:** Update commands from `csm` to `agm` for clarity
- [ ] **Optional:** Try other agents (Gemini, GPT) for different tasks
- [ ] **Optional:** Set up API keys for Gemini/GPT if desired

### For New Multi-Agent Users

- [ ] **Choose agent** for each new session based on task
- [ ] **Use `agm`** command for clarity (or `csm` alias)
- [ ] **Configure API keys** for desired agents (Claude, Gemini, GPT)
- [ ] **Read:** [AGENT-COMPARISON.md](AGENT-COMPARISON.md) to choose right agent

---

## Session Compatibility

### Existing Claude Sessions

**Question:** What happens to my existing Claude sessions?

**Answer:** They continue working without changes.

```bash
# Old session created with AGM
# Before AGM migration:
csm new old-session  # Used Claude by default

# After AGM migration:
csm resume old-session       # ✅ Works (uses Claude)
agm resume old-session       # ✅ Also works (same session, uses Claude)
```

**Agent persistence:** Sessions remember which agent they use. Existing Claude sessions continue using Claude.

---

### Mixing Agents

You can have multiple sessions with different agents simultaneously:

```bash
# Create sessions with different agents
agm new code-work --harness claude-code      # For coding
agm new research --harness gemini-cli       # For research
agm new chat --harness codex-cli              # For brainstorming

# List all sessions (shows agent)
agm list
# Output:
# code-work (claude)
# research (gemini)
# chat (gpt)

# Resume any session (agent auto-detected)
agm resume code-work   # Uses Claude automatically
agm resume research    # Uses Gemini automatically
```

---

## Workflow Changes

### Before (AGM): Single-Agent Workflow

```bash
# 1. Create session (always Claude)
csm new my-project

# 2. Resume session
csm resume my-project

# 3. Archive when done
csm archive my-project
```

### After (AGM): Multi-Agent Workflow

```bash
# 1. Choose agent for task
# Code work → Claude
agm new my-code-project --harness claude-code

# Research → Gemini
agm new my-research --harness gemini-cli

# 2. Resume session (agent auto-detected)
agm resume my-code-project    # Uses Claude
agm resume my-research        # Uses Gemini

# 3. Archive when done (unchanged)
agm archive my-code-project
```

**Key change:** Explicit agent selection during session creation.

---

## API Key Configuration

### AGM (Claude only)

**Before:**
```bash
# Only needed Claude API key
export ANTHROPIC_API_KEY=sk-ant-...
```

### AGM (Multi-agent)

**After:**
```bash
# Claude (same as before)
export ANTHROPIC_API_KEY=sk-ant-...

# Gemini (new - optional)
export GOOGLE_API_KEY=AIza...
export GOOGLE_PROJECT_ID=my-project

# GPT (new - optional)
export OPENAI_API_KEY=sk-...
```

**Note:** You only need API keys for agents you plan to use. If you only use Claude, just keep `ANTHROPIC_API_KEY`.

---

## Command Reference

### Session Creation

```bash
# AGM style (still works, defaults to Claude)
csm new my-session

# AGM style (explicit agent)
agm new my-session --harness claude-code
agm new my-session --harness gemini-cli
agm new my-session --harness codex-cli
```

### Session Operations (Unchanged)

```bash
# All these work with both csm/agm:
csm list                # or: agm list
csm resume <session>    # or: agm resume <session>
csm archive <session>   # or: agm archive <session>
csm rename <old> <new>  # or: agm rename <old> <new>
```

### New Features

```bash
# Agent-specific operations (AGM only)
agm new --harness gemini-cli research-session  # Use specific agent
agm list --harness claude-code                  # Filter by agent (if supported)
```

---

## Choosing an Agent

**Not sure which agent to use?** See [AGENT-COMPARISON.md](AGENT-COMPARISON.md).

**Quick guide:**
- **Code, debugging, reasoning:** `--harness claude-code`
- **Research, large docs (>200K tokens):** `--harness gemini-cli`
- **Chat, brainstorming, quick Q&A:** `--harness codex-cli`

---

## Troubleshooting

### Issue: Command not found after migration

**Symptom:**
```bash
csm new my-session
# bash: csm: command not found
```

**Solution:**
AGM may have replaced `csm` binary. Use `agm` instead:
```bash
agm new my-session --harness claude-code
```

Or reinstall AGM to restore `csm` alias.

---

### Issue: Session uses wrong agent

**Symptom:**
Session created without `--harness` flag uses unexpected harness.

**Solution:**
Always specify `--harness` explicitly:
```bash
# ❌ Unclear which agent
agm new my-session

# ✅ Explicit agent choice
agm new my-session --harness claude-code
```

Default harness (when `--harness` omitted) is Claude Code for AGM compatibility, but explicit is better.

---

### Issue: Existing sessions not working

**Symptom:**
Old Claude sessions fail to resume after AGM migration.

**Solution:**
1. Check session still exists: `agm list`
2. Try explicit command: `agm resume <session-name> --harness claude-code`
3. If still failing, check API key: `echo $ANTHROPIC_API_KEY`
4. Restore from archive if needed: `agm unarchive <session-name>`

---

## Gradual Transition

**You don't need to migrate all at once.**

### Phase 1: Keep Using AGM

```bash
# Continue using csm commands
csm new my-session    # Uses Claude by default
csm resume my-session
```

### Phase 2: Try Other Agents

```bash
# Use csm for Claude sessions
csm new code-work  # Claude

# Try agm for other agents
agm new research --harness gemini-cli  # Gemini for research
```

### Phase 3: Adopt AGM Fully

```bash
# Use agm for all sessions with explicit agents
agm new code-work --harness claude-code
agm new research --harness gemini-cli
agm new chat --harness codex-cli
```

---

## FAQ

**Q: Do I need to recreate my existing sessions?**
A: No. Existing sessions continue working without changes.

**Q: Can I still use `csm` commands?**
A: Yes. `csm` and `agm` are interchangeable. Both work.

**Q: Will my old sessions use Gemini or GPT automatically?**
A: No. Existing sessions continue using Claude. New sessions can choose any agent.

**Q: Do I need all three API keys (Claude, Gemini, GPT)?**
A: No. Only configure API keys for agents you plan to use.

**Q: How do I know which agent a session uses?**
A: Run `agm list` to see sessions with their agents.

**Q: Can I change an existing session's agent?**
A: No (not directly). Create a new session with the desired agent and manually migrate conversation history if needed.

---

## Next Steps

1. **Understand agents:** Read [AGENT-COMPARISON.md](AGENT-COMPARISON.md)
2. **Try other agents:** Create test sessions with Gemini or GPT
3. **Troubleshoot issues:** See [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
4. **Explore scenarios:** Check [BDD-CATALOG.md](BDD-CATALOG.md) for agent behavior examples

---

**Ready to leverage multi-agent capabilities!** Use the right tool for each task.
