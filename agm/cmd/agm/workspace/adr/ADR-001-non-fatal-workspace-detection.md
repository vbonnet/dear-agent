# ADR-001: Non-Fatal Workspace Detection

**Status**: Accepted
**Date**: 2026-02-18
**Deciders**: Engineering team
**Context**: Workspace-aware session management implementation

---

## Context and Problem Statement

When implementing workspace detection for AGM session creation, we need to decide how to handle errors in workspace detection. Should workspace detection errors be:
1. **Fatal**: Stop session creation if workspace detection fails
2. **Non-fatal**: Warn user and fall back to default behavior

Workspace detection can fail for several reasons:
- Missing `~/.agm/config.yaml` file
- Invalid YAML syntax in config file
- No enabled workspaces configured
- Workspace directory doesn't exist
- Permissions issues reading config

The core question: **Should AGM refuse to create sessions when workspace detection fails, or gracefully degrade to default behavior?**

---

## Decision Drivers

1. **Reliability**: AGM must work even when workspace config is broken
2. **User experience**: Users should be able to create sessions without workspace features
3. **Backward compatibility**: Existing AGM installations don't have workspace config
4. **Error handling philosophy**: Fail gracefully vs fail fast

---

## Considered Options

### Option 1: Fatal Workspace Detection (Fail Fast)

**Implementation**:
```go
func detectWorkspace(cfg *config.AGMConfig, currentDir string) error {
    // Load config
    if err := loadWorkspaceConfig(); err != nil {
        return fmt.Errorf("failed to load workspace config: %w", err)
    }

    // Detect workspace
    ws, err := detector.Detect(currentDir)
    if err != nil {
        return fmt.Errorf("workspace detection failed: %w", err)
    }

    cfg.Workspace = ws.Name
    return nil
}

// In new.go:
if err := detectWorkspace(cfg, currentDir); err != nil {
    return err  // Session creation fails
}
```

**Pros**:
- Clear error messages to user
- Forces users to fix config issues
- No silent failures

**Cons**:
- AGM unusable if workspace config broken
- Breaking change for existing users
- No graceful degradation
- Users can't create sessions without workspace features

---

### Option 2: Non-Fatal Workspace Detection (Graceful Degradation) ✅

**Implementation**:
```go
func detectWorkspace(cfg *config.AGMConfig, currentDir string) error {
    // Load config
    if err := loadWorkspaceConfig(); err != nil {
        fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
        return nil  // Non-fatal, continue without workspace
    }

    // Detect workspace
    ws, err := detector.Detect(currentDir)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
        return nil  // Non-fatal, continue without workspace
    }

    cfg.Workspace = ws.Name
    cfg.SessionsDir = ws.SessionsDir
    return nil
}

// In new.go:
_ = detectWorkspace(cfg, currentDir)  // Always succeeds
// Continue with session creation
```

**Pros**:
- AGM works even if workspace config broken
- Backward compatible (no workspace config = default behavior)
- Users can opt-out of workspace features
- Graceful degradation
- Clear warnings logged to stderr

**Cons**:
- Silent failures if user expects workspace detection
- May hide config errors from users
- Requires careful warning messages

---

### Option 3: Hybrid Approach (Fatal for Explicit, Non-Fatal for Auto)

**Implementation**:
```go
func detectWorkspace(cfg *config.AGMConfig, currentDir string, explicit bool) error {
    // Load config
    if err := loadWorkspaceConfig(); err != nil {
        if explicit {
            return fmt.Errorf("failed to load workspace config: %w", err)
        }
        fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
        return nil
    }

    // Detect workspace
    ws, err := detector.Detect(currentDir)
    if err != nil {
        if explicit {
            return fmt.Errorf("workspace detection failed: %w", err)
        }
        fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
        return nil
    }

    cfg.Workspace = ws.Name
    return nil
}

// In new.go:
if workspaceFlag == "auto" || workspaceFlag == "" {
    _ = detectWorkspace(cfg, currentDir, false)  // Non-fatal
} else {
    if err := detectWorkspace(cfg, currentDir, true); err != nil {
        return err  // Fatal for explicit --workspace flag
    }
}
```

**Pros**:
- Best of both worlds
- Explicit workspace requests fail fast
- Auto-detection degrades gracefully

**Cons**:
- More complex implementation
- Inconsistent error handling
- Users may not understand why errors differ

---

## Decision Outcome

**Chosen option**: **Option 2 - Non-Fatal Workspace Detection (Graceful Degradation)**

**Rationale**:
1. **AGM must always work**: Users need to create sessions even if workspace config is broken
2. **Backward compatibility**: Existing AGM installations don't have workspace config files
3. **Opt-in features**: Workspace detection is an enhancement, not a requirement
4. **Graceful degradation**: Better UX to warn and continue than to fail

**Implementation**:
- `detectWorkspace()` returns `nil` (never propagates errors)
- Warnings logged to stderr with clear error messages
- Default SessionsDir used on detection failure
- Session creation always succeeds

---

## Consequences

### Positive

1. **Reliability**: AGM works even with broken workspace config
2. **Backward compatibility**: Existing users unaffected (no config file = default behavior)
3. **User experience**: Users can create sessions without fixing workspace config first
4. **Graceful degradation**: Feature degrades to default behavior on error
5. **Clear warnings**: Users see warnings but can continue working

### Negative

1. **Silent failures**: Users may not notice workspace detection failed
2. **Hidden errors**: Config errors only show as warnings (not blocking)
3. **Debugging**: May be harder to debug workspace config issues

### Neutral

1. **Warning messages**: Must be clear and actionable
2. **Documentation**: Users need docs explaining workspace detection behavior
3. **Future work**: Could add `--strict` flag for fatal workspace detection

---

## Mitigation Strategies

### 1. Clear Warning Messages

**Problem**: Users may not notice workspace detection failed

**Solution**: Use clear, actionable warning messages:
```
Warning: Failed to load workspace config: file not found
Using default sessions directory: ~/.agm/sessions
To enable workspace detection, create ~/.agm/config.yaml
```

### 2. Diagnostic Command

**Problem**: Hard to debug workspace config issues

**Solution**: Add diagnostic command (future enhancement):
```bash
agm workspace diagnose
# Shows:
# - Config file location and status
# - Workspace definitions
# - Current directory workspace match
# - Errors and warnings
```

### 3. Documentation

**Problem**: Users don't understand workspace detection behavior

**Solution**: Document in user guide:
- Workspace detection is optional (enhancement)
- Errors degrade to default behavior
- How to fix common config errors

---

## Implementation Notes

### Error Handling Pattern

```go
// Load workspace config
workspaceConfig, err := workspace.LoadConfig(configPath)
if err != nil {
    fmt.Fprintf(os.Stderr, "Warning: Failed to load workspace config: %v\n", err)
    fmt.Fprintf(os.Stderr, "Using default sessions directory: %s\n", cfg.SessionsDir)
    return nil  // Non-fatal
}

// Create detector
detector := workspace.NewDetector(workspaceConfig)
if detector == nil {
    fmt.Fprintf(os.Stderr, "Warning: No enabled workspaces configured\n")
    return nil  // Non-fatal
}

// Detect workspace
matches, err := detector.Detect(currentDir)
if err != nil {
    fmt.Fprintf(os.Stderr, "Warning: Workspace detection failed: %v\n", err)
    return nil  // Non-fatal
}
```

### Warning Message Guidelines

1. **Format**: `Warning: [What failed]: [Why it failed]`
2. **Actionable**: Suggest next steps or fix
3. **Informative**: Explain fallback behavior
4. **Concise**: One or two lines max

---

## Validation

### Test Coverage

**Test case**: `TestDetectWorkspace_MissingConfig`
- Verifies no config file doesn't block session creation
- Checks warning message logged
- Ensures default SessionsDir used

**Test case**: `TestDetectWorkspace_InvalidConfig`
- Verifies invalid YAML doesn't block session creation
- Checks warning message logged
- Ensures default SessionsDir used

**Test case**: `TestDetectWorkspace_NoEnabledWorkspaces`
- Verifies empty config doesn't block session creation
- Checks warning message logged
- Ensures default SessionsDir used

---

## References

- **SPEC.md**: Error handling section documents all failure scenarios
- **ARCHITECTURE.md**: Design decision section explains rationale
- **workspace-detection.md**: User guide with troubleshooting tips
- **workspace_test.go**: Test coverage for error scenarios

---

## Alternatives Considered

### Alternative 1: Fail Fast by Default, Opt-In to Graceful

**Proposal**: Make workspace detection fatal by default, add `--no-strict-workspace` flag to allow failures

**Rejected because**:
- Breaks backward compatibility
- Users would need to add flag to every command
- Poor UX for users without workspace config

### Alternative 2: Two-Stage Detection (Try-Warn-Ask)

**Proposal**:
1. Try workspace detection
2. If fails, warn user
3. Ask: "Continue without workspace? [Y/n]"

**Rejected because**:
- Adds interactive prompt (slows UX)
- Breaks scripting/automation
- No graceful degradation (still blocks on user input)

---

**Decision Date**: 2026-02-18
**Status**: Implemented and tested
