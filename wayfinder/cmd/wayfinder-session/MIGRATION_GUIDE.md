# Wayfinder V1 → V2 Migration Guide

## Quick Start

### Dry-Run (Recommended First Step)

```bash
# Preview what will be migrated
wayfinder-session migrate-all the git history --dry-run
```

### Migrate All Projects

```bash
# Migrate all V1 projects in workspace
wayfinder-session migrate-all the git history
```

### Parallel Migration (Faster)

```bash
# Migrate with 8 parallel workers
wayfinder-session migrate-all the git history --parallel --workers 8
```

## What Gets Migrated

### File Structure

**Before (V1)**:
```
the git history/
└── WAYFINDER-STATUS.md  (V1 schema)
```

**After (V2)**:
```
the git history/
├── WAYFINDER-STATUS.md            (V2 schema)
└── WAYFINDER-STATUS.v1.backup.*.md (V1 backup)
```

### Schema Changes

#### V1 Schema (13 phases)
```yaml
schema_version: "1.0"
session_id: "abc123"
current_phase: "S8"
phases:
  - name: "W0"
    status: "completed"
```

#### V2 Schema (9 phases)
```yaml
schema_version: "2.0"
project_name: "my-project"
project_type: "feature"
risk_level: "M"
current_phase: "S8"
phase_history:
  - name: "D1"  # W0 → D1
    status: "completed"
roadmap:
  phases: []
quality_metrics:
  coverage_target: 80.0
```

### Data Preservation

All V1 data is preserved:
- ✅ Session ID and timestamps
- ✅ Project path
- ✅ Current phase (mapped to V2 phases)
- ✅ Phase history (with phase name mapping)
- ✅ Status (in_progress, completed, etc.)

New V2 fields initialized:
- 🆕 `project_name`: Extracted from path
- 🆕 `project_type`: Defaults to "feature"
- 🆕 `risk_level`: Inferred from project complexity
- 🆕 `roadmap`: Empty roadmap structure
- 🆕 `quality_metrics`: Default targets (80% coverage, 3.0 assertion density)

## Phase Mapping

### Consolidated Phases

V2 reduces 13 phases to 9:

| V1       | V2   | Consolidation |
|----------|------|---------------|
| W0       | W0   | Intake & Waypoint |
| D1-D4    | D1-D4| Discovery (unchanged) |
| **S4**   | **D4** | **Merged into D4** |
| **S5**   | **S6** | **Removed (Research merged)** |
| S6       | S6   | Design |
| S7       | S7   | Planning |
| **S8, S9, S10** | **S8** | **Merged into BUILD loop** |
| S11      | S11  | Retrospective |

### Mapping Details

- **W0**: Removed in V2, projects starting at W0 map to D1
- **S4**: Stakeholder Alignment merged into D4 (Solution Requirements)
- **S5**: Research removed, maps to S6 (Design)
- **S8/S9/S10**: Implementation, Validation, Deployment merged into S8 BUILD loop

## Migration Report

### Report Format

```
==============================================================
MIGRATION SUMMARY
==============================================================
Total projects: 15
  ✓ Successful: 13
  ✗ Failed:     1
  ⊘ Skipped:    1

Failed Projects:
  - /path/to/project4
    Error: conversion error

Warnings:
  - /path/to/project1
    * Some V1 phases removed in V2: 13 -> 9 phases
==============================================================
```

### Status Indicators

- `✓` Success - Migration completed
- `✗` Failed - Migration error (check logs)
- `⊘` Skipped - Already V2 or excluded

## Command Reference

### migrate-all

Migrate all V1 projects in a workspace.

#### Syntax

```bash
wayfinder-session migrate-all [workspace-root] [flags]
```

#### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | false | Preview changes without modifying files |
| `--parallel` | false | Migrate projects in parallel |
| `--workers` | 4 | Number of parallel workers (only with --parallel) |

#### Examples

```bash
# Dry-run to check what will be migrated
wayfinder-session migrate-all the git history --dry-run

# Migrate all projects sequentially
wayfinder-session migrate-all the git history

# Fast parallel migration
wayfinder-session migrate-all the git history --parallel --workers 8

# Migration with custom worker count
wayfinder-session migrate-all ~/src/ws/acme/wf --parallel --workers 4
```

## Troubleshooting

### Migration Failures

#### "No WAYFINDER-STATUS.md found"

**Cause**: Project directory doesn't contain a status file.

**Solution**: Ensure the workspace path is correct:
```bash
# Check if workspace exists
ls the git history
```

#### "V2 validation failed"

**Cause**: Converted V2 data doesn't meet schema requirements.

**Solution**: Check V1 file for corrupted data:
```bash
# Inspect V1 file
cat the git history/WAYFINDER-STATUS.md
```

#### "Failed to create backup"

**Cause**: Filesystem write permissions.

**Solution**: Check directory permissions:
```bash
# Fix permissions
chmod 755 the git history
```

### Restore from Backup

If migration fails, restore from backup:

```bash
# Find backup file
ls the git history/WAYFINDER-STATUS.v1.backup.*.md

# Restore manually
cp WAYFINDER-STATUS.v1.backup.20260220-153045.md WAYFINDER-STATUS.md
```

Or use the restore command (if implemented):

```bash
wayfinder-session migrate-restore /path/to/project /path/to/backup.md
```

### Common Issues

#### Already V2 Projects

**Symptom**: Project skipped with "Already V2 schema" warning.

**Solution**: This is expected. Project already migrated, no action needed.

#### Partial Batch Failure

**Symptom**: Some projects fail in batch migration.

**Solution**: Review failed projects in report, fix issues, re-run for failed projects only.

## Performance Guide

### Sequential vs Parallel

| Workspace Size | Recommended Mode | Expected Time |
|----------------|------------------|---------------|
| 1-5 projects   | Sequential       | <10 seconds |
| 6-20 projects  | Parallel (4 workers) | ~10-30 seconds |
| 20+ projects   | Parallel (8 workers) | ~30-60 seconds |

### Worker Count Selection

- **CPU cores**: Set workers = CPU cores
- **I/O bound**: More workers can help (up to 16)
- **Memory**: Each worker needs ~50MB RAM

### Optimization Tips

```bash
# Fast migration for large workspaces
wayfinder-session migrate-all the git history \
  --parallel \
  --workers 8

# Dry-run first to identify problems
wayfinder-session migrate-all the git history --dry-run

# Then migrate for real
wayfinder-session migrate-all the git history --parallel
```

## Safety & Rollback

### Backup Strategy

Every migration creates a backup:

```
WAYFINDER-STATUS.v1.backup.YYYYMMDD-HHMMSS.md
```

Backups never overwrite each other due to timestamp.

### Rollback Steps

1. **Stop using V2 features**: Don't modify V2 status file
2. **Restore from backup**: Copy backup to WAYFINDER-STATUS.md
3. **Verify V1 schema**: Check `schema_version: "1.0"`

### Disaster Recovery

```bash
# Find all backups in workspace
find the git history -name "WAYFINDER-STATUS.v1.backup.*.md"

# Restore specific project
cd the git history
cp WAYFINDER-STATUS.v1.backup.20260220-153045.md WAYFINDER-STATUS.md

# Verify restoration
grep "schema_version" WAYFINDER-STATUS.md
# Should show: schema_version: "1.0"
```

## Post-Migration Validation

### Verify V2 Schema

```bash
# Check schema version
cd the git history
grep "schema_version" WAYFINDER-STATUS.md
# Should show: schema_version: "2.0"

# Validate structure
wayfinder-session status
```

### Test V2 Features

```bash
# Test task management
wayfinder-session task list

# Test phase transitions
wayfinder-session next-phase
```

## FAQ

### Q: Can I migrate individual projects?

A: Yes, use `MigrateProject()` programmatically:

```go
result, err := migration.MigrateProject("/path/to/project")
```

Or manually by copying the status file to a temporary location and using the batch command on a single project.

### Q: What happens to V1-only phases?

A: They are mapped to the nearest V2 equivalent:
- W0 → D1 (start of Discovery)
- S5 → S6 (Research merged into Design)

### Q: Can I roll back after migration?

A: Yes, restore from the backup file created during migration.

### Q: Will migration break existing workflows?

A: No. All V1 data is preserved. V2 is backward-compatible for reading, but uses the new schema for writing.

### Q: How do I migrate just one workspace?

A: Specify the workspace root:

```bash
wayfinder-session migrate-all the git history
```

### Q: What if I have mixed V1/V2 projects?

A: The migrator detects schema version and skips V2 projects automatically.

## Exit Codes

| Code | Meaning | Action |
|------|---------|--------|
| 0 | Success | All projects migrated |
| 1 | Partial failure | Check report for failed projects |
| 2 | No projects found | Verify workspace path |

## See Also

- [SPEC.md](SPEC.md) - Wayfinder V2 Specification
- [internal/migration/README.md](internal/migration/README.md) - Technical details
- [Task 2.3 Documentation](docs/tasks/2.3-batch-migration.md) - Implementation notes
