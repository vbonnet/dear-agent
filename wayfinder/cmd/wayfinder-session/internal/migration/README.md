# Wayfinder V1 → V2 Migration System

## Overview

This package implements the batch migration system for converting Wayfinder V1 projects to V2 schema.

## Architecture

```
migration/
├── converter/        # V1 to V2 data conversion (Task 2.1)
│   ├── converter.go
│   └── converter_test.go
├── batch.go          # Batch migration orchestration (Task 2.3)
├── batch_test.go
├── migrator.go       # Single-file migration (Task 2.2)
├── migrator_test.go
└── README.md
```

## Features

### 1. Converter (Task 2.1)
- **File**: `converter/converter.go`
- **Purpose**: Convert V1 Status to V2 StatusV2 structure
- **Key Functions**:
  - `ConvertV1ToV2()`: Main conversion function
  - `convertPhase()`: Phase name mapping (W0-S11 → W0, D1-D4, S6-S8, S11)
  - `convertPhaseHistory()`: Phase history transformation
  - `inferRiskLevel()`: Infer risk based on project complexity
  - `extractProjectName()`: Extract project name from path

### 2. File Migrator (Task 2.2)
- **File**: `migrator.go`
- **Purpose**: Migrate a single project with backup and validation
- **Key Functions**:
  - `MigrateProject()`: Migrate single project
  - `createBackup()`: Create timestamped backup
  - `RestoreFromBackup()`: Restore from backup
  - `DryRun()`: Preview migration without changes

### 3. Batch Migrator (Task 2.3)
- **File**: `batch.go`
- **Purpose**: Migrate all projects in a workspace
- **Key Functions**:
  - `MigrateAll()`: Orchestrate batch migration
  - `findV1Projects()`: Scan workspace for V1 projects
  - `migrateSequential()`: Sequential migration
  - `migrateParallel()`: Parallel migration with workers

## Usage

### Command Line

```bash
# Migrate all projects in workspace
wayfinder-session migrate-all the git history

# Dry-run to preview changes
wayfinder-session migrate-all the git history --dry-run

# Parallel migration with 8 workers
wayfinder-session migrate-all the git history --parallel --workers 8
```

### Programmatic

```go
import "github.com/vbonnet/engram/core/cortex/cmd/wayfinder-session/internal/migration"

// Migrate single project
result, err := migration.MigrateProject("/path/to/project")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Backup: %s\n", result.BackupPath)

// Batch migration
options := migration.BatchMigrationOptions{
    WorkspaceRoot: "the git history",
    DryRun:        false,
    Parallel:      true,
    MaxWorkers:    4,
}

report, err := migration.MigrateAll(options)
if err != nil {
    log.Fatal(err)
}

report.PrintReport()
```

## Migration Process

### Single Project Migration

1. **Detect Schema Version**: Check if already V2 (skip if so)
2. **Read V1 Status**: Parse existing WAYFINDER-STATUS.md
3. **Create Backup**: Timestamped backup (WAYFINDER-STATUS.v1.backup.YYYYMMDD-HHMMSS.md)
4. **Convert to V2**: Transform data structures
5. **Validate V2**: Ensure schema compliance
6. **Write V2 File**: Overwrite with V2 format

### Batch Migration

1. **Scan Workspace**: Find all WAYFINDER-STATUS.md files
2. **Filter V1 Projects**: Exclude already-migrated V2 projects
3. **Migrate Each Project**: Sequential or parallel processing
4. **Generate Report**: Summary with successes, failures, warnings

## Phase Mapping

### V1 → V2 Phase Mapping

| V1 Phase | V2 Phase | Notes |
|----------|----------|-------|
| W0       | D1       | W0 removed in V2, maps to first phase |
| D1       | D1       | Discovery & Context |
| D2       | D2       | Investigation & Options |
| D3       | D3       | Architecture & Design Spec |
| D4       | D4       | Solution Requirements |
| S4       | D4       | Merged into D4 |
| S5       | S6       | Research removed, maps to next phase |
| S6       | S6       | Design |
| S7       | S7       | Planning & Task Breakdown |
| S8       | S8       | BUILD Loop |
| S9       | S8       | Validation merged into BUILD |
| S10      | S8       | Deployment merged into BUILD |
| S11      | S11      | Closure & Retrospective |

## Data Preservation

100% of V1 data is preserved in V2:

- **Session metadata**: Session ID, timestamps
- **Project info**: Project path, status
- **Phase history**: All phase records with timestamps
- **Completion data**: End dates, outcomes

## Error Handling

### Migration Failures

Failures are categorized and reported:

1. **File not found**: No WAYFINDER-STATUS.md
2. **Invalid V1 format**: Corrupted or malformed V1 file
3. **Conversion error**: Data transformation failed
4. **Validation error**: V2 schema validation failed
5. **Write error**: Filesystem write failed

### Backup and Restore

Backups are created before any modifications:

```bash
# Backup filename format
WAYFINDER-STATUS.v1.backup.20260220-153045.md

# Restore from backup (if migration fails)
wayfinder-session migrate-restore /path/to/project /path/to/backup.md
```

## Testing

### Run All Tests

```bash
cd cortex/cmd/wayfinder-session

# Converter tests
go test ./internal/converter/... -v

# Migrator tests
go test ./internal/migration/... -v -run "TestMigrateProject|TestDryRun|TestBackup"

# Batch migrator tests
go test ./internal/migration/... -v -run "TestMigrateAll|TestFindV1"
```

### Test Coverage

Target: >70% coverage

```bash
go test ./internal/converter/... -cover
go test ./internal/migration/... -cover
```

## Performance

### Sequential Migration

- **Speed**: ~1 second per project
- **Use case**: Small workspaces (<10 projects)

### Parallel Migration

- **Speed**: ~4x faster with 4 workers
- **Use case**: Large workspaces (10+ projects)
- **Workers**: Configurable (default: 4, max: 16)

### Benchmarks

```bash
# Run benchmarks
go test ./internal/migration/... -bench=. -benchmem
```

## Safety Features

1. **Backup before migration**: Original file preserved
2. **Schema validation**: V2 validation before write
3. **Dry-run mode**: Preview without modifications
4. **Already-V2 detection**: Skip migrated projects
5. **Error isolation**: Single project failure doesn't stop batch

## Exit Codes

| Code | Meaning |
|------|---------|
| 0    | All projects migrated successfully |
| 1    | One or more projects failed |
| 2    | No projects found to migrate |

## Dependencies

- **converter**: V1 to V2 conversion logic
- **status**: V1 and V2 schema definitions
- **workspace**: Project discovery

## Version Compatibility

- **V1 Schema**: 1.0
- **V2 Schema**: 2.0
- **Minimum Go**: 1.21

## See Also

- [SPEC.md](../../SPEC.md) - Wayfinder V2 Specification
- [Task 2.1](../../docs/tasks/2.1-converter.md) - Converter implementation
- [Task 2.2](../../docs/tasks/2.2-file-migration.md) - File migration
- [Task 2.3](../../docs/tasks/2.3-batch-migration.md) - Batch migration (this task)
