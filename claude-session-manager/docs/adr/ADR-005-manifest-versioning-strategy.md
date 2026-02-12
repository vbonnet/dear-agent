# ADR-005: Manifest Versioning Strategy

**Status:** Accepted
**Date:** 2026-01-16
**Deciders:** Foundation Engineering Team
**Related:** ADR-001 (Multi-Agent Architecture)

---

## Context

AGM manifests store session metadata (session ID, agent, project path, timestamps). As AGM evolves from AGM (v2 manifests) to multi-agent support (v3 manifests), we need a versioning strategy that:

1. Maintains backward compatibility (AGM users can upgrade seamlessly)
2. Enables forward evolution (new features don't break old manifests)
3. Supports migration (automated or manual)
4. Provides rollback capability (if migration fails)

### Current State

**AGM Manifests (v2)**:
```yaml
version: "2.0"
session_id: "abc-123"
tmux_session_name: "my-session"
lifecycle: "active"
context:
  project: "~/projects/myapp"
metadata:
  created_at: "2026-01-01T00:00:00Z"
  updated_at: "2026-01-16T10:00:00Z"
claude:
  uuid: "xyz-789"
  version: "0.7.1"
```

**Problems**:
- No `agent` field (implicitly Claude)
- `claude` section is top-level (not extensible for other agents)
- No workflow metadata
- No migration path for new fields

---

## Decision

We will implement **Read Old, Write New** versioning strategy with schema evolution support.

**Strategy**:
1. **Version Field**: Manifests include `version` field (semver-like)
2. **Read Multiple Versions**: AGM reads v2 and v3 manifests
3. **Write Latest Version**: AGM always writes v3 on updates
4. **Lazy Migration**: v2 → v3 migration happens on first write (transparent)
5. **Backward Compatible**: v2 manifests never auto-upgraded (read-only until user modifies)

---

## Alternatives Considered

### Alternative 1: Immediate Migration

**Approach**: On AGM install, auto-migrate all v2 manifests to v3

**Pros**:
- Clean cutover (all manifests same version)
- Simplifies code (only read v3)
- One-time migration event

**Cons**:
- ❌ Destructive (can't rollback to AGM)
- ❌ Risky (all manifests changed at once)
- ❌ Forces migration (user may not want to upgrade yet)
- ❌ Breaks co-existence (can't run AGM and AGM concurrently)

**Verdict**: Rejected. Too risky, prevents gradual adoption.

---

### Alternative 2: Dual Write (v2 and v3)

**Approach**: AGM writes both v2 and v3 manifests (manifest.yaml and manifest.v3.yaml)

**Pros**:
- Perfect backward compatibility (AGM reads v2, AGM reads v3)
- Co-existence (AGM and AGM can run concurrently)
- Rollback trivial (delete v3 manifests)

**Cons**:
- ❌ Storage overhead (2x manifests)
- ❌ Sync complexity (must keep v2 and v3 in sync)
- ❌ Divergence risk (if sync fails, which is source of truth?)
- ❌ Confusing (users see two manifests, which to edit?)

**Verdict**: Rejected. Complexity not worth co-existence benefit.

---

### Alternative 3: Read Old, Write New (CHOSEN)

**Approach**: AGM reads v2 and v3, writes v3 only, migration on first write

**Pros**:
- ✅ Backward compatible (v2 manifests continue working)
- ✅ Lazy migration (no upfront risk)
- ✅ User control (migration happens when user modifies session)
- ✅ Simple (single manifest, single source of truth)
- ✅ Rollback possible (restore from backup before write)

**Cons**:
- ⚠️ Codebase complexity (must support both versions)
- ⚠️ Testing burden (test v2 and v3 paths)
- ⚠️ Migration is implicit (user may not know it happened)

**Verdict**: ACCEPTED. Best balance of safety and simplicity.

---

## Implementation Details

### Schema Versions

**v2 (AGM)**:
```yaml
version: "2.0"
session_id: "..."
tmux_session_name: "..."
lifecycle: "active"
context:
  project: "..."
metadata:
  created_at: "..."
  updated_at: "..."
claude:  # Top-level
  uuid: "..."
  version: "..."
```

**v3 (AGM)**:
```yaml
version: "3.0"
session_id: "..."
tmux_session_name: "..."
agent: "claude"  # NEW: Required field
lifecycle: "active"
context:
  project: "..."
  workflow: "deep-research"  # NEW: Optional workflow
metadata:
  created_at: "..."
  updated_at: "..."
  created_by: "agm"  # NEW: Creator
  version: "3.0.0"   # NEW: AGM version
agent_metadata:  # NEW: Nested agent-specific data
  claude:
    uuid: "..."
    version: "..."
  gemini:
    conversation_id: "..."
    model: "..."
```

**Key Changes**:
- Added `agent` field (required)
- Added `context.workflow` (optional)
- Renamed `claude` → `agent_metadata.claude`
- Added `agent_metadata.gemini` support
- Added `metadata.created_by` and `metadata.version`

---

### Manifest Reader (Multi-Version)

```go
func ReadManifest(path string) (*Manifest, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var rawManifest map[string]interface{}
    if err := yaml.Unmarshal(data, &rawManifest); err != nil {
        return nil, err
    }

    version := rawManifest["version"].(string)

    switch version {
    case "2.0":
        return readV2Manifest(data)
    case "3.0":
        return readV3Manifest(data)
    default:
        return nil, fmt.Errorf("unsupported manifest version: %s", version)
    }
}

func readV2Manifest(data []byte) (*Manifest, error) {
    var v2 ManifestV2
    if err := yaml.Unmarshal(data, &v2); err != nil {
        return nil, err
    }

    // Convert v2 → v3 in-memory (don't write yet)
    return &Manifest{
        Version:          "3.0",
        SessionID:        v2.SessionID,
        TmuxSessionName:  v2.TmuxSessionName,
        Agent:            "claude",  // Implicit in v2
        Lifecycle:        v2.Lifecycle,
        Context:          v2.Context,
        Metadata:         v2.Metadata,
        AgentMetadata: AgentMetadata{
            Claude: v2.Claude,  // Migrate to nested structure
        },
    }, nil
}

func readV3Manifest(data []byte) (*Manifest, error) {
    var v3 Manifest
    if err := yaml.Unmarshal(data, &v3); err != nil {
        return nil, err
    }
    return &v3, nil
}
```

**Key Points**:
- Version detection from `version` field
- v2 converted to v3 in-memory (no disk write)
- Read is non-destructive (original file unchanged)

---

### Manifest Writer (Always v3)

```go
func WriteManifest(manifest *Manifest, path string) error {
    // Force version to 3.0
    manifest.Version = "3.0"
    manifest.Metadata.UpdatedAt = time.Now().UTC()
    manifest.Metadata.Version = agmVersion  // AGM binary version

    // Create backup before writing
    if err := createBackup(path); err != nil {
        return fmt.Errorf("backup failed: %w", err)
    }

    // Write v3 manifest
    data, err := yaml.Marshal(manifest)
    if err != nil {
        return err
    }

    // Atomic write (write to temp, rename)
    tmpPath := path + ".tmp"
    if err := os.WriteFile(tmpPath, data, 0600); err != nil {
        return err
    }

    return os.Rename(tmpPath, path)
}
```

**Key Points**:
- Always writes v3 (even if read v2)
- Backup created before write (rollback possible)
- Atomic write prevents corruption
- Updates timestamp and AGM version

---

### Migration Behavior

**Scenario 1: Read-Only Operations**
```bash
agm list                  # Reads v2, no write
agm resume my-session     # Reads v2, no write
```
**Result**: v2 manifest unchanged

**Scenario 2: Write Operations**
```bash
agm session rename my-session new-name
```
**Result**:
1. Read v2 manifest
2. Convert to v3 in-memory
3. Update name
4. Create backup of v2
5. Write v3 manifest
6. v2 manifest replaced with v3

**Scenario 3: Explicit Migration**
```bash
agm migrate
```
**Result**:
1. Find all v2 manifests
2. Backup each
3. Convert to v3
4. Write v3 manifests
5. Report results

---

### Backward Compatibility Guarantees

**AGM reads v2**: ✅ Always
**AGM writes v2**: ❌ Never (writes v3)
**AGM reads v3**: ❌ Fails (unknown fields)
**AGM writes v2**: ✅ Always (but AGM won't read after writing v3)

**Co-Existence**:
- ✅ AGM and AGM can coexist BEFORE first AGM write
- ❌ AGM and AGM CANNOT coexist AFTER first AGM write

**Rollback**:
- ✅ Restore from backup (`.backups/manifest.1`)
- ✅ Use `agm migrate --rollback`

---

## Consequences

### Positive

✅ **Backward Compatible**: v2 manifests continue working
✅ **Lazy Migration**: No upfront risk, happens incrementally
✅ **User Control**: Migration on first write (explicit action)
✅ **Rollback Possible**: Backups enable reverting migration
✅ **Future-Proof**: Schema evolution pattern established

### Negative

⚠️ **Code Complexity**: Must support v2 and v3 readers
⚠️ **Testing Burden**: Test both versions, migration path
⚠️ **Implicit Migration**: User may not know manifest upgraded
⚠️ **AGM Co-Existence Limited**: Can't run AGM after AGM writes

### Neutral

🔄 **Backup Storage**: 3 backups per session (~3KB each)
🔄 **Migration Transparency**: Happens invisibly (pro: seamless, con: not obvious)

---

## Mitigations

**Code Complexity**:
- Separate v2 and v3 types (clear separation)
- Migration logic isolated in converter functions
- Unit tests for each version

**Testing Burden**:
- Test fixtures for v2 and v3 manifests
- Integration tests for migration
- BDD scenarios for user-facing behavior

**Implicit Migration**:
- Log migration events (`agm.log`)
- `agm admin validate-manifests` reports versions
- Migration guide documents behavior

**AGM Co-Existence**:
- Document limitation clearly
- `agm migrate --validate` checks for AGM sessions
- Prompt user before destructive operations

---

## Validation

**BDD Scenarios**:
- Read v2 manifest → succeeds, no write
- Write v2 manifest → succeeds, writes v3
- Read v3 manifest → succeeds
- Explicit migration (`agm migrate`) → all v2 → v3
- Rollback (`agm migrate --rollback`) → restores v2 from backup

**Unit Tests**:
- Parse v2 manifest → correct in-memory v3
- Parse v3 manifest → correct in-memory v3
- Write v3 manifest → valid YAML
- Version detection → correct reader selected

**Integration Tests**:
- AGM session migrated → AGM can read/write
- AGM session created → v3 format
- Mixed v2/v3 sessions → both work

---

## Related Decisions

- **ADR-001**: Multi-Agent Architecture (agent field added in v3)
- **ADR-008**: Backup Strategy (backups enable rollback)
- **Future**: v4 schema evolution (will follow same pattern)

---

## Future Considerations

**v4 Schema** (Hypothetical):
- Multi-conversation support
- Cloud sync metadata
- Richer tagging system

**Migration Path**: Same pattern (read v2/v3, write v4)

**Deprecation**: Set sunset date for v2 support (e.g., AGM v5.0)

---

## References

- **Schema Versioning**: https://github.com/protocolbuffers/protobuf/blob/main/docs/schema_evolution.md
- **Semantic Versioning**: https://semver.org/
- **Database Migration Patterns**: https://www.liquibase.org/get-started/best-practices

---

**Implementation Status:** ✅ Complete (Shipped in AGM v3.0)
**Date Completed:** 2026-02-04
