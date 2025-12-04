# S9 Enhancement Proposal - Project Root Environment Variable

**Date**: 2025-12-03
**Phase**: S9 (Enhancement - Optional)
**Proposed Feature**: WORKSPACE_PROJECT_ROOT environment variable support
**Status**: Awaiting Review Council Approval
**User Request**: "Option 2 works for me if the reviewers think it's the best option"

---

## Context

**Current Project Status:**
- S8: ✅ **COMPLETE** (100% requirements met, production-approved)
- All D4 Requirements: ✅ 100% complete
- D1 Goals: ✅ 90% complete (9/10)
- Quality: ✅ EXCELLENT
- Decision: ✅ **PROJECT COMPLETE**

**User Request:**
User wants ability to specify a project root directory (e.g., `~/my-project/`) instead of hardcoded `~/` so that all workspace directories live under that root:
```
~/my-project/worktrees/
~/my-project/sessions/
~/my-project/src/
```

---

## Is This a New Requirement or Enhancement?

**Analysis:**

**NOT a new requirement** because:
- Original D4 requirements are 100% complete
- This is a **configuration flexibility enhancement**
- Project already works perfectly with `~/` as root
- User specifically said this is optional ("if reviewers think it's best")

**Enhancement classification:**
- **Type**: Usability improvement
- **Scope**: Small (affects 4 scripts, ~20 lines total)
- **Risk**: Low (backward compatible)
- **Priority**: User-requested, but optional
- **Effort**: ~1-2 hours (implementation + testing + docs)

**Decision**: This is **S9 Enhancement** (optional post-project work), not a requirement gap.

---

## Proposed Solution (Approach 2)

### Feature Description

Add support for `WORKSPACE_PROJECT_ROOT` environment variable that sets the base directory for all workspace components.

**Current behavior:**
```bash
# Hardcoded defaults in scripts:
DEFAULT_SESSIONS_BASE="$HOME/sessions"
DEFAULT_WORKTREES_BASE="$HOME/worktrees"  # In migrate-workspace.sh
DEFAULT_SRC_BASE="$HOME/src"
```

**Proposed behavior:**
```bash
# Defaults respect environment variable if set:
DEFAULT_SESSIONS_BASE="${WORKSPACE_PROJECT_ROOT:-$HOME}/sessions"
DEFAULT_WORKTREES_BASE="${WORKSPACE_PROJECT_ROOT:-$HOME}/worktrees"
DEFAULT_SRC_BASE="${WORKSPACE_PROJECT_ROOT:-$HOME}/src"
```

**User workflow:**
```bash
# One-time setup in ~/.bashrc or ~/.zshrc:
export WORKSPACE_PROJECT_ROOT=~/my-project

# All scripts automatically use that root:
./bin/migrate-workspace.sh          # Uses ~/my-project/worktrees, ~/my-project/sessions
./bin/resume-session.sh --list      # Uses ~/my-project/sessions
./bin/archive-session.sh SESSION    # Uses ~/my-project/sessions
./bin/session-dashboard.sh          # Uses ~/my-project/sessions
```

**Precedence (highest to lowest):**
1. Command-line flags (`--sessions-base`, `--base`) - highest priority
2. Environment variable (`WORKSPACE_PROJECT_ROOT`)
3. Default (`~/`) - lowest priority

---

## Implementation Plan

### Changes Required

**1. Update 4 scripts (20 lines total):**

**migrate-workspace.sh:**
```bash
# Before:
readonly DEFAULT_WORKTREES_BASE="$HOME/worktrees"
readonly DEFAULT_SESSIONS_BASE="$HOME/sessions"

# After:
readonly DEFAULT_WORKTREES_BASE="${WORKSPACE_PROJECT_ROOT:-$HOME}/worktrees"
readonly DEFAULT_SESSIONS_BASE="${WORKSPACE_PROJECT_ROOT:-$HOME}/sessions"
```

**resume-session.sh:**
```bash
# Before:
readonly DEFAULT_SESSIONS_BASE="$HOME/sessions"

# After:
readonly DEFAULT_SESSIONS_BASE="${WORKSPACE_PROJECT_ROOT:-$HOME}/sessions"
```

**archive-session.sh:**
```bash
# Before:
readonly DEFAULT_SESSIONS_BASE="$HOME/sessions"

# After:
readonly DEFAULT_SESSIONS_BASE="${WORKSPACE_PROJECT_ROOT:-$HOME}/sessions"
```

**session-dashboard.sh:**
```bash
# Before:
readonly DEFAULT_SESSIONS_BASE="$HOME/sessions"

# After:
readonly DEFAULT_SESSIONS_BASE="${WORKSPACE_PROJECT_ROOT:-$HOME}/sessions"
```

**2. Update help text (4 scripts):**

Add to each help message:
```
Environment Variables:
  WORKSPACE_PROJECT_ROOT    Base directory for all workspace components
                            (default: ~)
```

**3. Update USER-GUIDE.md:**

Add new section under "Advanced Usage":
```markdown
### Using Custom Project Root

Set a custom base directory for all workspace components:

# In ~/.bashrc or ~/.zshrc:
export WORKSPACE_PROJECT_ROOT=~/my-project

# All components now live under ~/my-project/:
#   ~/my-project/worktrees/
#   ~/my-project/sessions/
#   ~/my-project/src/

# Precedence:
1. Command-line flags (--sessions-base, --base)
2. WORKSPACE_PROJECT_ROOT environment variable
3. Default (~/)
```

**4. Add BATS tests (3 new tests):**

```bash
@test "env var: WORKSPACE_PROJECT_ROOT sets default sessions base" {
  export WORKSPACE_PROJECT_ROOT="$TEST_DIR/custom-root"
  mkdir -p "$WORKSPACE_PROJECT_ROOT/sessions"
  create_test_session "test" "active"

  run "$BIN_DIR/resume-session.sh" --list
  [ "$status" -eq 0 ]
}

@test "env var: command-line flag overrides WORKSPACE_PROJECT_ROOT" {
  export WORKSPACE_PROJECT_ROOT="$TEST_DIR/custom-root"
  local override="$TEST_DIR/override-sessions"
  mkdir -p "$override"

  run "$BIN_DIR/resume-session.sh" --sessions-base "$override" --list
  [ "$status" -eq 0 ]
}

@test "env var: defaults to HOME when not set" {
  unset WORKSPACE_PROJECT_ROOT
  # Should use default $HOME/sessions behavior
  run "$BIN_DIR/resume-session.sh" --list
  [ "$status" -eq 0 ]
}
```

**5. Update documentation:**
- USER-GUIDE.md: Add "Custom Project Root" section
- test/README.md: Document env var testing
- S9-COMPLETE.md: Summary of enhancement

---

## Backward Compatibility

**100% backward compatible:**

✅ Existing users (no env var set):
- Behavior unchanged
- Defaults to `~/sessions`, `~/worktrees`, `~/src`
- All existing scripts work exactly as before

✅ Existing users with command-line flags:
- Flags continue to work
- Flags override env var (highest priority)

✅ No breaking changes:
- No API changes
- No manifest format changes
- No directory structure changes

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Breaks existing setups | VERY LOW | HIGH | 100% backward compatible, defaults unchanged |
| Env var conflicts | LOW | LOW | Unique name `WORKSPACE_PROJECT_ROOT` |
| User confusion | LOW | LOW | Clear documentation, help text |
| Testing gaps | MEDIUM | LOW | Add 3 BATS tests for env var |

**Overall Risk**: ✅ **VERY LOW** - Simple, well-contained change

---

## Effort Estimate

**Implementation:** 1-2 hours

- Code changes: 30 minutes (20 lines across 4 files)
- Help text updates: 15 minutes
- BATS tests: 30 minutes (3 new tests)
- USER-GUIDE.md update: 15 minutes
- Testing: 15 minutes
- Documentation: 15 minutes

**Total**: ~2 hours maximum

---

## Benefits

**For Users:**

1. **Multi-project organization**
   - Can have `~/work-projects/`, `~/personal-projects/`, etc.
   - Each with own worktrees/sessions/src hierarchy

2. **Cleaner home directory**
   - Everything under one project root
   - Easier to manage, backup, or delete

3. **Flexibility**
   - Can switch between projects by changing env var
   - Can override on command line when needed

4. **No migration needed**
   - Existing users unaffected
   - New users can adopt immediately

**For Project:**

1. **More flexible architecture**
2. **Better multi-project support**
3. **Minimal code complexity**
4. **Maintains backward compatibility**

---

## Alternative Considered

**Alternative 1: Only use command-line flags (status quo)**
- ❌ Verbose: `--sessions-base ~/my-project/sessions` every time
- ❌ Easy to forget or mistype
- ✅ Already works today

**Alternative 3: Configuration file**
- ✅ Most flexible
- ❌ More complex (parsing, precedence, discovery)
- ❌ Overkill for this use case

**Decision**: Approach 2 (env var) is the sweet spot
- Simple to implement
- Convenient for users
- Backward compatible
- Can add config file later if needed

---

## Should This Be S9?

**Arguments FOR making this S9:**
1. User explicitly requested it
2. Low risk, high value
3. Small scope (1-2 hours)
4. Natural enhancement to completed project
5. Doesn't affect project completion status (already 100%)

**Arguments AGAINST making this S9:**
1. Project is already complete (100% requirements)
2. This is truly optional (nice-to-have)
3. Could be separate minor release (v1.1.0)
4. Not part of original D1-D4 goals

**Recommendation**: Treat as **S9 Optional Enhancement**
- Maintains project completion status
- Allows proper review/testing
- Documents enhancement properly
- User can decide to skip if desired

---

## Questions for Review Council

1. **Is this enhancement worth the effort?** (1-2 hours)
2. **Is Approach 2 (env var) the right solution?**
3. **Should we implement this now or defer to future release?**
4. **Any risks or concerns not identified?**
5. **Should this be S9 or a separate minor version bump?**

---

## Proposed Timeline

**If approved:**

1. **Implementation**: 1 hour
   - Update 4 scripts (code + help text)
   - Add 3 BATS tests
   - Update USER-GUIDE.md

2. **Testing**: 30 minutes
   - Run BATS suite
   - Manual testing with env var set/unset
   - Verify backward compatibility

3. **Documentation**: 30 minutes
   - Create S9-COMPLETE.md
   - Update USER-GUIDE.md
   - Add to FAQ if needed

4. **Review**: 30 minutes
   - Quick multi-persona review (smaller scope than S8)
   - Focus on backward compatibility, risk

**Total**: ~2-3 hours from approval to completion

---

## Decision Required

**User Approval**: ✅ Granted ("Option 2 works for me if reviewers think it's best")

**Review Council Approval**: ⏳ PENDING

**Questions for User:**
1. Proceed with S9 implementation now?
2. Or mark as "future enhancement" and close project as-is?

---

**Status**: Awaiting Review Council approval to proceed with S9

**User Preference**: "Option 2 works for me if the reviewers think it's the best option"
