# ADR-002: Workspace Isolation via Separate Databases

**Status**: Accepted
**Date**: 2026-03-07
**Deciders**: Development Team
**Context**: AGM Dolt Storage Migration - Security & Privacy

## Context

AGM supports multiple workspaces to separate professional work (Acme) from open-source development (OSS). It is **critical** that:
1. Sessions in one workspace NEVER appear in another workspace
2. No data leakage between workspaces (even accidentally)
3. Developers cannot accidentally contaminate professional work with OSS code
4. Zero cross-contamination for security and compliance

The previous YAML-based system relied on directory structure:
```
~/.agm/
├── oss/sessions/      # OSS workspace
└── acme/sessions/   # Acme Corp workspace
```

This is weak isolation - a misconfigured path could leak data.

## Decision

We will implement **defense-in-depth** workspace isolation using three layers:

### Layer 1: Separate Databases (Primary Defense)

Each workspace uses a **separate Dolt database**:

```
Workspace    Database Name    Location
---------    -------------    --------
oss          "oss"            ~/.dolt/dolt-db/oss/
acme       "acme"         ~/src/ws/acme/.dolt/dolt-db/acme/
test         "test"           ~/.dolt/dolt-db/test/
```

Configuration maps workspace → database:
```go
func DefaultConfig() (*Config, error) {
    workspace := getEnv("WORKSPACE", "")
    database := getEnv("DOLT_DATABASE", workspace)  // Defaults to workspace name

    return &Config{
        Workspace: workspace,
        Database:  database,  // "oss" → database "oss"
    }
}
```

### Layer 2: Workspace Column (Defense in Depth)

All tables include a `workspace` column:
```sql
CREATE TABLE agm_sessions (
    id VARCHAR(255) PRIMARY KEY,
    workspace VARCHAR(255) NOT NULL,  -- Always populated
    ...
);
```

This provides **defense in depth** - even if databases are somehow merged, the column prevents leakage.

### Layer 3: Filtered Queries (Runtime Enforcement)

All queries explicitly filter by workspace:
```go
query := `SELECT * FROM agm_sessions WHERE workspace = ?`
rows, err := db.Query(query, adapter.workspace)
```

Even if Layer 1 and Layer 2 fail, queries will only return matching workspace data.

## Rationale

### Why Separate Databases?

**1. Physical Isolation**
- Complete separation at database level
- No possibility of accidental cross-contamination
- Each workspace can have independent schema versions
- Failure in one workspace doesn't affect others

**2. Clear Security Boundary**
- Database name == workspace name (explicit)
- Connection strings include workspace identifier
- Easy to audit which database is being accessed
- No ambiguity about data ownership

**3. Independent Operations**
- Backup/restore per workspace
- Schema migrations per workspace
- Performance tuning per workspace
- Separate Dolt commit history

**4. Compliance**
- Acme Corp workspace can be encrypted separately
- Different retention policies per workspace
- Audit logs are workspace-specific
- Easier to demonstrate data separation for compliance

### Alternatives Considered

**1. Single Database with Workspace Column**
- ✅ Simpler configuration (one database)
- ✅ Fewer Dolt server processes
- ❌ Risk of accidental data leakage
- ❌ Harder to enforce separation
- ❌ All workspaces share schema version
- **Rejected**: Too risky for production use

**2. Separate Dolt Servers per Workspace**
- ✅ Maximum isolation
- ✅ Independent server processes
- ❌ Resource intensive (2+ servers)
- ❌ Different ports per workspace (3307, 3308, 3309...)
- ❌ Complex configuration
- **Rejected**: Overkill for single-user system

**3. Single Server, Separate Databases (Chosen)**
- ✅ Good isolation
- ✅ One server process
- ✅ Independent databases
- ✅ Shared port (3307)
- ✅ Simple configuration
- ✅ Resource efficient

## Consequences

### Positive

1. **Security**: Zero risk of cross-workspace data leakage
2. **Clarity**: Workspace → database mapping is explicit and obvious
3. **Flexibility**: Can encrypt/backup/migrate workspaces independently
4. **Testing**: Separate "test" database for integration tests
5. **Auditability**: Clear data ownership and access patterns

### Negative

1. **Configuration**: Must create multiple databases
2. **Schema Changes**: Migrations must run per workspace
3. **Queries**: Cannot easily query across workspaces (by design)
4. **Disk Space**: Each workspace has separate Dolt history

### Mitigation

**Configuration Complexity**:
- Provide setup script for all workspaces
- Document in SETUP.md
- Test fixtures handle database creation

**Schema Synchronization**:
- Same migration files apply to all workspaces
- Migration tool accepts `--workspace` flag
- Validate schema consistency across workspaces

**Cross-Workspace Queries**:
- Not a bug, it's a feature (isolation by design)
- If needed, use Corpus Callosum for controlled sharing
- Analytics can aggregate across workspaces explicitly

**Disk Space**:
- Each workspace tracks its own history (acceptable)
- Can compact/garbage-collect per workspace
- Monitor disk usage per workspace

## Implementation

### Database Creation

```bash
# Create databases for each workspace
dolt sql -q "CREATE DATABASE oss;"
dolt sql -q "CREATE DATABASE acme;"
dolt sql -q "CREATE DATABASE test;"
```

### Configuration Mapping

```go
// Environment-driven workspace → database mapping
WORKSPACE=oss      → database "oss"
WORKSPACE=acme   → database "acme"
WORKSPACE=test     → database "test"
```

### Migration Per Workspace

```bash
# Migrate each workspace independently
WORKSPACE=oss go run ./cmd/agm-migrate-dolt --yaml-only
WORKSPACE=acme go run ./cmd/agm-migrate-dolt --yaml-only
```

### Testing Isolation

```go
func TestWorkspaceIsolation(t *testing.T) {
    // Create OSS adapter
    os.Setenv("WORKSPACE", "oss")
    os.Unsetenv("DOLT_DATABASE")  // Forces database "oss"
    ossAdapter, _ := dolt.New(dolt.DefaultConfig())

    // Create Acme Corp adapter
    os.Setenv("WORKSPACE", "acme")
    os.Unsetenv("DOLT_DATABASE")  // Forces database "acme"
    acmeAdapter, _ := dolt.New(dolt.DefaultConfig())

    // Verify complete isolation
    // (8 subtests validate zero data leakage)
}
```

## Validation

### Workspace Isolation Tests

**TestWorkspaceIsolation** verifies:
1. ✅ Workspace names correctly set ("oss", "acme")
2. ✅ Sessions created in separate databases
3. ✅ GetSession() returns only own workspace data
4. ✅ Messages isolated per workspace
5. ✅ ListSessions() returns only own workspace
6. ✅ Tool calls isolated per workspace
7. ✅ Updates don't affect other workspaces
8. ✅ Deletes don't affect other workspaces

**Result**: 8/8 tests passing ✅

### Security Audit

**Query Review**:
- ✅ All queries include `WHERE workspace = ?`
- ✅ No raw session IDs without workspace check
- ✅ Batch operations filter by workspace
- ✅ Foreign key constraints within workspace

**Configuration Review**:
- ✅ Environment variable mapping tested
- ✅ Default database name == workspace name
- ✅ Connection strings validated
- ✅ Test fixtures prevent cross-contamination

### Production Validation

**Manual Testing**:
```bash
# Create session in OSS workspace
WORKSPACE=oss agm session create "OSS Session"

# Switch to Acme Corp workspace
WORKSPACE=acme agm session list
# Expected: Empty list (OSS session not visible)

# Verify in database
dolt sql -q "USE oss; SELECT COUNT(*) FROM agm_sessions;"    # 1
dolt sql -q "USE acme; SELECT COUNT(*) FROM agm_sessions;" # 0
```

## References

- **Architecture**: [../ARCHITECTURE.md](../ARCHITECTURE.md#workspace-isolation)
- **Specification**: [../SPEC.md](../SPEC.md#workspace-isolation)
- **Tests**: `workspace_isolation_test.go`
- **ADR-001**: [001-dolt-over-sqlite.md](./001-dolt-over-sqlite.md)
