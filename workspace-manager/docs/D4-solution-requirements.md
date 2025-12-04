# D4: Solution Requirements

**Date:** 2025-12-02

**Project:** Workspace & Session Management System

**Phase:** D4 - Solution Requirements

**Status:** 🔄 In Progress

---

## Purpose

Translate D3 approach decisions into detailed, actionable requirements for implementation.

**Input from D3:**
- 8 concrete decisions made
- 2 user modifications incorporated
- Unanimous multi-persona approval
- 2 conditions to address (sensitive data audit, pattern checkpoint)

**Output:** Detailed specifications ready for implementation (S4-S11)

---

## Requirements Framework

Each requirement includes:
- **Specification:** What needs to be built
- **Acceptance criteria:** How to verify it works
- **Priority:** Must-have / Should-have / Nice-to-have
- **Dependencies:** What else is needed
- **Implementation notes:** How to build it

---

## Requirement 1: Directory Structure

### R1.1: Main Repository Directory

**Specification:**
```bash
~/src/{platform}/{username}/{repository}/
```

**Examples:**
```bash
~/src/github/vbonnet/engram/              # Personal repo
~/src/github/vbonnet/engram-research/     # Personal repo
~/src/github/[REDACTED_EMPLOYER]-src/project-x/        # Work repo
~/src/gitlab/org/project-y/               # GitLab repo
```

**Creation rules:**
- Auto-create parent directories if needed
- Use `mkdir -p` for idempotent creation
- Set permissions: 755 (user rwx, group rx, other rx)

**Acceptance criteria:**
- [ ] All repos cloned to correct location
- [ ] Tab completion works
- [ ] Git operations work normally
- [ ] Can `cd` to any repo via pattern

**Priority:** MUST-HAVE

**Dependencies:** None

**Implementation:**
```bash
# Helper function
clone-repo() {
  local url="$1"
  # Parse: https://github.com/vbonnet/engram.git
  # Extract: platform=github, user=vbonnet, repo=engram
  local platform=$(echo "$url" | sed -n 's|.*://\([^/]*\)/.*|\1|p')
  local user=$(echo "$url" | sed -n 's|.*/\([^/]*\)/[^/]*\.git|\1|p')
  local repo=$(basename "$url" .git)

  local target="$SRC_ROOT/$platform/$user/$repo"
  mkdir -p "$(dirname "$target")"
  git clone "$url" "$target"
}
```

---

### R1.2: Worktree Directory Structure

**Specification:**
```bash
~/worktrees/{platform}/{username}/{repository}/{branch-name}/
```

**Examples:**
```bash
~/worktrees/github/vbonnet/engram/feature-bash-guidance/
~/worktrees/github/vbonnet/engram/fix-telemetry/
~/worktrees/github/vbonnet/engram/review-pr-123/
```

**Mirrors:** `~/src/` structure

**Creation rules:**
- Must mirror parent repo location
- Branch name used as final directory
- Auto-create intermediate directories

**Acceptance criteria:**
- [ ] Worktrees organized by parent repo
- [ ] Easy to find all worktrees for a repo
- [ ] gwq tool can discover all worktrees
- [ ] No name collisions

**Priority:** MUST-HAVE

**Dependencies:** R1.1 (main repos exist first)

**Implementation:**
```bash
# Helper function
create-worktree() {
  local repo_path="$1"      # ~/src/github/vbonnet/engram
  local branch="$2"         # feature-bash-guidance

  # Extract platform/user/repo from path
  local relative=${repo_path#$SRC_ROOT/}
  local worktree_base="$WORKTREES_ROOT/$relative"
  local worktree_path="$worktree_base/$branch"

  mkdir -p "$worktree_base"
  cd "$repo_path"
  git worktree add "$worktree_path" -b "$branch"
}
```

---

### R1.3: Session Directory Structure

**Specification:**
```bash
~/.claude/sessions/{session-id}/
├── manifest.yaml          # Session metadata
├── working/               # Non-ephemeral scratch space
│   ├── tool-results/     # Grep, find, API outputs
│   ├── analysis/         # Temporary analysis docs
│   └── scratch/          # Notes and intermediate work
└── artifacts/            # Final outputs (keep on archive)
    ├── D1-problem.md
    └── S7-plan.md
```

**Creation rules:**
- Auto-create on first tool call in session
- Session ID from Claude Code (existing pattern)
- Subdirectories created on-demand
- Permissions: 700 (user only)

**Acceptance criteria:**
- [ ] Session directory auto-created
- [ ] Subdirectories exist when needed
- [ ] Permissions prevent other users
- [ ] Structure consistent across sessions

**Priority:** MUST-HAVE

**Dependencies:** None (uses existing ~/.claude/ pattern)

**Implementation:**
```bash
# Auto-create session directory
ensure-session-dir() {
  local session_id="$1"
  local session_dir="$SESSIONS_ROOT/$session_id"

  if [[ ! -d "$session_dir" ]]; then
    mkdir -p "$session_dir"/{working/{tool-results,analysis,scratch},artifacts}
    chmod 700 "$session_dir"
    init-manifest "$session_id"
  fi
}
```

---

### R1.4: Archive Directory Structure

**Specification:**
```bash
~/src/github/vbonnet/engram-research/session-archives/{date}/{session-id}/
├── manifest.yaml
├── working/              # NON-ephemeral (kept for study)
│   ├── tool-results/
│   ├── analysis/
│   └── scratch/
└── artifacts/
    └── ...
```

**Organization:**
- By date (YYYY-MM-DD) for temporal browsing
- Session ID preserves uniqueness
- Complete session snapshot (manifest + working + artifacts)

**Acceptance criteria:**
- [ ] Archives organized by completion date
- [ ] All session content preserved
- [ ] Git-backed (in engram-research)
- [ ] Cross-machine portable

**Priority:** MUST-HAVE

**Dependencies:** R1.3 (session structure)

**Implementation:**
```bash
# Archive session
archive-session() {
  local session_id="$1"
  local date=$(date +%Y-%m-%d)
  local archive_path="$ARCHIVES_ROOT/$date/$session_id"

  mkdir -p "$archive_path"
  cp -r "$SESSIONS_ROOT/$session_id"/* "$archive_path/"

  # Update manifest status
  sed -i 's/status: "active"/status: "archived"/' "$archive_path/manifest.yaml"

  # Commit to git
  cd "$ARCHIVES_ROOT/.."
  git add "session-archives/$date/$session_id"
  git commit -m "Archive session $session_id ($date)"
}
```

---

### R1.5: Environment Variables

**Specification:**

```bash
# Add to ~/.bashrc or ~/.zshrc
export SRC_ROOT=~/src
export WORKTREES_ROOT=~/worktrees
export SESSIONS_ROOT=~/.claude/sessions
export ARCHIVES_ROOT=~/src/github/vbonnet/engram-research/session-archives
```

**Usage in manifests:**
- Use `{SRC_ROOT}` instead of hardcoded `/home/user/src`
- Enable cross-machine portability
- Substitute at runtime when reading manifests

**Acceptance criteria:**
- [ ] Variables defined in shell config
- [ ] Scripts use variables (not hardcoded paths)
- [ ] Manifests use variable placeholders
- [ ] Works on different machines

**Priority:** SHOULD-HAVE

**Dependencies:** None

---

## Requirement 2: Session Manifest Schema

### R2.1: Manifest File Format

**Specification:** YAML format

**File location:** `~/.claude/sessions/{session-id}/manifest.yaml`

**Base structure:**
```yaml
# Session identification
session_id: "claude-abc123-def456"
created: "2025-12-02T10:30:00Z"        # ISO 8601
last_activity: "2025-12-02T14:22:00Z"  # ISO 8601
status: "active"                        # active | completed | archived

# Work context
project: "bash-guidance-consolidation"
project_type: "wayfinder"              # wayfinder | research | bugfix | ad-hoc
description: |
  Consolidating bash guidance patterns across Engram codebase.
  Wayfinder methodology: D1-S11 phases.

# Git context
worktree:
  path: "{WORKTREES_ROOT}/github/vbonnet/engram/feature-bash-guidance"
  repo: "github.com/vbonnet/engram"
  branch: "feature/bash-guidance-consolidation"
  base_branch: "wayfinder-prototype"

# Artifacts tracking
artifacts:
  created:
    - path: "artifacts/S7-plan.md"
      size: "2.5KB"
      created: "2025-12-02T11:00:00Z"
    - path: "artifacts/S8-implementation.md"
      size: "8.3KB"
      created: "2025-12-02T13:15:00Z"
  working_files:
    - path: "working/tool-results/grep-output.txt"
      size: "45KB"
      created: "2025-12-02T10:45:00Z"

# Context audit (LangChain Pattern 4)
context_audit:
  tokens_consumed: 15234
  files_available: 500
  files_accessed: 12
  efficiency_ratio: 2.4              # % (good - only loaded what needed)

# Tags for filtering
tags:
  - "wayfinder"
  - "engram"
  - "bash-guidance"

# Resumption info
resumption:
  cwd: "{WORKTREES_ROOT}/github/vbonnet/engram/feature-bash-guidance"
  last_phase: "S8-implementation"
  next_steps: |
    Complete S8 implementation:
    - Finish BashGuidance consolidation
    - Run S9 validation tests
    - Proceed to S10 deployment
  blocked_on: null                   # null | "waiting for X"
```

**Acceptance criteria:**
- [ ] Valid YAML (parseable)
- [ ] All required fields present
- [ ] Variable placeholders used for paths
- [ ] Human-readable and editable
- [ ] Git-friendly diffs

**Priority:** MUST-HAVE

**Dependencies:** R1.3 (session structure)

---

### R2.2: Manifest Auto-Update

**Specification:**

Update manifest automatically after these events:
1. Tool call completed
2. Artifact created/modified
3. Worktree changed
4. Session phase changed (D1→D2→S4→etc)

**Update frequency:**
- Not time-based (no polling)
- Event-driven only
- Immediate update (don't batch)

**What gets updated:**
- `last_activity` timestamp (every event)
- `artifacts.created` list (on file creation)
- `context_audit.files_accessed` (on file read)
- `resumption.last_phase` (on phase transition)

**Acceptance criteria:**
- [ ] Manifest updates after tool calls
- [ ] Timestamp is current
- [ ] Artifact list is accurate
- [ ] No manual updates required

**Priority:** SHOULD-HAVE

**Dependencies:** R2.1 (manifest schema)

**Implementation notes:**
- Hook into Claude Code session events (if possible)
- Otherwise: Manual helper function called explicitly
- Start simple (manual), add automation later

---

### R2.3: Sensitive Data Audit (CRITICAL - Skeptic Condition #1) **[UPDATED]**

**Specification:**

Before archiving session, audit **ALL session content** for sensitive data:
- **manifest.yaml** (descriptions, resumption notes, etc.)
- **working/** directory (all subdirectories)
- **artifacts/** directory (all subdirectories)

**Patterns to detect:**
- API keys: `[A-Za-z0-9]{32,}`
- AWS credentials: `AKIA[A-Z0-9]{16}`
- Private keys: `-----BEGIN.*PRIVATE KEY-----`
- Tokens: `token[=:]\s*[A-Za-z0-9_-]{20,}`
- Passwords: `password[=:]\s*\S+`
- SSH keys: `ssh-rsa|ssh-ed25519`
- Database URLs: `postgresql://|mysql://.*password`

**Behavior:**
1. Scan manifest.yaml for secrets
2. Scan all files in working/ directory
3. Scan all files in artifacts/ directory
4. If potential secrets found in **any location**:
   - Show user which files contain what patterns
   - Prompt: "Potential secrets detected. Review before archiving? [Y/n]"
   - If Y: List files for review, wait for user confirmation
   - If n: Continue (user takes responsibility), note in commit message
5. If no secrets found: Proceed silently

**Known limitations (document in R6.1):**
- Symlinks: Follows symlinks by default (could escape audit scope)
- Binary files: May not detect secrets in compiled binaries
- Encrypted files: Cannot scan encrypted content
- False positives: May flag non-secret long strings (e.g., hashes)

**Acceptance criteria:**
- [ ] Scans manifest.yaml **[UPDATED]**
- [ ] Scans all files in working/
- [ ] Scans all files in artifacts/ **[UPDATED]**
- [ ] Detects common secret patterns
- [ ] Prompts user if found
- [ ] Allows user override with documented risk
- [ ] Does not archive without user confirmation if secrets found
- [ ] Documents limitations in user guide **[UPDATED]**

**Priority:** MUST-HAVE (Security requirement from Skeptic) **[UPDATED: HIGH → MUST-HAVE]**

**Dependencies:** R2.1 (manifest), R1.4 (archive structure)

**Implementation:**
```bash
audit-session-for-secrets() {
  local session_id="$1"
  local session_dir="$SESSIONS_ROOT/$session_id"

  # Scan manifest.yaml
  local manifest_findings=$(grep -E \
    -e '[A-Za-z0-9]{32,}' \
    -e 'AKIA[A-Z0-9]{16}' \
    -e '-----BEGIN.*PRIVATE KEY-----' \
    -e 'token[=:]\s*[A-Za-z0-9_-]{20,}' \
    -e 'password[=:]\s*\S+' \
    -e 'ssh-rsa|ssh-ed25519' \
    "$session_dir/manifest.yaml" 2>/dev/null || true)

  # Scan working/ directory
  local working_findings=$(grep -rE \
    -e '[A-Za-z0-9]{32,}' \
    -e 'AKIA[A-Z0-9]{16}' \
    -e '-----BEGIN.*PRIVATE KEY-----' \
    -e 'token[=:]\s*[A-Za-z0-9_-]{20,}' \
    -e 'password[=:]\s*\S+' \
    "$session_dir/working" 2>/dev/null || true)

  # Scan artifacts/ directory
  local artifacts_findings=$(grep -rE \
    -e '[A-Za-z0-9]{32,}' \
    -e 'AKIA[A-Z0-9]{16}' \
    -e 'password[=:]\s*\S+' \
    -e 'postgresql://|mysql://.*password' \
    "$session_dir/artifacts" 2>/dev/null || true)

  # Combine all findings
  local all_findings="$manifest_findings$working_findings$artifacts_findings"

  if [[ -n "$all_findings" ]]; then
    echo "⚠️  Potential secrets detected in session files:"
    echo "$all_findings" | head -20
    echo
    echo "Limitations: May not detect secrets in binaries, encrypted files, or via symlinks."
    echo "See USER-GUIDE.md for details."
    echo
    read -p "Review files before archiving? [Y/n] " response
    if [[ "$response" != "n" ]]; then
      return 1  # Abort archive, user should review
    else
      echo "User accepted risk. Archiving anyway." >&2
    fi
  fi

  return 0  # Safe to proceed
}
```

---

### R2.4: Pattern Analysis Checkpoint (Skeptic Condition #2)

**Specification:**

After 10 sessions archived, trigger pattern analysis review:

**Checkpoint behavior:**
1. Detect: 10 sessions in archives (automated count)
2. Notify: "Pattern analysis checkpoint reached (10 sessions archived)"
3. Prompt: "Analyze working/ patterns to optimize? [Y/n]"
4. If Y: Generate analysis report

**Analysis report includes:**
- Total storage: working/ across all sessions
- File type breakdown: tool-results/ vs analysis/ vs scratch/
- Re-read frequency: How often are working/ files accessed post-session?
- Value assessment: Which types contain useful info?
- Recommendation: What should be ephemeral vs kept?

**Example output:**
```
Pattern Analysis Report (10 sessions)
=====================================
Total working/ storage: 1.2MB
Breakdown:
  - tool-results/: 850KB (71%) - NEVER re-read
  - analysis/: 280KB (23%) - Re-read in 3/10 sessions
  - scratch/: 70KB (6%) - NEVER re-read

Recommendation:
  ✅ Keep analysis/ (valuable for resumption)
  ❌ Delete tool-results/ (never re-read, can regenerate)
  ❌ Delete scratch/ (truly ephemeral)

Storage savings: ~920KB/session (77%)
```

**Acceptance criteria:**
- [ ] Checkpoint triggers at 10 sessions
- [ ] User can trigger manually anytime
- [ ] Report analyzes actual usage patterns
- [ ] Provides data-driven recommendations
- [ ] User can update design based on findings

**Priority:** MEDIUM (Process improvement from Skeptic)

**Dependencies:** R1.4 (archives), R2.1 (manifest)

**Implementation notes:**
- Checkpoint counter in manifest or separate state file
- Analysis script: `analyze-working-patterns.sh`
- Manual trigger: `analyze-working-patterns` (runs anytime)

---

## Requirement 3: Helper Scripts

### R3.1: clone-repo Script

**Purpose:** Clone repository to correct hierarchical location

**Usage:**
```bash
clone-repo <git-url>
clone-repo https://github.com/vbonnet/engram.git
# Clones to: ~/src/github/vbonnet/engram/
```

**Behavior:**
1. Parse Git URL to extract platform, username, repo name
2. Determine target path: `~/src/{platform}/{user}/{repo}`
3. Create parent directories if needed
4. Clone repository
5. Confirm success

**Acceptance criteria:**
- [ ] Parses GitHub URLs correctly
- [ ] Parses GitLab URLs correctly
- [ ] Creates correct directory structure
- [ ] Handles HTTPS and SSH URLs
- [ ] Reports errors clearly

**Priority:** MUST-HAVE (Critical for migration)

**Dependencies:** R1.1 (directory structure)

---

### R3.2: create-worktree Script

**Purpose:** Create git worktree in correct hierarchical location

**Usage:**
```bash
create-worktree <branch-name>
# Run from within a repo in ~/src/

create-worktree feature-bash-guidance
# Creates: ~/worktrees/github/vbonnet/engram/feature-bash-guidance/
```

**Behavior:**
1. Detect current repo (from pwd)
2. Parse repo location to determine platform/user/repo
3. Construct worktree path mirroring structure
4. Create worktree directories
5. Run `git worktree add`
6. Report success with path

**Acceptance criteria:**
- [ ] Detects repo from current directory
- [ ] Creates worktree in correct mirror location
- [ ] Handles new branch creation
- [ ] Handles existing branch checkout
- [ ] Reports clear errors if not in repo

**Priority:** MUST-HAVE (Critical for workflow)

**Dependencies:** R1.1 (repos), R1.2 (worktree structure)

---

### R3.3: archive-session Script

**Purpose:** Archive completed session to engram-research

**Usage:**
```bash
archive-session <session-id>
archive-session claude-abc123
```

**Behavior:**
1. Verify session exists in ~/.claude/sessions/
2. **Run sensitive data audit** (R2.3 - CRITICAL)
3. If audit passes or user approves:
   - Copy entire session dir to archives/{date}/{id}/
   - Update manifest status to "archived"
   - Commit to git in engram-research
   - Optionally delete from ~/.claude/sessions/ (prompt user)
4. Report success

**Acceptance criteria:**
- [ ] Archives complete session (manifest + working + artifacts)
- [ ] Runs sensitive data audit (CRITICAL)
- [ ] Updates manifest status
- [ ] Commits to git
- [ ] Prompts before deleting local copy
- [ ] Preserves all files (including working/)

**Priority:** HIGH (User Advocate - critical for UX)

**Dependencies:** R1.3 (sessions), R1.4 (archives), R2.3 (audit - CRITICAL)

**Implementation:** See R1.4 + R2.3 audit integration

---

### R3.4: session-dashboard Script

**Purpose:** Show all active and recent sessions

**Usage:**
```bash
session-dashboard
# Or: claude-sessions (shorter alias)
```

**Output:**
```
Active Sessions (3)
===================
claude-abc123 | bash-guidance-consolidation | S8-implementation
  Worktree: ~/worktrees/github/vbonnet/engram/feature-bash-guidance
  Last activity: 2 hours ago
  Next: Complete S8, run S9 validation

claude-def456 | workspace-design | D4-requirements
  Worktree: ~/worktrees/github/vbonnet/engram-research/workspace-design
  Last activity: 5 minutes ago
  Next: Finish D4 requirements

claude-ghi789 | retro-tasks-review | ad-hoc
  Worktree: (none - research session)
  Last activity: 1 day ago
  Next: Review WF-010 implementation

Recent Archives (5)
===================
2025-12-01 | claude-xyz789 | dotfiles-wayfinder | Completed
2025-11-30 | claude-mno456 | bash-guidance | Completed
...
```

**Acceptance criteria:**
- [ ] Lists all active sessions
- [ ] Shows key metadata (project, phase, worktree)
- [ ] Shows last activity time
- [ ] Shows next steps
- [ ] Lists recent archives
- [ ] Fast execution (< 1 second)

**Priority:** HIGH (User Advocate - discoverability)

**Dependencies:** R2.1 (manifest schema)

**Implementation:**
```bash
session-dashboard() {
  echo "Active Sessions ($(ls -1 $SESSIONS_ROOT | wc -l))"
  echo "==================="

  for session in $SESSIONS_ROOT/*; do
    local manifest="$session/manifest.yaml"
    if [[ -f "$manifest" ]]; then
      # Parse YAML (simple grep for now)
      local id=$(basename "$session")
      local project=$(grep '^project:' "$manifest" | cut -d: -f2- | xargs)
      local phase=$(grep 'last_phase:' "$manifest" | cut -d: -f2- | xargs)
      local worktree=$(grep 'path:' "$manifest" | head -1 | cut -d: -f2- | xargs)
      local last_activity=$(grep 'last_activity:' "$manifest" | cut -d: -f2- | xargs)
      local next=$(grep 'next_steps:' "$manifest" -A1 | tail -1 | xargs)

      echo "$id | $project | $phase"
      echo "  Worktree: $worktree"
      echo "  Last activity: $(time-ago "$last_activity")"
      echo "  Next: $next"
      echo
    fi
  done
}
```

---

### R3.5: resume-session Script

**Purpose:** Resume a session by reading manifest and setting context

**Usage:**
```bash
resume-session <session-id>
resume-session claude-abc123
```

**Behavior:**
1. Read manifest.yaml
2. cd to worktree (if exists)
3. Display resumption info:
   - Project name and type
   - Last phase
   - Next steps
   - Artifacts created
4. Optionally: Update last_activity timestamp

**Output:**
```
Resuming session: claude-abc123
Project: bash-guidance-consolidation (wayfinder)
Current phase: S8-implementation

Worktree: ~/worktrees/github/vbonnet/engram/feature-bash-guidance
Branch: feature/bash-guidance-consolidation

Last activity: 2 hours ago

Next steps:
  - Complete S8 implementation
  - Run S9 validation tests
  - Proceed to S10 deployment

Artifacts created:
  - S7-plan.md (2.5KB)
  - S8-implementation.md (8.3KB)

Ready to continue!
```

**Acceptance criteria:**
- [ ] Changes to correct worktree directory
- [ ] Displays all resumption info
- [ ] Human-readable output
- [ ] Updates last_activity (optional)
- [ ] Works even if worktree doesn't exist

**Priority:** SHOULD-HAVE (Improves resumption UX)

**Dependencies:** R2.1 (manifest schema)

---

### R3.6: cleanup-merged-worktrees Script

**Purpose:** Find and remove merged worktrees (integrates with gwq)

**Usage:**
```bash
cleanup-merged-worktrees
# Or use gwq directly:
gwq
```

**Behavior:**
1. Find all worktrees in ~/worktrees/
2. Check if branch is merged to base branch
3. If merged:
   - Show worktree + branch
   - Prompt: "Remove worktree and delete branch? [Y/n/s]"
   - Y: Remove worktree + delete branch
   - n: Skip
   - s: Skip all remaining
4. Report results

**Acceptance criteria:**
- [ ] Finds all worktrees
- [ ] Correctly detects merged branches
- [ ] Prompts before deletion
- [ ] Removes worktree directory
- [ ] Optionally deletes branch
- [ ] Reports what was cleaned

**Priority:** SHOULD-HAVE (Maintenance automation)

**Dependencies:** R1.2 (worktree structure), gwq tool

---

## Requirement 4: Migration Plan

### R4.1: Pre-Migration Backup

**Specification:**

Before migration, create backup of current state:

**What to backup:**
- All repos currently in /tmp/ and ~/
- All worktrees
- All session breadcrumbs (claude-XXXX-cwd files)
- Current directory structure snapshot

**Backup location:**
```bash
~/migration-backup-2025-12-02/
├── repos/           # Git repos
├── worktrees/       # Worktree directories
├── sessions/        # Session breadcrumbs
└── snapshot.txt     # Directory listing
```

**Acceptance criteria:**
- [ ] All current work backed up
- [ ] Backup includes git repos (full .git/)
- [ ] Can restore if migration fails
- [ ] Backup is timestamped

**Priority:** MUST-HAVE (Safety requirement)

**Dependencies:** None

**Implementation:**
```bash
backup-pre-migration() {
  local backup_dir=~/migration-backup-$(date +%Y-%m-%d)
  mkdir -p "$backup_dir"/{repos,worktrees,sessions}

  # Backup repos
  find /tmp ~/  -maxdepth 2 -type d -name .git -exec dirname {} \; | while read repo; do
    cp -r "$repo" "$backup_dir/repos/"
  done

  # Backup worktrees
  find ~ /tmp -type d -name .git -path "*/worktrees/*" -exec dirname {} \; | while read wt; do
    cp -r "$wt" "$backup_dir/worktrees/"
  done

  # Snapshot
  tree -L 3 ~ > "$backup_dir/snapshot.txt"

  echo "Backup created: $backup_dir"
}
```

---

### R4.2: Migration Script

**Specification:**

Full upfront migration in phases:

**Phase 1: Setup (15 min)**
1. Create directory structure (~/src/, ~/worktrees/, ~/.claude/sessions/)
2. Set environment variables
3. Install gwq tool
4. Verify structure

**Phase 2: Migrate Repositories (30-60 min)**
1. Find all git repos in /tmp/ and ~/
2. For each repo:
   - Determine correct location in ~/src/
   - Move repo (or clone fresh if safer)
   - Verify git remotes work
   - Update any references
3. Verify all repos accessible

**Phase 3: Migrate Worktrees (30-60 min)**
1. Find all worktrees
2. For each worktree:
   - Determine parent repo
   - Create correct mirror location in ~/worktrees/
   - Remove old worktree
   - Create new worktree in correct location
3. Verify all worktrees functional

**Phase 4: Create Session Manifests (30-60 min)**
1. Find active sessions (from breadcrumbs or Claude Code state)
2. For each session:
   - Create manifest.yaml
   - Populate basic metadata (best effort)
   - Link to worktree if applicable
3. Archive old completed sessions

**Phase 5: Verification (15 min)**
1. Verify all repos in ~/src/ work
2. Verify all worktrees in ~/worktrees/ work
3. Verify session manifests are valid
4. Run gwq to check visibility
5. Run session-dashboard to check sessions

**Phase 6: Cleanup (15 min)**
1. Review old locations (/tmp/, ~/)
2. Confirm everything migrated
3. Delete old locations (with confirmation)
4. Remove backup after verification period

**Total time:** 3-4 hours

**Acceptance criteria:**
- [ ] All repos in correct locations
- [ ] All worktrees in correct locations
- [ ] All sessions have manifests
- [ ] Nothing left in /tmp/ or scattered in ~/
- [ ] Verification passes
- [ ] Rollback possible until cleanup phase

**Priority:** MUST-HAVE (Core migration process)

**Dependencies:** R4.1 (backup), R3.1 (clone-repo), R3.2 (create-worktree)

**Implementation:**
- Single migration script: `migrate-workspace.sh`
- Phases run sequentially
- Can pause between phases
- Each phase reports progress
- Verification at end of each phase

---

### R4.3: Migration Verification Checklist

**Specification:**

Checklist to verify migration success:

```markdown
## Pre-Migration
- [ ] Backup created
- [ ] Backup verified (can list contents)
- [ ] Estimated time available (3-4 hours)

## Post-Phase 1: Setup
- [ ] ~/src/ exists
- [ ] ~/worktrees/ exists
- [ ] ~/.claude/sessions/ exists
- [ ] Environment variables set
- [ ] gwq installed and working

## Post-Phase 2: Repos
- [ ] All repos found and catalogued
- [ ] All repos moved to ~/src/{platform}/{user}/{repo}/
- [ ] Git status works in all repos
- [ ] Git remotes work (test git fetch)
- [ ] No repos remain in /tmp/ or ~/

## Post-Phase 3: Worktrees
- [ ] All worktrees found and catalogued
- [ ] All worktrees in ~/worktrees/{platform}/{user}/{repo}/{branch}/
- [ ] Git status works in all worktrees
- [ ] Worktrees linked to correct parent repos
- [ ] gwq shows all worktrees
- [ ] No worktrees remain in old locations

## Post-Phase 4: Sessions
- [ ] All active sessions identified
- [ ] Manifests created for all sessions
- [ ] Manifests are valid YAML
- [ ] Worktree links are correct
- [ ] Old sessions archived

## Post-Phase 5: Final Verification
- [ ] session-dashboard shows all sessions
- [ ] gwq shows all worktrees
- [ ] Can cd to any repo via tab completion
- [ ] Can cd to any worktree via gwq
- [ ] All git operations work

## Post-Phase 6: Cleanup
- [ ] Old /tmp/ locations empty
- [ ] Old ~/ locations empty
- [ ] Backup can be deleted (after N days)
```

**Acceptance criteria:**
- [ ] All checklist items pass
- [ ] User confirms each phase before proceeding
- [ ] Verification failures halt migration
- [ ] Can rollback from backup if needed

**Priority:** MUST-HAVE (Safety + Quality)

**Dependencies:** R4.2 (migration script)

---

### R4.4: Rollback Procedure

**Specification:**

If migration fails or user wants to revert:

**Steps:**
1. Stop migration (if in progress)
2. Restore from backup:
   ```bash
   restore-from-backup ~/migration-backup-2025-12-02
   ```
3. Remove partially migrated structure:
   ```bash
   rm -rf ~/src/ ~/worktrees/ ~/.claude/sessions/
   ```
4. Verify backup restoration
5. Investigate failure reason

**Acceptance criteria:**
- [ ] Can restore to pre-migration state
- [ ] Restore is complete (nothing missing)
- [ ] Git repos work after restore
- [ ] Can retry migration after fix

**Priority:** MUST-HAVE (Safety requirement)

**Dependencies:** R4.1 (backup exists)

---

## Requirement 5: Tool Integration

### R5.1: Install gwq

**Specification:**

Install gwq (git worktree manager) from https://github.com/d-kuro/gwq

**Installation method:**
```bash
# Option 1: Go install (if Go available)
go install github.com/d-kuro/gwq@latest

# Option 2: Download binary
wget https://github.com/d-kuro/gwq/releases/download/vX.Y.Z/gwq_linux_amd64.tar.gz
tar xzf gwq_linux_amd64.tar.gz
mv gwq ~/bin/
```

**Acceptance criteria:**
- [ ] gwq command available
- [ ] gwq can discover worktrees in ~/worktrees/
- [ ] gwq fuzzy finder works
- [ ] gwq can remove worktrees

**Priority:** SHOULD-HAVE (Enhances workflow)

**Dependencies:** None (standalone tool)

**Fallback:** If gwq unavailable, use git worktree built-ins

---

### R5.2: Configure gwq

**Specification:**

Configure gwq to use ~/worktrees/ as primary location

**Configuration:**
```bash
# Add to ~/.bashrc or ~/.zshrc
export GWQ_ROOT="$WORKTREES_ROOT"
alias gw='gwq'
```

**Acceptance criteria:**
- [ ] gwq searches ~/worktrees/ by default
- [ ] `gw` alias works
- [ ] Fuzzy finder shows all worktrees

**Priority:** SHOULD-HAVE

**Dependencies:** R5.1 (gwq installed)

---

## Requirement 6: Documentation

### R6.1: User Guide

**Specification:**

Create user guide for new workspace structure:

**Contents:**
1. Overview of structure
2. Directory layout diagram
3. Daily workflows:
   - Cloning a new repo
   - Creating a worktree
   - Starting a session
   - Resuming a session
   - Archiving completed work
4. Helper scripts reference
5. Troubleshooting

**Location:** `~/src/github/vbonnet/engram-research/workspace-management/USER-GUIDE.md`

**Acceptance criteria:**
- [ ] Covers all common workflows
- [ ] Includes examples
- [ ] Explains helper scripts
- [ ] Troubleshooting section

**Priority:** SHOULD-HAVE (Onboarding + reference)

**Dependencies:** All requirements (documents final system)

---

### R6.2: Quick Reference Card

**Specification:**

One-page quick reference for common commands:

```markdown
# Workspace Quick Reference

## Clone Repository
clone-repo <url>

## Create Worktree
cd ~/src/github/user/repo
create-worktree <branch>

## Session Management
session-dashboard           # List all sessions
resume-session <id>        # Resume a session
archive-session <id>       # Archive completed session

## Worktree Management
gwq                        # Browse worktrees (fuzzy finder)
cleanup-merged-worktrees   # Remove merged worktrees

## Environment
$SRC_ROOT                  # ~/src
$WORKTREES_ROOT            # ~/worktrees
$SESSIONS_ROOT             # ~/.claude/sessions
```

**Location:** `~/.workspace-quickref.md`

**Acceptance criteria:**
- [ ] Fits on one page
- [ ] Covers most common tasks
- [ ] Easy to scan

**Priority:** NICE-TO-HAVE

---

## Requirements Summary

### Must-Have (MVP)

| Requirement | Component | Estimated Time |
|-------------|-----------|----------------|
| R1.1-R1.4 | Directory structure | 30 min |
| R2.1 | Manifest schema | 1 hour |
| R2.3 | Sensitive data audit | 1 hour |
| R3.1 | clone-repo script | 30 min |
| R3.2 | create-worktree script | 30 min |
| R3.3 | archive-session script | 1 hour |
| R4.1-R4.2 | Migration + backup | 2 hours |
| R4.3 | Verification checklist | 30 min |

**Total MVP:** ~7.5 hours coding + 3-4 hours migration = **~11 hours**

---

### Should-Have (Enhanced)

| Requirement | Component | Estimated Time |
|-------------|-----------|----------------|
| R1.5 | Environment variables | 15 min |
| R2.2 | Manifest auto-update | 1 hour |
| R2.4 | Pattern checkpoint | 1 hour |
| R3.4 | session-dashboard | 1 hour |
| R3.5 | resume-session | 30 min |
| R3.6 | cleanup-merged-worktrees | 30 min |
| R5.1-R5.2 | gwq integration | 30 min |
| R6.1 | User guide | 1 hour |

**Total Enhanced:** ~6 hours

---

### Nice-to-Have (Polish)

| Requirement | Component | Estimated Time |
|-------------|-----------|----------------|
| R6.2 | Quick reference | 30 min |

**Total Polish:** ~30 min

---

## Implementation Priority Order

### Sprint 1: Core Structure (Week 1, ~8 hours)
1. R1.1-R1.4: Directory structure
2. R2.1: Manifest schema
3. R3.1-R3.2: clone-repo, create-worktree scripts
4. R4.1: Backup mechanism

**Deliverable:** Can create new structure, migrate repos manually

---

### Sprint 2: Migration (Week 1, ~4 hours + migration time)
1. R4.2: Migration script
2. R4.3: Verification checklist
3. R4.4: Rollback procedure
4. **Execute migration** (3-4 hours)

**Deliverable:** Fully migrated workspace

---

### Sprint 3: Session Management (Week 2, ~3 hours)
1. R2.3: Sensitive data audit (CRITICAL)
2. R3.3: archive-session script
3. R3.4: session-dashboard

**Deliverable:** Can track and archive sessions

---

### Sprint 4: Automation & Polish (Week 2-3, ~4 hours)
1. R2.2: Manifest auto-update
2. R2.4: Pattern checkpoint
3. R3.5-R3.6: resume-session, cleanup-merged-worktrees
4. R5.1-R5.2: gwq integration
5. R6.1: User guide

**Deliverable:** Fully automated, documented system

---

## Acceptance Criteria Summary

**System is ready when:**
- [ ] All repos in ~/src/{platform}/{user}/{repo}/
- [ ] All worktrees in ~/worktrees/{platform}/{user}/{repo}/{branch}/
- [ ] All sessions in ~/.claude/sessions/{id}/ with manifests
- [ ] Completed work archived to engram-research/session-archives/
- [ ] Helper scripts installed and working
- [ ] Sensitive data audit runs on archive
- [ ] Pattern checkpoint documented (triggers after 10 sessions)
- [ ] Migration completed successfully
- [ ] Verification checklist passes 100%
- [ ] User can perform all daily workflows
- [ ] Documentation complete

---

## Risk Mitigation

### Risk 1: Migration breaks active work
- **Mitigation:** R4.1 backup, R4.3 verification, R4.4 rollback
- **Status:** ✅ Mitigated

### Risk 2: Sensitive data in archives
- **Mitigation:** R2.3 audit (HIGH priority, Skeptic requirement)
- **Status:** ✅ Mitigated

### Risk 3: "For now" becomes "forever" (working/ bloat)
- **Mitigation:** R2.4 pattern checkpoint (MEDIUM priority, Skeptic requirement)
- **Status:** ✅ Mitigated

### Risk 4: Helper scripts not used (discipline decay)
- **Mitigation:** R3.4 dashboard (visibility), R6.1 user guide (onboarding)
- **Status:** ✅ Mitigated

### Risk 5: Incomplete migration
- **Mitigation:** R4.3 verification checklist, phase-by-phase approach
- **Status:** ✅ Mitigated

---

## Dependencies Graph

```
R1.1 (repos) ─────→ R1.2 (worktrees) ─────→ R3.2 (create-worktree)
    │                                            │
    └─────→ R3.1 (clone-repo)                   │
                                                 │
R1.3 (sessions) ──→ R2.1 (manifest) ─────→ R2.3 (audit) ─────→ R3.3 (archive)
    │                   │                        │
    │                   └─────→ R2.2 (auto-update)
    │                   └─────→ R3.4 (dashboard)
    │
    └─────→ R1.4 (archives) ─────→ R2.4 (checkpoint)

R4.1 (backup) ─────→ R4.2 (migration) ─────→ R4.3 (verification)
                                               │
                                               └─────→ R4.4 (rollback)

R5.1 (gwq) ─────→ R5.2 (config)
                  └─────→ R3.6 (cleanup)
```

**Critical path:** R1 → R4 (migration) → R2.3 (audit) → R3.3 (archive)

---

## Next Steps

**Immediate:**
1. Review D4 requirements with user
2. Confirm priorities (MVP vs Enhanced vs Nice-to-have)
3. Confirm estimated times are reasonable

**After D4 Approval:**
- Proceed to S4: Architecture Design
  - Detailed script architecture
  - File formats
  - API/interface design
- Then S5-S11: Implementation through deployment

---

**Status:** ✅ D4 REQUIREMENTS COMPLETE

**Next:** User review and approval

**Then:** S4 - Architecture Design

---
