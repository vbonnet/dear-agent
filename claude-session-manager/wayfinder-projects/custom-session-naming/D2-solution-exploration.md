# D2: Solution Exploration - Custom Session Naming Integration

**Date:** 2025-12-10
**Project:** Custom Session Naming Integration
**Status:** Discovery Phase 2

---

## Executive Summary

This document explores technical solutions for custom session naming in CSM, investigates Claude Code's session naming capabilities, and designs the integration approach.

**Key Findings:**
- Claude Code session naming research needed (environment variables, config options)
- CSM architecture already supports independent naming via manifest
- `/clear` command handling requires UUID change detection
- Implementation can proceed with or without Claude Code integration

---

## Investigation 1: Claude Code Session Naming

### Research Objectives

1. Determine if Claude Code supports custom session names (officially or unofficially)
2. Identify mechanism for passing session names to Claude (if supported)
3. Evaluate feasibility of Claude Code integration

### Investigation Methods

#### Method 1: Environment Variable Testing

**Hypothesis:** Claude Code may accept session name via environment variable

**Test Plan:**
```bash
# Test potential environment variables
export CLAUDE_SESSION_NAME="test-custom-name"
claude

# Check if name appears in:
# - Claude UI (if visible)
# - History file (~/.config/claude/code/history.jsonl)
# - Session metadata
```

**Variables to Test:**
- `CLAUDE_SESSION_NAME`
- `CLAUDE_SESSION_ID`
- `CLAUDE_SESSION_TITLE`
- `CLAUDE_NAME`
- `SESSION_NAME`

**Expected Outcomes:**
- **If supported:** Session name appears in Claude UI or history file
- **If not supported:** No change in behavior, auto-generated UUID used

#### Method 2: CLI Flag Testing

**Hypothesis:** Claude Code may support undocumented `--session-name` flag

**Test Plan:**
```bash
# Test potential CLI flags
claude --session-name "test-custom-name"
claude --name "test-custom-name"
claude -n "test-custom-name"

# Check help output
claude --help | grep -i session
claude --help | grep -i name
```

**Expected Outcomes:**
- **If supported:** Flag accepted, session named accordingly
- **If not supported:** Unknown flag error

#### Method 3: Config File Investigation

**Hypothesis:** Claude config may support session naming

**Test Plan:**
```bash
# Locate Claude config
ls -la ~/.config/claude/

# Check for session-related config
cat ~/.config/claude/code/config.json | jq .
grep -r "session" ~/.config/claude/
```

**Expected Outcomes:**
- **If supported:** Config option for session naming exists
- **If not supported:** No session naming configuration found

#### Method 4: History File Analysis

**Hypothesis:** Claude tracks sessions in history file with identifiable metadata

**Test Plan:**
```bash
# Examine history file structure
cat ~/.config/claude/code/history.jsonl | jq . | head -50

# Look for session metadata fields
cat ~/.config/claude/code/history.jsonl | jq '.session_id, .uuid, .name' | sort -u
```

**Expected Outcomes:**
- Understand Claude's session identification mechanism
- Identify fields that could be leveraged for custom naming

---

## Investigation 2: `/clear` Command Behavior

### Research Objectives

1. Understand what happens when user runs `/clear` in Claude session
2. Determine how to detect UUID changes
3. Design algorithm for updating manifest after `/clear`

### Test Procedure

#### Step 1: Baseline Session

```bash
# Create new CSM session
csm new --name "clear-test" ~/tmp/test-clear

# Send first message
# In Claude: "Hello, please respond"

# Sync to populate UUID
csm sync

# Record baseline UUID
cat ~/src/ws/sessions/session-clear-test/manifest.yaml | grep uuid
```

#### Step 2: Execute `/clear`

```bash
# In Claude session: /clear
# Observe Claude's response
# Note any visible changes

# Check history file
cat ~/.config/claude/code/history.jsonl | jq -r '.uuid' | tail -5
```

#### Step 3: Detect Changes

```bash
# Sync again
csm sync

# Compare UUIDs
# Old UUID vs New UUID in manifest
```

### Expected Behavior

**Before `/clear`:**
```yaml
claude:
  uuid: abc-123-original
```

**After `/clear`:**
```yaml
claude:
  uuid: xyz-789-new  # Changed!
```

**Problem:** CSM sync creates **new session entry** instead of updating existing one.

### Proposed Detection Algorithm

```go
// Pseudocode for CSM sync logic
func detectClearCommand(tmuxSession string, newUUID string) bool {
    manifest := loadManifestForTmuxSession(tmuxSession)

    if manifest.Claude.UUID != newUUID {
        // UUID changed - possible /clear

        // Check if old UUID still active
        oldActive := isClaudeSessionActive(manifest.Claude.UUID)
        newActive := isClaudeSessionActive(newUUID)

        if !oldActive && newActive {
            // Old session gone, new session present
            // High confidence this is /clear
            return true
        }
    }

    return false
}
```

**Decision Logic:**
1. If UUID changed **and** old UUID inactive **and** new UUID active → Update manifest
2. If UUID changed **and** old UUID still active → User opened new session, create new manifest
3. If UUID unchanged → Normal sync update

---

## Investigation 3: CSM Architecture Analysis

### Current Naming Flow

**File:** `cmd/csm/new.go`

```go
// Current flow
workDir := getWorkDir()
tmuxName := generateTmuxName(workDir, existingSessions)  // Auto-generated
tmux.NewSession(tmuxName, workDir)
manifest := createManifest(tmuxName)  // Uses same name
```

**Generated Name Pattern:**
- Base: `filepath.Base(workDir)`
- Sanitize: Remove invalid characters
- Prefix: `claude-`
- Suffix: Numeric if conflict (`-2`, `-3`)

### Proposed Naming Flow with `--name` Flag

```go
// Proposed flow
workDir := getWorkDir()
customName := getFlagString("name")  // NEW: User-provided name

var tmuxName string
if customName != "" {
    // User provided custom name
    tmuxName = validateAndSanitize(customName)
} else {
    // Auto-generate (backward compatible)
    tmuxName = generateTmuxName(workDir, existingSessions)
}

tmux.NewSession(tmuxName, workDir)
manifest := createManifest(tmuxName, customName != "")  // Track if custom
```

### Manifest Changes

**Current Schema (v2.0):**
```yaml
schema_version: "2.0"
session_id: claude-myproject-session
name: claude-myproject  # Display name
tmux:
  session_name: claude-myproject  # Tmux identifier
```

**Proposed Enhancement:**
```yaml
schema_version: "2.0"
session_id: feature-auth-session
name: feature-auth  # Custom name
tmux:
  session_name: feature-auth
context:
  custom_name: true  # NEW: Track if user-defined
  naming_strategy: "user-defined"  # or "auto-generated"
```

**Why track `custom_name`?**
- Prevents accidental overwrite during sync
- Allows different behavior for auto vs custom names
- Useful for future features (e.g., "regenerate name" command)

---

## Solution Design

### Design Option 1: CSM-Only Implementation

**Description:** CSM tracks custom names independently, Claude session UUID remains independent.

**Architecture:**
```
User: csm new --name "my-session"
  ↓
CSM: Validate name
  ↓
CSM: Create tmux session "my-session"
  ↓
CSM: Start Claude in tmux (UUID auto-generated by Anthropic)
  ↓
CSM: Create manifest with custom name
  ↓
User: csm sync (populates Claude UUID)
```

**Pros:**
- ✅ No dependency on Claude Code changes
- ✅ Immediate implementation possible
- ✅ Full control over naming logic
- ✅ Works regardless of Claude Code feature status

**Cons:**
- ❌ Claude UI may not show custom name (if it has its own naming)
- ❌ Name only visible in CSM/tmux, not Claude interface

**Verdict:** ✅ **Recommended as MVP** (minimal risk, immediate value)

---

### Design Option 2: Claude Code Integration (If Supported)

**Description:** Pass custom name to Claude Code via environment variable or flag.

**Architecture:**
```
User: csm new --name "my-session"
  ↓
CSM: Validate name
  ↓
CSM: Create tmux session "my-session"
  ↓
CSM: Set CLAUDE_SESSION_NAME="my-session"  # If supported
  ↓
CSM: Start Claude in tmux
  ↓
Claude: Uses custom name for session (if feature exists)
  ↓
CSM: Sync discovers session with matching name
```

**Pros:**
- ✅ Full synchronization across CSM, tmux, and Claude
- ✅ Best user experience
- ✅ Name visible in Claude UI (if supported)

**Cons:**
- ❌ Depends on Claude Code feature (may not exist)
- ❌ Blocks implementation if waiting for feature
- ❌ May require changes in future if Claude changes API

**Verdict:** ⏳ **Future Enhancement** (pending Claude Code investigation)

---

### Design Option 3: Hybrid Approach

**Description:** CSM implements custom naming, with optional Claude integration if detected.

**Architecture:**
```go
func startClaude(tmuxName string, customName string) {
    // Always set tmux session name
    tmux.NewSession(tmuxName, workDir)

    // Conditionally pass to Claude if supported
    if claudeSupportsCustomNames() {
        env := os.Environ()
        env = append(env, fmt.Sprintf("CLAUDE_SESSION_NAME=%s", customName))
        tmux.SendCommandWithEnv(tmuxName, "claude", env)
    } else {
        tmux.SendCommand(tmuxName, "claude")
    }

    // Manifest tracks custom name regardless
    createManifest(tmuxName, customName, customNameProvided)
}
```

**Pros:**
- ✅ Works today (CSM-only mode)
- ✅ Automatically leverages Claude support when available
- ✅ No user-facing API changes when Claude support added
- ✅ Graceful degradation

**Cons:**
- ⚠️ More complex implementation
- ⚠️ Requires feature detection logic

**Verdict:** ✅ **Recommended for Production** (best of both worlds)

---

## Technical Specification

### Name Validation Rules

**Allowed Characters:**
- Alphanumeric: `a-z`, `A-Z`, `0-9`
- Separators: `-` (hyphen), `_` (underscore)
- **NOT allowed:** Spaces, special chars (`!@#$%^&*()`), `/`, `\`

**Length Constraints:**
- Minimum: 1 character
- Maximum: 80 characters (tmux supports 256, but keep it reasonable)

**Reserved Names:**
- None initially (could add later: `default`, `temp`, etc.)

**Conflict Detection:**
- Must check **all** tmux sessions (not just CSM-managed)
- Error if name conflicts with existing session

**Validation Function:**
```go
func validateSessionName(name string) error {
    if len(name) == 0 {
        return errors.New("session name cannot be empty")
    }

    if len(name) > 80 {
        return errors.New("session name too long (max 80 characters)")
    }

    // Check allowed characters
    validChars := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
    if !validChars.MatchString(name) {
        return errors.New("session name contains invalid characters (use a-z, A-Z, 0-9, -, _)")
    }

    // Check for conflicts
    existingSessions, _ := tmux.ListSessions()
    for _, existing := range existingSessions {
        if existing == name {
            return fmt.Errorf("session name '%s' already exists", name)
        }
    }

    return nil
}
```

---

### `/clear` Handling Design

**Algorithm:**

```go
// In csm sync command
func syncSession(tmuxSession string) error {
    // Get current Claude UUID from history
    currentUUID := getCurrentClaudeUUID()

    // Load manifest for this tmux session
    manifest := loadManifestByTmuxName(tmuxSession)

    if manifest != nil {
        oldUUID := manifest.Claude.UUID

        if oldUUID != "" && oldUUID != currentUUID {
            // UUID changed - check if /clear scenario

            oldActive := isUUIDActive(oldUUID)
            newActive := isUUIDActive(currentUUID)

            if !oldActive && newActive {
                // Old session gone, new active
                // Update manifest with new UUID
                manifest.Claude.UUID = currentUUID
                manifest.UpdatedAt = time.Now()

                // Add to history notes
                if manifest.Context.Notes == "" {
                    manifest.Context.Notes = fmt.Sprintf("Session cleared at %s (UUID changed from %s to %s)",
                        time.Now().Format(time.RFC3339), oldUUID[:8], currentUUID[:8])
                }

                return saveManifest(manifest)
            }
        }
    }

    // Normal sync logic...
}
```

**User Workflow:**
```bash
# User in session "feature-auth"
User: /clear

# Claude creates new session with new UUID

# User runs sync
$ csm sync

# CSM output:
# ✓ Detected session clear in 'feature-auth'
# ✓ Updated manifest with new UUID (xyz-789)
# ✓ Preserved session name and context
```

---

### CLI Interface Design

#### `csm new --name` Command

**Usage:**
```bash
csm new --name <session-name> [directory]
```

**Examples:**
```bash
# Custom name with current directory
csm new --name "feature-user-auth"

# Custom name with specific directory
csm new --name "bug-fix-4532" ~/src/repos/myapp

# Auto-generated name (backward compatible)
csm new ~/src/repos/myapp  # Creates claude-myapp
```

**Flags:**
```
--name string    Custom session name (optional, auto-generated if not provided)
```

**Error Messages:**
```bash
# Invalid characters
$ csm new --name "my session"
Error: session name contains invalid characters (use a-z, A-Z, 0-9, -, _)

# Conflict
$ csm new --name "feature-auth"
Error: session name 'feature-auth' already exists
Use 'tmux list-sessions' to see existing sessions

# Too long
$ csm new --name "very-long-name-..."
Error: session name too long (max 80 characters)
```

---

#### `csm rename` Command (P1 Priority)

**Usage:**
```bash
csm rename <current-name> <new-name>
```

**Examples:**
```bash
# Rename existing session
csm rename claude-myapp feature-user-auth

# Error if source doesn't exist
csm rename nonexistent new-name
Error: session 'nonexistent' not found

# Error if target exists
csm rename feature-auth bug-fix
Error: session 'bug-fix' already exists
```

**Implementation:**
```go
func renameSession(oldName string, newName string) error {
    // Validate new name
    if err := validateSessionName(newName); err != nil {
        return err
    }

    // Load manifest
    manifest := loadManifestByTmuxName(oldName)
    if manifest == nil {
        return fmt.Errorf("session '%s' not found", oldName)
    }

    // Rename tmux session
    if err := tmux.RenameSession(oldName, newName); err != nil {
        return err
    }

    // Update manifest
    manifest.Name = newName
    manifest.Tmux.SessionName = newName
    manifest.UpdatedAt = time.Now()

    // Move manifest directory
    oldDir := getManifestDir(oldName)
    newDir := getManifestDir(newName)
    if err := os.Rename(oldDir, newDir); err != nil {
        // Rollback tmux rename
        tmux.RenameSession(newName, oldName)
        return err
    }

    // Save updated manifest
    return saveManifest(newDir, manifest)
}
```

---

## Implementation Roadmap

### Phase 1: CSM-Only Implementation (MVP)

**Goal:** Basic custom naming without Claude integration

**Tasks:**
1. Add `--name` flag to `csm new` command
2. Implement `validateSessionName()` function
3. Update manifest creation to track custom names
4. Test with various name inputs
5. Update documentation

**Deliverables:**
- `csm new --name` works
- Manifests track custom names
- `csm list` displays custom names
- `csm resume` works with custom names

**Effort:** ~2-3 hours

---

### Phase 2: `/clear` Handling

**Goal:** Gracefully handle UUID changes when user runs `/clear`

**Tasks:**
1. Implement UUID change detection in `csm sync`
2. Add logic to update manifest vs create new entry
3. Test with actual `/clear` commands
4. Add "session cleared" notes to manifest
5. Update documentation

**Deliverables:**
- `csm sync` detects `/clear`
- Manifest updated (not duplicated)
- Session name preserved after `/clear`

**Effort:** ~2 hours

---

### Phase 3: `csm rename` Command (P1)

**Goal:** Allow renaming existing sessions

**Tasks:**
1. Add `rename` subcommand
2. Implement atomic rename (tmux + manifest + directory)
3. Handle rollback on errors
4. Test edge cases (active sessions, conflicts)
5. Update documentation

**Deliverables:**
- `csm rename old new` works
- Atomic updates (all or nothing)
- Clear error messages

**Effort:** ~1.5 hours

---

### Phase 4: Claude Code Integration (If Supported)

**Goal:** Pass custom names to Claude if feature exists

**Tasks:**
1. Complete Claude Code investigation (D2 Method 1-4)
2. Implement feature detection
3. Add environment variable or flag passing
4. Test integration
5. Update documentation

**Deliverables:**
- CSM detects Claude naming support
- Passes custom name to Claude
- Synchronized naming across tools

**Effort:** ~2 hours (conditional on Claude support)

---

## Decision Matrix

### Implementation Approach

| Criteria | CSM-Only | Claude Integration | Hybrid |
|----------|----------|-------------------|--------|
| Immediate value | ✅ High | ❌ Blocked | ✅ High |
| Claude Code dependency | ✅ None | ❌ High | ⚠️ Optional |
| User experience | ⚠️ Good | ✅ Excellent | ✅ Excellent |
| Complexity | ✅ Low | ⚠️ Medium | ⚠️ Medium |
| Maintenance | ✅ Low | ⚠️ Medium | ⚠️ Medium |
| Future-proof | ⚠️ Medium | ⚠️ Low | ✅ High |

**Recommendation:** ✅ **Hybrid Approach**
- Implement CSM-only functionality first (Phase 1-3)
- Add Claude integration as enhancement (Phase 4 when confirmed)

---

## Risk Mitigation

### Risk 1: Claude Doesn't Support Custom Names

**Mitigation:**
- CSM provides value independently
- Naming at tmux/CSM level still improves UX
- No blocker for implementation

**Fallback Plan:**
- Implement CSM-only (Phases 1-3)
- Monitor Claude Code feature requests (#2112, #6006)
- Add integration when feature available

---

### Risk 2: `/clear` Detection Fails

**Mitigation:**
- Require user confirmation before updating UUID
- Provide `--force` flag for manual UUID update
- Document expected workflow after `/clear`

**Fallback Plan:**
```bash
# If auto-detection fails:
$ csm sync --force --uuid <new-uuid>
# Manually specify new UUID
```

---

### Risk 3: Name Conflicts

**Mitigation:**
- Check **all** tmux sessions (not just CSM)
- Clear error messages with suggestions
- Optional `--force` to kill conflicting session

**Example:**
```bash
$ csm new --name "existing-session"
Error: session 'existing-session' already exists

Suggestions:
  • Choose a different name: csm new --name "existing-session-2"
  • Kill existing session: tmux kill-session -t existing-session
  • Resume existing session: csm resume existing-session
```

---

## Next Steps (D3)

### Investigation Tasks

1. **Claude Code Session Naming Test**
   - Execute Method 1-4 from Investigation 1
   - Document results in D3
   - Decide on Claude integration feasibility

2. **`/clear` Behavior Test**
   - Execute test procedure from Investigation 2
   - Confirm UUID change detection works
   - Prototype sync update logic

3. **Prototype Development**
   - Create proof-of-concept for `--name` flag
   - Implement validation function
   - Test with edge cases

### Deliverables for D3

- **D3-requirements.md**: Detailed functional requirements
- **Proof-of-concept code:**
  - `csm new --name` flag parsing
  - Name validation
  - Manifest creation with custom name
- **Test results:** Claude Code investigation findings

---

## References

### CSM Source Files

- `cmd/csm/new.go` - Session creation
- `cmd/csm/sync.go` - Session synchronization
- `internal/manifest/manifest.go` - Manifest structure
- `internal/tmux/tmux.go` - Tmux operations

### Claude Code Files (To Investigate)

- `~/.config/claude/code/history.jsonl` - Session history
- `~/.config/claude/code/config.json` - Configuration
- Claude binary/help output

### External References

- GitHub Issue #2112: `--session-name` flag proposal
- GitHub Issue #6006: Session renaming feature request
- Tmux documentation: Session naming constraints

---

## Solution Exploration: ✅ **COMPLETE**

**Recommendation:**
- **Implement Hybrid Approach (CSM-only + Optional Claude integration)**
- **Priority: Phase 1-3 (CSM-only) → Phase 4 (Claude integration if confirmed)**
- **Next: D3-requirements.md (detailed functional specs)**

---

**Created:** 2025-12-10
**Author:** Claude Sonnet 4.5
**Status:** ✅ COMPLETE
