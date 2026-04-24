# Manifest v2 → v3 Migration Guide

**Current Status:** AGM uses manifest schema version 2.0. This guide documents the migration path for future v3 adoption.

**Note:** As of January 2026, AGM uses `schema_version: "2.0"`. This guide prepares for eventual v3 migration when released.

---

## What Changed in v3

### Breaking Changes

1. **Session ID Format**
   - **v2 Bug (Fixed):** Legacy sessions used `session_id: session-<name>` pattern
   - **v2 Current:** Uses proper UUIDs (`session_id: <uuid>`)
   - **v3 (Future):** Continues UUID format with enhanced validation

2. **Schema Version Field**
   - **v2:** `schema_version: "2.0"`
   - **v3:** `schema_version: "3.0"` (when released)

3. **Additional Fields** (TBD)
   - Future v3 may add agent-specific metadata
   - May include enhanced session lifecycle tracking

---

## Current v2 Manifest Format

**Example manifest.yaml (v2):**

```yaml
schema_version: "2.0"
session_id: a1b2c3d4-e5f6-7890-abcd-ef1234567890  # Proper UUID format
name: my-session
created_at: "2026-01-24T10:00:00Z"
agent: claude
status: active
```

**Validation:**
- ✅ `session_id` must be valid UUID format (8-4-4-4-12 hex characters)
- ✅ `session_id` must NOT use legacy `session-<name>` pattern (bug was fixed)
- ✅ `schema_version` must be `"2.0"`
- ✅ `name` field required

---

## Migration Steps (When v3 Released)

### Step 1: Backup Existing Sessions

```bash
# Backup sessions directory
cp -r ~/sessions ~/sessions.backup

# Or archive all sessions
for session in $(agmlist --format=name); do
  agmarchive "$session"
done
```

### Step 2: Update Schema Version

**Automated approach** (when migration tool released):
```bash
# Future migration command (TBD)
agmmigrate-manifests --from=2.0 --to=3.0
```

**Manual approach:**
```bash
# Edit each manifest.yaml
cd ~/sessions/<session-name>
# Change: schema_version: "2.0"
# To: schema_version: "3.0"
```

### Step 3: Validate Migration

```bash
# Verify manifest format (when v3 released)
agmvalidate-manifest ~/sessions/<session-name>/manifest.yaml

# Test session resume
agmresume <session-name>
```

### Step 4: Verify Session ID Format

v3 will continue using proper UUID format. Verify existing sessions:

```bash
# Check session IDs in manifest files
grep "session_id:" ~/sessions/*/manifest.yaml

# Should see UUIDs like:
# session_id: a1b2c3d4-e5f6-7890-abcd-ef1234567890

# Should NOT see legacy pattern (this was a bug, now fixed):
# session_id: session-my-session-name  # ❌ Old bug (fixed in v2)
```

---

## Validation

**Current v2 validation:**

AGM validates manifests to ensure:
- Schema version is `"2.0"`
- Session ID is proper UUID (not legacy `session-<name>` bug pattern)
- Required fields present (`name`, `session_id`, etc.)

**Test validation:**
```bash
# Create test session
agmnew test-validation --harness claude-code --detached

# Verify manifest
cat ~/sessions/test-validation/manifest.yaml

# Should contain:
# - schema_version: "2.0"
# - session_id: <valid-uuid>  (not "session-test-validation")
# - name: test-validation

# Cleanup
agmarchive test-validation --force
```

---

## Common Issues

### Issue: Legacy Session ID Pattern

**Symptom:**
```yaml
session_id: session-my-old-session  # ❌ Legacy bug pattern
```

**Solution:**
This was a bug in older versions. Newer AGM versions generate proper UUIDs. If you have old sessions with this pattern:

1. Archive the old session: `agmarchive old-session`
2. Create new session with same name: `agmnew old-session --harness <harness>`
3. Manually restore conversation history if needed

### Issue: Invalid UUID Format

**Symptom:**
```
Error: Invalid session_id format in manifest
```

**Solution:**
Ensure session_id follows UUID format: `8-4-4-4-12` hex characters.

```yaml
# ✅ Valid:
session_id: a1b2c3d4-e5f6-7890-abcd-ef1234567890

# ❌ Invalid:
session_id: my-custom-id
session_id: 12345
```

---

## Backward Compatibility

**v2 → v3 compatibility** (when v3 released):
- v2 manifests will likely be auto-upgraded on first session resume
- AGM will maintain backward compatibility for reading v2 manifests
- New sessions will use v3 format automatically

**Rollback:**
If v3 migration causes issues:
1. Restore backup: `cp -r ~/sessions.backup ~/sessions`
2. Downgrade AGM to last v2-compatible version
3. Report issue to AGM maintainers

---

## Testing Migration

**Before migrating production sessions:**

1. Create test sessions with v2 format
2. Test migration on test sessions
3. Verify all functionality (resume, archive, rename, etc.)
4. Only then migrate production sessions

**Test script:**
```bash
# Create test session
agmnew migration-test --harness claude-code --detached

# Archive it
agmarchive migration-test

# After v3 migration, restore and test
agmunarchive migration-test
agmresume migration-test

# Verify it works
# Cleanup
agmarchive migration-test --force
```

---

## When Will v3 Be Released?

**Current Status:** AGM uses schema version 2.0.

v3 migration will be announced when:
- Multi-agent support requires enhanced manifest format
- Session lifecycle tracking needs expansion
- Community feedback drives schema improvements

**Stay updated:**
- Watch [AGM releases](https://github.com/vbonnet/dear-agent/releases)
- Check AGM documentation for v3 announcement

---

## Need Help?

- **Current v2 issues:** See [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
- **Agent selection:** See [AGENT-COMPARISON.md](AGENT-COMPARISON.md)
- **Multi-agent migration:** See [MIGRATION-CLAUDE-MULTI.md](MIGRATION-CLAUDE-MULTI.md)
- **BDD test scenarios:** See [BDD-CATALOG.md](BDD-CATALOG.md)
