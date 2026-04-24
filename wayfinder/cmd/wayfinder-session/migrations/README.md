# Wayfinder Database Migrations

This directory contains SQL migration scripts for the Wayfinder database schema.

## Migration Files

- **001_create_projects_table.sql** - Initial projects table
- **002_create_phases_table.sql** - Phase tracking table (v1.x)
- **003_create_phase_validations_table.sql** - Phase validation results
- **004_rename_phases_to_waypoints.sql** - Rename phase → waypoint (v2.0)
- **ROLLBACK_004.sql** - Rollback waypoint → phase

## Running Migrations

### Apply Migration 004 (Phase → Waypoint Rename)

**Prerequisites**:
1. Backup database: `mysqldump wayfinder_db > backup_$(date +%Y%m%d_%H%M%S).sql`
2. Verify backup integrity: Test restore to a temporary database
3. Stop all Wayfinder services accessing the database

**Apply migration**:
```bash
mysql wayfinder_db < migrations/004_rename_phases_to_waypoints.sql
```

**Verify migration**:
```bash
# Check tables renamed
mysql wayfinder_db -e "SHOW TABLES LIKE 'wayfinder_w%';"
# Expected: wayfinder_waypoints, wayfinder_waypoint_validations

# Check backward-compatible views exist
mysql wayfinder_db -e "SHOW FULL TABLES WHERE Table_type = 'VIEW';"
# Expected: wayfinder_phases, wayfinder_phase_validations

# Verify data preserved
mysql wayfinder_db -e "SELECT COUNT(*) FROM wayfinder_waypoints;"
mysql wayfinder_db -e "SELECT COUNT(*) FROM wayfinder_phases;"  # via view
# Counts should match
```

## Rollback Procedures

### Rollback Migration 004

**If migration fails or needs to be reversed**:

```bash
# Apply rollback script
mysql wayfinder_db < migrations/ROLLBACK_004.sql

# Verify rollback
mysql wayfinder_db -e "SHOW TABLES LIKE 'wayfinder_p%';"
# Expected: wayfinder_phases, wayfinder_phase_validations

# Confirm views are gone
mysql wayfinder_db -e "SHOW FULL TABLES WHERE Table_type = 'VIEW';"
# Expected: Empty result

# Verify data preserved
mysql wayfinder_db -e "SELECT COUNT(*) FROM wayfinder_phases;"
```

## Migration 004 Details

### What Changes

**Tables renamed**:
- `wayfinder_phases` → `wayfinder_waypoints`
- `wayfinder_phase_validations` → `wayfinder_waypoint_validations`

**Columns renamed**:
- `phase_id` → `waypoint_id`
- `phase_name` → `waypoint_name`

**Indexes renamed**:
- `idx_phase_name` → `idx_waypoint_name`
- `idx_project_phase` → `idx_project_waypoint`

**Foreign keys renamed**:
- `fk_phase_project` → `fk_waypoint_project`
- `fk_validation_phase` → `fk_validation_waypoint`

### Backward Compatibility

Migration 004 creates **backward-compatible views** to support v1.x clients:

- **wayfinder_phases** view - Maps waypoints table to old phase interface
- **wayfinder_phase_validations** view - Maps waypoint validations to old interface

This allows v1.x clients to continue working during the transition period.

### Breaking Changes

**For v2.0 clients**:
- Must update queries to use `wayfinder_waypoints` table
- Must update column references: `phase_id` → `waypoint_id`, `phase_name` → `waypoint_name`
- Old table names will **not** work directly (views are read-only)

**View limitations**:
- Views are **read-only** - INSERT/UPDATE/DELETE will fail
- v1.x clients cannot write to database after migration
- v1.x clients can only read data via views

## Testing Migrations

### Test on Sample Database

```bash
# Create test database
mysql -e "CREATE DATABASE wayfinder_test;"

# Load schema (migrations 001-003)
mysql wayfinder_test < migrations/001_create_projects_table.sql
mysql wayfinder_test < migrations/002_create_phases_table.sql
mysql wayfinder_test < migrations/003_create_phase_validations_table.sql

# Insert sample data
mysql wayfinder_test <<EOF
INSERT INTO wayfinder_projects (project_id, project_name, created_at)
VALUES ('test-proj-1', 'Test Project', NOW());

INSERT INTO wayfinder_phases (phase_id, project_id, phase_name, status)
VALUES
  ('phase-1', 'test-proj-1', 'S1-Requirements', 'completed'),
  ('phase-2', 'test-proj-1', 'S2-Research', 'in_progress');

INSERT INTO wayfinder_phase_validations (validation_id, phase_id, validator_name, status, validation_time)
VALUES ('val-1', 'phase-1', 'requirements_validator', 'passed', NOW());
EOF

# Count records before migration
echo "BEFORE MIGRATION:"
mysql wayfinder_test -e "SELECT COUNT(*) AS phase_count FROM wayfinder_phases;"
mysql wayfinder_test -e "SELECT COUNT(*) AS validation_count FROM wayfinder_phase_validations;"

# Apply migration 004
mysql wayfinder_test < migrations/004_rename_phases_to_waypoints.sql

# Verify migration
echo "AFTER MIGRATION:"
mysql wayfinder_test -e "SELECT COUNT(*) AS waypoint_count FROM wayfinder_waypoints;"
mysql wayfinder_test -e "SELECT COUNT(*) AS phase_count_via_view FROM wayfinder_phases;"
mysql wayfinder_test -e "SELECT COUNT(*) AS validation_count FROM wayfinder_waypoint_validations;"

# Test rollback
mysql wayfinder_test < migrations/ROLLBACK_004.sql

# Verify rollback
echo "AFTER ROLLBACK:"
mysql wayfinder_test -e "SELECT COUNT(*) AS phase_count FROM wayfinder_phases;"
mysql wayfinder_test -e "SELECT COUNT(*) AS validation_count FROM wayfinder_phase_validations;"

# Cleanup
mysql -e "DROP DATABASE wayfinder_test;"
```

## Migration Checklist

Before running migration 004:

- [ ] Backup database: `mysqldump wayfinder_db > backup_YYYYMMDD_HHMMSS.sql`
- [ ] Test backup restore on temporary database
- [ ] Document current table row counts
- [ ] Stop all Wayfinder services (wayfinder-session, orchestrator, etc.)
- [ ] Tag codebase: `git tag v1.x.x-last-phase-schema`
- [ ] Test migration on test database first
- [ ] Schedule maintenance window (estimated 5 minutes downtime)

After migration 004:

- [ ] Verify table counts match pre-migration
- [ ] Verify backward-compatible views exist
- [ ] Test read access via views (v1.x compatibility)
- [ ] Update application code to v2.0 (use waypoint terminology)
- [ ] Deploy v2.0 application code
- [ ] Monitor for errors
- [ ] Document migration completion in CHANGELOG.md

## Troubleshooting

### Migration Fails Mid-Transaction

MySQL transactions ensure atomicity:
- If any step fails, **entire transaction rolls back**
- Database remains in pre-migration state
- Safe to retry after fixing issue

### Views Not Created

Check for existing views:
```bash
mysql wayfinder_db -e "SHOW FULL TABLES WHERE Table_type = 'VIEW';"
```

If views already exist, drop them manually:
```bash
mysql wayfinder_db -e "DROP VIEW IF EXISTS wayfinder_phases, wayfinder_phase_validations;"
```

Then re-run migration 004.

### Foreign Key Constraint Errors

If foreign key rename fails:
- Check dependent data exists: `SELECT * FROM wayfinder_projects LIMIT 5;`
- Verify referential integrity: No orphaned phase records
- Check InnoDB engine: `SHOW TABLE STATUS LIKE 'wayfinder_phases';`

### Rollback Fails

If rollback encounters errors:
1. Check current schema state: `SHOW CREATE TABLE wayfinder_waypoints;`
2. Manually drop views if they block rollback
3. Verify table names before continuing
4. Restore from backup as last resort

## Best Practices

1. **Always backup before migrations**
2. **Test on non-production database first**
3. **Schedule downtime window**
4. **Document pre-migration state** (table counts, schema dumps)
5. **Keep migration scripts in version control**
6. **Never modify migration files after applied** (create new migration instead)
7. **Monitor application logs after migration**
8. **Keep rollback script tested and ready**

## Migration History

| Version | Migration | Date | Description |
|---------|-----------|------|-------------|
| 1.0.0 | 001 | 2025-01-15 | Create projects table |
| 1.0.0 | 002 | 2025-01-15 | Create phases table |
| 1.0.0 | 003 | 2025-01-15 | Create phase validations table |
| 2.0.0 | 004 | 2026-03-03 | Rename phase → waypoint terminology |

## Support

For migration issues:
- Check TROUBLESHOOTING.md in project root
- Review ROLLBACK-PLAN.md in swarm directory
- Contact: engram-research team
