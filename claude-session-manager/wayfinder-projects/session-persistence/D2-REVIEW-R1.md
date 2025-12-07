# D2 Architecture Review - Round 1

**Date**: December 7, 2025
**Document**: D2-ARCHITECTURE.md
**Review Type**: Multi-Persona Review

---

## Reviewer 1: Product Manager

**Perspective**: User value, feature completeness, roadmap alignment

### Strengths ✅

1. **Clear value proposition**: "Sessions never die" - love this!
2. **Excellent UX focus**: Auto-recreation is exactly what users want
3. **Context tracking solves real problem**: Users forget what sessions are for
4. **Phased approach**: Priorities are well-defined (Phase A/B before C/D)
5. **Workspace integration**: Configurable sessions-dir enables workspace architecture

### Concerns ⚠️

1. **Feature scope creep**: 18 implementation items feels heavy for one phase
   - Consider splitting into 2 smaller phases?
   - Phase A+B (infrastructure + resume) vs Phase C+D (context + lifecycle)

2. **Archive feature priority**: Is archive really Priority 2?
   - How often do users actually need this?
   - Could defer to Phase 4 or 5?

3. **Missing features from original ask**:
   - Session log backup for reference (mentioned in problem statement)
   - Search across session history
   - Should these be in scope or explicitly deferred?

4. **Migration risk**: "Lazy migration" on v1 → v2 sounds risky
   - What if write fails during migration?
   - Should we offer explicit `csm migrate` command?

### Questions ❓

1. What happens if user has 100+ sessions? Does `csm list` become slow?
2. Can users rename sessions (change tmux name)?
3. What's the story for sharing sessions between team members?

### Recommendation

**Score**: 8.0/10 - Good architecture, but scope feels large

**Suggestions**:
- Split into smaller phases (MVP = A+B only)
- Defer archive feature (Priority 3)
- Add explicit migration command
- Document performance at scale (100+ sessions)

---

## Reviewer 2: Software Architect

**Perspective**: System design, scalability, maintainability

### Strengths ✅

1. **Clean separation of concerns**: Storage layer is well-defined
2. **State machine is clear**: active → stopped → archived makes sense
3. **Batch status detection**: Smart optimization for `csm list`
4. **Schema versioning**: Forward-thinking to include version field
5. **Configuration hierarchy**: CLI > env > config > default is industry standard

### Concerns ⚠️

1. **Status field introduces inconsistency**:
   - Status can be stale (manifest says "active" but tmux is gone)
   - Architecture relies on "always check tmux on resume"
   - Why store status at all if we can't trust it?
   - **Alternative**: Compute status dynamically, don't store it

2. **Context struct is unbounded**:
   - `notes` field could be huge (user pastes novel)
   - No max length validation
   - Could bloat manifest file
   - **Recommendation**: Add size limits (notes < 1KB, purpose < 256 chars)

3. **Tmux detection is fragile**:
   - What if tmux binary not in PATH?
   - What if tmux server died but sessions exist?
   - Need more error handling

4. **Migration strategy lacks rollback**:
   - If v2 write fails, v1 manifest is lost
   - Should backup original manifest before migration
   - **Pattern**: `manifest.yaml` → `manifest.yaml.v1.bak` → write v2

5. **Directory structure unclear for workspace mode**:
   - If `sessions_dir = "$DEVLOG_ROOT/sessions"`, where do manifests go?
   - Same flat structure? Nested by project?
   - Need to clarify

### Architecture Improvements 🔧

**1. Don't store status - compute it dynamically:**

```go
// CURRENT (proposed)
type Manifest struct {
    Status string  // "active" | "stopped" | "archived"
    // ...
}

// BETTER
type Manifest struct {
    Lifecycle string  // "" | "archived"  (only store archived)
    // ...
}

func (m *Manifest) GetStatus() string {
    if m.Lifecycle == "archived" {
        return "archived"
    }
    if tmux.SessionExists(m.Tmux.SessionName) {
        return "active"
    }
    return "stopped"
}
```

**Benefits**:
- Status is always correct (no staleness)
- Simpler manifest
- Less to maintain

**2. Add validation layer:**

```go
type Context struct {
    Purpose string   `yaml:"purpose" validate:"max=256"`
    Tags    []string `yaml:"tags" validate:"max=10,dive,max=32"`
    Notes   string   `yaml:"notes" validate:"max=1024"`
}

func (c *Context) Validate() error {
    // Check size limits
    // Sanitize inputs
}
```

### Recommendation

**Score**: 7.5/10 - Solid design, but some architectural concerns

**Critical fixes**:
- Don't store status field (compute dynamically)
- Add validation for context fields
- Add migration rollback (backup original)

**Nice-to-haves**:
- Document workspace directory structure
- Add error handling for missing tmux binary

---

## Reviewer 3: QA Engineer

**Perspective**: Testability, edge cases, failure modes

### Test Coverage Analysis 📋

**What's testable**:
- ✅ Schema migration (v1 → v2)
- ✅ Status detection logic
- ✅ Tmux auto-recreation
- ✅ Config file loading
- ✅ Batch tmux detection

**What's hard to test**:
- ⚠️ Actual reboot scenario (need test fixtures)
- ⚠️ Tmux attach behavior (interactive)
- ⚠️ Race conditions (status updates)

### Edge Cases Not Covered 🐛

1. **Concurrent access**: Two terminals run `csm resume` simultaneously
   - Race condition on manifest write
   - Both try to create tmux session
   - **Fix**: Add file locking or atomic writes

2. **Partial failures**: Tmux created but Claude fails to start
   - Left with empty tmux session
   - Manifest says "active" but Claude isn't running
   - **Fix**: Rollback tmux if Claude fails

3. **Directory permissions**: Session dir not writable
   - Manifest write fails
   - Session created but not tracked
   - **Fix**: Check permissions early, better error message

4. **Symlink chaos**: Worktree is a symlink, gets deleted
   - Tmux can't cd to worktree
   - Session fails to recreate
   - **Fix**: Resolve symlinks before storing in manifest

5. **UUID collision**: Two sessions with same UUID
   - Claude might reuse UUIDs (unlikely but possible)
   - CSM creates duplicate manifests
   - **Fix**: Check for existing manifest before creating

6. **Archive → Resume**: User archives, then tries to resume
   - Should it unarchive automatically?
   - Or refuse to resume?
   - **Need**: Define behavior

### Test Strategy Gaps 📝

**Missing from architecture**:
- No mention of integration tests
- No test fixtures for tmux scenarios
- No performance benchmarks (100+ sessions claim)
- No migration test cases

### Recommendation

**Score**: 7.0/10 - Good coverage but missing critical edge cases

**Required additions**:
1. Add concurrency handling (file locking)
2. Add rollback on partial failures
3. Define archive → resume behavior
4. Add test strategy section to D3

**Nice-to-haves**:
- Document all edge cases
- Create test fixture library

---

## Reviewer 4: DevOps/SRE

**Perspective**: Operations, monitoring, failure recovery

### Operational Concerns 🔧

1. **No observability**:
   - Can't tell if auto-recreation is working
   - No metrics on how often sessions are stopped
   - No logging for debugging
   - **Fix**: Add optional debug logging

2. **No health checks**:
   - How do we know if CSM is working correctly?
   - Can't monitor session creation failures
   - **Fix**: Add `csm doctor` checks for session health

3. **No disaster recovery**:
   - What if all manifests are corrupted?
   - Can we rebuild from ~/.claude/ data?
   - **Fix**: Add `csm rebuild` command to recreate manifests from Claude data

4. **Configuration drift**:
   - User changes sessions-dir, old sessions are "lost"
   - No migration path for moving sessions
   - **Fix**: Add `csm config` command to show current settings

5. **Backup strategy unclear**:
   - Original requirement: "backup session logs for reference"
   - Architecture doesn't address this
   - Should sessions be backed up separately from ~/.claude/?

### Deployment Concerns 📦

1. **Config file location**: `~/.config/csm/config.yaml`
   - Will it be created automatically?
   - What if XDG_CONFIG_HOME is set?
   - **Fix**: Document config file discovery

2. **Version upgrades**: What if user upgrades CSM binary?
   - Old manifests incompatible with new CSM?
   - Need upgrade notes
   - **Fix**: Add version compatibility matrix

3. **Multi-user systems**: What if multiple users share ~/.claude/?
   - Race conditions on history.jsonl?
   - Permissions issues?
   - **Fix**: Document single-user assumption

### Monitoring & Debugging

**Missing from architecture**:
- No log output structure
- No debug mode (`--debug` flag)
- No status/health endpoint
- No metrics collection

**Recommended**:
```bash
csm debug                   # Show system status
csm debug --check-all       # Validate all sessions
csm debug --export          # Export data for bug reports
```

### Recommendation

**Score**: 7.5/10 - Good design but missing operational tooling

**Critical**:
- Add logging infrastructure
- Address backup strategy (from original requirements)
- Add disaster recovery plan

**Nice-to-have**:
- Health checks
- Debug commands
- Monitoring hooks

---

## Reviewer 5: End User / Developer

**Perspective**: Daily usage, UX, documentation

### UX Evaluation 😊

**What I love** ❤️:
1. `csm resume` auto-recreating tmux is AMAZING
2. Purpose field will help me remember what sessions are for
3. Status indicators in `csm list` are clear
4. Configurable directory is great for my workspace setup

**What confuses me** 🤔:

1. **Too many commands**: new, resume, list, archive, context, edit, info
   - Which do I use when?
   - Can we consolidate?
   - Maybe `csm set-context` instead of both `context` and `edit`?

2. **Archive vs Delete**: What's the difference?
   - If I archive, can I delete the manifest later?
   - Where does archived data live?
   - Do I need to clean up archives manually?

3. **Status field confusion**: Doc says status can be stale
   - So why show it in `csm list`?
   - Will it mislead me?
   - Better to just show "✓" if tmux exists (like currently)

4. **Migration anxiety**: "Lazy migration on write"
   - What if something goes wrong?
   - Will my sessions break?
   - Should I backup first?
   - **Need**: Clear migration guide

### Missing Documentation 📚

- No examples of full workflows
- No troubleshooting guide
- No FAQ for common issues
- No migration guide for existing users

### Feature Requests 🙏

1. **Session templates**: `csm new --template backend-dev`
   - Pre-fill purpose, tags, worktree
   - Save common configurations

2. **Session groups**: Organize related sessions
   - Tag-based grouping in `csm list`
   - `csm list --tag=feature-x`

3. **Quick switch**: `csm switch claude-2`
   - Resume without leaving current shell
   - Use tmux switch-client

4. **Session history**: What did I do in this session?
   - Show recent messages
   - `csm history claude-1`

### Recommendation

**Score**: 8.5/10 - Excellent UX, just needs polish

**Suggestions**:
- Consolidate `context` and `edit` into `csm set`
- Add migration guide to docs
- Add workflow examples
- Consider session templates (future)

---

## Aggregated Review Results

| Reviewer | Score | Key Concerns |
|----------|-------|--------------|
| Product Manager | 8.0/10 | Scope too large, missing log backup |
| Software Architect | 7.5/10 | Store lifecycle not status, add validation |
| QA Engineer | 7.0/10 | Missing edge cases, need concurrency handling |
| DevOps/SRE | 7.5/10 | No observability, missing backup strategy |
| End User | 8.5/10 | Too many commands, migration anxiety |

**Average Score**: 7.7/10 ❌ **BELOW THRESHOLD (8.5/10)**

---

## Critical Issues to Address

### Must Fix (Blocking approval)

1. **Don't store status field** (Architect)
   - Compute dynamically from tmux existence
   - Only store `lifecycle` field for archived state

2. **Add concurrency handling** (QA)
   - File locking for manifest writes
   - Prevent race conditions

3. **Add validation for context fields** (Architect)
   - Max sizes: purpose 256 chars, notes 1KB
   - Prevent manifest bloat

4. **Address backup strategy** (DevOps, PM)
   - Original requirement not addressed
   - Need to define: what gets backed up, when, where

5. **Add migration rollback** (Architect)
   - Backup manifest.yaml before migration
   - Recovery path if migration fails

### Should Fix (Recommended)

6. **Reduce scope** (PM)
   - Split Phase C+D into separate phase
   - Focus on A+B for this phase

7. **Add logging infrastructure** (DevOps)
   - Debug mode for troubleshooting
   - Log file location

8. **Consolidate commands** (User)
   - Merge `context` and `edit` into `csm set`
   - Reduce cognitive load

---

## Recommendations for Revision

### Architecture Changes

1. **Status field** → **Lifecycle field**:
```yaml
# BEFORE
status: "stopped"

# AFTER
lifecycle: ""  # "" = normal, "archived" = archived
# Status is computed: active (tmux exists) | stopped (tmux missing)
```

2. **Add validation**:
```go
type Context struct {
    Purpose string   `yaml:"purpose" validate:"max=256"`
    Tags    []string `yaml:"tags" validate:"max=10,dive,max=32"`
    Notes   string   `yaml:"notes" validate:"max=1024"`
}
```

3. **Add backup strategy section**:
- Define what gets backed up (logs? history entries?)
- When (on archive? on demand?)
- Where (session-dir? separate backup-dir?)

### Scope Changes

**Phase A+B** (Session Persistence Core):
1-9. Infrastructure + Resume enhancement
- Focus on getting auto-recreation working
- Defer context and archive features

**Phase C** (Context & Lifecycle) - Separate phase:
10-18. Context management + Archive

### Documentation Additions

- Migration guide for existing users
- Workflow examples
- Troubleshooting FAQ
- Edge case handling documentation

---

## Next Steps

1. Revise D2 architecture based on feedback
2. Address all "Must Fix" issues
3. Consider scope reduction (split phases)
4. Run Round 2 review
5. Target score: ≥8.5/10

**Status**: ❌ REVISION NEEDED - Round 2 Review Required
