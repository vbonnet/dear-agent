# ADR-003: Embedded Migration System with Checksum Validation

**Status**: Accepted
**Date**: 2026-03-07
**Deciders**: Development Team
**Context**: AGM Dolt Storage - Schema Evolution

## Context

Database schemas evolve over time. New features require new tables, columns, or indexes. AGM needs a migration system that:

1. **Tracks Applied Migrations**: Knows which migrations have been applied
2. **Prevents Re-execution**: Doesn't apply the same migration twice
3. **Validates Integrity**: Detects if migration SQL was modified after application
4. **Supports Rollback**: Can revert to previous schema versions
5. **Works Across Workspaces**: Same migrations apply to OSS, Acme, and test workspaces

## Decision

We will implement an **embedded migration system** using:
- **go:embed** to bundle SQL files into the binary
- **Migration registry table** to track applied migrations
- **SHA256 checksums** to validate migration integrity
- **Component-aware** system for multi-component databases

## Architecture

### Migration Files (SQL)

```
internal/dolt/migrations/
├── 001_initial_schema.sql          # Sessions table
├── 002_messages_table.sql          # Messages table
├── 003_add_tool_calls.sql          # Tool calls table
├── 004_add_session_tags.sql        # Tags (future)
├── 005_add_message_embeddings.sql  # Embeddings (future)
└── 006_add_performance_indexes.sql # Indexes
```

Each file contains **one logical change** (single table or set of related indexes).

### Migration Structure

```go
type Migration struct {
    Version       int           // Sequential: 1, 2, 3, ...
    Name          string        // Descriptive: "add_tool_calls"
    SQL           string        // Embedded SQL content
    Checksum      string        // SHA256 hash of SQL
    TablesCreated []string      // For audit trail
}
```

### Embedding Mechanism

```go
//go:embed migrations/001_initial_schema.sql
var migration001 string

//go:embed migrations/002_messages_table.sql
var migration002 string

func AllMigrations() []Migration {
    return []Migration{
        {
            Version:  1,
            Name:     "initial_schema",
            SQL:      migration001,
            Checksum: computeChecksum(migration001),
            TablesCreated: []string{"agm_sessions"},
        },
        {
            Version:  2,
            Name:     "messages_table",
            SQL:      migration002,
            Checksum: computeChecksum(migration002),
            TablesCreated: []string{"agm_messages"},
        },
        // ...
    }
}
```

### Migration Registry

```sql
CREATE TABLE agm_migrations (
    id INT AUTO_INCREMENT PRIMARY KEY,
    component VARCHAR(255) NOT NULL,     -- 'agm' (allows multi-component DB)
    version INT NOT NULL,                -- Migration version
    name VARCHAR(255) NOT NULL,          -- Migration name
    checksum VARCHAR(128) NOT NULL,      -- SHA256 hash (64 hex chars)
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    applied_by VARCHAR(255),             -- 'agm-1.4.0' (component version)
    execution_time_ms INT,               -- Performance tracking
    tables_created JSON,                 -- ["agm_sessions", "agm_messages"]
    UNIQUE KEY (component, version)      -- One version per component
);
```

### Application Flow

```
1. ensureMigrationRegistry()
   ↓ Create agm_migrations table if not exists

2. getAppliedMigrations()
   ↓ SELECT version FROM agm_migrations WHERE component='agm'
   ↓ Returns: [1, 2, 3]  (already applied)

3. For each migration in AllMigrations():
   ↓ If version in appliedVersions:
   ↓   validateMigrationChecksum(migration)  # Verify no modifications
   ↓   continue
   ↓
   ↓ Else (new migration):
   ↓   BEGIN TRANSACTION
   ↓   EXEC migration.SQL
   ↓   INSERT INTO agm_migrations (component, version, name, checksum, ...)
   ↓   COMMIT

4. Mark migrationsApplied = true (skip future calls)
```

## Rationale

### Why Embedded (go:embed)?

**1. Deployment Simplicity**
- Migration SQL bundled in binary
- No external files to distribute
- Cannot be lost or modified at runtime

**2. Version Control**
- SQL files in Git alongside code
- Changes tracked in commits
- Code review for schema changes

**3. Build-Time Validation**
- SQL syntax errors caught at build time
- Missing files fail compilation
- Type-safe migration struct

**4. Consistency**
- Same SQL for all deployments
- Cannot accidentally use wrong version
- Binary hash includes migration SQL

### Why Checksum Validation?

**Problem**: Someone modifies `001_initial_schema.sql` after it's been applied to production.

**Without Checksums**:
- New deployments get different schema
- Production and dev databases diverge
- Debugging becomes nightmare
- Data corruption risk

**With Checksums**:
```
Error: migration 1 checksum mismatch
  stored:   5750a28848c941504990952cbcd290aa2d50fca8ba8e022b4992f1b5e91cd81f
  expected: 8184630b6383da6be728aaaf3a979bd4c83b1cf7c7f8cd4b92af768dc5198058
```

**Resolution**: **Never modify existing migrations**. Create new migration instead.

### Why Component-Aware?

**Future Scenario**: Corpus Callosum also uses Dolt database.

**Without Component Awareness**:
```
agm_sessions           # AGM
agm_messages           # AGM
knowledge_nodes        # Corpus Callosum (CONFLICT: no namespace)
```

**With Component Awareness**:
```
agm_sessions           # AGM component (prefix: agm_)
agm_messages           # AGM component
cc_knowledge_nodes     # Corpus Callosum (prefix: cc_)
cc_embeddings          # Corpus Callosum
```

**Migration Registry**:
```sql
SELECT * FROM agm_migrations;
+-----------+---------+----------------+
| component | version | name           |
+-----------+---------+----------------+
| agm       | 1       | initial_schema |
| agm       | 2       | messages_table |
| cc        | 1       | knowledge_base |
+-----------+---------+----------------+
```

Each component manages its own migrations independently.

## Alternatives Considered

### 1. External Migration Tool (e.g., golang-migrate)

**Pros**:
- ✅ Battle-tested
- ✅ Rich feature set
- ✅ Community support

**Cons**:
- ❌ External dependency
- ❌ Requires separate migration files
- ❌ No go:embed support (must package files separately)
- ❌ Overkill for simple use case

**Verdict**: Rejected - too heavyweight for single-user system

### 2. SQL Files in Repository (Not Embedded)

**Pros**:
- ✅ Easy to edit
- ✅ No build step

**Cons**:
- ❌ Files can be lost
- ❌ Deployment complexity
- ❌ Cannot modify without redeploying
- ❌ Version skew risk

**Verdict**: Rejected - deployment fragility

### 3. Code-Based Migrations (No SQL Files)

```go
func Migration001(db *sql.DB) error {
    _, err := db.Exec(`CREATE TABLE agm_sessions (...)`)
    return err
}
```

**Pros**:
- ✅ Type-safe
- ✅ No SQL parsing

**Cons**:
- ❌ SQL harder to review
- ❌ Long strings in Go code
- ❌ Cannot preview SQL easily

**Verdict**: Rejected - SQL files more readable

### 4. Timestamp-Based Versions (Not Sequential)

**Example**: `20260307120000_initial_schema.sql`

**Pros**:
- ✅ Avoids version conflicts in parallel dev
- ✅ Shows when migration was created

**Cons**:
- ❌ Harder to reason about order
- ❌ Not needed for single developer
- ❌ Sequential numbers clearer

**Verdict**: Rejected - sequential versions simpler

## Consequences

### Positive

1. **Reliability**: Checksum validation prevents schema drift
2. **Simplicity**: No external migration tool required
3. **Auditability**: Migration history in database
4. **Extensibility**: Component-aware for future multi-component DB
5. **Maintainability**: SQL files in version control

### Negative

1. **Immutability**: Cannot modify existing migrations (must create new)
2. **Build Dependency**: Changes require recompile
3. **No Down Migrations**: Only supports forward migrations
4. **Manual Rollback**: Must use Dolt branches/commits for rollback

### Mitigation

**Immutability**:
- Document: "Never modify existing migrations"
- Code review: Reject changes to existing SQL files
- If mistake: Create new migration to fix

**Build Dependency**:
- Acceptable: Schema changes should be intentional
- CI/CD rebuilds automatically

**No Down Migrations**:
- Use Dolt branches for testing
- Rollback via `dolt checkout <commit>`
- Production changes are forward-only (by design)

## Implementation Details

### Checksum Computation

```go
func computeChecksum(sql string) string {
    hash := sha256.Sum256([]byte(sql))
    return hex.EncodeToString(hash[:])  // 64 hex chars (removed "sha256:" prefix)
}
```

**Why SHA256?**
- Cryptographically secure
- 64 hex characters (fits in VARCHAR(128))
- Standard library support
- Industry standard

### Migration Application

```go
func (a *Adapter) applyMigration(migration Migration) error {
    tx, err := a.conn.Begin()
    defer tx.Rollback()

    startTime := time.Now()

    // Execute migration SQL
    if _, err := tx.Exec(migration.SQL); err != nil {
        return fmt.Errorf("failed to execute migration SQL: %w", err)
    }

    executionTime := time.Since(startTime).Milliseconds()

    // Record in registry
    _, err = tx.Exec(`
        INSERT INTO agm_migrations
        (component, version, name, checksum, applied_by, execution_time_ms, tables_created)
        VALUES ('agm', ?, ?, ?, 'agm-1.4.0', ?, ?)
    `, migration.Version, migration.Name, migration.Checksum, executionTime, tablesJSON)

    if err != nil {
        return fmt.Errorf("failed to record migration: %w", err)
    }

    return tx.Commit()
}
```

### Idempotency

Migrations are **idempotent** - safe to run multiple times:

```go
if _, err := adapter.ApplyMigrations(); err != nil {
    return err
}
// Safe to call again - already applied migrations are skipped
adapter.ApplyMigrations()  // No-op
```

## Testing

### Unit Tests

```go
func TestMigrationChecksumValidation(t *testing.T) {
    // Simulate checksum mismatch
    adapter := getTestAdapter(t)

    // Manually modify stored checksum
    adapter.conn.Exec(`
        UPDATE agm_migrations
        SET checksum = 'wrong-checksum'
        WHERE version = 1
    `)

    // Apply migrations again
    err := adapter.ApplyMigrations()

    // Expect error
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "checksum mismatch")
}
```

### Integration Tests

All integration tests run migrations automatically:

```go
func getTestAdapter(t *testing.T) *Adapter {
    adapter, _ := dolt.New(config)

    // Migrations applied here
    if err := adapter.ApplyMigrations(); err != nil {
        t.Fatalf("Failed to apply migrations: %v", err)
    }

    return adapter
}
```

**Result**: If migrations fail, all tests fail immediately.

## Maintenance

### Adding New Migration

1. Create SQL file: `007_add_new_feature.sql`
2. Add go:embed directive:
   ```go
   //go:embed migrations/007_add_new_feature.sql
   var migration007 string
   ```
3. Add to AllMigrations():
   ```go
   {
       Version: 7,
       Name: "add_new_feature",
       SQL: migration007,
       Checksum: computeChecksum(migration007),
       TablesCreated: []string{"agm_new_table"},
   }
   ```
4. Run tests: `go test ./internal/dolt/`
5. Deploy: Migrations auto-apply on startup

### Rollback Procedure

**Option 1: Dolt Branches** (Recommended for Testing)
```bash
# Create branch before migration
dolt checkout -b before-migration-7

# Apply migration (version 7)
go run ./cmd/agm-migrate-dolt

# Rollback if needed
dolt checkout main
dolt reset --hard before-migration-7
```

**Option 2: Manual Revert** (Production)
```sql
-- Drop added table
DROP TABLE agm_new_table;

-- Remove migration record
DELETE FROM agm_migrations WHERE version = 7;
```

**Option 3: Forward-Only Fix** (Preferred)
```bash
# Don't rollback - fix forward
# Create migration 008 to undo changes from 007
```

## References

- **Go Embed**: https://pkg.go.dev/embed
- **Migration Files**: `internal/dolt/migrations/*.sql`
- **Migration Code**: `internal/dolt/migrations.go`
- **Tests**: `internal/dolt/adapter_test.go`
- **ADR-001**: [001-dolt-over-sqlite.md](./001-dolt-over-sqlite.md)
