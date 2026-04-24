# Dolt Workspace Integration

Per-workspace Dolt database management for the modular component system.

## Overview

This package provides workspace-scoped Dolt database integration with support for:
- **Per-workspace database isolation**: Separate Dolt database for each workspace
- **Component migration system**: Version-controlled schema migrations
- **Component registry**: Track installed components and table prefixes
- **Workspace isolation**: Physical separation prevents cross-contamination

## Architecture

### Workspace Structure

```
./                    # OSS workspace
  ├── .dolt/                      # Dolt database directory
  │   ├── .dolt/                  # Dolt Git metadata
  │   ├── server.yaml             # Dolt server config (port 3307)
  │   └── dolt-db/                # Database files
  └── ... (workspace content)

~/src/ws/acme/                 # Acme workspace (completely isolated)
  ├── .dolt/                      # Separate Dolt database
  │   ├── .dolt/                  # Independent Git metadata
  │   ├── server.yaml             # Workspace-specific config (port 3308)
  │   └── dolt-db/                # Database files
  └── ... (workspace content)
```

### Database Schema

Each workspace database contains:
- `dolt_migrations` - Migration registry tracking applied migrations
- `component_registry` - Component registry with prefix ownership
- Component tables (e.g., `agm_sessions`, `wf_projects`)

## Quick Start

### 1. Create Workspace Adapter

```go
import "github.com/vbonnet/engram/core/pkg/workspace/dolt"

// Get default config for workspace
config := dolt.GetDefaultConfig("oss", "~/projects/myworkspace")

// Create adapter
adapter, err := dolt.NewWorkspaceAdapter(config)
if err != nil {
    log.Fatal(err)
}
defer adapter.Close()
```

### 2. Connect to Database

```go
ctx := context.Background()

// Connect to Dolt database
if err := adapter.Connect(ctx); err != nil {
    log.Fatal(err)
}

// Initialize registry tables (first time only)
if err := adapter.InitializeDatabase(ctx); err != nil {
    log.Fatal(err)
}
```

### 3. Register Component

```go
import "encoding/json"

// Create component manifest
manifest := dolt.ComponentManifest{
    Name:    "agm",
    Version: "1.0.0",
    Storage: dolt.ComponentStorage{
        Engine: "dolt",
        Prefix: "agm_",
    },
}

manifestJSON, _ := json.Marshal(manifest)

// Register component
info := &dolt.ComponentInfo{
    Name:     "agm",
    Version:  "1.0.0",
    Prefix:   "agm_",
    Status:   string(dolt.StatusInstalled),
    Manifest: string(manifestJSON),
}

err := adapter.RegisterComponent(ctx, info)
if err != nil {
    log.Fatal(err)
}
```

### 4. Apply Migrations

```go
// Load migration files from directory
migrations, err := dolt.LoadMigrationFiles("./migrations")
if err != nil {
    log.Fatal(err)
}

// Get unapplied migrations
unapplied, err := adapter.GetUnappliedMigrations(ctx, "agm", migrations)
if err != nil {
    log.Fatal(err)
}

// Apply migrations
results, err := adapter.ApplyMigrations(ctx, "agm", unapplied, "agm-1.0.0")
if err != nil {
    log.Fatal(err)
}

for _, result := range results {
    if result.Success {
        log.Printf("Applied migration %d: %s (%dms)",
            result.Migration.Version,
            result.Migration.Name,
            result.ExecutionTimeMs)
    }
}
```

### 5. Commit Changes to Dolt

```go
// Commit changes to Dolt Git
err = adapter.CommitChanges(ctx, "agm: Apply initial migrations")
if err != nil {
    log.Fatal(err)
}
```

## Key Features

### Per-Workspace Isolation

Each workspace has a completely isolated Dolt database:
- **Physical isolation**: Separate database files on disk
- **Network isolation**: Different ports (OSS: 3307, Acme: 3308)
- **Git isolation**: Independent Dolt Git repositories
- **Zero cross-contamination**: Queries cannot access other workspaces

```go
// OSS workspace
ossConfig := dolt.GetDefaultConfig("oss", "~/projects/myworkspace")
ossAdapter, _ := dolt.NewWorkspaceAdapter(ossConfig)

// Acme workspace (completely separate)
acmeConfig := dolt.GetDefaultConfig("acme", "~/src/ws/acme")
acmeAdapter, _ := dolt.NewWorkspaceAdapter(acmeConfig)

// Data in OSS is NOT visible in Acme (and vice versa)
```

### Migration System

Components define SQL migrations that are tracked and applied per-workspace:

**Migration File Structure:**
```
agm/migrations/
  ├── 001_initial_schema.sql
  ├── 002_add_tool_calls.sql
  └── 003_add_session_tags.sql
```

**Migration File Format:**
```sql
-- AGM Migration 001: Initial Schema
-- Description: Create core AGM tables
-- Author: AGM Team
-- Date: 2026-02-19

CREATE TABLE IF NOT EXISTS agm_sessions (
  id VARCHAR(255) PRIMARY KEY,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  ...
);
```

**Features:**
- Idempotent migrations (`CREATE IF NOT EXISTS`)
- Version tracking in `dolt_migrations` table
- Checksum validation
- Dependency resolution
- Automatic table ownership tracking

### Component Registry

Components must register their table prefix to prevent collisions:

```go
// Validate prefix availability
err := dolt.ValidatePrefixAvailable(ctx, adapter, "agm_")
if err != nil {
    // Prefix already claimed or invalid
    log.Fatal(err)
}

// Register component
info := &dolt.ComponentInfo{
    Name:   "agm",
    Prefix: "agm_",  // Enforced as unique
    ...
}
adapter.RegisterComponent(ctx, info)
```

**Prefix Rules:**
- Must end with underscore (`_`)
- Lowercase alphanumeric only
- 2-50 characters
- Cannot use reserved prefixes (`dolt_`, `mysql_`, etc.)
- Must be unique per workspace

### Table Prefix Enforcement

All component tables must use the component's registered prefix:

```sql
-- AGM component (prefix: agm_)
agm_sessions
agm_messages
agm_tool_calls

-- Wayfinder component (prefix: wf_)
wf_projects
wf_tasks
wf_dependencies
```

**Discovery:**
```go
// Get all tables for component
tables, err := adapter.GetAllComponentTables(ctx, "agm")
// Returns: ["agm_sessions", "agm_messages", "agm_tool_calls"]

// Find orphaned tables (not registered in migrations)
orphans, err := adapter.FindOrphanedTables(ctx, "agm")
```

## API Reference

### WorkspaceAdapter

Main interface for Dolt database operations.

**Creation:**
```go
func NewWorkspaceAdapter(config *DoltConfig) (*WorkspaceAdapter, error)
func GetDefaultConfig(workspaceName, workspaceRoot string) *DoltConfig
```

**Connection:**
```go
func (a *WorkspaceAdapter) Connect(ctx context.Context) error
func (a *WorkspaceAdapter) Close() error
func (a *WorkspaceAdapter) Ping(ctx context.Context) error
func (a *WorkspaceAdapter) InitializeDatabase(ctx context.Context) error
```

**Migrations:**
```go
func (a *WorkspaceAdapter) GetAppliedMigrations(ctx context.Context, component string) ([]*Migration, error)
func (a *WorkspaceAdapter) IsMigrationApplied(ctx context.Context, component string, version int) (bool, error)
func (a *WorkspaceAdapter) ApplyMigration(ctx context.Context, component string, migration *MigrationFile, appliedBy string) (*MigrationResult, error)
func (a *WorkspaceAdapter) ApplyMigrations(ctx context.Context, component string, migrations []*MigrationFile, appliedBy string) ([]*MigrationResult, error)
func (a *WorkspaceAdapter) GetUnappliedMigrations(ctx context.Context, component string, allMigrations []*MigrationFile) ([]*MigrationFile, error)
```

**Components:**
```go
func (a *WorkspaceAdapter) RegisterComponent(ctx context.Context, info *ComponentInfo) error
func (a *WorkspaceAdapter) GetComponent(ctx context.Context, name string) (*ComponentInfo, error)
func (a *WorkspaceAdapter) ListComponents(ctx context.Context) ([]*ComponentInfo, error)
func (a *WorkspaceAdapter) UpdateComponentStatus(ctx context.Context, name string, status ComponentStatus) error
func (a *WorkspaceAdapter) UnregisterComponent(ctx context.Context, name string) error
func (a *WorkspaceAdapter) GetComponentDependents(ctx context.Context, componentName string) ([]string, error)
```

**Dolt Operations:**
```go
func (a *WorkspaceAdapter) CommitChanges(ctx context.Context, message string) error
func (a *WorkspaceAdapter) GetDoltStatus(ctx context.Context) (string, error)
func (a *WorkspaceAdapter) GetDoltLog(ctx context.Context, limit int) (string, error)
func (a *WorkspaceAdapter) StartDoltServer(ctx context.Context) error
```

**Utilities:**
```go
func LoadMigrationFiles(migrationsDir string) ([]*MigrationFile, error)
func ParseManifest(manifestJSON string) (*ComponentManifest, error)
func ValidateManifest(manifest *ComponentManifest) error
func IsDoltInstalled() bool
```

## Testing

### Unit Tests

```bash
cd ./engram/core
go test ./pkg/workspace/dolt/... -v
```

### Integration Tests

Integration tests require a running Dolt instance:

```bash
# Full tests (requires Dolt)
go test ./pkg/workspace/dolt/... -v

# Skip integration tests
go test ./pkg/workspace/dolt/... -v -short
```

### Isolation Tests

Verify workspace isolation:

```go
// See isolation_test.go for examples
func TestWorkspaceIsolation(t *testing.T) {
    // Create two workspace adapters
    oss := setupWorkspace("oss", 3307)
    acme := setupWorkspace("acme", 3308)

    // Insert data in OSS
    // Verify data NOT in Acme
}
```

## Examples

### Complete Component Installation

```go
package main

import (
    "context"
    "log"

    "github.com/vbonnet/engram/core/pkg/workspace/dolt"
)

func main() {
    ctx := context.Background()

    // 1. Setup adapter
    config := dolt.GetDefaultConfig("oss", "~/projects/myworkspace")
    adapter, _ := dolt.NewWorkspaceAdapter(config)
    defer adapter.Close()

    _ = adapter.Connect(ctx)
    _ = adapter.InitializeDatabase(ctx)

    // 2. Register component
    info := &dolt.ComponentInfo{
        Name:    "agm",
        Version: "1.0.0",
        Prefix:  "agm_",
        Status:  string(dolt.StatusInstalled),
    }
    _ = adapter.RegisterComponent(ctx, info)

    // 3. Load and apply migrations
    migrations, _ := dolt.LoadMigrationFiles("./agm/migrations")
    unapplied, _ := adapter.GetUnappliedMigrations(ctx, "agm", migrations)
    results, _ := adapter.ApplyMigrations(ctx, "agm", unapplied, "agm-1.0.0")

    for _, r := range results {
        log.Printf("Applied: %s", r.Migration.Name)
    }

    // 4. Commit to Dolt
    _ = adapter.CommitChanges(ctx, "agm: Install component")
}
```

### Querying Component Data

```go
// Get database connection
db := adapter.DB()

// Standard SQL queries
rows, err := db.QueryContext(ctx, "SELECT * FROM agm_sessions WHERE status = ?", "active")
defer rows.Close()

for rows.Next() {
    // Process rows
}
```

### Cross-Component Queries

```go
// JOIN across components (same database)
query := `
SELECT
  wf.name AS project_name,
  agm.id AS session_id
FROM wf_projects wf
LEFT JOIN agm_sessions agm ON wf.metadata->>'$.session_id' = agm.id
WHERE wf.status = 'active'
`

rows, err := db.QueryContext(ctx, query)
```

## Migration Guide

See [MIGRATION-GUIDE.md](./MIGRATION-GUIDE.md) for detailed migration patterns and examples.

## Error Handling

Common errors:

```go
var (
    ErrDoltNotInstalled     // Dolt not found on system
    ErrDoltServerNotRunning // Cannot connect to Dolt server
    ErrMigrationFailed      // Migration SQL execution failed
    ErrPrefixCollision      // Prefix already claimed
    ErrReservedPrefix       // Attempt to use reserved prefix
    ErrDuplicateMigration   // Migration already applied
    ErrDependencyNotMet     // Migration dependency missing
)
```

Example:

```go
err := adapter.RegisterComponent(ctx, info)
if errors.Is(err, dolt.ErrPrefixCollision) {
    // Handle prefix collision
}
```

## Performance

- **Query overhead**: ~3.5x MySQL (acceptable for local operations)
- **Connection pooling**: Automatic (max 25 connections)
- **Migration time**: ~50ms per migration (typical)
- **Isolation overhead**: None (separate databases)

## Dependencies

```go
import (
    _ "github.com/go-sql-driver/mysql"  // Required for Dolt connection
)
```

## Contributing

When adding features:
1. Add tests (unit + integration)
2. Update documentation
3. Maintain backward compatibility
4. Follow table prefix conventions

## License

See main engram LICENSE file.
