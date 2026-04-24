# Wayfinder Storage Configuration

## Overview

Wayfinder now supports configurable storage modes as part of the centralized component storage initiative (Phase 4, Task 4.2).

## Storage Modes

### Centralized Mode (Default)

**Current Behavior - Preserved**

Wayfinder data is stored in git-tracked workspace:
```
wf/
├── project-1/
│   └── WAYFINDER-STATUS.md
├── project-2/
│   └── WAYFINDER-STATUS.md
```

**Configuration:**
```yaml
# ~/.wayfinder/config.yaml
storage:
  mode: centralized              # Default mode
  workspace: engram-research     # Workspace name or absolute path
  relative_path: wf              # Path within workspace (default: wf)
  auto_symlink: true             # Auto-create symlinks (future feature)
```

**Benefits:**
- Git-tracked project history
- Portable across machines (clone repo = clone all projects)
- Multi-workspace support (separate projects per workspace)
- Corpus-callosum integration for cross-component queries

### Dotfile Mode (New Option)

**New Feature**

Store Wayfinder data in traditional dotfile location:
```
~/.wayfinder/
├── project-1/
│   └── WAYFINDER-STATUS.md
├── project-2/
│   └── WAYFINDER-STATUS.md
```

**Configuration:**
```yaml
# ~/.wayfinder/config.yaml
storage:
  mode: dotfile                  # Opt-in to dotfile mode
```

**Use Cases:**
- Quick prototyping without workspace
- CI/CD environments
- Users who prefer dotfiles over centralized storage

## Configuration

### Config File Location

`~/.wayfinder/config.yaml`

### Default Configuration

If no config file exists, Wayfinder defaults to centralized mode (preserves current behavior):

```yaml
storage:
  mode: centralized
  workspace: engram-research
  relative_path: wf
  auto_symlink: true
```

### Full Schema

```yaml
# ~/.wayfinder/config.yaml

storage:
  # Storage mode: determines where Wayfinder data is stored
  # Type: enum[dotfile, centralized]
  # Default: centralized
  mode: centralized

  # Workspace identifier for centralized mode
  # Type: string | null
  # Default: engram-research
  # Examples: "engram-research", "./engram-research"
  workspace: engram-research

  # Relative path within workspace (centralized mode only)
  # Type: string
  # Default: wf
  relative_path: wf

  # Explicit centralized path (alternative to workspace auto-detection)
  # Type: string | null
  # Default: null
  centralized_path: null

  # Enable automatic symlink creation (future feature)
  # Type: boolean
  # Default: true
  auto_symlink: true
```

## Workspace Detection

When `mode: centralized`, Wayfinder uses a 6-priority workspace detection algorithm:

1. **Explicit absolute path** - `workspace: ~/src/engram-research`
2. **Test mode** - `ENGRAM_TEST_MODE=1` + `ENGRAM_TEST_WORKSPACE`
3. **Environment variable** - `ENGRAM_WORKSPACE=/path/to/workspace`
4. **Auto-detect from $PWD** - Search upward for workspace markers (`.git`, `WORKSPACE.yaml`)
5. **Config default** - `workspace: engram-research` searches common locations:
   - `./engram-research`
   - `~/src/engram-research`
   - `~/engram-research`
6. **Not found** - Error with helpful message

## Usage

### Check Current Storage Mode

```bash
# View current config (if file exists)
cat ~/.wayfinder/config.yaml

# Or check where projects are stored
ls wf/  # Centralized mode
ls ~/.wayfinder/                            # Dotfile mode
```

### Switch to Dotfile Mode

Create `~/.wayfinder/config.yaml`:
```yaml
storage:
  mode: dotfile
```

All new Wayfinder projects will be created in `~/.wayfinder/` instead of workspace.

### Switch to Centralized Mode

Create `~/.wayfinder/config.yaml`:
```yaml
storage:
  mode: centralized
  workspace: engram-research
```

Or delete the config file to use defaults:
```bash
rm ~/.wayfinder/config.yaml
```

### Multi-Workspace Setup

Use different workspaces per machine via environment variable:

```bash
# Machine A
export ENGRAM_WORKSPACE=/work/workspace

# Machine B
export ENGRAM_WORKSPACE=/personal/workspace
```

Or use explicit paths in config:
```yaml
storage:
  mode: centralized
  workspace: /custom/path/to/workspace
  relative_path: wf
```

## Implementation Details

### Config Package

Location: `internal/config/config.go`

**Key Functions:**
- `Load()` - Load config from `~/.wayfinder/config.yaml` (returns default if not found)
- `DefaultConfig()` - Returns default configuration (centralized mode)
- `GetStoragePath()` - Returns absolute path where Wayfinder data should be stored
- `DetectWorkspace()` - Implements 6-priority workspace detection
- `Validate()` - Validates configuration

**Example Usage:**
```go
import "github.com/vbonnet/engram/core/cortex/cmd/wayfinder-session/internal/config"

// Load config
cfg, err := config.Load()
if err != nil {
    return err
}

// Get storage path
storagePath, err := cfg.GetStoragePath()
if err != nil {
    return err
}

// Use storage path for project operations
// ...
```

### Storage Path Resolution

**Dotfile Mode:**
```go
storagePath := "~/.wayfinder"  // Expanded to ~/.wayfinder
```

**Centralized Mode:**
```go
workspace := DetectWorkspace("engram-research")  // Returns: ./engram-research
storagePath := filepath.Join(workspace, "wf")     // Returns: wf
```

### Backward Compatibility

**Guarantee:** Existing Wayfinder installations continue to work without changes.

**Default Behavior:** If no config file exists, Wayfinder defaults to centralized mode with workspace `engram-research` (current behavior).

**Migration:** Users can opt-in to dotfile mode by creating config file. No automatic migration occurs.

## Testing

### Unit Tests

Location: `internal/config/config_test.go`

**Test Coverage:**
- Default config validation
- Valid and invalid configurations
- Storage path resolution (dotfile and centralized modes)
- Workspace detection (all 6 priorities)
- Workspace markers (`.git`, `WORKSPACE.yaml`)
- Path expansion (`~/`, environment variables)
- Config save/load roundtrip

**Run Tests:**
```bash
cd core/cortex/cmd/wayfinder-session
go test ./internal/config/... -v
```

### Integration Tests

**Test Scenarios:**
1. Fresh install (no config) → defaults to centralized mode
2. Dotfile mode → projects created in `~/.wayfinder/`
3. Centralized mode with explicit workspace → projects created in workspace
4. Workspace detection → finds workspace from $PWD
5. Environment variable override → uses `ENGRAM_WORKSPACE`
6. Test mode → uses `ENGRAM_TEST_WORKSPACE`

## Examples

### Example 1: Default (Centralized Mode)

No config file, uses defaults:
```bash
# Start wayfinder project
cd wf/my-project
wayfinder-session start my-project

# Project created at:
# wf/my-project/WAYFINDER-STATUS.md
```

### Example 2: Dotfile Mode

Create config:
```yaml
# ~/.wayfinder/config.yaml
storage:
  mode: dotfile
```

Usage:
```bash
# Start wayfinder project
wayfinder-session start my-project

# Project created at:
# ~/.wayfinder/my-project/WAYFINDER-STATUS.md
```

### Example 3: Custom Workspace

Create config:
```yaml
# ~/.wayfinder/config.yaml
storage:
  mode: centralized
  workspace: /work/my-workspace
  relative_path: wf
```

Usage:
```bash
# Start wayfinder project
wayfinder-session start my-project

# Project created at:
# /work/my-workspace/wf/my-project/WAYFINDER-STATUS.md
```

### Example 4: Environment Variable Override

```bash
# Set workspace via environment
export ENGRAM_WORKSPACE=/custom/workspace

# Start wayfinder project
wayfinder-session start my-project

# Project created at:
# /custom/workspace/wf/my-project/WAYFINDER-STATUS.md
```

## Future Enhancements

### Symlink Bootstrap (Phase 4.2 Extension)

When switching from dotfile to centralized mode, automatically:
1. Backup existing `~/.wayfinder/` data
2. Move data to centralized location
3. Create symlink: `~/.wayfinder → wf`

**Status:** Not implemented in initial version (Task 4.2 focuses on config support only)

### Corpus Callosum Integration (Phase 3)

Register Wayfinder schema with corpus-callosum for cross-component queries:
```bash
# Register wayfinder schema
cc register --component wayfinder --schema ./schema/wayfinder-v1.schema.json

# Query all wayfinder projects
cc query --components wayfinder --filter "status:in_progress"

# Cross-component query (wayfinder + agm + beads)
cc query --components wayfinder,agm,beads --filter "topic:authentication"
```

**Status:** Schema ready in `schema/wayfinder-v1.schema.json`, integration pending Phase 3 completion

## Architecture Decision

**ADR:** Centralized Storage as Default

**Context:**
- Wayfinder data was already centralized in `engram-research/wf/` before this task
- Git-tracked project history provides valuable version control
- Multi-workspace support enables separation of work/personal projects
- Corpus-callosum integration requires centralized storage

**Decision:**
- Default to centralized mode (preserves existing behavior)
- Add dotfile mode as opt-in feature (for users who prefer dotfiles)
- Use `wf/` as relative path (not `.wayfinder/`) since Wayfinder data is public project metadata

**Consequences:**
- Existing installations continue working without changes (backward compatible)
- Users can opt-in to dotfile mode if needed
- Future symlink bootstrap feature will enable seamless migration

## Related Documentation

- [Centralized Storage Spec](SPEC.md)
- [Config Schema](CONFIG-SCHEMA.md)
- [Symlink Bootstrap Pattern](SYMLINK-BOOTSTRAP.md)
- [Workspace Detection Algorithm](swarm/workspace-aware-tools/README.md)

## Support

For issues or questions:
1. Check workspace detection: `echo $ENGRAM_WORKSPACE`
2. Verify config: `cat ~/.wayfinder/config.yaml`
3. Test workspace detection: Create test project and check location
4. See troubleshooting guide in main SPEC.md

## Changelog

### v0.2.0 (2026-02-21) - Storage Config Support

**Added:**
- Storage mode configuration (`dotfile` or `centralized`)
- Workspace detection (6-priority algorithm)
- Config file support (`~/.wayfinder/config.yaml`)
- Comprehensive test suite

**Changed:**
- Default to centralized mode (preserves existing behavior)
- Workspace-aware project path resolution

**Backward Compatible:**
- No breaking changes
- Existing installations continue working
- Config file optional (uses defaults if not found)
