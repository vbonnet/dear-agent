# AGM Migration Troubleshooting

Comprehensive troubleshooting guide for AGM migration issues.

---

## Quick Diagnosis

Run these commands to diagnose issues:

```bash
# 1. Validate sessions
./scripts/agm-migration-validate.sh

# 2. Health check
agm doctor --validate

# 3. Check agent availability
agm agent list

# 4. View logs
tail -50 ~/.config/agm/logs/agm.log
```

---

## Common Issues

### 1. Validation Errors

#### Issue: manifest.yaml Not Valid YAML

**Symptom:**
```
✗ Session 'my-session': manifest.yaml is not valid YAML
```

**Causes:**
- Syntax error in YAML (indentation, special characters)
- File corruption
- Manual editing errors

**Solutions:**

**A. Check syntax:**
```bash
# Use Python to validate YAML
python3 <<EOF
import yaml
with open('~/.claude-sessions/my-session/manifest.yaml') as f:
    try:
        yaml.safe_load(f)
        print("YAML is valid")
    except yaml.YAMLError as e:
        print(f"YAML error: {e}")
EOF
```

**B. Common syntax fixes:**

```yaml
# ❌ Wrong: Missing quotes with special characters
name: my:session-with-colons

# ✅ Correct: Quote strings with special characters
name: "my:session-with-colons"

# ❌ Wrong: Inconsistent indentation
context:
  project: ~/src
    purpose: coding

# ✅ Correct: Consistent 2-space indentation
context:
  project: ~/src
  purpose: coding

# ❌ Wrong: Tab characters (YAML doesn't allow tabs)
name:\tmy-session

# ✅ Correct: Use spaces only
name: my-session
```

**C. Restore from backup:**
```bash
# If you have backups
cp ~/.claude-sessions.backup/my-session/manifest.yaml \
   ~/.claude-sessions/my-session/manifest.yaml

# Verify fix
./scripts/agm-migration-validate.sh my-session
```

---

#### Issue: Missing Required Fields

**Symptom:**
```
✗ Session 'my-session': Missing required fields: session_id created_at
```

**Cause:** Incomplete or corrupted manifest.

**Solution:**

**A. Add missing fields manually:**
```bash
# Edit manifest
nano ~/.claude-sessions/my-session/manifest.yaml

# Add missing fields:
session_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"  # Generate UUID
created_at: "2026-01-01T00:00:00Z"                   # Approximate creation time
```

**B. Generate UUID:**
```bash
# Linux/Mac
uuidgen | tr '[:upper:]' '[:lower:]'

# Python
python3 -c "import uuid; print(str(uuid.uuid4()))"
```

**C. Recreate session if necessary:**
```bash
# Archive corrupt session
agm archive my-session

# Create new session
agm new my-session --harness claude-code --project ~/src/my-project

# Manually restore conversation history if needed
```

---

#### Issue: Invalid UUID Format

**Symptom:**
```
✗ Session 'old-session': Invalid UUID format: session-old-session
```

**Cause:** Legacy session ID format from older versions.

**Explanation:**
Older versions used `session-<name>` format instead of proper UUIDs. This format is no longer supported.

**Solution:**

**Option 1: Manual UUID Update (Recommended)**
```bash
# 1. Generate new UUID
NEW_UUID=$(uuidgen | tr '[:upper:]' '[:lower:]')
echo "New UUID: $NEW_UUID"

# 2. Backup manifest
cp ~/.claude-sessions/old-session/manifest.yaml \
   ~/.claude-sessions/old-session/manifest.yaml.backup

# 3. Update session_id in manifest
sed -i "s/^session_id: .*/session_id: $NEW_UUID/" \
   ~/.claude-sessions/old-session/manifest.yaml

# 4. Validate fix
./scripts/agm-migration-validate.sh old-session
```

**Option 2: Archive and Recreate**
```bash
# Archive old session
agm archive old-session

# Create new session with same name
agm new old-session --harness claude-code

# Manually migrate conversation history if critical
# (advanced - requires copying JSONL files)
```

---

#### Issue: Unsupported Schema Version

**Symptom:**
```
⚠ Session 'my-session': Unsupported schema version '1.0' (expected 2.0 or 3.0)
```

**Cause:** Session created with older version (schema v1.0).

**Solution:**

**Automatic migration (if available):**
```bash
# Use built-in migration (if agm migrate command exists)
agm migrate v1-to-v2 my-session

# Verify
./scripts/agm-migration-validate.sh my-session
```

**Manual migration:**
```bash
# 1. Backup manifest
cp ~/.claude-sessions/my-session/manifest.yaml \
   ~/.claude-sessions/my-session/manifest.yaml.v1.backup

# 2. Update schema version
sed -i 's/^schema_version: "1.0"/schema_version: "2.0"/' \
   ~/.claude-sessions/my-session/manifest.yaml

# 3. Add required v2 fields (if missing)
# Edit manifest and add:
#   context:
#     project: ~/src/my-project
#     purpose: ""
#     tags: []
#     notes: ""

# 4. Validate
./scripts/agm-migration-validate.sh my-session
```

---

### 2. Agent Configuration Issues

#### Issue: API Key Not Configured

**Symptom:**
```bash
agm agent list
# Output:
# Available agents:
#   claude  (Anthropic) - API key not configured
#   gemini  (Google)    - API key not configured
#   gpt     (OpenAI)    - API key not configured
```

**Solution:**

**A. Add API keys to environment:**
```bash
# Edit ~/.bashrc or ~/.zshrc
nano ~/.bashrc

# Add:
export ANTHROPIC_API_KEY=sk-ant-your-key-here   # Claude
export GOOGLE_API_KEY=AIza-your-key-here        # Gemini
export OPENAI_API_KEY=sk-your-key-here          # GPT

# Reload
source ~/.bashrc

# Verify
agm agent list
```

**B. Verify API key format:**
```bash
# Claude keys start with: sk-ant-
echo $ANTHROPIC_API_KEY | grep -q '^sk-ant-' && echo "Valid format" || echo "Invalid format"

# Gemini keys start with: AIza
echo $GOOGLE_API_KEY | grep -q '^AIza' && echo "Valid format" || echo "Invalid format"

# GPT keys start with: sk-
echo $OPENAI_API_KEY | grep -q '^sk-' && echo "Valid format" || echo "Invalid format"
```

**C. Test API key:**
```bash
# Create test session
agm new --harness claude-code test-api-key

# If session starts successfully, API key works
agm archive test-api-key
```

---

#### Issue: Agent Not Available

**Symptom:**
```
Error: Agent 'gemini' is not available
```

**Cause:** Agent not installed or API key invalid.

**Solution:**

**A. Check agent installation:**
```bash
# Verify AGM supports the agent
agm agent list

# If agent missing from list, update AGM:
cd main/agm
git pull
go build -o agm ./cmd/agm
```

**B. Verify API key:**
```bash
# Check environment variable
echo $GOOGLE_API_KEY  # For Gemini
echo $OPENAI_API_KEY  # For GPT

# Test API key with curl (Gemini example)
curl -H "Content-Type: application/json" \
     -d '{"contents":[{"parts":[{"text":"test"}]}]}' \
     "https://generativelanguage.googleapis.com/v1/models/gemini-pro:generateContent?key=$GOOGLE_API_KEY"
```

---

### 3. Session Resume Issues

#### Issue: Session Won't Resume

**Symptom:**
```
Error: Failed to resume session 'my-session'
```

**Solutions:**

**A. Run health check:**
```bash
agm doctor --validate my-session

# Follow fix recommendations from output
```

**B. Check tmux status:**
```bash
# List tmux sessions
tmux list-sessions

# If session exists with different name, attach manually
tmux attach-session -t <actual-session-name>
```

**C. Try explicit agent:**
```bash
# Force specific agent
agm resume my-session --harness claude-code
```

**D. Check session lock:**
```bash
# If session locked, unlock it
agm unlock my-session

# Then retry
agm resume my-session
```

---

#### Issue: UUID Mismatch

**Symptom:**
```
Error: Session UUID mismatch
```

**Cause:** Manifest UUID doesn't match Claude history.

**Solution:**

**A. Fix UUID association:**
```bash
# Run fix command
agm fix my-session

# Choose correct UUID from suggestions
# Or enter UUID manually
```

**B. Clear UUID and re-associate:**
```bash
# Clear existing UUID
agm fix --clear my-session

# Recreate session
agm new my-session --harness claude-code

# Fix will auto-detect UUID from history
agm fix --all
```

---

### 4. Migration Script Issues

#### Issue: Sessions Directory Not Found

**Symptom:**
```
✗ Sessions directory not found: ~/.claude-sessions
```

**Cause:** Sessions stored in non-standard location.

**Solution:**

**A. Find sessions directory:**
```bash
# Search for manifest files
find ~ -name "manifest.yaml" -path "*/sessions/*" 2>/dev/null | head -5

# If found elsewhere, set environment variable
export SESSIONS_DIR=/path/to/your/sessions
./scripts/agm-migration-validate.sh
```

**B. Create sessions directory:**
```bash
# If no sessions exist
mkdir -p ~/.claude-sessions

# AGM will create sessions here by default
```

---

#### Issue: Python Not Available

**Symptom:**
```
./scripts/agm-migration-validate.sh: line 95: python3: command not found
```

**Solution:**

**A. Install Python:**
```bash
# Ubuntu/Debian
sudo apt-get install python3 python3-yaml

# macOS
brew install python3

# Verify
python3 --version
```

**B. Skip Python validation (less thorough):**
```bash
# Edit script to remove Python YAML check
# Or manually check YAML syntax online
```

---

### 5. Performance Issues

#### Issue: Validation Very Slow

**Symptom:** Validation script takes minutes to complete.

**Cause:** Many sessions (hundreds), slow disk I/O.

**Solution:**

**A. Validate specific session:**
```bash
# Instead of all sessions
./scripts/agm-migration-validate.sh <session-name>
```

**B. Archive old sessions:**
```bash
# Archive inactive sessions
agm list --stopped --older-than 90d | xargs -I {} agm archive {}

# Reduces validation scope
```

**C. Run in background:**
```bash
# Long-running validation
nohup ./scripts/agm-migration-validate.sh > validation-report.txt 2>&1 &

# Check progress
tail -f validation-report.txt
```

---

## Advanced Troubleshooting

### Debugging Manifest Issues

**Inspect manifest structure:**
```bash
# Pretty-print YAML
python3 <<EOF
import yaml, pprint
with open('~/.claude-sessions/my-session/manifest.yaml') as f:
    pprint.pprint(yaml.safe_load(f))
EOF
```

**Compare with valid manifest:**
```bash
# Find a working session
agm list | head -1  # Note working session name

# Compare manifests
diff ~/.claude-sessions/working-session/manifest.yaml \
     ~/.claude-sessions/broken-session/manifest.yaml
```

---

### Debugging Agent Issues

**Test agent directly:**
```bash
# Claude
curl -X POST https://api.anthropic.com/v1/messages \
  -H "x-api-key: $ANTHROPIC_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{"model":"claude-3-5-sonnet-20241022","messages":[{"role":"user","content":"test"}],"max_tokens":10}'

# Gemini
curl -H "Content-Type: application/json" \
  -d '{"contents":[{"parts":[{"text":"test"}]}]}' \
  "https://generativelanguage.googleapis.com/v1/models/gemini-pro:generateContent?key=$GOOGLE_API_KEY"

# GPT
curl https://api.openai.com/v1/chat/completions \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4","messages":[{"role":"user","content":"test"}]}'
```

---

### Checking Logs

**AGM logs:**
```bash
# Recent errors
tail -50 ~/.config/agm/logs/agm.log | grep ERROR

# Migration logs
tail -50 ~/.config/agm/migration.log

# Session-specific logs
tail -50 ~/.claude-sessions/my-session/session.log
```

---

## Recovery Procedures

### Complete Rollback

**If migration causes critical issues:**

```bash
# 1. Stop all AGM sessions
agm list | xargs -I {} agm kill {}

# 2. Restore from backup
rm -rf ~/.claude-sessions
mv ~/.claude-sessions.backup ~/.claude-sessions

# 3. Reinstall AGM (if needed)
go install github.com/vbonnet/ai-tools/agm/cmd/agm@latest

# 4. Verify sessions
agm list
```

---

### Partial Recovery

**If only some sessions affected:**

```bash
# 1. Identify broken sessions
./scripts/agm-migration-validate.sh | grep "Invalid"

# 2. Restore specific sessions
cp -r ~/.claude-sessions.backup/broken-session \
      ~/.claude-sessions/broken-session

# 3. Verify fix
./scripts/agm-migration-validate.sh broken-session
```

---

### Manual Manifest Repair

**Template for valid v2.0 manifest:**

```yaml
schema_version: "2.0"
session_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
name: "my-session"
created_at: "2026-01-01T00:00:00Z"
updated_at: "2026-02-04T12:00:00Z"
lifecycle: ""
context:
  project: "~/src/my-project"
  purpose: "Development work"
  tags: []
  notes: ""
claude:
  uuid: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
tmux:
  session_name: "my-session"
```

**How to use:**
1. Copy template above
2. Replace UUIDs with `uuidgen | tr '[:upper:]' '[:lower:]'`
3. Update name, project, dates
4. Save to `~/.claude-sessions/my-session/manifest.yaml`
5. Validate: `./scripts/agm-migration-validate.sh my-session`

---

## Getting Help

**Before requesting help, gather:**

1. **Validation output:**
   ```bash
   ./scripts/agm-migration-validate.sh > validation-output.txt 2>&1
   ```

2. **Health check output:**
   ```bash
   agm doctor --validate > doctor-output.txt 2>&1
   ```

3. **Manifest sample (sanitize sensitive data):**
   ```bash
   cat ~/.claude-sessions/problem-session/manifest.yaml
   ```

4. **Error logs:**
   ```bash
   tail -100 ~/.config/agm/logs/agm.log
   ```

**Request help:**
- File issue: https://github.com/vbonnet/ai-tools/issues
- Include outputs from above
- Describe expected vs actual behavior
- Mention AGM version: `agm version`

---

## Prevention Best Practices

**Avoid future issues:**

1. **Regular backups:**
   ```bash
   # Weekly backup
   cp -r ~/.claude-sessions ~/.claude-sessions-backup-$(date +%Y%m%d)
   ```

2. **Validation before major changes:**
   ```bash
   # Before AGM updates
   ./scripts/agm-migration-validate.sh
   ```

3. **Avoid manual editing:**
   ```bash
   # ❌ Don't manually edit manifests
   nano ~/.claude-sessions/my-session/manifest.yaml

   # ✅ Use AGM commands
   agm rename old-name new-name
   ```

4. **Monitor health:**
   ```bash
   # Monthly health check
   agm doctor --validate
   ```

---

**Troubleshooting Guide Version**: 1.0
**Last Updated**: 2026-02-04
**Related Docs**: [AGM-MIGRATION-GUIDE.md](AGM-MIGRATION-GUIDE.md), [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
