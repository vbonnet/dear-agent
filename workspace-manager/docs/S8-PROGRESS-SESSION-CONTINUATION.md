# S8 Session Management Scripts - Progress & Continuation Guide

**Date**: 2025-12-03
**Phase**: S8 - Session Management Scripts (IN PROGRESS)
**Status**: 2/3 primary scripts complete, secondary deliverables pending
**Estimated Completion**: 5-7 hours remaining

---

## Quick Status

**What's Done** ✅:
- resume-session.sh (~320 lines) - List and display sessions
- archive-session.sh (~380 lines) - Archive to git with secret audit
- All shellcheck clean (0 warnings)

**What's Next** 🔲:
1. session-dashboard.sh (~300-400 lines) - Interactive management
2. BATS test suite (~200-300 lines) - Automated testing
3. User guide (~500-800 lines) - Documentation

**Time Spent**: ~3 hours
**Time Remaining**: ~5-7 hours

---

## Project Context

### The Big Picture

**Overall Goal**: Workspace management system for Claude Code sessions

**Requirements** (D4):
- ✅ R1: Hierarchical directory structure (S6 - COMPLETE)
- ✅ R2: Session manifests with auto-update & audit (S6 - COMPLETE)
- ⏳ R3: Session management CLI (S8 - IN PROGRESS, 67% done)
- ✅ R4: Migration plan (S7 - COMPLETE)

**Project Completion**: 75% → 100% after S8

### What We Built in Previous Phases

**S6 (Core Libraries)** - 5 library modules:
1. `lib/common-utils.sh` - Logging, validation, user interaction
2. `lib/path-utils.sh` - Git URL parsing, hierarchical paths
3. `lib/manifest-utils.sh` - YAML operations, R2.2 auto-update
4. `lib/audit-utils.sh` - R2.3 secret detection (7 patterns)
5. `lib/git-utils.sh` - Git operations

**S7 (Migration Script)**:
- `bin/migrate-workspace.sh` - Migrates existing worktrees to new structure
- Creates session manifests with directory structure
- Dry-run mode, error handling, verification
- 100% success criteria met

**All Previous Work**:
- Reviewed and approved by Review Council (unanimous 5/5)
- Shellcheck clean
- 20 library functions validated in production
- Comprehensive documentation (~6,000 lines total)

---

## S8 Implementation Details

### Directory Structure Created by S7

```
~/src/{platform}/{user}/{repo}/           # Bare git repos
~/worktrees/{platform}/{user}/{repo}/{branch}/  # Working trees
~/sessions/{session-id}/                  # Session data
  ├── manifest.yaml                       # Session metadata
  ├── working/                            # Temporary work files
  └── artifacts/                          # Session artifacts
```

### Session Manifest Format (YAML)

```yaml
# Session Manifest
session_id: github.com-vbonnet-engram-research-main
created_at: 2025-12-03T00:30:40Z
last_activity: 2025-12-03T00:30:41Z

repository:
  url: https://github.com/vbonnet/engram-research.git
  path: main

worktree:
  path: /home/user/worktrees/github.com/vbonnet/engram-research/main
  branch: main
  commit: 8546336

artifacts: []

status: active  # active | archived
```

### Scripts Implemented

#### 1. resume-session.sh (✅ COMPLETE)

**Location**: `bin/resume-session.sh`
**Lines**: ~320
**Purpose**: Display session information for resuming Claude Code

**Key Functions**:
- `list_sessions()` - Find all sessions in ~/sessions/
- `find_manifest()` - Locate manifest by session ID or path
- `display_session_list()` - Show all sessions with metadata
- `display_session_details()` - Show detailed session info

**Command-line Options**:
- `--list` - List all available sessions
- `--sessions-base DIR` - Override sessions directory
- `--help` - Show help text

**Libraries Used**:
- common-utils.sh (logging, validation)
- manifest-utils.sh (update_manifest_activity for R2.2)
- git-utils.sh (verify worktree is still valid)

**Key Features**:
- Shows repository info (URL, branch, commit)
- Shows paths (worktree, session, working, artifacts)
- Shows timeline (created, last activity, status)
- Verifies worktree still exists and is git repo
- Warns if branch mismatch detected
- Lists artifacts from previous session
- Updates last_activity timestamp (R2.2)

**Usage Examples**:
```bash
# List all sessions
./bin/resume-session.sh --list

# Show session details
./bin/resume-session.sh github.com-vbonnet-engram-research-main

# Use custom sessions directory
./bin/resume-session.sh --sessions-base /custom/path SESSION_ID
```

**Shellcheck**: ✅ Clean (0 warnings, only SC1091 info)

#### 2. archive-session.sh (✅ COMPLETE)

**Location**: `bin/archive-session.sh`
**Lines**: ~380
**Purpose**: Archive session to git with optional push and cleanup

**Key Functions**:
- `find_session_dir()` - Locate session directory by ID
- `init_git_repo()` - Initialize git in session dir if needed
- `archive_to_git()` - Create archive commit
- `push_to_remote()` - Push to remote (optional)
- `cleanup_session()` - Remove working/artifacts after archive (optional)
- `run_archive()` - Main workflow orchestration

**Command-line Options**:
- `--push` - Push archive to remote repository
- `--cleanup` - Remove local files after archiving
- `--dry-run` - Preview without making changes
- `--remote NAME` - Git remote name (default: origin)
- `--branch NAME` - Git branch name (default: main)
- `--sessions-base DIR` - Override sessions directory
- `--help` - Show help text

**Libraries Used**:
- common-utils.sh (logging, confirm_action)
- manifest-utils.sh (update_manifest_field, update_manifest_activity)
- audit-utils.sh (R2.3 pre-archive secret audit)
- git-utils.sh (is_git_repo)

**Key Features**:
- **Pre-archive secret audit (R2.3)** - Prevents committing secrets!
- Initializes git repo if not already initialized
- Commits all session files (manifest, working/, artifacts/)
- Optional push to remote
- Optional cleanup of local files (with confirmation)
- Dry-run mode for preview
- Updates manifest: status → "archived", archived_at timestamp
- Updates last_activity (R2.2)

**Archive Workflow**:
```
1. Pre-archive secret audit (R2.3)
   ├─ Scan manifest.yaml
   ├─ Scan working/
   └─ Scan artifacts/

2. Initialize git repository (if needed)
   └─ git init in session directory

3. Create archive commit
   ├─ git add .
   └─ git commit -m "Archive session: {id}"

4. Push to remote (--push)
   └─ git push origin main

5. Cleanup local files (--cleanup)
   ├─ Confirm with user
   ├─ Remove working/*
   └─ Remove artifacts/*

6. Update manifest
   ├─ status: archived
   └─ archived_at: timestamp
```

**Usage Examples**:
```bash
# Archive locally only
./bin/archive-session.sh github.com-vbonnet-engram-research-main

# Archive and push to remote
./bin/archive-session.sh --push github.com-vbonnet-engram-research-main

# Archive, push, and cleanup
./bin/archive-session.sh --push --cleanup github.com-vbonnet-engram-research-main

# Preview what would be archived
./bin/archive-session.sh --dry-run github.com-vbonnet-engram-research-main
```

**Shellcheck**: ✅ Clean (0 warnings, only SC1091 info)

---

## What Needs to Be Built

### 3. session-dashboard.sh (🔲 NOT STARTED)

**Location**: `bin/session-dashboard.sh` (create this file)
**Estimated Lines**: ~300-400
**Purpose**: Interactive dashboard to view and manage all sessions

**Required Features**:

1. **List All Sessions** (default mode)
   - Show table with: session_id, status, branch, last_activity
   - Color-coded status (green=active, blue=archived)
   - Sort by last_activity (most recent first)

2. **Filtering Options**:
   - `--status active|archived` - Filter by status
   - `--repo PATTERN` - Filter by repository URL pattern
   - `--since DATE` - Sessions active since date

3. **Sorting Options**:
   - `--sort created|activity|status` - Sort order

4. **Summary Statistics**:
   - Total sessions count
   - Active vs archived breakdown
   - Total disk usage estimate

5. **Quick Actions** (optional, nice-to-have):
   - Interactive menu for resume/archive/delete

**Design Pattern** (copy from resume/archive scripts):
```bash
#!/usr/bin/env bash
set -euo pipefail

# Load libraries
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="$(cd "$SCRIPT_DIR/../lib" && pwd)"
source "$LIB_DIR/common-utils.sh"
source "$LIB_DIR/manifest-utils.sh"

# Configuration
readonly DEFAULT_SESSIONS_BASE="$HOME/sessions"
SESSIONS_BASE="$DEFAULT_SESSIONS_BASE"
FILTER_STATUS=""
SORT_BY="activity"

# Functions
show_help() { ... }
parse_arguments() { ... }
list_all_sessions() { ... }
display_dashboard() { ... }
main() { ... }

main "$@"
```

**Key Functions to Implement**:

```bash
# List all sessions with metadata
list_all_sessions() {
  # Find all manifest.yaml files
  # For each manifest:
  #   - Extract session_id (dirname)
  #   - Read status, created_at, last_activity, repo URL, branch
  #   - Output in parseable format
}

# Display dashboard table
display_dashboard() {
  # Print header
  # For each session:
  #   - Format row with columns
  #   - Color-code by status
  # Print summary statistics
}

# Calculate disk usage (optional)
calculate_disk_usage() {
  # du -sh for each session directory
  # Sum total
}
```

**Output Format Example**:
```
╔════════════════════════════════════════════════════════════════╗
║                    Session Dashboard                           ║
╚════════════════════════════════════════════════════════════════╝

ID                                          Status    Branch  Last Activity
─────────────────────────────────────────────────────────────────────────────
github.com-vbonnet-engram-research-main     active    main    2025-12-03 00:30
github.com-user-project-feature-branch      archived  feat    2025-12-02 15:45

Summary:
  Total sessions: 2
  Active: 1
  Archived: 1
  Disk usage: ~125 MB
```

**Libraries to Use**:
- common-utils.sh (logging, colors)
- manifest-utils.sh (reading manifests)

**Shellcheck**: Must pass with 0 warnings

**Time Estimate**: 2-3 hours

---

### 4. BATS Test Suite (🔲 NOT STARTED)

**Location**: `test/session-management.bats` (create this file and directory)
**Estimated Lines**: ~200-300
**Purpose**: Automated testing for all session management scripts

**BATS Framework**:
- Install: `npm install -g bats` or `brew install bats-core`
- Test files: `test/*.bats`
- Run: `bats test/session-management.bats`

**Test Structure**:
```bash
#!/usr/bin/env bats

# Setup/teardown
setup() {
  # Create temp test directory
  export TEST_DIR="$(mktemp -d)"
  export SESSIONS_BASE="$TEST_DIR/sessions"
  mkdir -p "$SESSIONS_BASE"
}

teardown() {
  # Cleanup
  rm -rf "$TEST_DIR"
}

# Tests
@test "resume-session: list shows all sessions" {
  # Create test session
  # Run resume-session.sh --list
  # Assert output contains session ID
}

@test "resume-session: display shows session details" {
  # Create test session
  # Run resume-session.sh SESSION_ID
  # Assert output contains repo URL, branch, paths
}

@test "resume-session: errors on missing session" {
  # Run with non-existent session ID
  # Assert exit code 1
  # Assert error message
}

@test "archive-session: creates git commit" {
  # Create test session
  # Run archive-session.sh SESSION_ID
  # Assert git commit exists
  # Assert manifest status = archived
}

@test "archive-session: dry-run makes no changes" {
  # Create test session
  # Run archive-session.sh --dry-run SESSION_ID
  # Assert no git repo created
  # Assert manifest unchanged
}

@test "archive-session: detects secrets" {
  # Create test session with secret in working/
  # Run archive-session.sh SESSION_ID
  # Assert secret detection triggered
}

@test "dashboard: shows all sessions" {
  # Create multiple test sessions
  # Run session-dashboard.sh
  # Assert all sessions listed
}

@test "dashboard: filters by status" {
  # Create active and archived sessions
  # Run session-dashboard.sh --status active
  # Assert only active sessions shown
}

@test "migration: handles submodules" {
  # Create repo with submodules
  # Run migrate-workspace.sh
  # Assert submodules preserved
}

@test "migration: handles large repos" {
  # Create large test repo (use sparse checkout)
  # Run migrate-workspace.sh
  # Assert completes successfully
}
```

**Test Coverage Targets**:
- resume-session.sh: 5-7 tests
- archive-session.sh: 6-8 tests (including R2.3 audit)
- session-dashboard.sh: 4-6 tests
- migrate-workspace.sh edge cases: 3-5 tests
- **Total**: ~20-25 tests

**CI/CD Integration** (optional):
```yaml
# .github/workflows/test.yml
name: Test
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - name: Install BATS
        run: npm install -g bats
      - name: Run tests
        run: bats test/
```

**Time Estimate**: 2-3 hours

---

### 5. User Guide (🔲 NOT STARTED)

**Location**: `docs/USER-GUIDE.md` (create this file)
**Estimated Lines**: ~500-800
**Purpose**: Step-by-step guide for users

**Required Sections**:

1. **Introduction**
   - What is this workspace management system?
   - Why use it? (benefits)
   - Quick overview of concepts (sessions, manifests, hierarchical structure)

2. **Getting Started**
   - Prerequisites (git, bash, Claude Code)
   - Installation (if needed)
   - First-time setup

3. **Common Workflows**

   **Workflow 1: Migrate Existing Workspace**
   ```bash
   # Step 1: Preview migration
   ./bin/migrate-workspace.sh --dry-run

   # Step 2: Run migration
   ./bin/migrate-workspace.sh

   # Step 3: Verify sessions created
   ./bin/resume-session.sh --list
   ```

   **Workflow 2: Resume a Session**
   ```bash
   # List sessions
   ./bin/resume-session.sh --list

   # Show session details
   ./bin/resume-session.sh github.com-user-repo-main

   # Resume in Claude Code
   cd ~/worktrees/github.com/user/repo/main
   # Start Claude Code
   ```

   **Workflow 3: Archive Completed Work**
   ```bash
   # Archive session
   ./bin/archive-session.sh github.com-user-repo-main

   # Archive and push to remote
   ./bin/archive-session.sh --push github.com-user-repo-main

   # Archive, push, and cleanup local files
   ./bin/archive-session.sh --push --cleanup github.com-user-repo-main
   ```

   **Workflow 4: Manage Multiple Sessions**
   ```bash
   # View all sessions
   ./bin/session-dashboard.sh

   # Filter active sessions
   ./bin/session-dashboard.sh --status active

   # Sort by last activity
   ./bin/session-dashboard.sh --sort activity
   ```

4. **Troubleshooting**

   **Issue: "Worktree not found"**
   - Cause: Worktree moved or deleted
   - Solution: Update manifest or re-clone

   **Issue: "Secrets detected in session"**
   - Cause: Sensitive data in working/ or artifacts/
   - Solution: Review and remove before archiving

   **Issue: "No sessions found"**
   - Cause: Haven't migrated yet
   - Solution: Run migrate-workspace.sh

   **Issue: "Git push failed"**
   - Cause: Remote not configured
   - Solution: Add remote to session git repo

5. **Advanced Usage**

   - Custom session directories
   - Multiple git remotes
   - Automated archiving scripts
   - Integration with CI/CD

6. **Reference**

   - Directory structure diagram
   - Manifest file format
   - All command-line options
   - Secret detection patterns (R2.3)

7. **FAQ**

   - What happens to my old worktrees after migration?
   - Can I migrate the same worktree twice?
   - How do I delete a session?
   - How much disk space do sessions use?
   - Can I use this with non-Claude Code projects?

**Writing Tips**:
- Use clear headers and subsections
- Include code examples for every workflow
- Use emojis sparingly (📁 📍 ⚠️ ✅ ❌)
- Provide screenshots or ASCII diagrams where helpful
- Link to other docs (S7-COMPLETE.md, library docs)

**Time Estimate**: 2-3 hours

---

## Technical Patterns to Follow

### Library Usage Pattern

All scripts should follow this pattern:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Get script directory and load libraries
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="$(cd "$SCRIPT_DIR/../lib" && pwd)"

# Source libraries (only what you need)
source "$LIB_DIR/common-utils.sh"
source "$LIB_DIR/manifest-utils.sh"
# ... other libraries as needed

# Your code here
```

### Common Functions Available

From `common-utils.sh`:
- `log_info "message"` - Blue info message
- `log_warn "message"` - Yellow warning
- `log_error "message"` - Red error
- `log_success "message"` - Green success
- `log_debug "message"` - Gray debug (if DEBUG=1)
- `confirm_action "prompt"` - Ask yes/no, returns 0 for yes

From `manifest-utils.sh`:
- `update_manifest_field "path" "field" "value"` - Update YAML field
- `update_manifest_activity "path"` - Update last_activity (R2.2)

From `audit-utils.sh`:
- `audit_session_for_secrets "session_dir"` - Scan for secrets (R2.3)
- `audit_and_confirm "session_dir"` - Scan and ask user

From `git-utils.sh`:
- `is_git_repo "path"` - Check if directory is git repo
- `get_current_branch "path"` - Get current branch name
- `get_current_commit "path"` - Get current commit hash

### Error Handling Pattern

```bash
# Good: Check return code
if ! some_function "$arg"; then
  log_error "Operation failed"
  return 1
fi

# Good: Die on error with set -e
set -euo pipefail  # At top of script

# Good: Rollback on failure
if ! risky_operation; then
  cleanup_partial_work
  return 1
fi
```

### Argument Parsing Pattern

```bash
parse_arguments() {
  while [[ $# -gt 0 ]]; do
    case $1 in
      --help|-h)
        show_help
        exit 0
        ;;
      --flag)
        FLAG=true
        shift
        ;;
      --option)
        OPTION="$2"
        shift 2
        ;;
      -*)
        log_error "Unknown option: $1"
        show_help
        exit 1
        ;;
      *)
        # Positional argument
        POSITIONAL="$1"
        shift
        ;;
    esac
  done
}
```

### Shellcheck Compliance

All scripts must pass:
```bash
shellcheck bin/script-name.sh 2>&1 | grep -v "SC1091"
```

Only SC1091 (sourced files not found) is acceptable.

---

## Success Criteria for S8

**Must Have** (to complete S8):
1. ✅ resume-session.sh functional and tested
2. ✅ archive-session.sh functional and tested
3. 🔲 session-dashboard.sh functional and tested
4. 🔲 BATS tests cover core workflows
5. 🔲 User guide covers common use cases
6. 🔲 Shellcheck clean for all scripts
7. 🔲 R3 (Session Management CLI) 100% complete
8. 🔲 Multi-persona review approval

**Should Have** (from S7 retrospective action items):
1. 🔲 Document "run one at a time" constraint
2. 🔲 Add disk space warning (in user guide)
3. 🔲 Test with variety of repositories (BATS)

**Nice to Have**:
1. Progress bar for operations
2. Interactive mode for dashboard
3. Session deletion capability

---

## Next Immediate Steps

**Priority 1** (complete primary deliverables):
1. Implement session-dashboard.sh (~2-3 hours)
2. Test all 3 scripts manually
3. Fix any integration bugs found

**Priority 2** (complete secondary deliverables):
4. Create BATS test suite (~2-3 hours)
5. Write user guide (~2-3 hours)

**Priority 3** (wrap up S8):
6. Run full test suite
7. Multi-persona review
8. Get approval to proceed to S9

---

## Files and Locations

**Completed**:
- ✅ `/tmp/engram-research/.../bin/resume-session.sh`
- ✅ `/tmp/engram-research/.../bin/archive-session.sh`

**To Create**:
- 🔲 `/tmp/engram-research/.../bin/session-dashboard.sh`
- 🔲 `/tmp/engram-research/.../test/session-management.bats`
- 🔲 `/tmp/engram-research/.../docs/USER-GUIDE.md`

**Base Directory**:
`/tmp/engram-research/wayfinder-projects/workspace-design/workspace-management/`

**Git Remote**:
`github.com/vbonnet/engram-research` (main branch)

---

## Review and Approval Chain

**Completed Reviews** (14 total):
1. D2-D4 Discovery phases (all approved)
2. S4-S7 Implementation phases (all approved)
3. S11 Retrospective (approved)

**Next Review** (after S8 complete):
- S8 Formal Review (5 personas)
- S8 Approval to Proceed to S9

**Review Pattern**:
1. Complete implementation
2. Create S8-COMPLETE.md (summary of work)
3. Create S8-FORMAL-REVIEW.md (5-persona review)
4. Create S8-APPROVAL-TO-PROCEED-S9.md (decision doc)
5. Get user approval
6. Proceed to S9

---

## Key Learnings from S7 (Apply to S8)

1. **Budget 40-50% time for documentation** (not 20-30%)
2. **Integration testing finds 1-3 bugs** (budget 1 hour for fixes)
3. **Shellcheck early and often** (prevents issues)
4. **Source guards essential** (already in place)
5. **Never delete CWD in Claude Code** (session breaks - engram created)

---

## Estimated Timeline to S8 Complete

**Remaining Work**:
- session-dashboard.sh: 2-3 hours
- BATS test suite: 2-3 hours
- User guide: 2-3 hours
- Integration fixes: 1 hour
- Review documentation: 1-2 hours

**Total**: 8-12 hours → Estimate 8-10 hours (conservative)

**After S8**: Project 100% feature complete (R1, R2, R3, R4 all done)

---

## Contact for Questions

**Git Repository**: github.com/vbonnet/engram-research
**Documentation**: `docs/` directory (S7-COMPLETE, S11-RETROSPECTIVE, etc.)
**Libraries**: `lib/` directory (5 modules, all with source guards)
**Scripts**: `bin/` directory (migrate, resume, archive, dashboard)

**Session Continuation**: Read this document, review completed scripts, then continue with session-dashboard.sh implementation.

---

**Document Created**: 2025-12-03
**Status**: S8 67% complete
**Next**: Implement session-dashboard.sh

