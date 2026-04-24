# Wayfinder V2 Migration Quick Start

Get started with V1→V2 migration in 5 minutes.

## Quick Check

Before migrating, verify your project is V1:

```bash
# Check schema version
head -5 WAYFINDER-STATUS.md | grep schema_version

# Expected output for V1:
# schema_version: "1.0"
```

## Basic Migration

### Step 1: Dry-Run (Preview)

Always preview changes first:

```bash
wayfinder-session migrate . --dry-run
```

This shows what will change without modifying files.

### Step 2: Migrate

If the preview looks good, migrate:

```bash
wayfinder-session migrate .
```

This creates:
- Updated `WAYFINDER-STATUS.md` (V2 format)
- Backup `WAYFINDER-STATUS.md.v1.backup`

### Step 3: Verify

Check the migrated file:

```bash
head -10 WAYFINDER-STATUS.md
```

Expected output:
```yaml
---
schema_version: "2.0"
project_name: your-project
project_type: feature
risk_level: M
current_phase: S8
status: in-progress
created_at: 2026-02-01T10:00:00Z
updated_at: 2026-02-20T15:30:00Z
```

## Common Use Cases

### Migrate with Custom Metadata

```bash
wayfinder-session migrate . \
  --project-name "My Project" \
  --project-type infrastructure \
  --risk-level L
```

### Migrate Multiple Projects

```bash
# Create a script
for dir in project1 project2 project3; do
  wayfinder-session migrate $dir
done
```

### Verbose Output

See detailed migration report:

```bash
wayfinder-session migrate . --verbose
```

## Phase Mapping Reference

Quick reference for how V1 phases map to V2:

| V1 | V2 | Change |
|----|----|----|
| W0 | W0 | Same |
| D1 | D1 | Same |
| D2 | D2 | Same |
| D3 | D3 | Same |
| D4 | D4 | Same |
| S4 | D4 | Merged |
| S5 | S6 | Merged |
| S6 | S6 | Same |
| S7 | S7 | Same |
| S8 | S8 | Same |
| S9 | S8 | Merged |
| S10 | S8 | Merged |
| S11 | S11 | Same |

**Result**: 13 phases → 9 phases

## Flags Reference

| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | false | Preview without modifying files |
| `--verbose` | false | Show detailed report |
| `--backup` | true | Create backup file |
| `--project-name` | (auto) | Override project name |
| `--project-type` | feature | Set project type |
| `--risk-level` | M | Set risk level |
| `--preserve-session-id` | false | Keep V1 session ID as tag |

## Troubleshooting

### "Already using V2 schema"

Your file is already V2. No migration needed.

### "Invalid schema version"

File might be corrupted or not a Wayfinder status file.

Solution:
```bash
# Check file format
head -20 WAYFINDER-STATUS.md
```

### "Missing required field"

V1 file is incomplete.

Solution: Add missing fields to V1 file before migrating.

### Backup file exists

Previous migration backup exists.

Solution:
```bash
# Remove old backup
rm WAYFINDER-STATUS.md.v1.backup

# Or rename it
mv WAYFINDER-STATUS.md.v1.backup WAYFINDER-STATUS.md.v1.backup.old
```

## FAQ

**Q: Can I undo a migration?**

A: Yes, restore from backup:
```bash
cp WAYFINDER-STATUS.md.v1.backup WAYFINDER-STATUS.md
```

**Q: What if migration fails?**

A: The original file is preserved via backup. Simply restore it.

**Q: Can I migrate without backup?**

A: Yes, but not recommended:
```bash
wayfinder-session migrate . --backup=false
```

**Q: How long does migration take?**

A: Typically <100ms per project.

**Q: Is data lost during migration?**

A: No. 100% of V1 data is preserved in V2 format.

**Q: Can I migrate in-progress projects?**

A: Yes. The current phase is preserved.

## Next Steps

After migration:

1. **Review** the migrated WAYFINDER-STATUS.md
2. **Verify** all data was preserved
3. **Remove** backup if satisfied: `rm WAYFINDER-STATUS.md.v1.backup`
4. **Continue** working with V2 features

## Get Help

```bash
# Show help
wayfinder-session migrate --help

# View examples
wayfinder-session migrate --help | grep -A 20 Examples
```

## Examples

### Example 1: Simple Migration

```bash
$ cd my-project
$ wayfinder-session migrate . --dry-run

Migration Summary:
Project Name:     my-project
Schema:           1.0 → 2.0
V1 Phases:        8
V2 Phase History: 7

$ wayfinder-session migrate .
✓ Created backup: WAYFINDER-STATUS.md.v1.backup
✓ Migration complete!
```

### Example 2: Infrastructure Project

```bash
$ wayfinder-session migrate ~/projects/auth-service \
    --project-name "Authentication Service" \
    --project-type infrastructure \
    --risk-level XL

✓ Migration complete!
```

### Example 3: Batch Migration

```bash
$ for dir in oss-*; do
    echo "Migrating $dir..."
    wayfinder-session migrate $dir
  done

Migrating oss-wp12...
✓ Migration complete!
Migrating oss-wp13...
✓ Migration complete!
```

## Resources

- [README.md](README.md) - Full documentation
- [TESTING.md](TESTING.md) - Testing guide
- [IMPLEMENTATION.md](IMPLEMENTATION.md) - Implementation details
- [SPEC.md](../../SPEC.md) - Wayfinder V2 specification
