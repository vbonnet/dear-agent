# AGM Migration Guide

Complete guide for migrating from Agent Session Manager (AGM) to AI/Agent Gateway Manager (AGM) multi-agent architecture.

---

## Overview

**AGM** (AI/Agent Gateway Manager) evolved from Agent Session Manager (AGM) to support multiple AI agents (Claude, Gemini, GPT) while maintaining backward compatibility with existing Claude-only sessions.

**Migration Impact:**
- **Low risk**: Existing sessions continue working without changes
- **Backward compatible**: `csm` command remains as alias for `agm`
- **Incremental adoption**: Try new agents without migrating all sessions at once

---

## Pre-Migration Checklist

Before migrating, verify your environment:

```bash
# 1. Check AGM installation
agm version

# 2. Validate existing sessions
./scripts/agm-migration-validate.sh

# 3. Check available agents
agm agent list

# 4. Backup sessions (recommended)
cp -r ~/.claude-sessions ~/.claude-sessions.backup
```

**Requirements:**
- ✅ AGM installed (`agm` command available)
- ✅ Sessions directory exists (`~/.claude-sessions/`)
- ✅ Manifest schema version 2.0 (check with validation script)
- ✅ API keys configured for desired agents (Claude, Gemini, or GPT)

---

## Migration Scenarios

### Scenario 1: Keep Using Claude (No Changes Needed)

**If you're happy with Claude and don't need other agents:**

```bash
# Continue using existing workflow
agm list                    # Shows all Claude sessions
agm resume my-session       # Resumes Claude sessions as before
agm new coding-work         # Creates new Claude session (default)
```

**No action required.** AGM is backward compatible with AGM.

---

### Scenario 2: Try Other Agents (Incremental Adoption)

**If you want to experiment with Gemini or GPT:**

```bash
# Create new session with Gemini (for research)
agm new --harness gemini-cli research-project

# Create new session with GPT (for brainstorming)
agm new --harness codex-cli ideation-session

# Your existing Claude sessions remain unchanged
agm list  # Shows mix of Claude, Gemini, and GPT sessions
```

**Agent Selection Guide:**
- **Claude**: Code, debugging, long-context reasoning (200K tokens)
- **Gemini**: Research, summarization, massive context (1M tokens)
- **GPT**: Chat, brainstorming, general Q&A

See [AGENT-COMPARISON.md](AGENT-COMPARISON.md) for detailed comparison.

---

### Scenario 3: Fully Adopt Multi-Agent Workflow

**If you want to use different agents for different tasks:**

1. **Configure API keys** for all agents:

```bash
# Add to ~/.bashrc or ~/.zshrc
export ANTHROPIC_API_KEY=sk-ant-...    # Claude
export GOOGLE_API_KEY=AIza...          # Gemini
export OPENAI_API_KEY=sk-...           # GPT
```

2. **Create sessions with explicit agent selection**:

```bash
# Use Claude for code work
agm new --harness claude-code backend-api

# Use Gemini for research tasks
agm new --harness gemini-cli market-analysis

# Use GPT for quick chats
agm new --harness codex-cli daily-standup
```

3. **Resume sessions** (agent auto-detected):

```bash
agm resume backend-api      # Uses Claude
agm resume market-analysis  # Uses Gemini
agm resume daily-standup    # Uses GPT
```

---

## Migration Steps

### Step 1: Validate Existing Sessions

Run validation script to check session compatibility:

```bash
cd main/agm
./scripts/agm-migration-validate.sh
```

**Expected output:**
```
=========================================
AGM Migration Validation
=========================================

ℹ Scanning sessions directory: ~/.claude-sessions

ℹ Validating session: my-session
✓ Session 'my-session': All checks passed (4/4)

=========================================
Validation Summary
=========================================
Total sessions:   5
Valid sessions:   5
Invalid sessions: 0

✓ All sessions are valid and compatible with AGM!
```

**If validation fails**, see [Troubleshooting](#troubleshooting) section.

---

### Step 2: Update Commands (Optional)

**Transition from `csm` to `agm` commands:**

```bash
# Old (AGM)              # New (AGM)
csm new my-session      → agm new my-session
csm list                → agm list
csm resume my-session   → agm resume my-session
csm archive old-work    → agm archive old-work
```

**Note:** `csm` commands still work as aliases. This step is **optional** for clarity.

---

### Step 3: Configure Multi-Agent Support (Optional)

**Add API keys for Gemini and/or GPT** (only if you plan to use them):

```bash
# Add to ~/.bashrc or ~/.zshrc

# Gemini (Google)
export GOOGLE_API_KEY=your_google_api_key
export GOOGLE_PROJECT_ID=your_project_id

# GPT (OpenAI)
export OPENAI_API_KEY=your_openai_api_key

# Reload shell
source ~/.bashrc
```

**Verify agent availability:**
```bash
agm agent list
# Expected output:
# Available agents:
#   claude  (Anthropic) - API key configured
#   gemini  (Google)    - API key configured
#   gpt     (OpenAI)    - API key configured
```

---

### Step 4: Test New Agents (Optional)

**Create test sessions** with different agents:

```bash
# Test Claude (should work - existing setup)
agm new --harness claude-code test-claude
agm resume test-claude
# ... interact with Claude ...
agm archive test-claude

# Test Gemini (if API key configured)
agm new --harness gemini-cli test-gemini
agm resume test-gemini
# ... interact with Gemini ...
agm archive test-gemini

# Test GPT (if API key configured)
agm new --harness codex-cli test-gpt
agm resume test-gpt
# ... interact with GPT ...
agm archive test-gpt
```

---

### Step 5: Adopt Multi-Agent Workflow

**Start using agents based on task requirements:**

```bash
# Code review → Claude
agm new --harness claude-code pr-review-backend

# Research task → Gemini
agm new --harness gemini-cli competitor-analysis

# Brainstorming → GPT
agm new --harness codex-cli product-ideas
```

**List sessions with agents:**
```bash
agm list
# Output shows agent for each session:
# pr-review-backend     (claude)   active
# competitor-analysis   (gemini)   stopped
# product-ideas         (gpt)      active
```

---

## Validation and Verification

### Validate Migration Success

```bash
# 1. Check all sessions are valid
./scripts/agm-migration-validate.sh

# 2. Run health check
agm doctor

# 3. Verify sessions can be resumed
agm list
agm resume <session-name>  # Test resuming a session
```

### Expected Validation Output

**Successful validation:**
```
✓ All sessions are valid and compatible with AGM!
```

**Validation with warnings (informational):**
```
ℹ Session 'old-session': No agent field (v2 manifests pre-date multi-agent support)
  Sessions without agent field default to 'claude' when resumed
✓ Session 'old-session': All checks passed (4/4)
```

This is **normal** for sessions created before multi-agent support. They will work correctly.

---

## Rollback Procedure

**If you encounter issues**, rollback to previous state:

### Option 1: Continue Using AGM Commands

```bash
# Use csm instead of agm (backward compatible)
csm list
csm resume my-session
```

No rollback needed - `csm` command continues working.

### Option 2: Restore Session Backup

```bash
# If you created backup (recommended before migration)
mv ~/.claude-sessions ~/.claude-sessions-migration-backup
mv ~/.claude-sessions.backup ~/.claude-sessions

# Verify sessions restored
csm list
```

### Option 3: Reinstall Previous Version

```bash
# If AGM version is problematic, reinstall previous version
go install github.com/vbonnet/ai-tools/agm/cmd/csm@v2.0.0
```

---

## Troubleshooting

### Issue: Validation Script Fails

**Symptom:**
```
✗ Session 'my-session': manifest.yaml is not valid YAML
```

**Solution:**
```bash
# Check manifest syntax
cat ~/.claude-sessions/my-session/manifest.yaml

# If corrupted, restore from backup or recreate
# See docs/TROUBLESHOOTING.md for manifest repair procedures
```

---

### Issue: Agent Not Available

**Symptom:**
```bash
agm agent list
# Output:
# Available agents:
#   claude  (Anthropic) - API key not configured
```

**Solution:**
```bash
# Add API key to environment
export ANTHROPIC_API_KEY=sk-ant-your-key-here

# Verify
agm agent list
```

---

### Issue: Session Won't Resume

**Symptom:**
```
Error: Failed to resume session 'my-session'
```

**Solution:**
```bash
# 1. Run health check
agm doctor --validate

# 2. Check session manifest
cat ~/.claude-sessions/my-session/manifest.yaml

# 3. Try explicit agent
agm resume my-session --harness claude-code

# 4. Check logs
tail -f ~/.config/agm/logs/agm.log
```

---

### Issue: UUID Format Error

**Symptom:**
```
✗ Session 'old-session': Invalid UUID format: session-old-session
```

**Cause:** Legacy session ID format from very old AGM versions.

**Solution:**
```bash
# Archive old session
agm archive old-session

# Create new session with same name
agm new old-session --harness claude-code

# Manually migrate conversation history if needed
# (advanced - see docs/MANUAL-SESSION-MIGRATION.md)
```

---

## Post-Migration Best Practices

### 1. Explicit Agent Selection

**Always specify agent** when creating sessions:

```bash
# ✅ Explicit (recommended)
agm new --harness claude-code coding-work

# ⚠️ Implicit (defaults to Claude, less clear)
agm new coding-work
```

### 2. Agent-Specific Naming

**Use naming conventions** to indicate agent:

```bash
agm new --harness claude-code   code-review-backend
agm new --harness gemini-cli   research-market-trends
agm new --harness codex-cli      brainstorm-product-ideas
```

### 3. Regular Health Checks

**Run validation periodically:**

```bash
# Weekly health check
agm doctor --validate

# Session validation
./scripts/agm-migration-validate.sh
```

### 4. Backup Strategy

**Backup sessions before major changes:**

```bash
# Before AGM updates
cp -r ~/.claude-sessions ~/.claude-sessions-backup-$(date +%Y%m%d)

# Automatic backup (add to cron)
0 0 * * 0 cp -r ~/.claude-sessions ~/backups/claude-sessions-$(date +\%Y\%m\%d)
```

---

## Migration Timeline Recommendations

### Week 1: Evaluation Phase
- ✅ Run validation script
- ✅ Test AGM with existing Claude sessions
- ✅ Verify backward compatibility

### Week 2: Experimentation Phase
- ✅ Configure API keys for Gemini/GPT
- ✅ Create test sessions with different agents
- ✅ Compare agent capabilities on sample tasks

### Week 3: Adoption Phase
- ✅ Start using agent selection for new sessions
- ✅ Monitor session health with `agm doctor`
- ✅ Document team conventions (agent per task type)

### Week 4: Optimization Phase
- ✅ Review agent usage patterns
- ✅ Archive test sessions
- ✅ Establish team best practices

---

## FAQs

**Q: Do I need to migrate all sessions at once?**
A: No. Existing sessions continue working. Migrate incrementally by creating new sessions with different agents.

**Q: Will my existing Claude sessions break?**
A: No. AGM is backward compatible. Existing sessions use Claude by default.

**Q: Can I change a session's agent after creation?**
A: Not directly. Create new session with desired agent and manually migrate conversation history if needed.

**Q: Do I need API keys for all agents?**
A: No. Only configure API keys for agents you plan to use. Claude-only users only need `ANTHROPIC_API_KEY`.

**Q: What if validation finds issues?**
A: Most issues are auto-fixable. Run `agm doctor --validate --fix`. For manual fixes, see [TROUBLESHOOTING.md](TROUBLESHOOTING.md).

**Q: Can I rollback after migration?**
A: Yes. Use session backups or continue using `csm` commands (backward compatible).

---

## Additional Resources

- **[Agent Comparison Guide](AGENT-COMPARISON.md)** - Choose the right agent
- **[Troubleshooting Guide](TROUBLESHOOTING.md)** - Common issues and solutions
- **[Command Reference](AGM-COMMAND-REFERENCE.md)** - Complete AGM command list
- **[FAQ](FAQ.md)** - Frequently asked questions

---

## Support

**Need help?**
- Check [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for common issues
- Run `agm doctor --validate` for health diagnostics
- Review session logs: `~/.config/agm/logs/agm.log`
- File issue: https://github.com/vbonnet/ai-tools/issues

---

**Migration Status**: Living Document
**Last Updated**: 2026-02-04
**Version**: 1.0
