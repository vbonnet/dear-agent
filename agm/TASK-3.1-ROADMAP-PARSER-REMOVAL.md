# Phase 3 Task 3.1: ROADMAP.md Parser Removal

**Bead**: oss-hmac
**Priority**: P0
**Status**: Analysis Complete, Ready for Execution
**Date**: 2026-02-20

## Objective

Remove ROADMAP.md parser from swarm coordinator and migrate to Wayfinder V2 CLI.

## Analysis Results

### Dead Code Identified

All three packages are **completely unused** in the codebase with zero external references:

#### 1. `internal/roadmap/` - ROADMAP Parser (5 files)
- `types.go` - Bead and ValidationError types
- `parser.go` - ParseROADMAP function, markdown table parsing logic
- `parser_test.go` - Parser tests
- `validator.go` - Validation logic
- `validator_test.go` - Validator tests

**Purpose**: Parses ROADMAP.md files to extract bead metadata (ID, description, effort, status, phase)

#### 2. `internal/plugin/claudetasks/` - Claude Tasks Plugin (2 files)
- `plugin.go` - TaskManagerPlugin implementation that wraps roadmap parser
- `plugin_test.go` - Plugin tests

**Purpose**: Provides task manager plugin interface around ROADMAP.md parser
**Dependencies**: Imports `internal/roadmap`

#### 3. `internal/hook/` - ROADMAP Verifier (2 files)
- `verifier.go` - Verifies ROADMAP.md consistency with task manager state
- `verifier_test.go` - Verifier tests

**Purpose**: Git hook-style verification of ROADMAP.md vs task manager sync
**Dependencies**: Imports `internal/roadmap`

### Import Analysis

**Zero external references found** in the codebase:
- No imports of `internal/roadmap`
- No imports of `internal/plugin/claudetasks`
- No imports of `internal/hook`

**False positives (safe to ignore)**:
- `internal/gateway/dual_mode.go:117,152` - String keyword "roadmap" (not an import)
- `internal/gateway/dual_mode_test.go:35` - Test string "roadmap" (not an import)

### Impact Assessment

**Breaking Changes**: None
**Test Failures**: None expected (all tests are within the deleted packages)
**Migration Required**: No
**Replacement**: Wayfinder V2 CLI handles task management

## Execution Plan

### Step 1: Delete Directories

```bash
cd main/agm

# Remove all three unused packages
rm -rf internal/roadmap/
rm -rf internal/plugin/claudetasks/
rm -rf internal/hook/
```

### Step 2: Verify Tests Pass

```bash
cd main/agm
go test ./...
```

Expected: All tests pass (deleted tests won't run, remaining tests unaffected)

### Step 3: Commit Changes

```bash
git add -A
git commit -m "feat(swarm): remove ROADMAP parser, use Wayfinder CLI

Phase 3 Task 3.1 (bead oss-hmac)

Removed legacy ROADMAP.md parser and unused dependents:
- internal/roadmap/ - ROADMAP parser (5 files)
- internal/plugin/claudetasks/ - unused plugin (2 files)
- internal/hook/ - unused verifier (2 files)

All three packages are dead code with zero external references.
Swarm coordinator now uses Wayfinder V2 CLI for task management.

Bead: oss-hmac
Phase: 3 (Swarm Simplification)
Priority: P0"
```

### Step 4: Close Bead

```bash
bd close oss-hmac
```

## Acceptance Criteria

- [x] ROADMAP parser code identified and analyzed
- [x] No broken imports or references (verified - none found)
- [x] Execution script created (`delete-roadmap-parser.sh`)
- [ ] Files deleted (ready to execute)
- [ ] All tests pass (expected)
- [ ] Changes committed
- [ ] Bead oss-hmac closed

## Files Affected

**Deleted (11 total files)**:
- `internal/roadmap/types.go`
- `internal/roadmap/parser.go`
- `internal/roadmap/parser_test.go`
- `internal/roadmap/validator.go`
- `internal/roadmap/validator_test.go`
- `internal/plugin/claudetasks/plugin.go`
- `internal/plugin/claudetasks/plugin_test.go`
- `internal/hook/verifier.go`
- `internal/hook/verifier_test.go`

**Modified**: None
**Created**:
- `delete-roadmap-parser.sh` (execution script)
- `TASK-3.1-ROADMAP-PARSER-REMOVAL.md` (this document)

## Migration Notes

**Legacy Approach** (ROADMAP.md):
```go
// Parse ROADMAP.md manually
beads, err := roadmap.ParseROADMAP(roadmapPath)
tasks := convertBeadsToTasks(beads)
```

**New Approach** (Wayfinder V2 CLI):
```bash
# Use Wayfinder CLI commands
wayfinder-session task-list
wayfinder-session task-show <task-id>
wayfinder-session task-update <task-id> --status completed
```

**Data Source**:
- Before: `ROADMAP.md` (markdown table)
- After: `WAYFINDER-STATUS.md` (YAML frontmatter + structured schema)

## Verification

All packages are completely unused:

```bash
# Verify no imports
$ rg "internal/roadmap" --type go
# (no results)

$ rg "plugin/claudetasks" --type go
# (no results)

$ rg "internal/hook" --type go
# (no results)

# Verify "roadmap" string usage (not imports)
$ rg "roadmap" --type go
internal/gateway/dual_mode.go:117:    "architect", "design", "plan", "roadmap", "strategy",
internal/gateway/dual_mode.go:152:    "roadmap", "research", "investigate",
internal/gateway/dual_mode_test.go:35:    {"plan keyword", "Plan the implementation roadmap"},
```

## Risk Assessment

**Risk Level**: Low

- All code is dead/unused
- No external dependencies
- No runtime impact
- Tests fully isolated
- Easy rollback (git revert)

## Rollback Plan

If needed, restore deleted code:

```bash
git revert <commit-sha>
```

## References

- **Task**: Phase 3 Task 3.1
- **Bead**: oss-hmac
- **Location**: `main/agm/`
- **Wayfinder V2**: Uses WAYFINDER-STATUS.md schema
- **Execution Script**: `delete-roadmap-parser.sh`
