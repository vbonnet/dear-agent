# Health Check & Auto-Fix System - Specification

**Version**: 1.0
**Date**: 2026-03-14
**Status**: Implemented

## Overview

The health check and auto-fix system (`engram doctor`) validates Claude Code hook configurations and automatically repairs common configuration errors. This spec defines the enhanced auto-fix capabilities added to resolve hook extension mismatches, path errors, and marketplace configuration issues.

## Problem Statement

Claude Code startup was failing with 19+ hook errors due to:
1. **Extension mismatches**: Settings reference `.py` files but hooks are Go binaries
2. **Invalid paths**: Wrong hook paths from repository restructuring
3. **Marketplace crashes**: `source="directory"` format crashes Claude Code
4. **Missing hooks**: References to non-existent/disabled hooks

## Goals

1. **Zero-touch fixes**: Automatically repair all common configuration errors
2. **Safety**: Always create backups before modifications
3. **Idempotency**: Running doctor multiple times produces same result
4. **Validation**: Health checks detect issues; auto-fix resolves them

## Scope

### In Scope
- Extension mismatch detection and removal (`.py` → binary)
- Hook path validation and correction (known path mappings)
- Marketplace configuration validation and conversion
- Removal of non-existent hook references
- Backup creation for all file modifications

### Out of Scope
- Hook installation/deployment
- Settings.json generation from scratch
- Custom path corrections (only known mappings)
- Hook execution validation

## Health Checks

### Check: Hook Extension Match
**Function**: `checkHookExtensionMatch()`
**Purpose**: Detect when settings reference `.py` files but binary exists without extension

**Status Levels**:
- `ok`: All hook commands match actual file names
- `warning`: Extension mismatches found (auto-fixable)

**Example Issue**:
```json
// settings.json references:
"command": "~/.claude/hooks/posttool-auto-commit-beads.py"

// But file system has:
~/.claude/hooks/posttool-auto-commit-beads (binary, no .py)
```

### Check: Hook Paths Valid
**Function**: `checkHookPathsValid()`
**Purpose**: Validate all hook paths exist; suggest corrections for known wrong paths

**Status Levels**:
- `ok`: All hook paths exist
- `warning`: Paths need correction OR paths missing (with suggestions)

**Known Path Corrections**:
```
/main/hooks/              → /hooks/
/src/ws/oss/.claude/      → /src/ws/oss/repos/engram-research/.claude/
~/.claude/hooks/sessionstart/ → ~/.claude/hooks/session-start/
```

### Check: Marketplace Config Valid
**Function**: `checkMarketplaceConfigValid()`
**Purpose**: Validate marketplace configurations don't use `source="directory"` (crashes Claude Code)

**Status Levels**:
- `ok`: No marketplace config OR all marketplaces valid
- `error`: Invalid `source="directory"` entries found (CRITICAL)

**Invalid Format**:
```json
{
  "engram": {
    "source": {
      "source": "directory",
      "path": "/path/to/engram"
    }
  }
}
```

**Valid Format**:
```json
{
  "engram": {
    "source": "/path/to/engram"
  }
}
```

## Auto-Fix Functions

### Fix: Extension Mismatches
**Function**: `fixHookExtensionMismatches()`
**Scope**: `settings.json` + discovered `hooks.json` files

**Algorithm**:
1. Read `settings.json`
2. Extract all hook commands with `.py` extension
3. For each `.py` reference:
   - Check if file without `.py` exists
   - If yes: Remove `.py` extension from settings
4. Discover plugin `hooks.json` files (from settings)
5. Apply same fix to each plugin hooks file
6. Create backup (`.bak`) before writing

**Example Transformation**:
```json
// Before:
"command": "~/.claude/hooks/posttool-auto-commit-beads.py"

// After (if binary exists):
"command": "~/.claude/hooks/posttool-auto-commit-beads"
```

### Fix: Hook Paths
**Function**: `fixHookPaths()`
**Scope**: `settings.json`

**Algorithm**:
1. Extract all hook commands from settings
2. For each command:
   - Check if file exists
   - If not, try known path corrections
   - If corrected path exists, replace
3. Create backup before writing

**Path Correction Table**:
| Wrong Pattern | Correct Pattern |
|--------------|-----------------|
| `/main/hooks/` | `/hooks/` |
| `~/.claude/hooks/sessionstart/` | `~/.claude/hooks/session-start/` |
| `/src/ws/oss/.claude/` | `/src/ws/oss/repos/engram-research/.claude/` |

### Fix: Marketplace Config
**Function**: `fixMarketplaceConfig()`
**Scope**: `~/.claude/plugins/known_marketplaces.json`

**Algorithm**:
1. Read marketplace config
2. Parse JSON
3. For each marketplace entry:
   - If `source` is object with `source="directory"`:
     - Extract `path` field
     - Replace object with direct path string
4. Create backup before writing

**Transformation**:
```json
// Before:
{
  "engram": {
    "source": {"source": "directory", "path": "/path"}
  }
}

// After:
{
  "engram": {
    "source": "/path"
  }
}
```

### Fix: Remove Non-Existent Hooks
**Function**: `removeNonExistentHooks()`
**Scope**: `settings.json`

**Algorithm**:
1. Read settings.json
2. Extract all hook entries
3. For each hook command:
   - Expand home directory (`~`)
   - Check if file exists
   - If not: Remove hook entry
4. Create backup before writing

**Filtering Rules**:
- Keep: Hooks that exist on filesystem
- Remove: Hooks that don't exist (file not found)
- Remove: Hooks ending in `.disabled`

## Safety Guarantees

### Backups
- **Format**: `{original-file}.bak`
- **Timing**: Created immediately before write
- **Overwrite**: Yes (backup reflects state before current fix)

### Idempotency
- Running auto-fix twice produces identical result
- No backups created if no changes needed
- Second run reports "No fixable issues found"

### Validation
- JSON validity checked before and after modifications
- File existence verified before path corrections
- Changes logged with specific actions taken

## Testing Strategy

### Unit Tests
**Location**: `core/internal/health/*_test.go`

**Coverage**:
- `TestFixHookExtensionMismatches`: Extension removal validation
- `TestFixHookPaths`: Path correction validation
- `TestFixMarketplaceConfig`: Marketplace format conversion
- `TestRemoveNonExistentHooks`: Missing hook removal
- `TestFixExtensionsNoChanges`: Idempotency validation

**Sandboxing**: All tests use `t.TempDir()` for isolation

### Integration Tests
**Location**: `core/test/integration/doctor_sandbox_test.go`

**Coverage**:
- `TestDoctorAutoFixInSandbox`: End-to-end fix workflow
- `TestFreshInstallInSandbox`: Clean install scenario
- `TestDoctorIdempotent`: Verify repeated runs produce same result

**Sandboxing**: All tests use isolated `HOME` directories

## Usage

### Command
```bash
engram doctor --auto-fix
```

### Interactive Mode
```
ℹ Preview of fixes to apply:
  ✓ Remove references to missing/disabled hook files
  ✓ Update hooks.json to match actual binary names
  ✓ Remove invalid entries, back up original

Apply these fixes? (Y/n):
```

### Output
```
Applying fixes...
✓ Applied 5 fixes successfully!

Hooks:
  ✅ hook_extension_match
  ✅ hook_paths_valid

Marketplace:
  ✅ 1 marketplace(s) configured
```

## Performance

- **Typical runtime**: <100ms (including backup creation)
- **File I/O**: Minimal (read → validate → write pattern)
- **Memory**: Low (streaming JSON parsing)

## Success Criteria

1. ✅ Zero Claude Code startup hook errors after running doctor
2. ✅ All tests pass (unit + integration)
3. ✅ Idempotent: Multiple runs produce same result
4. ✅ Safe: Backups created before all modifications
5. ✅ Fast: Completes in <1 second for typical configs

## Future Enhancements

1. **Settings generation**: Auto-generate `settings.json` on fresh install
2. **Hook validation**: Verify hooks are executable
3. **Path discovery**: Auto-discover hooks in common locations
4. **Custom corrections**: User-defined path mappings in config

## References

- ADR-001: Hook Extension Fix Strategy
- ADR-002: Path Correction Mappings
- ADR-003: Marketplace Source Format
- `/docs/PRE-COMMIT-HOOKS.md`: Hook deployment guide
- `/hooks/DEPLOYMENT-CHECKLIST.md`: Hook installation checklist
