# Centralized Storage Support in AGM

**Status**: Implemented (Phase 4.1 - Task oss-vsez)
**Version**: 1.0
**Date**: 2026-02-21

## Overview

AGM now supports **centralized component storage**, allowing session data to be stored in a git-tracked repository (engram-research) instead of scattered dotfiles.

This makes AGM data:
- **Portable**: Clone repo on new machine = all session data available
- **Git-tracked**: Full history, backups, collaboration
- **Organized**: All component data in one place
- **Discoverable**: Corpus-callosum protocol for cross-component queries

## Storage Modes

AGM supports two storage modes:

### 1. Dotfile Mode (Default)

Traditional storage in `~/.agm/`:

```yaml
# ~/.config/agm/config.yaml
storage:
  mode: dotfile          # Default mode (backward compatible)
  workspace: ""          # Not used in dotfile mode
  relative_path: ".agm"  # Not used in dotfile mode
```

**Storage location**: `~/.agm/`

### 2. Centralized Mode (Opt-in)

Storage in workspace repository with symlink:

```yaml
# ~/.config/agm/config.yaml
storage:
  mode: centralized
  workspace: engram-research         # Workspace name or absolute path
  relative_path: .agm                # Path within workspace
```

**Storage location**: `.agm/`
**Symlink**: `~/.agm` → `.agm/`

## Quick Start

### Enable Centralized Mode

1. **Edit config**:

```bash
# Create or edit ~/.config/agm/config.yaml
cat >> ~/.config/agm/config.yaml <<EOF
storage:
  mode: centralized
  workspace: engram-research
  relative_path: .agm
EOF
```

2. **Run AGM** (symlink is created automatically):

```bash
agm session list
# AGM will:
# 1. Detect workspace at ./engram-research
# 2. Create ~/.agm -> engram-research/.agm symlink
# 3. Migrate existing data (if any)
# 4. Create backup at ~/.agm.backup.<pid>
```

3. **Verify setup**:

```bash
ls -la ~/.agm
# Expected output: lrwxrwxrwx ... ~/.agm -> .agm

ls .agm/
# Expected output: sessions/ config.yaml (your data, now git-tracked)
```

### Disable Centralized Mode (Rollback)

1. **Edit config back to dotfile mode**:

```yaml
# ~/.config/agm/config.yaml
storage:
  mode: dotfile
```

2. **Restore from backup** (if needed):

```bash
# Find backup
ls -d ~/.agm.backup.*

# Remove symlink and restore backup
rm ~/.agm
mv ~/.agm.backup.<pid> ~/.agm
```

## Workspace Detection

AGM uses a 6-priority workspace detection algorithm:

1. **Explicit path** in config: `workspace: /absolute/path/to/engram-research`
2. **Test mode**: `ENGRAM_TEST_MODE=1` + `ENGRAM_TEST_WORKSPACE=/path`
3. **Environment variable**: `ENGRAM_WORKSPACE=/path`
4. **Auto-detect from PWD**: Searches upward for `.git/` or `WORKSPACE.yaml`
5. **Common locations**:
   - `./engram-research`
   - `~/src/ws/engram-research/repos/engram-research`
   - `~/src/engram-research`
   - `~/engram-research`
6. **Error**: Workspace not found (AGM will warn and continue in dotfile mode)

## Configuration Schema

### Full Configuration Example

```yaml
# ~/.config/agm/config.yaml

# Storage configuration (centralized component storage)
storage:
  mode: centralized              # Mode: "dotfile" or "centralized"
  workspace: engram-research     # Workspace name or absolute path
  relative_path: .agm            # Path within workspace (default: .agm)

# Legacy workspace detection (still supported)
workspace: oss                   # Explicit workspace for session isolation
workspace_config: ~/.agm/config.yaml  # Workspace config path

# Other AGM settings
sessions_dir: ~/sessions         # Overrides storage.mode (if set explicitly)
log_level: info
log_file: ""

timeout:
  tmux_commands: 5s
  enabled: true

lock:
  enabled: true
  path: /tmp/agm-{UID}/agm.lock

health_check:
  enabled: true
  cache_duration: 5s
  probe_timeout: 2s
```

### Storage Configuration Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `storage.mode` | string | `"dotfile"` | Storage mode: `"dotfile"` or `"centralized"` |
| `storage.workspace` | string | `""` | Workspace name or absolute path (for centralized mode) |
| `storage.relative_path` | string | `".agm"` | Path within workspace (default: `.agm`) |

### Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `ENGRAM_WORKSPACE` | Override workspace detection | `export ENGRAM_WORKSPACE=/path/to/workspace` |
| `ENGRAM_TEST_MODE` | Enable test mode (for testing) | `export ENGRAM_TEST_MODE=1` |
| `ENGRAM_TEST_WORKSPACE` | Test workspace path | `export ENGRAM_TEST_WORKSPACE=/tmp/test` |

## Implementation Details

### Symlink Bootstrap

When centralized mode is enabled, AGM automatically creates a symlink:

```
~/.agm → .agm
```

**Bootstrap workflow**:

1. **No existing ~/.agm**: Create symlink directly
2. **Existing symlink**: Verify target is correct, update if needed
3. **Existing directory**:
   - Backup to `~/.agm.backup.<pid>`
   - Copy data to centralized location
   - Create symlink
   - Keep backup for safety

### Code Structure

```
internal/config/
├── config.go        # Config struct with StorageConfig field
├── storage.go       # Storage path resolution and symlink management
└── storage_test.go  # Tests for storage functionality
```

**Key functions**:

- `GetStoragePath(cfg)` - Resolves absolute storage path based on mode
- `DetectWorkspace(nameOrPath)` - 6-priority workspace detection
- `EnsureSymlinkBootstrap(cfg)` - Creates/updates symlink for centralized mode
- `VerifyStorageIntegrity(cfg)` - Validates storage configuration

### Initialization

Bootstrap is called during AGM startup in `cmd/agm/main.go`:

```go
func loadConfigWithFlags() (*config.Config, error) {
    // ... load config ...

    // Centralized storage support: Create symlink if centralized mode is enabled
    if cfg.Storage.Mode == "centralized" {
        if err := config.EnsureSymlinkBootstrap(cfg); err != nil {
            // Log warning but don't fail - allow AGM to continue in degraded mode
            fmt.Fprintf(os.Stderr, "Warning: Failed to setup centralized storage symlink: %v\n", err)
        }
    }

    return cfg, nil
}
```

## Testing

### Unit Tests

```bash
# Run storage config tests
go test ./internal/config/... -v

# Run specific test
go test ./internal/config -run TestGetStoragePath -v
```

**Test coverage**:
- Storage path resolution (dotfile vs centralized)
- Workspace detection (all 6 priorities)
- Symlink creation and migration
- Config validation
- Directory copying

### Integration Testing

```bash
# Test with test mode
export ENGRAM_TEST_MODE=1
export ENGRAM_TEST_WORKSPACE=/tmp/test-workspace
mkdir -p /tmp/test-workspace

# Create test config
cat > ~/.config/agm/config.yaml <<EOF
storage:
  mode: centralized
  workspace: test-workspace
  relative_path: .agm
EOF

# Run AGM
agm session list

# Verify symlink created
ls -la ~/.agm
readlink ~/.agm  # Should point to /tmp/test-workspace/.agm
```

### Manual Testing Checklist

- [ ] Fresh install with centralized mode creates symlink
- [ ] Existing dotfile directory migrates correctly
- [ ] Backup is created during migration
- [ ] Symlink points to correct location
- [ ] Data is accessible through symlink
- [ ] Git tracks centralized storage
- [ ] Rollback to dotfile mode works
- [ ] Multi-workspace scenarios work
- [ ] Environment variable overrides work

## Troubleshooting

### Symlink Creation Failed

**Error**: `Failed to setup centralized storage symlink: workspace 'engram-research' not found`

**Solution**:
1. Use absolute path in config:
   ```yaml
   storage:
     workspace: ./engram-research
   ```

2. Or set environment variable:
   ```bash
   export ENGRAM_WORKSPACE=./engram-research
   agm session list
   ```

3. Or clone workspace to expected location:
   ```bash
   git clone <repo> ./engram-research
   ```

### Data Not Syncing

**Problem**: Changes in `~/.agm/` not appearing in repository

**Diagnosis**:
```bash
# Check symlink
ls -la ~/.agm
readlink ~/.agm

# Verify target
ls -la $(readlink ~/.agm)

# Check they point to same inode
stat ~/.agm
stat $(readlink ~/.agm)
```

**Solution**: Recreate symlink:
```bash
rm ~/.agm
agm session list  # Symlink will be recreated
```

### Permission Denied

**Problem**: Cannot write to centralized storage

**Diagnosis**:
```bash
# Check permissions
ls -la .agm

# Check disk space
df -h ./engram-research
```

**Solution**:
```bash
# Fix permissions
chmod 755 .agm
chmod 644 .agm/config.yaml
```

### Multiple Workspaces Conflict

**Problem**: Two engram-research clones (work vs personal)

**Solution 1** - Use different component names:
```yaml
# Work: ~/.config/agm/config.yaml
storage:
  mode: centralized
  workspace: ~/work/engram-research
  relative_path: .agm-work
```

**Solution 2** - Use absolute paths:
```yaml
# Work
storage:
  workspace: ~/work/engram-research

# Personal
storage:
  workspace: ~/personal/engram-research
```

**Solution 3** - Use environment variable per session:
```bash
# Work sessions
ENGRAM_WORKSPACE=~/work/engram-research agm session list

# Personal sessions
ENGRAM_WORKSPACE=~/personal/engram-research agm session list
```

## Migration Guide

### From Dotfile to Centralized

**Preparation**:
1. Ensure engram-research is cloned:
   ```bash
   git clone <repo> ./engram-research
   ```

2. Backup existing data:
   ```bash
   cp -r ~/.agm ~/.agm.manual-backup
   ```

**Migration**:
1. Update config:
   ```yaml
   # ~/.config/agm/config.yaml
   storage:
     mode: centralized
     workspace: engram-research
     relative_path: .agm
   ```

2. Run AGM (migration happens automatically):
   ```bash
   agm session list
   ```

3. Verify migration:
   ```bash
   # Check symlink
   ls -la ~/.agm

   # Check data in repo
   ls .agm/

   # Verify sessions work
   agm session list
   ```

4. Git commit (optional but recommended):
   ```bash
   cd ./engram-research
   git add .agm/
   git commit -m "feat: migrate AGM data to centralized storage

   Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
   ```

**Rollback** (if needed):
```bash
# Remove symlink
rm ~/.agm

# Restore from backup
mv ~/.agm.manual-backup ~/.agm

# Update config back to dotfile mode
# Edit ~/.config/agm/config.yaml:
#   storage:
#     mode: dotfile
```

## Backward Compatibility

- **Default mode**: Dotfile (no breaking changes)
- **Existing configs**: Continue to work (mode defaults to "dotfile")
- **SessionsDir override**: Takes precedence over storage.mode
- **Workspace detection**: Unchanged (still works)
- **Existing data**: Automatically migrated on mode switch

## Future Enhancements

1. **CLI commands** (not yet implemented):
   ```bash
   agm storage migrate --workspace engram-research
   agm storage rollback
   agm storage status
   agm storage verify
   ```

2. **Corpus-callosum integration** (Phase 3):
   - Component registration
   - Cross-component queries
   - Unified discovery

3. **Multiple workspaces support**:
   - Per-workspace configs
   - Workspace switching
   - Isolation improvements

## References

- **Spec**: `SPEC.md`
- **ROADMAP**: `ROADMAP.md`
- **Config schema**: `CONFIG-SCHEMA.md`
- **Symlink bootstrap**: `SYMLINK-BOOTSTRAP.md`
- **Workspace detection**: `docs/workspace-detection.md`

## Credits

- **Implementation**: Task oss-vsez (Phase 4.1)
- **Architecture**: Centralized Component Storage Swarm
- **Precedent**: AGM workspace data location (existing .agm/sessions pattern)

---

**Status**: Implementation Complete
**Tests**: Unit tests passing
**Documentation**: Complete
**Ready for**: Integration testing and deployment
