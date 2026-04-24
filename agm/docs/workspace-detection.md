# Workspace Detection in AGM

AGM uses automatic workspace detection to enable workspace-specific session storage. This allows you to isolate sessions by workspace (e.g., `oss`, `acme`, `personal`) without manual configuration.

## How It Works

When you run AGM without explicitly setting a sessions directory, it automatically detects which workspace you're in based on your current working directory.

### Detection Priority

AGM follows a 6-priority detection algorithm (from highest to lowest priority):

1. **Explicit `--workspace` flag** (highest priority)
   ```bash
   agm --workspace=oss session list
   ```

2. **`WORKSPACE` environment variable**
   ```bash
   WORKSPACE=oss agm session list
   ```

3. **Auto-detect from PWD** (walk up directory tree)
   ```bash
   cd .
   agm session list  # Detects 'oss' workspace
   ```

4. **Default workspace from config**
   ```yaml
   # ~/.agm/config.yaml
   version: 1
   default_workspace: oss
   ```

5. **Interactive prompt** (disabled in AGM - non-interactive CLI)

6. **Error / Fallback** → Uses `~/.claude/sessions` (backward compatible)

### Sessions Directory Location

Once a workspace is detected, AGM stores sessions at:
```
{workspace_root}/.agm/sessions/
```

For example:
- **oss workspace**: `~/.agm/sessions/`
- **acme workspace**: `~/src/ws/acme/.agm/sessions/`
- **No workspace**: `~/.claude/sessions/` (default fallback)

## Configuration

### Workspace Config File

Default location: `~/.agm/config.yaml`

```yaml
version: 1
default_workspace: oss  # Optional: fallback workspace

workspaces:
  - name: oss
    root: ~/projects/myworkspace
    enabled: true

  - name: acme
    root: ~/src/ws/acme
    enabled: true

  - name: personal
    root: ~/src/personal
    enabled: false  # Disabled workspaces are skipped
```

### Custom Config Path

You can override the workspace config path:

```yaml
# ~/.config/agm/config.yaml
workspace_config: /custom/path/to/workspace-config.yaml
```

## Edge Cases

AGM handles the following edge cases gracefully:

### 1. Missing Workspace Config

If `~/.agm/config.yaml` doesn't exist:
- **Behavior**: Falls back to `~/.claude/sessions`
- **Message**: None (silent fallback)
- **Debug**: `agm --debug` shows "Info: No workspace config found"

### 2. Invalid/Corrupted Config

If config exists but is invalid YAML or missing required fields:
- **Behavior**: Falls back to `~/.claude/sessions`
- **Message**: Warning printed to stderr with details
- **Example**:
  ```
  Warning: Failed to load workspace config from ~/.agm/config.yaml: invalid version
           Using default sessions directory. Fix config or remove it to clear this warning.
  ```

### 3. Directory Outside Any Workspace

If you're in a directory not under any workspace root:
- **Behavior**: Uses default workspace (if configured) or falls back to `~/.claude/sessions`
- **Message**: None (silent fallback)
- **Debug**: `agm --debug` shows "Info: No workspace detected for /path/to/dir"

### 4. Nested Workspaces (Ambiguous Path)

If a directory matches multiple workspaces (e.g., nested workspace roots):
- **Behavior**: Uses first match found while walking up the directory tree
- **Note**: The engram detector walks from deepest to shallowest, so the most specific workspace wins

### 5. Disabled Workspaces

If you're in a directory under a disabled workspace:
- **Behavior**: Workspace is skipped, falls back to default or `~/.claude/sessions`
- **Use case**: Temporarily disable a workspace without deleting its config

### 6. Explicit Flag with Unknown Workspace

If you specify `--workspace=nonexistent`:
- **Behavior**: Falls back to `~/.claude/sessions`
- **Message**: Warning printed:
  ```
  Warning: Workspace 'nonexistent' not found or disabled: workspace not found: nonexistent
           Using default sessions directory.
  ```

## Overriding Detection

### Explicit Sessions Directory

If you set sessions directory explicitly, workspace detection is **skipped entirely**:

```bash
# Via flag
agm --sessions-dir=/custom/path session list

# Via environment
AGM_SESSIONS_DIR=/custom/path agm session list

# Via config
# ~/.config/agm/config.yaml
sessions_dir: /custom/path
```

### Explicit Workspace in Config

If you set workspace in AGM config, detection is **skipped entirely**:

```yaml
# ~/.config/agm/config.yaml
workspace: oss  # Force 'oss' workspace always
```

## Debugging

To see detailed workspace detection information:

```bash
agm --debug session list
```

Example output:
```
Info: No workspace config found at ~/.agm/config.yaml, using default sessions dir
# OR
Info: Detected workspace 'oss' at ~/projects/myworkspace
      Using sessions directory: ~/.agm/sessions
```

## Testing

Workspace detection has comprehensive test coverage in `cmd/agm/workspace_test.go`:

- ✅ Missing config file
- ✅ Invalid/corrupted config
- ✅ Explicit `--workspace` flag
- ✅ Unknown workspace in flag
- ✅ Auto-detection from PWD
- ✅ Auto-detection from nested directories
- ✅ Multiple configured workspaces
- ✅ Directory outside any workspace
- ✅ Default workspace fallback
- ✅ Disabled workspaces (skipped)

Run tests:
```bash
cd agm
go test -v ./cmd/agm -run TestDetectWorkspace
```

## Implementation Details

### Code Location

- **Main logic**: `cmd/agm/main.go` → `loadConfigWithFlags()` and `detectWorkspace()`
- **Tests**: `cmd/agm/workspace_test.go`
- **Detector library**: `engram/core/pkg/workspace` (shared across tools)

### Detection Flow

```
loadConfigWithFlags()
  ├─ Load AGM config from ~/.config/agm/config.yaml
  ├─ Check: Is SessionsDir explicitly set? (flag/env/config)
  │   └─ YES → Skip workspace detection (user override)
  ├─ Check: Is Workspace already set in config?
  │   └─ YES → Skip workspace detection (explicit selection)
  └─ NO → Call detectWorkspace()
       ├─ Determine workspace config path (default: ~/.agm/config.yaml)
       ├─ Check: Does config file exist?
       │   └─ NO → Return (fall back to ~/.claude/sessions)
       ├─ Load workspace config with NewDetectorWithInteractive()
       │   └─ FAIL → Print warning, return (fall back)
       ├─ Get current working directory
       │   └─ FAIL → Return (fall back)
       ├─ Run detector.Detect(cwd, workspaceFlag)
       │   ├─ Priority 1: Check --workspace flag
       │   ├─ Priority 2: Check WORKSPACE env var
       │   ├─ Priority 3: Auto-detect from PWD (walk up tree)
       │   ├─ Priority 4: Use default_workspace from config
       │   ├─ Priority 5: Interactive prompt (disabled)
       │   └─ Priority 6: Error
       ├─ SUCCESS → Set cfg.Workspace and cfg.SessionsDir
       └─ FAIL → Return (fall back to ~/.claude/sessions)
```

## Migration from Single Sessions Directory

If you previously used a single `~/.claude/sessions` directory and want to migrate to workspace-specific sessions:

1. Create workspace config (`~/.agm/config.yaml`)
2. AGM will automatically use workspace-specific sessions for new sessions
3. Old sessions in `~/.claude/sessions` remain accessible (backward compatible)
4. Optionally migrate old sessions:
   ```bash
   # List old sessions
   agm --sessions-dir=~/.claude/sessions session list

   # Migrate manually by moving session directories
   mv ~/.claude/sessions/my-session ~/.agm/sessions/
   ```

## Security Considerations

- Workspace config is loaded from `~/.agm/config.yaml` (user-writable)
- Config file should have restrictive permissions (0600 recommended)
- AGM validates config schema before using it
- Invalid config triggers clear warnings rather than silent failures
- All paths are normalized and validated before use

## Related Documentation

- **Workspace Config Schema**: See `engram/core/pkg/workspace/types.go`
- **Detection Algorithm**: See `engram/core/pkg/workspace/detector.go`
- **AGM Configuration**: See `docs/configuration.md`
