# ADR-001: Hook Extension Fix Strategy

**Status**: Accepted
**Date**: 2026-03-14
**Deciders**: Claude Sonnet 4.5
**Context**: Phase 2 - Engram Doctor Auto-Fix Enhancement

## Context

Claude Code startup was failing with 19 hook errors because `settings.json` referenced hook files with `.py` extensions, but the actual hook binaries (compiled from Go) don't have extensions. This mismatch causes Claude Code to report "Hook commands missing" errors.

**Root Cause**:
- Hooks were originally implemented in Python (`.py` files)
- Hooks migrated to Go (compiled binaries without extensions)
- Configuration files still reference old `.py` names

**Example**:
```json
// settings.json references:
"command": "~/.claude/hooks/posttool-auto-commit-beads.py"

// But actual file is:
~/.claude/hooks/posttool-auto-commit-beads (Go binary, no extension)
```

## Decision

We will **automatically remove `.py` extensions** from hook command paths when:
1. Settings reference a `.py` file
2. The corresponding binary (without `.py`) exists on the filesystem

**Auto-fix applies to**:
- `~/.claude/settings.json` (primary hook configuration)
- `~/.claude/plugins/*/hooks.json` (plugin-specific hooks discovered from settings)

**Detection**: `checkHookExtensionMatch()` in `HealthChecker`
**Fix**: `fixHookExtensionMismatches()` in `Tier1Fixer`

## Rationale

### Why Auto-Fix?

**Alternatives Considered**:

1. **Manual user fix** (rejected)
   - Burden on every engram user
   - Error-prone (users might not know correct paths)
   - Breaks user experience on upgrade

2. **Update hooks to match .py names** (rejected)
   - Hooks are Go binaries (no .py extension standard)
   - Would require renaming deployed binaries
   - Inconsistent with Go ecosystem conventions

3. **Generate new settings.json** (rejected for this phase)
   - Would overwrite user customizations
   - Complex merge logic required
   - Deferred to Phase 4 (optional)

4. **Auto-fix by removing .py** (✅ **SELECTED**)
   - Zero user intervention required
   - Safe (backups created)
   - Idempotent (can run multiple times)
   - Surgical (only changes specific fields)

### Why Binary Name Check?

Before removing `.py`, we verify the binary exists to avoid breaking references:

```go
// Only fix if binary exists
withoutExt := strings.TrimSuffix(cmd, ".py")
expanded := expandHome(withoutExt)
if _, err := os.Stat(expanded); err == nil {
    // Safe to remove .py
    cmd = withoutExt
}
```

This prevents:
- Removing `.py` from legitimate Python scripts
- Breaking hook references if binary missing

## Implementation

### Health Check
```go
func (hc *HealthChecker) checkHookExtensionMatch() CheckResult {
    commands := hc.discoverConfiguredHookCommands()

    var mismatches []string
    for _, cmd := range commands {
        if strings.HasSuffix(cmd, ".py") {
            withoutExt := strings.TrimSuffix(cmd, ".py")
            expanded := expandHome(withoutExt)
            if _, err := os.Stat(expanded); err == nil {
                mismatches = append(mismatches, cmd)
            }
        }
    }

    if len(mismatches) > 0 {
        return CheckResult{
            Status: "warning",
            Message: fmt.Sprintf("Extension mismatches found: %d hook(s)", len(mismatches)),
            Fix: "engram doctor --auto-fix",
        }
    }

    return CheckResult{Status: "ok"}
}
```

### Auto-Fix
```go
func (f *Tier1Fixer) fixHookExtensionMismatches() error {
    settingsPath := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")

    // Fix settings.json
    settingsData, _ := os.ReadFile(settingsPath)
    if err := f.fixExtensionsInFile(settingsPath, settingsData); err != nil {
        return err
    }

    // Discover and fix plugin hooks.json files
    settingsData, _ = os.ReadFile(settingsPath) // Re-read after potential update
    for _, hooksFile := range discoverPluginHooksFiles(settingsData) {
        expanded := expandHome(hooksFile)
        data, _ := os.ReadFile(expanded)
        f.fixExtensionsInFile(expanded, data)
    }

    return nil
}

func (f *Tier1Fixer) fixExtensionsInFile(path string, data []byte) error {
    original := string(data)
    modified := original

    // Find all .py references and remove if binary exists
    commands := extractHookCommands(data)
    for _, cmd := range commands {
        if strings.HasSuffix(cmd, ".py") {
            withoutExt := strings.TrimSuffix(cmd, ".py")
            expanded := expandHome(withoutExt)
            if _, err := os.Stat(expanded); err == nil {
                // Binary exists - safe to remove .py
                modified = strings.ReplaceAll(modified, `"`+cmd+`"`, `"`+withoutExt+`"`)
            }
        }
    }

    // Only write if changes made
    if modified != original {
        createBackup(path)
        return os.WriteFile(path, []byte(modified), 0644)
    }

    return nil
}
```

## Consequences

### Positive
- ✅ **Zero-touch fix**: Users don't need to manually edit configs
- ✅ **Safe**: Backups created before modifications
- ✅ **Fast**: <100ms for typical configurations
- ✅ **Idempotent**: Running multiple times produces same result
- ✅ **Targeted**: Only modifies problematic fields
- ✅ **Multi-file**: Handles both settings.json and plugin hooks

### Negative
- ⚠️ **String replacement risk**: Simple string replace could affect comments/metadata
  - *Mitigation*: JSON structure preserved; only command strings modified
- ⚠️ **Binary name assumption**: Assumes Go binaries have no extension
  - *Mitigation*: Filesystem check before removal (only fix if binary exists)

### Neutral
- Settings.json remains valid JSON
- Existing hooks continue to work without changes
- Future hooks should be deployed without `.py` extension (documented)

## Testing

### Unit Test
**File**: `core/internal/health/fix_test.go`

```go
func TestFixHookExtensionMismatches(t *testing.T) {
    tmpDir := t.TempDir()

    // Create binary (no .py)
    hookPath := filepath.Join(tmpDir, ".claude/hooks/posttool-auto-commit-beads")
    os.WriteFile(hookPath, []byte("#!/bin/bash\necho test"), 0755)

    // Create settings referencing .py
    settings := `{"hooks":{"PostToolUse":[{"hooks":[{"command":"` + hookPath + `.py"}]}]}}`
    settingsPath := filepath.Join(tmpDir, ".claude/settings.json")
    os.WriteFile(settingsPath, []byte(settings), 0644)

    // Run fix
    fixer := NewTier1Fixer(tmpDir)
    fixer.fixHookExtensionMismatches()

    // Verify .py removed
    fixedData, _ := os.ReadFile(settingsPath)
    if strings.Contains(string(fixedData), ".py") {
        t.Error("Extension .py was not removed")
    }
}
```

### Integration Test
**File**: `core/test/integration/doctor_sandbox_test.go`

```go
func TestDoctorAutoFixInSandbox(t *testing.T) {
    tmpHome := t.TempDir()
    setupBrokenConfig(tmpHome) // Creates .py references

    engramBinary := buildEngramBinary(t)
    cmd := exec.Command(engramBinary, "doctor", "--auto-fix")
    cmd.Env = append(os.Environ(), "HOME="+tmpHome)
    cmd.Run()

    // Verify no .py extensions remain
    settingsData, _ := os.ReadFile(filepath.Join(tmpHome, ".claude/settings.json"))
    if strings.Contains(string(settingsData), ".py") {
        t.Error("Settings still contains .py extensions after fix")
    }
}
```

## Rollback

If this fix causes issues:

1. **Immediate**: Restore from backup
   ```bash
   cp ~/.claude/settings.json.bak ~/.claude/settings.json
   ```

2. **Permanent**: Revert code changes and rebuild
   ```bash
   git revert <commit-hash>
   make -C core build
   ```

## Future Work

1. **Hook deployment**: Update `hooks/deploy.sh` to warn if .py files deployed
2. **Validation**: Add pre-commit check to block .py extension in configs
3. **Documentation**: Update PRE-COMMIT-HOOKS.md with extension guidance

## References

- Issue: 19 hook errors on Claude Code startup
- Related: ADR-002 (Path Correction Strategy)
- Code: `core/internal/health/fix.go:fixHookExtensionMismatches()`
- Tests: `core/internal/health/fix_test.go:TestFixHookExtensionMismatches`
