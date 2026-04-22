> **SUPERSEDED** (2026-03-26): This document describes the old Python-era implementation.
> The YAML-based pattern database approach was replaced by compiled Go regex patterns.
> See `cmd/pretool-bash-blocker/SPEC.md` and `internal/validator/patterns.go` for the current implementation.
> Kept for historical reference.

# Work Stream 5: PreTool Hook Unification

**Status**: ✅ Implemented
**Date**: 2026-02-15
**Dependencies**: WS2 (pattern database) ✅ complete

## Overview

This work stream unified existing PreTool hooks to use centralized pattern databases, eliminating hardcoded patterns and improving maintainability.

## New Validators

### 1. pretool-bash-validator

**Purpose**: Validates bash commands using centralized pattern database

**Key Features**:
- Loads patterns from `engram/patterns/bash-anti-patterns.yaml`
- Only validates patterns with `tier2_validation: true`
- Uses `tier1_example` field for user-friendly error messages
- Automatic quote stripping to avoid false positives
- Backward compatible with existing error format

**Usage**:
```bash
# The validator runs automatically on all Bash tool calls
# It loads the pattern database at runtime
# Exit code 0 = allow, exit code 2 = block
```

**Pattern Database**:
- 31 total patterns
- 15 patterns with `tier2_validation: true` (enforced)
- 16 patterns with `tier2_validation: false` (guidance only)

**Tier 2 Validated Patterns**:
1. `cd-chaining` - Command chaining with cd
2. `cd-semicolon-chain` - cd with semicolon
3. `cat-file-read` - Using cat to read files
4. `grep-search` - Using bash grep
5. `find-file-search` - Using bash find
6. `echo-redirect` - Using echo >
7. `cat-heredoc` - Using cat << heredoc
8. `sed-replace` - Using sed for text replacement
9. `pipe-to-grep` - Piping to grep
10. `for-loop` - Using for loop
11. `while-loop` - Using while loop
12. `double-ampersand-chain` - Command chaining with &&
13. `semicolon-chain` - Command chaining with semicolon
14. `redirect-output` - Output redirection >
15. `append-redirect` - Append redirection >>

---

### 2. pretool-beads-validator

**Purpose**: Prevents direct .beads/ access using centralized pattern database

**Key Features**:
- Loads patterns from `engram/patterns/beads-anti-patterns.yaml`
- Validates both file operations (Read/Write/Edit) and bash commands
- Uses `tier1_example` for error messages
- Fallback protection if PyYAML unavailable

**Usage**:
```bash
# Blocks direct access to .beads/ directories
# Exit code 0 = allow, exit code 1 = block
```

**Validated Patterns** (6 total):
1. `sqlite3-direct` - Direct sqlite3 access to .beads/
2. `direct-beads-modification` - rm/cp/mv/touch/chmod .beads/
3. `beads-cat-read` - cat .beads/
4. `beads-ls-directory` - ls .beads/
5. `beads-grep-search` - grep .beads/
6. `beads-find-search` - find .beads/

**Alternative**: Use `bd` CLI tool for all beads operations

---

### 3. pretool-git-validator

**Purpose**: Enforces git worktree usage for multi-agent safety

**Key Features**:
- Loads patterns from `engram/patterns/git-anti-patterns.yaml`
- Context aware - allows `git worktree` commands
- Strips quoted content to avoid false positives in commit messages
- Only processes git commands (ignores non-git Bash calls)

**Usage**:
```bash
# Enforces worktree-based workflow for multi-agent safety
# Exit code 0 = allow, exit code 1 = block
```

**Validated Patterns** (4 total):
1. `git-branch-multiagent` - Blocks `git branch` (suggests worktree)
2. `git-checkout-main-branch` - Blocks `git checkout main/master`
3. `git-modify-files-main-branch` - Blocks commits in main branch
4. `git-push-force-main` - Blocks `git push --force origin main`

**Recommended Workflow**:
```bash
# 1. Create worktree
git worktree add ../feature-name -b feature-branch

# 2. Work in worktree
cd ../feature-name && edit files

# 3. Commit in worktree
git add . && git commit -m "changes"

# 4. Push from worktree
git push origin feature-branch

# 5. Clean up
git worktree remove ../feature-name
```

---

## Testing

Run the test suite:

```bash
cd ./engram/hooks
bash test-validators.sh
```

**Test Coverage**:
- 8 bash validator tests
- 7 beads validator tests
- 8 git validator tests
- **Total: 23 tests**

---

## Files Created/Modified

### New Files:
1. `engram/hooks/pretool-bash-validator` - Bash validator with pattern DB
2. `engram/hooks/pretool-beads-validator` - Beads validator with pattern DB
3. `engram/hooks/pretool-git-validator` - Git worktree validator
4. `engram/hooks/test-validators.sh` - Test suite
5. `engram/hooks/README-WS5-VALIDATORS.md` - This file

### Modified Files:
1. `engram/patterns/bash-anti-patterns.yaml`
   - Added `tier2_validation` flag to all 31 patterns
   - 15 patterns set to `true` (enforced)
   - 16 patterns set to `false` (guidance only)

2. `engram/patterns/PRETOOL-HOOKS-INVENTORY.md`
   - Updated table to show legacy and new validators
   - Added detailed documentation for new validators
   - Marked legacy validators with ⚠️ status

---

## Integration

### Hook Registration

These validators need to be registered as PreToolUse hooks in Claude Code's configuration. The exact registration method depends on the hook framework being used.

**Recommended sequence**: bash → beads → git

### Dependencies

**Required**:
- Python 3.6+
- PyYAML library (`pip install pyyaml`)

**Optional**:
- Pattern databases at `engram/patterns/`
- Fallback behavior if databases unavailable

---

## Migration from Legacy

### pretool-bash-blocker.py → pretool-bash-validator

**Differences**:
- **Old**: Hardcoded patterns in Python code
- **New**: Patterns loaded from YAML database
- **Benefit**: Easier to update patterns without code changes

**Migration path**:
1. Test new validator alongside old validator
2. Verify all patterns are detected correctly
3. Switch hook registration from old to new
4. Archive old validator

### pretool-beads-protection.py → pretool-beads-validator

**Differences**:
- **Old**: Simple string matching (`"/.beads/" in path`)
- **New**: Regex patterns from YAML database
- **Benefit**: More comprehensive coverage (6 patterns vs 1)

**Migration path**:
1. Test new validator alongside old validator
2. Verify all access methods are blocked
3. Switch hook registration from old to new
4. Archive old validator

---

## Success Criteria

- ✅ Bash validator uses pattern DB (not hardcoded patterns)
- ✅ Beads validator created and blocks all 6 patterns
- ✅ Git validator created and blocks all 4 patterns
- ✅ All validators use tier1_example for error messages
- ✅ Test suite created with 23 tests
- ✅ Inventory updated

**Remaining Tasks** (for deployment):
- [ ] Register hooks in Claude Code configuration
- [ ] Test in live environment
- [ ] Monitor violation logs
- [ ] Archive legacy validators

---

## Pattern Database Integration

All validators now follow the unified architecture:

```
┌─────────────────────────────────────┐
│  Pattern Database (YAML)            │
│  - bash-anti-patterns.yaml          │
│  - beads-anti-patterns.yaml         │
│  - git-anti-patterns.yaml           │
└─────────────┬───────────────────────┘
              │
              ▼
┌─────────────────────────────────────┐
│  PreTool Validators (Python)        │
│  - pretool-bash-validator           │
│  - pretool-beads-validator          │
│  - pretool-git-validator            │
└─────────────┬───────────────────────┘
              │
              ▼
┌─────────────────────────────────────┐
│  Claude Code Hook Framework         │
│  (intercepts tool calls)            │
└─────────────────────────────────────┘
```

**Benefits**:
1. Single source of truth (pattern databases)
2. Easy to update patterns without code changes
3. Consistent error messaging across validators
4. Tier-based enforcement (tier1=AI, tier2=hooks, tier3=recovery)

---

## Logging

All validators log violations to:
```
~/.claude-tool-violations.log
```

**Log format**:
```
[2026-02-15 14:30:00] TIER2_VIOLATION: cd-chaining
  Tool: Bash
  Command: cd /repo && git push
  ---
```

Enable debug logging:
```bash
DEBUG=1 <validator command>
```

---

## Documentation

- **Pattern Databases**: `engram/patterns/`
- **Inventory**: `engram/patterns/PRETOOL-HOOKS-INVENTORY.md`
- **Architecture**: `engram/patterns/SPEC-ARCHITECTURE.md` (if exists)

---

## Contact

**Work Stream**: WS5 - PreTool Hook Unification
**Owner**: engram-research team
**Timeline**: 8-10 hours (estimated)
**Completion Date**: 2026-02-15
