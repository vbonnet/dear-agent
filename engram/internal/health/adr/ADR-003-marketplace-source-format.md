# ADR-003: Marketplace Source Format Validation and Conversion

**Status**: Accepted
**Date**: 2026-03-14
**Deciders**: Claude Sonnet 4.5
**Context**: Phase 2 - Engram Doctor Auto-Fix Enhancement

## Context

Claude Code crashes on startup when `~/.claude/plugins/known_marketplaces.json` contains marketplace entries with `source="directory"`. This is a **CRITICAL** issue that prevents Claude Code from starting entirely.

**Root Cause**:
- Invalid marketplace configuration format
- Claude Code's marketplace loader doesn't handle `source="directory"`
- Error manifests as immediate crash (not graceful degradation)

**Invalid Configuration**:
```json
{
  "engram": {
    "source": {
      "source": "directory",
      "path": "./engram"
    },
    "installLocation": "./engram"
  }
}
```

**Error Message**:
```
Invalid marketplace entries — WILL CRASH CLAUDE CODE!
engram (source="directory")
```

## Decision

We will **automatically convert invalid marketplace source formats** to valid formats:

1. **Detect invalid**: `source="directory"` in nested source object
2. **Convert to valid**: Direct path string format
3. **Preserve valid**: GitHub/URL source formats and direct path strings

**Detection**: `checkMarketplaceConfigValid()` in `HealthChecker`
**Fix**: `fixMarketplaceConfig()` in `Tier1Fixer`

## Valid Marketplace Formats

### Format 1: Direct Path (Primary)
```json
{
  "engram": {
    "source": "./engram",
    "installLocation": "./engram"
  }
}
```
- **Use case**: Local filesystem marketplace
- **Field**: `source` is a **string**
- **Status**: ✅ Valid

### Format 2: GitHub Source (Legacy but Valid)
```json
{
  "anthropic": {
    "source": {
      "source": "github",
      "owner": "anthropics",
      "repo": "plugins"
    },
    "installLocation": "~/.claude/plugins"
  }
}
```
- **Use case**: Remote GitHub marketplace
- **Field**: `source` is an **object** with `source="github"`
- **Status**: ✅ Valid

### Format 3: URL Source (Legacy but Valid)
```json
{
  "custom": {
    "source": {
      "source": "url",
      "url": "https://example.com/marketplace.json"
    },
    "installLocation": "~/.claude/plugins/custom"
  }
}
```
- **Use case**: Remote URL marketplace
- **Field**: `source` is an **object** with `source="url"`
- **Status**: ✅ Valid

### Format 4: Directory Source (INVALID)
```json
{
  "engram": {
    "source": {
      "source": "directory",
      "path": "/path/to/marketplace"
    }
  }
}
```
- **Field**: `source` is an **object** with `source="directory"`
- **Status**: ❌ **INVALID** (crashes Claude Code)

## Rationale

### Why Auto-Convert?

**Alternatives Considered**:

1. **Remove invalid entry** (rejected)
   - Loses user's marketplace configuration
   - User must re-add manually
   - Disrupts workflow

2. **Warn user** (rejected)
   - User still can't start Claude Code
   - No actionable fix provided
   - Requires manual editing

3. **Auto-convert to valid format** (✅ **SELECTED**)
   - Preserves user's intent (local marketplace)
   - Fixes critical crash
   - Zero user intervention
   - Safe (backups created)

### Why Direct Path Format?

When converting `source="directory"`, we use direct path string format because:
1. **Simplicity**: Clearest representation of local filesystem source
2. **Compatibility**: Works across all Claude Code versions
3. **Intent preservation**: User wanted local marketplace (path format achieves this)

### Why Accept Multiple Formats?

We validate but accept both **string** and **object** source formats because:
1. **Backward compatibility**: Existing configs use GitHub/URL formats
2. **Claude Code support**: Official client supports multiple source types
3. **Future-proofing**: New source types may be added

## Implementation

### Validation Logic

```go
func (hc *HealthChecker) checkMarketplaceConfigValid() CheckResult {
    mktPath := filepath.Join(os.Getenv("HOME"), ".claude/plugins/known_marketplaces.json")
    data, err := os.ReadFile(mktPath)
    if os.IsNotExist(err) {
        return CheckResult{Status: "ok", Message: "No marketplace config (none installed)"}
    }

    var marketplaces map[string]interface{}
    if err := json.Unmarshal(data, &marketplaces); err != nil {
        return CheckResult{Status: "error", Message: "Marketplace config unparseable"}
    }

    var invalid []string

    for name, value := range marketplaces {
        entryMap, ok := value.(map[string]interface{})
        if !ok {
            continue
        }

        source, ok := entryMap["source"]
        if !ok {
            continue
        }

        switch sourceVal := source.(type) {
        case string:
            // Direct path format - valid
            if sourceVal == "" {
                invalid = append(invalid, fmt.Sprintf("%s (empty source)", name))
            }

        case map[string]interface{}:
            // Nested object format - check nested source field
            if nestedSource, ok := sourceVal["source"].(string); ok {
                switch nestedSource {
                case "github", "url":
                    // Valid source types
                    continue
                case "directory":
                    // INVALID - will crash Claude Code
                    invalid = append(invalid, fmt.Sprintf("%s (source=%q)", name, nestedSource))
                default:
                    invalid = append(invalid, fmt.Sprintf("%s (unknown source=%q)", name, nestedSource))
                }
            }

        default:
            invalid = append(invalid, fmt.Sprintf("%s (invalid source type: %T)", name, source))
        }
    }

    if len(invalid) > 0 {
        return CheckResult{
            Status: "error",
            Message: "Invalid marketplace entries — WILL CRASH CLAUDE CODE!",
            Details: strings.Join(invalid, ", "),
            Fix: "engram doctor --auto-fix",
        }
    }

    return CheckResult{
        Status: "ok",
        Message: fmt.Sprintf("%d marketplace(s) configured", len(marketplaces)),
    }
}
```

### Auto-Fix Logic

```go
func (f *Tier1Fixer) fixMarketplaceConfig() error {
    mktPath := filepath.Join(os.Getenv("HOME"), ".claude/plugins/known_marketplaces.json")
    data, err := os.ReadFile(mktPath)
    if os.IsNotExist(err) {
        return nil // No marketplace config
    }

    var marketplaces map[string]interface{}
    if err := json.Unmarshal(data, &marketplaces); err != nil {
        return err
    }

    modified := false

    for name, value := range marketplaces {
        entryMap, ok := value.(map[string]interface{})
        if !ok {
            continue
        }

        source, ok := entryMap["source"]
        if !ok {
            continue
        }

        // Check if source is object with source="directory"
        if sourceMap, ok := source.(map[string]interface{}); ok {
            if nestedSource, ok := sourceMap["source"].(string); ok && nestedSource == "directory" {
                // Extract path from object
                if path, ok := sourceMap["path"].(string); ok && path != "" {
                    // Convert to direct path format
                    entryMap["source"] = path
                    modified = true
                }
            }
        }
    }

    if modified {
        createBackup(mktPath)

        updatedData, err := json.MarshalIndent(marketplaces, "", "  ")
        if err != nil {
            return err
        }

        return os.WriteFile(mktPath, updatedData, 0644)
    }

    return nil
}
```

## Validation Rules

### Type Checking

**Source field can be**:
1. **String**: Direct path (valid)
2. **Object**: Must have `source` field with value `"github"` or `"url"` (valid)
3. **Object with `source="directory"`**: INVALID (auto-fixed)
4. **Other types**: Invalid (reported as error)

### Empty Path Handling

```go
if sourceVal == "" {
    return error("empty source")
}
```

Empty source paths are invalid and cannot be auto-fixed (user must provide valid path).

## Consequences

### Positive
- ✅ **Prevents crashes**: Critical fix for Claude Code startup
- ✅ **Preserves intent**: Converts to equivalent valid format
- ✅ **Backward compatible**: Accepts legacy GitHub/URL formats
- ✅ **Safe**: Backups created before modification
- ✅ **Clear error**: Reports exact invalid entries

### Negative
- ⚠️ **Assumes intent**: Converts `directory` to path string (might not be user's intent)
  - *Mitigation*: User can manually change back (with backup available)
- ⚠️ **Format migration**: Shifts from object to string format
  - *Mitigation*: Both formats supported by validation logic

### Neutral
- Multiple valid formats supported (string and object)
- Future source types can be added to validation logic
- Backup allows rollback if conversion unwanted

## Testing

### Unit Test

```go
func TestFixMarketplaceConfig(t *testing.T) {
    tmpDir := t.TempDir()

    // Create plugins directory
    pluginsDir := filepath.Join(tmpDir, ".claude/plugins")
    os.MkdirAll(pluginsDir, 0755)

    // Create invalid marketplace config
    invalid := map[string]interface{}{
        "engram": map[string]interface{}{
            "source": map[string]interface{}{
                "source": "directory",
                "path":   "~/src/engram",
            },
            "installLocation": "~/src/engram",
        },
    }

    mktPath := filepath.Join(pluginsDir, "known_marketplaces.json")
    invalidJSON, _ := json.MarshalIndent(invalid, "", "  ")
    os.WriteFile(mktPath, invalidJSON, 0644)

    // Set HOME for test
    os.Setenv("HOME", tmpDir)
    defer os.Setenv("HOME", os.Getenv("HOME"))

    // Run fix
    fixer := NewTier1Fixer(tmpDir)
    fixer.fixMarketplaceConfig()

    // Verify conversion
    fixedData, _ := os.ReadFile(mktPath)
    var fixed map[string]interface{}
    json.Unmarshal(fixedData, &fixed)

    engram := fixed["engram"].(map[string]interface{})
    source := engram["source"]

    // Source should now be a string, not an object
    if sourceStr, ok := source.(string); !ok {
        t.Errorf("Source is not a string after fix, type: %T", source)
    } else if sourceStr != "~/src/engram" {
        t.Errorf("Expected path string, got: %v", sourceStr)
    }
}
```

### Integration Test

```go
func TestDoctorAutoFixInSandbox(t *testing.T) {
    tmpHome := t.TempDir()

    // Setup broken marketplace config
    setupBrokenConfig(tmpHome)

    // Run doctor
    engramBinary := buildEngramBinary(t)
    cmd := exec.Command(engramBinary, "doctor", "--auto-fix")
    cmd.Env = append(os.Environ(), "HOME="+tmpHome)
    cmd.Run()

    // Verify marketplace valid
    assertValidMarketplace(t, tmpHome)

    // Verify no source="directory" remains
    mktData, _ := os.ReadFile(filepath.Join(tmpHome, ".claude/plugins/known_marketplaces.json"))
    if strings.Contains(string(mktData), `"source":"directory"`) {
        t.Error("Marketplace still contains source=\"directory\" after fix")
    }
}
```

## Error Severity

**Why ERROR instead of WARNING?**

This issue is marked as `error` (not `warning`) because:
1. **Crash severity**: Claude Code will not start
2. **User impact**: Complete blocker (no workaround)
3. **Data loss risk**: User cannot access Claude Code until fixed
4. **Priority**: Must be fixed before any other issues

**Display**:
```
Marketplace:
  ❌ Invalid marketplace entries — WILL CRASH CLAUDE CODE!
     engram (source="directory")
     Fix: engram doctor --auto-fix
```

## Rollback

If marketplace conversion causes issues:

1. **Immediate**: Restore from backup
   ```bash
   cp ~/.claude/plugins/known_marketplaces.json.bak ~/.claude/plugins/known_marketplaces.json
   ```

2. **Manual fix**: Edit marketplace config directly
   ```json
   {
     "engram": {
       "source": "/corrected/path/to/engram"
     }
   }
   ```

3. **Remove marketplace**: Delete config entirely
   ```bash
   rm ~/.claude/plugins/known_marketplaces.json
   ```

## Future Work

1. **Source type documentation**: Document all valid marketplace source formats
2. **Validation on marketplace add**: Prevent `source="directory"` at creation time
3. **Migration guide**: Help users update old configs manually
4. **Claude Code fix**: Submit patch to handle `directory` gracefully (if possible)

## References

- Issue: "WILL CRASH CLAUDE CODE!" error on startup
- Related: Health check system (checks.go)
- Code: `core/internal/health/fix.go:fixMarketplaceConfig()`
- Tests: `core/internal/health/fix_test.go:TestFixMarketplaceConfig`
- Claude Code: Marketplace loader implementation
