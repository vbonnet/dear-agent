# ADR-001: Choose Dolt Over SQLite for AGM Storage

**Status**: Accepted
**Date**: 2026-03-07
**Deciders**: Development Team
**Context**: AGM Phase 3 Storage Migration

## Context

AGM currently uses a dual-layer storage system:
- SQLite database for session metadata (`~/.agm/agm.db`)
- JSONL files for conversation history (`~/.agm/sessions/*/messages.jsonl`)

This approach has several issues:
1. **Data Fragmentation**: Session data split across two storage systems
2. **Corruption Risk**: JSONL files can be manually edited, causing syntax errors
3. **Limited Querying**: Cannot easily query across sessions or message content
4. **No Version History**: Changes aren't tracked, making debugging difficult
5. **Workspace Isolation Weak**: Relies on directory structure only

## Decision

We will migrate AGM storage from SQLite+JSONL to Dolt database.

**Dolt** is a SQL database that provides Git-like version control for data:
- MySQL-compatible SQL interface
- Git-like operations (commit, branch, merge, diff)
- Full ACID transaction support
- Built-in version history
- Schema evolution via migrations

## Rationale

### Why Dolt?

**1. Version Control for Data**
- Every change creates a commit (like Git)
- Can rollback to any point in history
- Blame shows who/what modified data
- Diff shows exact changes between versions

**2. Corruption Prevention**
- Database files are binary (not human-editable)
- Transactions ensure atomicity
- Foreign key constraints enforce integrity
- No partial writes on failure

**3. Workspace Isolation**
- Per-workspace Dolt instances (OSS vs Acme)
- Physical separation of databases
- Zero risk of cross-contamination
- Separate ports per workspace (3307, 3308)

**4. Query Capabilities**
- Full SQL for complex queries
- Join sessions with messages
- Aggregate tool usage statistics
- Semantic search (future with embeddings)

**5. Migration Support**
- Built-in schema migration system
- Checksum verification
- Rollback capabilities
- Idempotent operations

### Alternatives Considered

**1. Keep SQLite+JSONL**
- ❌ Doesn't solve corruption issues
- ❌ No version history
- ❌ Limited querying (no JOINs across files)
- ❌ Workspace isolation still weak

**2. Pure SQLite (migrate JSONL to tables)**
- ✅ Simpler than Dolt
- ✅ No server process required
- ❌ No version history
- ❌ No Git-like operations
- ❌ Same file for all workspaces (not physically isolated)

**3. PostgreSQL**
- ✅ Mature, battle-tested
- ✅ Full SQL support
- ❌ Heavyweight for single-user system
- ❌ No version control features
- ❌ Requires server management

**4. File-based (Pure YAML)**
- ✅ Human-readable
- ✅ Git-trackable
- ❌ No transactions
- ❌ Manual editing causes corruption
- ❌ Poor query performance

## Consequences

### Positive

1. **Data Integrity**: Transactions prevent corruption
2. **Debugging**: Version history shows all changes
3. **Security**: Physical workspace separation
4. **Features**: Enables future semantic search, analytics
5. **Maintainability**: Schema migrations handle evolution

### Negative

1. **Complexity**: Requires running Dolt server process
2. **Performance**: 3-5x slower than direct file access (acceptable: still <10ms)
3. **Disk Space**: Dolt history uses more space than JSONL
4. **Dependencies**: Adds Dolt to system requirements
5. **Learning Curve**: Team must learn Dolt-specific operations

### Mitigation Strategies

**Complexity**:
- Provide systemd service for auto-start
- Document server setup in README
- Include troubleshooting guide

**Performance**:
- Benchmark queries (<10ms acceptable)
- Add indexes for common queries
- Use connection pooling

**Disk Space**:
- Monitor database size
- Provide cleanup procedures
- Document space requirements

**Dependencies**:
- Pin Dolt version in documentation
- Test against multiple Dolt versions
- Provide installation guide

**Learning Curve**:
- Comprehensive documentation (SPEC, ARCHITECTURE, ADRs)
- Example queries in README
- Troubleshooting guide

## Implementation

### Phase 1: Foundation (Complete)
- ✅ Create Dolt adapter (`internal/dolt/`)
- ✅ Implement CRUD operations
- ✅ Build migration system
- ✅ Write integration tests

### Phase 2: Migration (Complete)
- ✅ Build migration tool (`cmd/agm-migrate-dolt/`)
- ✅ Migrate user data (116 sessions)
- ✅ Validate data integrity
- ✅ Archive old files

### Phase 3: Integration (Future)
- ⏳ Update AGM daemon to use Dolt
- ⏳ Add storage backend abstraction
- ⏳ End-to-end testing
- ⏳ Performance validation

## Validation

### Success Criteria
- ✅ All sessions migrated without data loss (116/116)
- ✅ Integration tests passing (8/8)
- ✅ Workspace isolation verified
- ✅ Query latency <10ms (measured)
- ✅ Version history functional

### Rollback Plan
- Old YAML manifests archived (not deleted)
- Can restore from backup in <5 minutes
- Migration tool is idempotent (can re-run)

## References

- **Dolt Documentation**: https://docs.dolthub.com/
- **Specification**: [../SPEC.md](../SPEC.md)
- **Architecture**: [../ARCHITECTURE.md](../ARCHITECTURE.md)
- **Roadmap**: `swarm/agm-dolt-storage-migration/ROADMAP.md`
