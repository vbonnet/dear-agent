# ADR-003: Interactive Workspace Selection for Ambiguous Matches

**Status**: Accepted
**Date**: 2026-02-18
**Deciders**: Engineering team
**Context**: Workspace detection UX design

---

## Context and Problem Statement

When implementing workspace detection for `agm session new`, we encounter scenarios where the current directory matches multiple workspace roots:

**Example**:
```
Workspaces configured:
- oss:    ~/projects/myworkspace
- acme: ~/src/ws/acme

Current directory: .
                          ↓
              Could match both:
              - oss workspace (directory is inside ~/projects/myworkspace)
              - Could be nested workspace in future
```

The core question: **How should AGM handle ambiguous workspace matches?**

---

## Decision Drivers

1. **User control**: Users should control which workspace is selected
2. **Zero-touch UX**: Single match should auto-select (no user input)
3. **Predictability**: Behavior should be consistent and understandable
4. **Error prevention**: Wrong workspace selection can cause data misplacement
5. **UX consistency**: Follow existing AGM patterns (e.g., `--agent` flag)

---

## Considered Options

### Option 1: Auto-Select First Match

**Implementation**:
```go
func detectWorkspace(cfg *config.AGMConfig, currentDir string) error {
    matches, err := detector.Detect(currentDir)
    if err != nil || len(matches) == 0 {
        return nil  // No match, use default
    }

    // Always use first match (NO INTERACTION)
    cfg.Workspace = matches[0].Name
    cfg.SessionsDir = matches[0].SessionsDir
    return nil
}
```

**Pros**:
- Simple implementation
- No user interaction required
- Fast (no prompts)

**Cons**:
- **Wrong workspace selection**: User has no control
- **Unpredictable**: Order of matches may be arbitrary
- **Error-prone**: Auto-selection can cause data misplacement
- **No user feedback**: User doesn't know workspace was selected

**Risk**: HIGH - Wrong workspace selection is a critical error

---

### Option 2: Interactive Selection for Multiple Matches ✅

**Implementation**:
```go
func detectWorkspace(cfg *config.AGMConfig, currentDir string) error {
    matches, err := detector.Detect(currentDir)
    if err != nil || len(matches) == 0 {
        return nil  // No match, use default
    }

    // Single match: Auto-select (zero-touch UX)
    if len(matches) == 1 {
        cfg.Workspace = matches[0].Name
        cfg.SessionsDir = matches[0].SessionsDir
        return nil
    }

    // Multiple matches: Interactive selection
    fmt.Println("Multiple workspaces matched current directory:")
    for i, ws := range matches {
        fmt.Printf("  %d) %s (%s)\n", i+1, ws.Name, ws.Root)
    }
    fmt.Print("Select workspace (or 0 for no workspace): ")

    var choice int
    fmt.Scanf("%d", &choice)

    if choice == 0 || choice > len(matches) {
        return nil  // User chose no workspace
    }

    ws := matches[choice-1]
    cfg.Workspace = ws.Name
    cfg.SessionsDir = ws.SessionsDir
    return nil
}
```

**Pros**:
- **User control**: User explicitly selects workspace
- **Zero-touch for common case**: Single match auto-selects
- **Error prevention**: User confirms ambiguous matches
- **Clear feedback**: User sees which workspaces matched

**Cons**:
- Adds interactive prompt (slows UX)
- Doesn't work in scripts (non-interactive)

**Risk**: LOW - Interactive selection prevents wrong choices

---

### Option 3: Precedence Rules (Most Specific Match)

**Implementation**:
```go
func detectWorkspace(cfg *config.AGMConfig, currentDir string) error {
    matches, err := detector.Detect(currentDir)
    if err != nil || len(matches) == 0 {
        return nil  // No match, use default
    }

    // Sort by path length (most specific first)
    sort.Slice(matches, func(i, j int) bool {
        return len(matches[i].Root) > len(matches[j].Root)
    })

    // Auto-select most specific match
    cfg.Workspace = matches[0].Name
    cfg.SessionsDir = matches[0].SessionsDir

    // Log selected workspace for transparency
    fmt.Printf("Workspace auto-selected: %s\n", matches[0].Name)
    return nil
}
```

**Pros**:
- Deterministic (always selects most specific)
- No user interaction required
- Fast (no prompts)

**Cons**:
- **Assumes most specific = correct**: Not always true
- **No user control**: User can't override auto-selection
- **Confusing**: User may not understand precedence rules
- **Hard to debug**: Why was workspace X selected over Y?

**Risk**: MEDIUM - Precedence rules may surprise users

---

### Option 4: Explicit Flag Required for Ambiguity

**Implementation**:
```go
func detectWorkspace(cfg *config.AGMConfig, currentDir string, workspaceFlag string) error {
    matches, err := detector.Detect(currentDir)
    if err != nil || len(matches) == 0 {
        return nil  // No match, use default
    }

    // Multiple matches: Require explicit flag
    if len(matches) > 1 && workspaceFlag == "" {
        return fmt.Errorf("multiple workspaces matched, specify --workspace flag")
    }

    // Single match or explicit flag: Use it
    if len(matches) == 1 {
        cfg.Workspace = matches[0].Name
    } else {
        cfg.Workspace = workspaceFlag
    }

    return nil
}
```

**Pros**:
- No ambiguity (user must specify)
- Works in scripts (non-interactive)
- Explicit is better than implicit

**Cons**:
- **Poor UX**: User must re-run command with flag
- **Error-prone**: User may not know which workspace to choose
- **Extra step**: Slows common workflow

**Risk**: MEDIUM - Poor UX for interactive use

---

## Decision Outcome

**Chosen option**: **Option 2 - Interactive Selection for Multiple Matches**

**Rationale**:
1. **User control**: User explicitly selects workspace for ambiguous cases
2. **Zero-touch UX**: Single match auto-selects (common case)
3. **Error prevention**: Interactive selection prevents wrong workspace
4. **UX consistency**: Follows `--agent` flag pattern (auto-detect + interactive)
5. **Transparency**: User sees which workspaces matched

**Implementation**:
- Single match: Auto-select workspace (zero user input)
- Multiple matches: Show numbered list, prompt for selection
- No match: Use default SessionsDir (no workspace)
- User can select "0" to skip workspace selection

---

## Consequences

### Positive

1. **User control**: User chooses workspace for ambiguous cases
2. **Zero-touch for common case**: Most users see no prompt
3. **Error prevention**: Interactive selection prevents wrong workspace
4. **Transparency**: User sees matched workspaces before selection
5. **Consistent UX**: Follows existing AGM patterns

### Negative

1. **Interactive prompt**: Adds latency for ambiguous cases
2. **Scripting**: Doesn't work in non-interactive scripts (need explicit flag)
3. **Complexity**: More code than auto-selection

### Neutral

1. **Edge case handling**: Ambiguous matches are rare (most users have 1-2 workspaces)
2. **Future enhancement**: Could add `--workspace=auto-first` to skip prompt

---

## Implementation Details

### Interactive Selection Flow

```
User runs: agm session new

1. Detect workspaces from current directory
   ↓
2. If 0 matches:
   → Use default (no workspace)
   → No prompt
   ↓
3. If 1 match:
   → Auto-select workspace
   → No prompt (zero-touch UX)
   ↓
4. If 2+ matches:
   → Show numbered list
   → Prompt user to select
   → User selects workspace (1-N) or 0 for none
   ↓
5. Continue with session creation
```

### Code Structure

```go
// cmd/agm/new.go
func runSessionNew(cmd *cobra.Command, args []string) error {
    // 1. Auto-detect workspace
    _ = detectWorkspace(&cfg, currentDir)

    // 2. If no workspace detected, check for multiple matches
    matches, _ := detector.Detect(currentDir)
    if len(matches) > 1 {
        // Interactive selection
        ws, err := promptWorkspaceSelection(matches)
        if err != nil {
            return err
        }
        if ws != nil {
            cfg.Workspace = ws.Name
            cfg.SessionsDir = ws.SessionsDir
        }
    }

    // 3. Continue with session creation
    // ...
}

func promptWorkspaceSelection(matches []workspace.Workspace) (*workspace.Workspace, error) {
    fmt.Println("Multiple workspaces matched current directory:")
    for i, ws := range matches {
        fmt.Printf("  %d) %s (%s)\n", i+1, ws.Name, ws.Root)
    }
    fmt.Print("Select workspace (or 0 for no workspace): ")

    var choice int
    _, err := fmt.Scanf("%d", &choice)
    if err != nil {
        return nil, fmt.Errorf("invalid input: %w", err)
    }

    if choice == 0 || choice > len(matches) {
        return nil, nil  // No workspace selected
    }

    return &matches[choice-1], nil
}
```

### UX Examples

**Example 1: Single match (zero-touch)**
```bash
$ cd .
$ agm session new
Session created: wise-meadow
Workspace: oss
# No prompt - auto-detected
```

**Example 2: Multiple matches (interactive)**
```bash
$ cd .
$ agm session new
Multiple workspaces matched current directory:
  1) oss (~/projects/myworkspace)
  2) ai-tools (./repos/ai-tools)
Select workspace (or 0 for no workspace): 1
Session created: wise-meadow
Workspace: oss
```

**Example 3: No match (default)**
```bash
$ cd ~/tmp/random-project
$ agm session new
Session created: wise-meadow
# No workspace (uses default SessionsDir)
```

---

## UX Consistency with --agent Flag

AGM's `--agent` flag uses similar pattern:

**Current behavior**:
```bash
# Auto-detect if unambiguous
agm session new
→ Detects Claude if only one Claude instance

# Interactive selection if ambiguous
agm session new
→ Shows list if multiple Claude instances
→ User selects from numbered list

# Explicit flag overrides detection
agm session new --agent=claude
→ Uses specified agent directly
```

**New workspace behavior** (consistent):
```bash
# Auto-detect if unambiguous
agm session new
→ Detects workspace if only one match

# Interactive selection if ambiguous
agm session new
→ Shows list if multiple workspace matches
→ User selects from numbered list

# Explicit flag overrides detection
agm session new --workspace=oss
→ Uses specified workspace directly
```

**Pattern**: Auto-detect → Interactive → Explicit flag

---

## Scripting Support

### Non-Interactive Use Cases

**Problem**: Interactive prompts block scripts

**Solution**: Use explicit `--workspace` flag in scripts

```bash
#!/bin/bash
# Script to create AGM sessions

# Explicit workspace (no prompt)
agm session new --workspace=oss

# Or disable workspace detection
agm session new --workspace=none
```

**Future enhancement**: Add `AGM_WORKSPACE` environment variable

```bash
export AGM_WORKSPACE=oss
agm session new  # Uses environment variable (no prompt)
```

---

## Validation

### Test Coverage

**Test case**: `TestDetectWorkspace_SingleMatch`
- Current directory matches one workspace
- Verifies auto-selection (no prompt)
- Ensures workspace set correctly

**Test case**: `TestDetectWorkspace_MultipleMatches`
- Current directory matches multiple workspaces
- Verifies interactive prompt shown
- Ensures user selection honored

**Test case**: `TestDetectWorkspace_NoMatch`
- Current directory doesn't match any workspace
- Verifies no prompt shown
- Ensures default SessionsDir used

---

## Future Enhancements

### 1. Auto-Selection Precedence Rules (Optional)

**Feature**: Add `--workspace=auto-first` flag to skip prompt

**Behavior**:
- Multiple matches: Auto-select first match (most specific)
- No user interaction
- Log selected workspace for transparency

**Use case**: Scripts that need workspace but don't want prompts

### 2. Environment Variable Override

**Feature**: `AGM_WORKSPACE` environment variable

**Behavior**:
```bash
export AGM_WORKSPACE=oss
agm session new  # Uses oss workspace (no detection, no prompt)
```

**Precedence**:
1. Explicit `--workspace` flag (highest priority)
2. `AGM_WORKSPACE` environment variable
3. Auto-detection + interactive selection
4. Default SessionsDir (no workspace)

### 3. Workspace Pinning

**Feature**: Pin current directory to workspace

**Behavior**:
```bash
cd .
agm workspace pin oss  # Creates .agm-workspace file
agm session new        # Uses pinned workspace (no detection)
```

**Use case**: Avoid repeated interactive selection for specific directories

---

## References

- **SPEC.md**: Interactive workspace detection section
- **ARCHITECTURE.md**: UX pattern documentation
- **workspace-detection.md**: User guide with selection examples
- **cmd/agm/new.go**: Interactive selection implementation

---

## Alternatives Considered

### Alternative 1: Always Require Explicit Flag

**Proposal**: Remove auto-detection, always require `--workspace` flag

**Rejected because**:
- Poor UX (extra flag for every command)
- Inconsistent with `--agent` pattern
- Zero-touch UX is important

### Alternative 2: Configuration File Setting

**Proposal**: Add `default_workspace` to config

**Rejected because**:
- Doesn't solve ambiguity (still need detection)
- Adds config complexity
- Users want context-aware selection, not global default

### Alternative 3: Directory-Specific Config (.agm-workspace)

**Proposal**: Create `.agm-workspace` file in each project directory

**Rejected because**:
- Requires creating files in every project
- Pollutes project directories
- Doesn't work for directories user doesn't own

---

## Related Decisions

- **ADR-001**: Non-fatal workspace detection (enables graceful degradation)
- **ADR-002**: Atomic config updates (used when user adds workspaces)

---

**Decision Date**: 2026-02-18
**Status**: Implemented in cmd/agm/new.go
