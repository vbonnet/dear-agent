# AGM Troubleshooting Guide

Common issues and solutions for AGM (Agent Grid Manager).

---

## Quick Fixes (FAQ)

### Session not found

**Symptom:**
```
Error: session "my-session" not found
```

**Solution:**
1. Check session exists: `agm list`
2. Check for typos in session name
3. Session may be archived: `agm list --archived` (if supported)
4. If archived, restore: `agm unarchive my-session`

---

### Cannot resume archived session

**Symptom:**
```
Error: cannot resume archived session "my-session"
```

**Solution:**
Archived sessions must be restored first:
```bash
# Restore from archive
agm unarchive my-session

# Then resume
agm resume my-session
```

---

### Invalid harness name

**Symptom:**
```
Error: unknown agent "cluade" (typo in "claude")
```

**Solution:**
Use correct agent names:
- `claude` (not "cluade", "clade", "claud")
- `gemini` (not "geminai", "gemni")
- `gpt` (not "gpt4", "chatgpt")

```bash
# ❌ Wrong
agm new my-session --harness cluade

# ✅ Correct
agm new my-session --harness claude-code
```

---

### API key not configured

**Symptom:**
```
Error: ANTHROPIC_API_KEY not set
```

**Solution:**
Configure API key for the agent you're using:

```bash
# For Claude
export ANTHROPIC_API_KEY=sk-ant-...

# For Gemini
export GOOGLE_API_KEY=AIza...
export GOOGLE_PROJECT_ID=my-project

# For GPT
export OPENAI_API_KEY=sk-...
```

Add to your shell profile (`.bashrc`, `.zshrc`) for persistence:
```bash
echo 'export ANTHROPIC_API_KEY=sk-ant-...' >> ~/.bashrc
source ~/.bashrc
```

---

### Session UUID format error

**Symptom:**
```
Error: invalid session_id format in manifest
```

**Solution:**
This indicates corrupted manifest file. Session ID must be valid UUID format.

**Check manifest:**
```bash
cat ~/sessions/my-session/manifest.yaml
```

**Should see:**
```yaml
session_id: a1b2c3d4-e5f6-7890-abcd-ef1234567890  # ✅ Valid UUID
```

**Should NOT see:**
```yaml
session_id: session-my-session  # ❌ Legacy bug pattern (fixed)
session_id: my-custom-id        # ❌ Invalid format
```

**Fix:**
1. Archive broken session: `agm archive my-session --force`
2. Create new session: `agm new my-session --harness <harness>`
3. Manually restore conversation history if needed

---

### Tmux session not found

**Symptom:**
```
Error: tmux session not found
```

**Solution:**
1. Check tmux sessions: `tmux ls`
2. AGM session name should match tmux session name
3. If tmux session missing, session may have crashed
4. Restart session: `agm resume my-session` (creates new tmux session)

---

### Command not found: agm

**Symptom:**
```bash
agm new my-session
# bash: agm: command not found
```

**Solution:**
1. Verify AGM installed: `which agm`
2. If not installed, install AGM:
   ```bash
   go install github.com/vbonnet/dear-agent/agm/cmd/agm@latest
   ```
3. Ensure `$GOPATH/bin` in PATH:
   ```bash
   export PATH=$PATH:$(go env GOPATH)/bin
   ```
4. Restart shell or source profile

---

### Agent detection failure

**Symptom:**
Session uses wrong agent or fails to detect agent.

**Solution:**
1. Check manifest file:
   ```bash
   cat ~/sessions/my-session/manifest.yaml | grep agent
   ```
2. Should show: `agent: claude` (or `gemini`, `gpt`)
3. If missing or wrong, manually fix manifest or recreate session

---

### Cannot send message to archived session

**Symptom:**
```
Error: session "my-session" is archived
```

**Solution:**
Archived sessions are read-only. Restore first:
```bash
agm unarchive my-session
agm resume my-session
# Now you can send messages
```

---

## Detailed Troubleshooting

### Problem: UUID Discovery Failure

**Symptoms:**
- Error: "Could not detect session UUID"
- Session appears in `tmux ls` but not `agm list`
- History.jsonl issues

**Cause:**
AGM's UUID discovery system fails to locate session in `~/.claude/history.jsonl`.

**Diagnosis:**
```bash
# Check history.jsonl exists
ls -lh ~/.claude/history.jsonl

# Verify format (each line should be valid JSON)
head -5 ~/.claude/history.jsonl | jq .

# Check for corruption
jq . ~/.claude/history.jsonl >/dev/null && echo "Valid JSON" || echo "Corrupted"
```

**Solution:**
1. **If history.jsonl missing:**
   ```bash
   # Create directory
   mkdir -p ~/.claude

   # Recreate session
   agm archive old-session --force
   agm new old-session --harness claude-code
   ```

2. **If history.jsonl corrupted:**
   ```bash
   # Backup corrupted file
   cp ~/.claude/history.jsonl ~/.claude/history.jsonl.backup

   # Restore from backup (if available)
   cp ~/.claude/history.jsonl.YYYY-MM-DD ~/.claude/history.jsonl

   # Or remove and recreate sessions
   rm ~/.claude/history.jsonl
   # Sessions will need recreation
   ```

3. **If format invalid:**
   - Each line must be valid JSON object
   - Fix manually or regenerate sessions

---

### Problem: Session Lock Conflicts

**Symptoms:**
- Error: "Session locked by another process"
- Cannot resume session
- Stale lock files

**Cause:**
Previous AGM process crashed without releasing lock.

**Diagnosis:**
```bash
# Check for lock files
ls -la ~/sessions/my-session/.lock

# Check process holding lock (if shown in error)
ps aux | grep <pid-from-error>
```

**Solution:**
1. **If process not running:**
   ```bash
   # Remove stale lock
   rm ~/sessions/my-session/.lock

   # Resume session
   agm resume my-session
   ```

2. **If process still running:**
   - Verify it's actually using the session
   - If safe, kill process: `kill <pid>`
   - Then remove lock and resume

---

### Problem: Agent-Specific Command Failures

**Symptoms:**
- Error: "command not supported by this agent"
- `RenameSession` fails for specific agent
- `SetDirectory` not working

**Cause:**
Not all agents support all CommandTranslator commands. Some commands are agent-specific.

**Diagnosis:**
Check agent capabilities:
- Claude: Full command support
- Gemini: Core commands (RenameSession, SetDirectory)
- GPT: Core commands

**Solution:**
1. **If command unsupported:**
   - Use different agent that supports command
   - Or perform operation manually

2. **Graceful degradation:**
   AGM should return `ErrNotSupported` for unsupported commands, not crash.
   If crashing, report bug.

**Example:**
```bash
# RenameSession may not work for all agents
agm rename old-name new-name --harness gemini-cli
# If fails: Use Claude instead, or manually update manifest
```

---

### Problem: Multi-Agent Confusion

**Symptoms:**
- Wrong agent used for session
- Agent switching unexpectedly
- Cannot determine which agent session uses

**Cause:**
Agent selection not explicitly set or manifest corrupted.

**Diagnosis:**
```bash
# List sessions with agents
agm list  # Should show agent per session

# Check specific session
cat ~/sessions/my-session/manifest.yaml | grep agent
```

**Solution:**
1. **Always specify agent explicitly:**
   ```bash
   # ❌ Unclear (defaults to Claude)
   agm new my-session

   # ✅ Explicit
   agm new my-session --harness claude-code
   ```

2. **Verify agent after creation:**
   ```bash
   agm new my-session --harness gemini-cli
   agm list | grep my-session
   # Should show: my-session (gemini)
   ```

3. **If wrong agent:**
   - Archive session: `agm archive my-session`
   - Recreate with correct harness: `agm new my-session --harness <correct-harness>`

---

### Problem: Performance/Latency Issues

**Symptoms:**
- Slow session creation
- Delayed responses
- Timeout errors

**Cause:**
Network latency, API rate limits, or large context.

**Diagnosis:**
```bash
# Test with debug mode (if available)
agm new test-session --harness claude-code --debug

# Check API status
curl https://status.anthropic.com  # Claude
curl https://status.cloud.google.com  # Gemini
curl https://status.openai.com  # GPT
```

**Solution:**
1. **Network issues:**
   - Check internet connection
   - Try again later
   - Use different agent (Gemini is faster for some tasks)

2. **Rate limits:**
   - Wait before retrying
   - Reduce request frequency
   - Check API quota

3. **Large context:**
   - Reduce message history
   - Use agent with larger context window (Gemini: 1M tokens)

---

## Agent-Specific Troubleshooting

### Claude-Specific Issues

**Issue: Claude API rate limit**
- **Solution:** Wait 60 seconds, then retry. Or upgrade API tier.

**Issue: Context too long (>200K tokens)**
- **Solution:** Use Gemini (1M context) or trim message history.

---

### Gemini-Specific Issues

**Issue: Google Cloud authentication failure**
- **Solution:**
  ```bash
  gcloud auth login
  gcloud auth application-default login
  export GOOGLE_PROJECT_ID=my-project
  ```

**Issue: Vertex AI API not enabled**
- **Solution:** Enable in Google Cloud Console:
  https://console.cloud.google.com/apis/library/aiplatform.googleapis.com

---

### GPT-Specific Issues

**Issue: OpenAI API key invalid**
- **Solution:** Regenerate key at https://platform.openai.com/api-keys

**Issue: Model not available**
- **Solution:** Check OpenAI model availability. Some models require special access.

---

## Still Need Help?

1. **Check BDD scenarios:** [BDD-CATALOG.md](BDD-CATALOG.md) for behavior examples
2. **Review agent comparison:** [AGENT-COMPARISON.md](AGENT-COMPARISON.md)
3. **Migration issues:** [MIGRATION-CLAUDE-MULTI.md](MIGRATION-CLAUDE-MULTI.md)
4. **File issue:** [GitHub Issues](https://github.com/vbonnet/dear-agent/issues)
5. **Debug mode:** Run commands with `--debug` flag (if available) for verbose output

---

## Reporting Bugs

When reporting issues, include:
- AGM version: `agm --version`
- Agent used: Claude/Gemini/GPT
- Command that failed (full command)
- Error message (full output)
- Manifest file (if session-specific): `cat ~/sessions/<session>/manifest.yaml`
- Steps to reproduce

**Example bug report:**
```
**AGM Version:** 2.1.0
**Agent:** Gemini
**Command:** agm resume research-session
**Error:** Error: session "research-session" not found
**Manifest:** (attached)
**Steps:**
1. Created session: agm new research-session --harness gemini-cli
2. Paused session (closed terminal)
3. Tried to resume: agm resume research-session
4. Got error above
```

This helps maintainers diagnose and fix issues quickly.
