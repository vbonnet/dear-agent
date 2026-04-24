# Health Check & Auto-Fix System - Architecture

**Version**: 1.0
**Date**: 2026-03-14
**Status**: Implemented

## Overview

The health check system provides automated validation and repair of Claude Code configuration issues. This document describes the architecture, design patterns, and implementation details of the enhanced auto-fix capabilities.

## System Architecture

```
┌─────────────────────────────────────────────────────────┐
│                   engram doctor                          │
│                   (CLI Command)                          │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│              HealthChecker                               │
│         (Orchestrates checks)                            │
├─────────────────────────────────────────────────────────┤
│  • RunChecks()                                           │
│  • checkHookExtensionMatch()                             │
│  • checkHookPathsValid()                                 │
│  • checkMarketplaceConfigValid()                         │
│  • checkCoreEngramsAccessible()                          │
│  • discoverConfiguredHookCommands()                      │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│              Tier1Fixer                                  │
│         (Auto-fix implementation)                        │
├─────────────────────────────────────────────────────────┤
│  • FixAll()                                              │
│  • fixHookExtensionMismatches()                          │
│  • fixHookPaths()                                        │
│  • fixMarketplaceConfig()                                │
│  • removeNonExistentHooks()                              │
│  • fixExtensionsInFile()                                 │
│  • fixPathsInFile()                                      │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│           Configuration Files                            │
├─────────────────────────────────────────────────────────┤
│  • ~/.claude/settings.json                               │
│  • ~/.claude/plugins/*/hooks.json                        │
│  • ~/.claude/plugins/known_marketplaces.json             │
└─────────────────────────────────────────────────────────┘
```

## Core Components

### HealthChecker

**Purpose**: Validates configuration state and detects issues

**Key Responsibilities**:
1. Execute all health checks
2. Discover hook configurations across multiple files
3. Validate hook paths and extensions
4. Check marketplace configurations
5. Report issues with severity levels

**Design Pattern**: **Observer Pattern**
- Each check is independent
- Results collected and aggregated
- Checks can run in isolation for testing

**Key Methods**:

```go
type HealthChecker struct {
    workspace string
}

// RunChecks executes all health checks
func (hc *HealthChecker) RunChecks(interactive bool) []CheckResult

// Check hook extension matches
func (hc *HealthChecker) checkHookExtensionMatch() CheckResult

// Validate hook paths exist
func (hc *HealthChecker) checkHookPathsValid() CheckResult

// Validate marketplace config format
func (hc *HealthChecker) checkMarketplaceConfigValid() CheckResult

// Discover all configured hooks
func (hc *HealthChecker) discoverConfiguredHookCommands() []string
```

### Tier1Fixer

**Purpose**: Implements auto-fix logic for common configuration errors

**Key Responsibilities**:
1. Apply fixes with backup creation
2. Modify JSON configurations safely
3. Validate changes before writing
4. Ensure idempotency
5. Report fix actions

**Design Pattern**: **Command Pattern**
- Each fix is an independent operation
- Fixes can be composed
- Easy to add new fix types
- Rollback via backups

**Key Methods**:

```go
type Tier1Fixer struct {
    workspace string
}

// FixAll applies all tier 1 fixes
func (f *Tier1Fixer) FixAll() error

// Fix .py extension mismatches
func (f *Tier1Fixer) fixHookExtensionMismatches() error

// Fix hook path errors
func (f *Tier1Fixer) fixHookPaths() error

// Fix marketplace source format
func (f *Tier1Fixer) fixMarketplaceConfig() error

// Remove non-existent hooks
func (f *Tier1Fixer) removeNonExistentHooks() error

// Low-level: Fix extensions in specific file
func (f *Tier1Fixer) fixExtensionsInFile(path string, data []byte) error

// Low-level: Fix paths in specific file
func (f *Tier1Fixer) fixPathsInFile(path string, data []byte) error
```

## Data Flow

### Health Check Flow

```
1. User runs: engram doctor

2. HealthChecker.RunChecks()
   ↓
3. For each check:
   - Read config file(s)
   - Parse JSON
   - Extract relevant fields
   - Validate against rules
   - Return CheckResult
   ↓
4. Aggregate results
   ↓
5. Display summary
   - ✅ Passed checks
   - ⚠️  Warnings (auto-fixable)
   - ❌ Errors (critical)
```

### Auto-Fix Flow

```
1. User runs: engram doctor --auto-fix

2. Interactive prompt (if enabled)
   ↓
3. Tier1Fixer.FixAll()
   ↓
4. For each fix function:
   - Read config file
   - Parse JSON
   - Apply transformations
   - Create backup (.bak)
   - Write modified file
   - Log action
   ↓
5. Re-run health checks
   ↓
6. Display results
```

### Extension Fix Flow

```
fixHookExtensionMismatches():

1. Read settings.json
   ↓
2. Extract all hook commands
   ↓
3. For each command ending in .py:
   - Check if binary exists (without .py)
   - If yes: Remove .py from command
   ↓
4. Discover plugin hooks.json files
   ↓
5. For each plugin hooks file:
   - Apply same .py removal logic
   ↓
6. Write updated files (with backups)
```

### Path Correction Flow

```
fixHookPaths():

1. Read settings.json
   ↓
2. Extract all hook commands
   ↓
3. For each command:
   - Expand ~ to home directory
   - Check if file exists
   - If not: Try known path corrections
   - If corrected path exists: Replace
   ↓
4. Write updated settings (with backup)
```

## Configuration Discovery

### Multi-File Hook Discovery

The system discovers hook configurations across multiple files:

1. **Primary**: `~/.claude/settings.json`
   - Main hook configuration
   - May reference plugin hooks

2. **Plugin Hooks**: `~/.claude/plugins/*/hooks.json`
   - Discovered by parsing settings.json
   - Each plugin can have independent hooks

3. **Marketplace**: `~/.claude/plugins/known_marketplaces.json`
   - Separate validation logic
   - Critical: Invalid format crashes Claude Code

**Discovery Algorithm**:

```go
func discoverPluginHooksFiles(settingsData []byte) []string {
    // Parse settings.json
    var settings map[string]interface{}
    json.Unmarshal(settingsData, &settings)

    // Extract plugin hook file references
    hooks := settings["hooks"].(map[string]interface{})

    // Look for "hook_file" fields
    var hooksFiles []string
    for _, category := range hooks {
        // Nested traversal to find "hook_file" fields
        // ...
    }

    return hooksFiles
}
```

## Safety Mechanisms

### Backup Creation

**Timing**: Immediately before file write
**Format**: `{original-file}.bak`
**Overwrite**: Yes (reflects state before current fix)

```go
func createBackup(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        return err
    }
    return os.WriteFile(path+".bak", data, 0644)
}
```

### JSON Validation

**Before Write**:
```go
// Ensure valid JSON before writing
var testParse map[string]interface{}
if err := json.Unmarshal(modified, &testParse); err != nil {
    return fmt.Errorf("fix produced invalid JSON: %w", err)
}
```

### Idempotency

**Achieved via**:
1. **Conditional fixes**: Only modify if issue detected
2. **No-op on correct state**: Skip backup if no changes
3. **Repeated validation**: Checks pass after first fix

```go
// Example: Only create backup if changes made
if modified != original {
    createBackup(path)
    os.WriteFile(path, modified, 0644)
}
```

## Path Correction Strategy

### Known Mappings

The system maintains a table of known incorrect → correct path mappings:

```go
var pathCorrections = []struct {
    wrong   string
    correct string
}{
    {"/main/hooks/", "/hooks/"},
    {"~/.claude/hooks/sessionstart/", "~/.claude/hooks/session-start/"},
    // ... more mappings
}
```

### Correction Algorithm

```go
func (hc *HealthChecker) suggestPathCorrection(wrongPath string) string {
    for _, correction := range pathCorrections {
        if strings.Contains(wrongPath, correction.wrong) {
            candidate := strings.Replace(wrongPath, correction.wrong, correction.correct, 1)
            expanded := expandHome(candidate)
            if _, err := os.Stat(expanded); err == nil {
                return candidate // Corrected path exists
            }
        }
    }
    return "" // No correction found
}
```

## Marketplace Format Handling

### Multiple Valid Formats

The system accepts both formats for backward compatibility:

**Format 1: Direct path (fixed format)**
```json
{
  "engram": {
    "source": "/path/to/engram"
  }
}
```

**Format 2: Nested object (legacy, but valid if not "directory")**
```json
{
  "engram": {
    "source": {
      "source": "github",
      "owner": "...",
      "repo": "..."
    }
  }
}
```

**Invalid Format (WILL CRASH)**:
```json
{
  "engram": {
    "source": {
      "source": "directory",  // ← INVALID
      "path": "/path"
    }
  }
}
```

### Validation Logic

```go
switch sourceVal := source.(type) {
case string:
    // Direct path - valid
    if sourceVal == "" {
        return error("empty source")
    }
case map[string]interface{}:
    // Nested object - check source field
    nestedSource := sourceVal["source"].(string)
    switch nestedSource {
    case "github", "url":
        return ok
    case "directory":
        return error("invalid source=directory")
    }
}
```

## Error Handling

### Levels

1. **Info**: Informational messages (e.g., "No hooks configured")
2. **Warning**: Auto-fixable issues (e.g., "Extension mismatches found")
3. **Error**: Critical issues (e.g., "Marketplace will crash Claude Code")

### Reporting

```go
type CheckResult struct {
    Category string
    Status   string  // "ok", "warning", "error", "info"
    Message  string
    Details  string
    Fix      string  // Suggested fix command
}
```

### Exit Codes

- `0`: All checks passed
- `1`: Warnings (auto-fixable)
- `2`: Errors (critical)

## Testing Architecture

### Unit Test Structure

**Location**: `core/internal/health/*_test.go`

**Pattern**: Table-driven tests with sandboxed environments

```go
func TestFixHookExtensionMismatches(t *testing.T) {
    // Create isolated temp directory
    tmpDir := t.TempDir()

    // Setup broken config
    setupBrokenConfig(tmpDir)

    // Run fix
    fixer := NewTier1Fixer(tmpDir)
    fixer.fixHookExtensionMismatches()

    // Verify fix applied
    assertValidConfig(t, tmpDir)
    assertBackupCreated(t, tmpDir)
}
```

### Integration Test Structure

**Location**: `core/test/integration/doctor_sandbox_test.go`

**Pattern**: End-to-end tests with isolated `HOME` directories

```go
func TestDoctorAutoFixInSandbox(t *testing.T) {
    // Create isolated HOME
    tmpHome := t.TempDir()

    // Setup broken config
    setupBrokenConfig(tmpHome)

    // Build engram binary
    binary := buildEngramBinary(t)

    // Run doctor with HOME override
    cmd := exec.Command(binary, "doctor", "--auto-fix")
    cmd.Env = append(os.Environ(), "HOME="+tmpHome)
    cmd.Run()

    // Verify all fixes applied
    assertValidConfig(t, tmpHome)
}
```

## Performance Characteristics

### Time Complexity

- **Health Checks**: O(n) where n = number of configured hooks
- **Extension Fix**: O(n) where n = hook commands
- **Path Fix**: O(n × m) where m = number of known corrections
- **Marketplace Fix**: O(k) where k = number of marketplaces

### Space Complexity

- **In-memory**: O(n) for JSON parsing
- **Disk**: O(1) for backups (one backup per file)

### Typical Performance

- **Settings.json** (50 hooks): ~10ms
- **Marketplace config** (5 entries): ~2ms
- **Total runtime**: <100ms (including file I/O)

## Extension Points

### Adding New Checks

1. Add check method to `HealthChecker`:
   ```go
   func (hc *HealthChecker) checkNewValidation() CheckResult {
       // Validation logic
   }
   ```

2. Register in `RunChecks()`:
   ```go
   checks = append(checks, hc.checkNewValidation())
   ```

### Adding New Fixes

1. Add fix method to `Tier1Fixer`:
   ```go
   func (f *Tier1Fixer) fixNewIssue() error {
       // Fix logic with backup
   }
   ```

2. Register in `FixAll()`:
   ```go
   if err := f.fixNewIssue(); err != nil {
       return err
   }
   ```

## Dependencies

### Internal
- `internal/config`: Configuration management
- `pkg/progress`: Progress reporting

### External
- `encoding/json`: JSON parsing/serialization
- `os`: File I/O
- `path/filepath`: Path manipulation

## Future Architecture

### Planned Enhancements

1. **Plugin System**: Allow third-party checks/fixes
2. **Async Checks**: Parallel execution for performance
3. **Fix Transactions**: All-or-nothing fix application
4. **Telemetry**: Track common issues across users
5. **Auto-discovery**: Find hooks without config references

## References

- SPEC.md: Functional specification
- ADR-001: Hook Extension Fix Strategy
- ADR-002: Path Correction Mappings
- ADR-003: Marketplace Source Format
