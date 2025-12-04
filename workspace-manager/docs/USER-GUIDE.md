# Claude Code Workspace Management - User Guide

**Version**: 1.0
**Date**: 2025-12-03
**Status**: Production Ready

---

## Table of Contents

1. [Introduction](#introduction)
2. [Getting Started](#getting-started)
3. [Common Workflows](#common-workflows)
4. [Command Reference](#command-reference)
5. [Troubleshooting](#troubleshooting)
6. [Advanced Usage](#advanced-usage)
7. [Reference](#reference)
8. [FAQ](#faq)

---

## Introduction

### What is this?

The Claude Code Workspace Management system provides a structured approach to organizing and managing your Claude Code sessions across multiple git repositories and branches.

### Why use it?

**Problems it solves:**

- **No more lost work in /tmp/**: Session data is preserved in permanent locations
- **Clear organization**: Hierarchical directory structure mirrors git repository organization
- **Fast session resumption**: Quickly find and resume previous work with session manifests
- **Project context tracking**: Every session maintains metadata about repository, branch, and artifacts
- **Git-backed archival**: Archive completed sessions to git for long-term storage
- **Secret detection**: Automatic scanning prevents accidental commit of sensitive data

**Benefits:**

✅ Zero data loss from temporary directories
✅ Find any session instantly with hierarchical paths
✅ Resume work exactly where you left off
✅ Track all artifacts and working files per session
✅ Automatic backups via git archival
✅ Confidence that your work is safe and organized

### Key Concepts

**Hierarchical Directory Structure:**
```
~/src/{platform}/{user}/{repo}/                    # Bare git repositories
~/worktrees/{platform}/{user}/{repo}/{branch}/     # Git worktrees
~/sessions/{session-id}/                           # Session data
```

**Session Manifest (manifest.yaml):**
- YAML file tracking session metadata
- Auto-updates last_activity timestamp on access (R2.2)
- Contains repository URL, branch, commit, paths, status
- Tracked in git for archival

**Session States:**
- `active`: Currently in use or available for resumption
- `archived`: Committed to git, optionally pushed to remote

---

## Getting Started

### Prerequisites

- **Git**: Version 2.13+ (for worktree support)
- **Bash**: Version 4.0+
- **Claude Code**: Any recent version
- **Disk Space**: ~2x your current worktree size during migration

### Installation

The workspace management scripts are located in:
```
~/worktrees/github.com/vbonnet/engram-research/main/wayfinder-projects/workspace-design/workspace-management/
```

**No installation needed** - scripts run directly from the repository.

### First-Time Setup

**Step 1: Verify your current worktrees**

Check what worktrees you currently have:
```bash
find ~ -maxdepth 2 -type d -name ".git" 2>/dev/null | grep -v "/.git/" | head -10
```

**Step 2: Preview the migration (recommended)**

Run the migration script in dry-run mode to see what changes will be made:
```bash
cd ~/worktrees/github.com/vbonnet/engram-research/main/wayfinder-projects/workspace-design/workspace-management
./bin/migrate-workspace.sh --dry-run
```

This shows you:
- Which worktrees will be migrated
- Where they'll be placed in the new structure
- What session directories will be created
- Estimated disk space needed

**Step 3: Run the migration**

When you're ready to proceed:
```bash
./bin/migrate-workspace.sh
```

**What happens:**
1. Finds all existing git worktrees in your home directory
2. Parses repository URLs to determine platform/user/repo structure
3. Creates new hierarchical directories
4. Copies (not moves) worktrees to new locations
5. Creates session manifests with metadata
6. Verifies all operations succeeded

**Important:** The script **copies** your worktrees (original files remain unchanged) for safety.

**Step 4: Verify migration succeeded**

List all sessions created:
```bash
./bin/resume-session.sh --list
```

View details of a specific session:
```bash
./bin/resume-session.sh SESSION_ID
```

---

## Common Workflows

### Workflow 1: Migrate Existing Workspace

**Goal**: Move from ad-hoc worktree organization to structured hierarchy

**Steps:**

```bash
# 1. Navigate to workspace management directory
cd ~/worktrees/github.com/vbonnet/engram-research/main/wayfinder-projects/workspace-design/workspace-management

# 2. Preview migration (safe, no changes made)
./bin/migrate-workspace.sh --dry-run

# Review output:
# - Check worktrees found
# - Verify new paths look correct
# - Note disk space requirement

# 3. Run actual migration
./bin/migrate-workspace.sh

# Output will show:
# ✓ Found N worktrees
# ✓ Migrating each worktree...
# ✓ Creating session manifests...
# ✓ Verifying migration...
# ✓ Migration complete!

# 4. Verify sessions created
./bin/resume-session.sh --list

# 5. View dashboard of all sessions
./bin/session-dashboard.sh
```

**Expected outcome**: All worktrees copied to `~/worktrees/{platform}/{user}/{repo}/{branch}/`, session manifests created in `~/sessions/{session-id}/`

**Time**: 1-5 minutes depending on worktree count and size

---

### Workflow 2: Resume a Session

**Goal**: Continue working on a previous Claude Code session

**Steps:**

```bash
# 1. List all available sessions
./bin/resume-session.sh --list

# Output shows:
# 📁 github.com-vbonnet-engram-research-main
#    Repository: https://github.com/vbonnet/engram-research.git
#    Branch: main
#    Status: active
#    Created: 2025-12-03T10:30:00Z
#    Last activity: 2025-12-03T14:45:20Z

# 2. View detailed information for a session
./bin/resume-session.sh github.com-vbonnet-engram-research-main

# Output shows:
# 📍 Repository Information:
#    URL: https://github.com/vbonnet/engram-research.git
#    Branch: main
#    Commit: 246fccb
#
# 📂 Paths:
#    Worktree: ~/worktrees/github.com/vbonnet/engram-research/main
#    Session:  ~/sessions/github.com-vbonnet-engram-research-main
#    Working:  ~/sessions/github.com-vbonnet-engram-research-main/working
#    Artifacts: ~/sessions/github.com-vbonnet-engram-research-main/artifacts
#
# ✅ Worktree Status: Available
#    Current branch: main
#    Current commit: 246fccb

# 3. Change to worktree directory
cd ~/worktrees/github.com/vbonnet/engram-research/main

# 4. Start Claude Code in this directory
# (Command depends on your Claude Code installation)
# The session files are automatically available in ~/sessions/SESSION_ID/
```

**Expected outcome**: You're in the correct working directory with access to previous session's working files and artifacts

**Time**: < 1 minute

---

### Workflow 3: Archive Completed Work

**Goal**: Save a completed session to git for long-term storage

**Scenario 3a: Archive locally only**

```bash
# Archive session (creates git commit)
./bin/archive-session.sh github.com-vbonnet-engram-research-main

# Output shows:
# Step 1: Pre-archive secret audit (R2.3)...
# ✓ No sensitive data detected
#
# Step 2: Initialize git repository...
# ✓ Git repository initialized
#
# Step 3: Create archive commit...
# ✓ Archive commit created
#
# Updating manifest status...
# ✓ Manifest updated
#
# ✓ Archive complete!
# Session files are archived in:
#   ~/sessions/github.com-vbonnet-engram-research-main/.git
```

**Scenario 3b: Archive and push to remote**

```bash
# First, configure remote (one-time setup per session)
cd ~/sessions/github.com-vbonnet-engram-research-main
git remote add origin https://github.com/user/session-archive.git

# Then archive and push
cd ~/worktrees/github.com/vbonnet/engram-research/main/wayfinder-projects/workspace-design/workspace-management
./bin/archive-session.sh --push github.com-vbonnet-engram-research-main

# Output includes:
# Step 4: Push to remote...
# ✓ Pushed to origin/main
```

**Scenario 3c: Archive, push, and cleanup local files**

```bash
# Archive, push to remote, and remove local working/artifacts files
./bin/archive-session.sh --push --cleanup github.com-vbonnet-engram-research-main

# Output includes:
# Step 5: Cleanup local files...
# Remove working/ and artifacts/ contents? (y/N): y
#   Removed working/ contents
#   Removed artifacts/ contents
# ✓ Cleanup complete
#
# Local files cleaned up
```

**Expected outcome**: Session archived in git, manifest status updated to "archived", optionally pushed to remote and cleaned up

**Time**: < 1 minute (excluding network time for push)

---

### Workflow 4: Manage Multiple Sessions

**Goal**: View, filter, and organize multiple active sessions

**Steps:**

```bash
# 1. View all sessions in dashboard
./bin/session-dashboard.sh

# Output:
# ╔════════════════════════════════════════════════════════════════╗
# ║                    Session Dashboard                           ║
# ╚════════════════════════════════════════════════════════════════╝
#
# Session ID                                    Status     Branch          Last Activity
# ─────────────────────────────────────────────────────────────────────────────────────
# github.com-vbonnet-engram-research-main       active     main            2025-12-03 14:45
# github.com-user-test-project-feature          archived   feature-branch  2025-12-02 16:30
#
# Summary:
#   Total sessions: 2
#   Active: 1
#   Archived: 1
#   Disk usage: ~125 MB

# 2. Filter by status (show only active sessions)
./bin/session-dashboard.sh --status active

# 3. Filter by repository pattern
./bin/session-dashboard.sh --repo engram-research

# 4. Show sessions active since specific date
./bin/session-dashboard.sh --since 2025-12-01

# 5. Sort by creation date (instead of last activity)
./bin/session-dashboard.sh --sort created

# 6. Combine filters
./bin/session-dashboard.sh --status active --repo engram --sort activity
```

**Expected outcome**: Clear overview of all sessions with filtering and sorting capabilities

**Time**: < 10 seconds

---

## Command Reference

### migrate-workspace.sh

**Purpose**: Migrate existing git worktrees to new hierarchical structure

**Usage:**
```bash
migrate-workspace.sh [OPTIONS]
```

**Options:**

| Option | Description |
|--------|-------------|
| `--dry-run` | Preview changes without making them |
| `--base DIR` | Override worktrees base directory (default: `~/worktrees`) |
| `--sessions-base DIR` | Override sessions base directory (default: `~/sessions`) |
| `--help` | Show help message |

**Examples:**
```bash
# Preview migration
./bin/migrate-workspace.sh --dry-run

# Run migration
./bin/migrate-workspace.sh

# Migrate with custom directories
./bin/migrate-workspace.sh --base /custom/worktrees --sessions-base /custom/sessions
```

**Exit codes:**
- `0`: Success
- `1`: Error (validation failed, migration failed, or verification failed)

---

### resume-session.sh

**Purpose**: Display session information for resuming Claude Code work

**Usage:**
```bash
resume-session.sh [OPTIONS] [SESSION_ID]
```

**Options:**

| Option | Description |
|--------|-------------|
| `--list` | List all available sessions |
| `--sessions-base DIR` | Override sessions directory (default: `~/sessions`) |
| `--help` | Show help message |

**Examples:**
```bash
# List all sessions
./bin/resume-session.sh --list

# Show session details
./bin/resume-session.sh github.com-vbonnet-engram-research-main

# Use custom sessions directory
./bin/resume-session.sh --sessions-base /custom/sessions SESSION_ID
```

**What it does:**
- Displays repository URL, branch, commit
- Shows worktree path, session path, working path, artifacts path
- Verifies worktree still exists and is valid git repo
- Warns if current branch doesn't match manifest
- Lists artifacts from previous session
- Updates last_activity timestamp (R2.2)

**Exit codes:**
- `0`: Success
- `1`: Error (session not found or invalid)

---

### archive-session.sh

**Purpose**: Archive session to git with optional push and cleanup

**Usage:**
```bash
archive-session.sh [OPTIONS] SESSION_ID
```

**Options:**

| Option | Description |
|--------|-------------|
| `--push` | Push archive to remote repository |
| `--cleanup` | Remove local files after archiving |
| `--dry-run` | Preview what would be archived |
| `--remote NAME` | Git remote name (default: `origin`) |
| `--branch NAME` | Git branch name (default: `main`) |
| `--sessions-base DIR` | Override sessions directory (default: `~/sessions`) |
| `--help` | Show help message |

**Examples:**
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

**What it does:**
1. **Pre-archive secret audit (R2.3)**: Scans for sensitive data
2. Initializes git repository if needed
3. Creates git commit of all session files
4. Optionally pushes to remote (with `--push`)
5. Optionally removes working/ and artifacts/ contents (with `--cleanup`)
6. Updates manifest status to "archived"

**Secret Detection Patterns (R2.3):**
- API keys and tokens
- AWS credentials
- Private keys (SSH, PGP)
- Passwords and secrets
- Database connection strings
- OAuth tokens
- Generic secrets (password=, secret=, token=)

**Exit codes:**
- `0`: Success
- `1`: Error (session not found, secrets detected and not confirmed, git operations failed)

---

### session-dashboard.sh

**Purpose**: Interactive dashboard for viewing and managing all sessions

**Usage:**
```bash
session-dashboard.sh [OPTIONS]
```

**Options:**

| Option | Description |
|--------|-------------|
| `--status STATUS` | Filter by status (`active` or `archived`) |
| `--repo PATTERN` | Filter by repository URL pattern |
| `--since DATE` | Show sessions active since date (YYYY-MM-DD) |
| `--sort FIELD` | Sort by field (`created`, `activity`, `status`) [default: `activity`] |
| `--sessions-base DIR` | Override sessions directory (default: `~/sessions`) |
| `--help` | Show help message |

**Examples:**
```bash
# Show all sessions
./bin/session-dashboard.sh

# Show only active sessions
./bin/session-dashboard.sh --status active

# Filter by repository
./bin/session-dashboard.sh --repo engram-research

# Show recent sessions
./bin/session-dashboard.sh --since 2025-12-01

# Sort by creation date
./bin/session-dashboard.sh --sort created

# Combine filters
./bin/session-dashboard.sh --status active --repo engram --sort activity
```

**Output format:**
- Table with session ID, status (color-coded), branch, last activity
- Summary statistics: total sessions, active count, archived count
- Disk usage estimate (for ≤10 sessions)
- Quick reference commands

**Exit codes:**
- `0`: Success
- `1`: Error (invalid options)

---

## Troubleshooting

### Issue: "Worktree not found"

**Symptoms:**
```
❌ Worktree not found: ~/worktrees/github.com/user/repo/branch
```

**Cause**: The worktree directory was moved, renamed, or deleted after session was created

**Solutions:**

1. **If worktree was moved:** Update the manifest manually
   ```bash
   # Edit manifest.yaml
   nano ~/sessions/SESSION_ID/manifest.yaml

   # Update the worktree.path field to new location
   ```

2. **If worktree was deleted:** Re-clone the repository
   ```bash
   # Clone to correct location
   git clone https://github.com/user/repo.git ~/worktrees/github.com/user/repo/branch
   cd ~/worktrees/github.com/user/repo/branch
   git checkout branch
   ```

3. **If you don't need this session:** Archive it and move on
   ```bash
   ./bin/archive-session.sh --push SESSION_ID
   ```

---

### Issue: "Secrets detected in session"

**Symptoms:**
```
⚠️  WARNING: Sensitive data detected in session
Detected patterns in: working/config.yml
  - Line 12: api_key=sk_live_XXXXXXXXXXXX
```

**Cause**: Session contains files with sensitive data (API keys, passwords, tokens, etc.)

**Solutions:**

1. **Remove the sensitive data:**
   ```bash
   # Edit the file to remove secrets
   nano ~/sessions/SESSION_ID/working/config.yml

   # Replace with environment variable or placeholder
   # Before: api_key=sk_live_XXXXXXXXXXXX
   # After:  api_key=${API_KEY}

   # Try archiving again
   ./bin/archive-session.sh SESSION_ID
   ```

2. **If false positive:** Confirm and proceed anyway
   ```
   # The script will ask for confirmation
   Continue with archive despite sensitive data? (y/N): y
   ```

3. **Add to .gitignore:** Prevent sensitive files from being archived
   ```bash
   cd ~/sessions/SESSION_ID
   echo "working/config.yml" >> .gitignore
   ```

**Prevention**: Always use environment variables or secret management for credentials

---

### Issue: "No sessions found"

**Symptoms:**
```
⚠️  WARNING: No sessions found in: ~/sessions
```

**Cause**: Haven't run migration script yet, or sessions are in different directory

**Solutions:**

1. **Run migration:**
   ```bash
   ./bin/migrate-workspace.sh
   ```

2. **Check custom location:** If sessions are elsewhere, use `--sessions-base`
   ```bash
   ./bin/resume-session.sh --sessions-base /path/to/sessions --list
   ```

---

### Issue: "Git push failed"

**Symptoms:**
```
❌ Failed to push to remote
```

**Cause**: Remote repository not configured or authentication failed

**Solutions:**

1. **Configure remote:**
   ```bash
   cd ~/sessions/SESSION_ID
   git remote add origin https://github.com/user/session-archive.git
   ```

2. **Check authentication:**
   ```bash
   # Test git access
   git ls-remote origin

   # If using HTTPS, update credentials
   git credential fill

   # If using SSH, check keys
   ssh -T git@github.com
   ```

3. **Archive without push:** Archive locally first, push manually later
   ```bash
   # Archive locally
   ./bin/archive-session.sh SESSION_ID

   # Push manually when ready
   cd ~/sessions/SESSION_ID
   git push origin main
   ```

---

### Issue: "Branch mismatch detected"

**Symptoms:**
```
⚠️  Branch mismatch! Expected: feature-branch, Found: main
```

**Cause**: Git worktree checked out different branch than what's in manifest

**Solutions:**

1. **Checkout correct branch:**
   ```bash
   cd ~/worktrees/github.com/user/repo/branch
   git checkout feature-branch
   ```

2. **Update manifest:** If the branch change is intentional
   ```bash
   nano ~/sessions/SESSION_ID/manifest.yaml
   # Update worktree.branch field
   ```

---

### Issue: "Disk space full during migration"

**Symptoms:**
```
❌ Failed to copy worktree: No space left on device
```

**Cause**: Insufficient disk space (migration requires ~2x worktree size)

**Solutions:**

1. **Check available space:**
   ```bash
   df -h ~
   ```

2. **Free up space:**
   ```bash
   # Remove large files
   du -sh ~/Downloads/* | sort -hr | head -10

   # Clean package caches
   sudo apt-get clean  # Ubuntu/Debian
   brew cleanup        # macOS
   ```

3. **Migrate to external drive:**
   ```bash
   # Use custom base directories on larger drive
   ./bin/migrate-workspace.sh \
     --base /mnt/external/worktrees \
     --sessions-base /mnt/external/sessions
   ```

---

## Advanced Usage

### Custom Project Root (S9 Enhancement)

**Use case**: Organize all workspace components under a custom directory instead of `~/`

**Overview**: By setting the `WORKSPACE_PROJECT_ROOT` environment variable, you can specify a base directory for all workspace components (worktrees, sessions, src). This is useful for:
- Cleaner home directory organization
- Easier backup (just backup one directory)
- Multiple project contexts (work/personal)
- Client-specific organization

**Setup (one-time):**

Add to your shell configuration file (`~/.bashrc`, `~/.zshrc`, or `~/.config/fish/config.fish`):

```bash
# For bash/zsh:
export WORKSPACE_PROJECT_ROOT=~/my-project

# For fish:
set -x WORKSPACE_PROJECT_ROOT ~/my-project
```

Then reload your shell configuration:
```bash
# Bash:
source ~/.bashrc

# Zsh:
source ~/.zshrc

# Fish:
source ~/.config/fish/config.fish
```

**Directory Structure**:

After setting `WORKSPACE_PROJECT_ROOT=~/my-project`, all workspace components will live under that directory:
```
~/my-project/
├── worktrees/
│   └── github.com/user/repo/branch/
├── sessions/
│   └── github.com-user-repo-branch/
└── src/
    └── github.com/user/repo/
```

**Examples:**

**Scenario 1: Work and Personal Projects**
```bash
# In ~/.bashrc:
alias work-mode='export WORKSPACE_PROJECT_ROOT=~/work'
alias personal-mode='export WORKSPACE_PROJECT_ROOT=~/personal'

# Switch contexts:
$ work-mode
$ ./bin/session-dashboard.sh --status active
# Shows only work sessions from ~/work/sessions/

$ personal-mode
$ ./bin/session-dashboard.sh --status active
# Shows only personal sessions from ~/personal/sessions/
```

**Scenario 2: Client-Specific Organization**
```bash
# In ~/.bashrc:
export WORKSPACE_PROJECT_ROOT=~/clients/acme

# All client work lives under:
# ~/clients/acme/worktrees/
# ~/clients/acme/sessions/
# ~/clients/acme/src/
```

**Precedence**:

When determining the base directory, scripts follow this precedence (highest to lowest):
1. **Command-line flags** (`--sessions-base`, `--base`) - highest priority
2. **Environment variable** (`WORKSPACE_PROJECT_ROOT`)
3. **Default** (`~/`) - lowest priority

Example:
```bash
# Environment variable sets base:
export WORKSPACE_PROJECT_ROOT=~/my-project
./bin/resume-session.sh --list
# Uses: ~/my-project/sessions/

# Command-line flag overrides environment variable:
./bin/resume-session.sh --sessions-base /custom/path --list
# Uses: /custom/path/
```

**Migration**:

**Option 1: Fresh start with environment variable**
```bash
# Set environment variable
export WORKSPACE_PROJECT_ROOT=~/my-project

# Run migration (will use new location)
./bin/migrate-workspace.sh
# Creates: ~/my-project/worktrees/, ~/my-project/sessions/
```

**Option 2: Move existing data**
```bash
# Create new project directory
mkdir -p ~/my-project

# Move existing directories
mv ~/sessions ~/my-project/sessions
mv ~/worktrees ~/my-project/worktrees
mv ~/src ~/my-project/src

# Set environment variable
export WORKSPACE_PROJECT_ROOT=~/my-project

# Scripts automatically find everything
./bin/session-dashboard.sh
```

**Automation Considerations**:

When using scripts in cron jobs or systemd services, the environment variable may not be set. Use explicit flags:
```bash
# In cron:
0 2 * * * /path/to/archive-session.sh --sessions-base ~/my-project/sessions --push SESSION_ID

# Or set in crontab:
WORKSPACE_PROJECT_ROOT=/home/user/my-project
0 2 * * * /path/to/archive-session.sh --push SESSION_ID
```

**Troubleshooting**:

**Q: I set the environment variable but scripts still use `~/`**

A: Ensure you:
1. Added `export WORKSPACE_PROJECT_ROOT=...` to the correct shell config file
2. Reloaded your shell (`source ~/.bashrc` or restart terminal)
3. Verify with `echo $WORKSPACE_PROJECT_ROOT`

**Q: Different behavior in different terminals**

A: You may be using different shells (bash vs zsh vs fish). Add the export to each shell's config file.

**Q: Scripts in cron don't see the environment variable**

A: Cron doesn't load shell configuration. Either:
- Set the variable in crontab directly
- Use explicit `--sessions-base` flags

---

### Custom Session Directories

**Use case**: Store sessions on different drive or shared location

**Setup:**
```bash
# Set environment variables (add to ~/.bashrc for persistence)
export WORKTREES_BASE="/mnt/external/worktrees"
export SESSIONS_BASE="/mnt/external/sessions"

# All scripts support --sessions-base override
./bin/resume-session.sh --sessions-base "$SESSIONS_BASE" --list
./bin/archive-session.sh --sessions-base "$SESSIONS_BASE" SESSION_ID
./bin/session-dashboard.sh --sessions-base "$SESSIONS_BASE"
```

---

### Multiple Git Remotes

**Use case**: Push session archives to multiple backup locations

**Setup:**
```bash
cd ~/sessions/SESSION_ID

# Add multiple remotes
git remote add origin https://github.com/user/primary.git
git remote add backup https://gitlab.com/user/backup.git
git remote add local /mnt/backup/sessions/SESSION_ID.git

# Push to all remotes
git push origin main
git push backup main
git push local main

# Or create push-all script
cat > push-all.sh << 'EOF'
#!/bin/bash
for remote in origin backup local; do
  echo "Pushing to $remote..."
  git push "$remote" main || echo "Failed: $remote"
done
EOF
chmod +x push-all.sh
```

**Using with archive script:**
```bash
# Push to specific remote
./bin/archive-session.sh --push --remote backup SESSION_ID
```

---

### Automated Archiving Scripts

**Use case**: Archive all active sessions older than 30 days automatically

**Script:**
```bash
#!/bin/bash
# auto-archive.sh - Archive old sessions

SESSIONS_BASE="$HOME/sessions"
DAYS_OLD=30
CUTOFF_DATE=$(date -d "$DAYS_OLD days ago" +%Y-%m-%d)

# Find sessions older than cutoff
for manifest in "$SESSIONS_BASE"/*/manifest.yaml; do
  session_dir=$(dirname "$manifest")
  session_id=$(basename "$session_dir")

  # Get last activity date
  last_activity=$(grep "^last_activity:" "$manifest" | cut -d' ' -f2- | cut -d'T' -f1)

  # Compare dates
  if [[ "$last_activity" < "$CUTOFF_DATE" ]]; then
    echo "Archiving old session: $session_id (last activity: $last_activity)"
    ./bin/archive-session.sh --push "$session_id"
  fi
done

echo "Auto-archive complete"
```

**Cron job:** Run weekly
```bash
# Add to crontab (crontab -e)
0 2 * * 0 /path/to/auto-archive.sh >> /var/log/auto-archive.log 2>&1
```

---

### Integration with CI/CD

**Use case**: Automatically test session archival in CI pipeline

**GitHub Actions example:**
```yaml
# .github/workflows/archive-sessions.yml
name: Archive Sessions

on:
  schedule:
    - cron: '0 0 * * 0'  # Weekly on Sunday
  workflow_dispatch:

jobs:
  archive:
    runs-on: ubuntu-latest

    steps:
      - name: Checkout
        uses: actions/checkout@v3

      - name: Setup Git
        run: |
          git config --global user.name "Session Archiver"
          git config --global user.email "archive@example.com"

      - name: Archive sessions
        run: |
          cd wayfinder-projects/workspace-design/workspace-management

          # Archive all active sessions
          for session in ~/sessions/*; do
            session_id=$(basename "$session")
            ./bin/archive-session.sh --push "$session_id" || true
          done

      - name: Report results
        run: |
          ./bin/session-dashboard.sh --status archived
```

---

## Reference

### Directory Structure

**Complete hierarchy:**
```
~
├── src/                                    # Bare git repositories
│   └── {platform}/                         # github.com, gitlab.com, etc.
│       └── {user}/                         # GitHub username or org
│           └── {repo}/                     # Repository name
│               ├── HEAD
│               ├── config
│               ├── objects/
│               └── refs/
│
├── worktrees/                              # Git worktrees (actual code)
│   └── {platform}/
│       └── {user}/
│           └── {repo}/
│               └── {branch}/               # Branch name or commit hash
│                   ├── .git                # Git worktree link
│                   ├── src/
│                   └── ...
│
└── sessions/                               # Session data
    └── {session-id}/                       # Format: platform-user-repo-branch
        ├── manifest.yaml                   # Session metadata
        ├── working/                        # Temporary work files
        ├── artifacts/                      # Session artifacts
        └── .git/                           # Git repo (after archival)
```

**Example:**
```
~/src/github.com/vbonnet/engram-research/
~/worktrees/github.com/vbonnet/engram-research/main/
~/sessions/github.com-vbonnet-engram-research-main/
```

---

### Manifest File Format

**Location**: `~/sessions/{session-id}/manifest.yaml`

**Structure:**
```yaml
# Session Manifest
session_id: github.com-vbonnet-engram-research-main
created_at: 2025-12-03T10:30:00Z
last_activity: 2025-12-03T14:45:20Z

repository:
  url: https://github.com/vbonnet/engram-research.git
  path: main

worktree:
  path: /home/user/worktrees/github.com/vbonnet/engram-research/main
  branch: main
  commit: 246fccb

artifacts: []

status: active  # active | archived
```

**Fields:**

| Field | Type | Description | Auto-updated |
|-------|------|-------------|--------------|
| `session_id` | string | Unique session identifier | No |
| `created_at` | ISO 8601 | Session creation timestamp | No |
| `last_activity` | ISO 8601 | Last access timestamp | Yes (R2.2) |
| `repository.url` | string | Git repository URL | No |
| `repository.path` | string | Branch or ref path | No |
| `worktree.path` | string | Absolute path to worktree | No |
| `worktree.branch` | string | Current branch name | No |
| `worktree.commit` | string | Current commit hash | No |
| `artifacts` | array | List of artifact filenames | No |
| `status` | enum | Session state (active/archived) | Yes |
| `archived_at` | ISO 8601 | Archive timestamp | Yes (on archive) |

**Auto-update mechanism (R2.2):**
- `resume-session.sh` updates `last_activity` on every access
- `archive-session.sh` sets `status: archived` and `archived_at`

---

### Secret Detection Patterns (R2.3)

The archive script scans for these patterns before committing:

| Pattern | Regex | Example Match |
|---------|-------|---------------|
| API Keys | `[Aa][Pp][Ii]_?[Kk][Ee][Yy].*['"]?[A-Za-z0-9]{20,}` | `api_key="sk_live_xxxxx"` |
| AWS Credentials | `AKIA[0-9A-Z]{16}` | `AKIAIOSFODNN7EXAMPLE` |
| Private Keys | `-----BEGIN.*PRIVATE KEY-----` | SSH/PGP key headers |
| Passwords | `[Pp][Aa][Ss][Ss][Ww][Oo][Rr][Dd].*['"]?[^ \t\n]{8,}` | `password="secret123"` |
| Database URLs | `(mongodb|postgres|mysql)://[^@]+@` | `postgres://user:pass@host` |
| OAuth Tokens | `[Oo][Aa][Uu][Tt][Hh].*['"]?[A-Za-z0-9\-._~+/]+=*` | `oauth_token="xxxxx"` |
| Generic Secrets | `(secret|token|key).*['"]?[A-Za-z0-9]{16,}` | `secret_key="xxxxx"` |

**Exclusions:**
- Git repository URLs (`https://...` in URLs)
- Configuration templates with placeholders

**What to do if detected:**
1. Review the flagged files
2. Remove or replace sensitive data with environment variables
3. Re-run archive script
4. Consider adding files to `.gitignore`

---

### Command-Line Options Summary

**Common options across all scripts:**

| Option | Scripts | Description |
|--------|---------|-------------|
| `--help` | All | Show help message and exit |
| `--sessions-base DIR` | All | Override sessions directory |
| `--dry-run` | migrate, archive | Preview without making changes |

**Script-specific options:**

**migrate-workspace.sh:**
- `--base DIR`: Override worktrees directory
- `--dry-run`: Preview migration

**resume-session.sh:**
- `--list`: List all sessions

**archive-session.sh:**
- `--push`: Push to remote after archiving
- `--cleanup`: Remove local files after archiving
- `--remote NAME`: Git remote name (default: origin)
- `--branch NAME`: Git branch name (default: main)
- `--dry-run`: Preview archive

**session-dashboard.sh:**
- `--status STATUS`: Filter by status (active/archived)
- `--repo PATTERN`: Filter by repository pattern
- `--since DATE`: Filter by date (YYYY-MM-DD)
- `--sort FIELD`: Sort by field (created/activity/status)

---

## FAQ

### Q: What happens to my old worktrees after migration?

**A:** The migration script **copies** worktrees to the new location, leaving originals untouched. This is for safety - you can manually delete old worktrees after verifying the migration succeeded.

**Cleanup:**
```bash
# Verify new worktrees work
cd ~/worktrees/github.com/user/repo/main
git status

# If everything looks good, remove old worktree
rm -rf ~/old-worktree-location
```

---

### Q: Can I migrate the same worktree twice?

**A:** Yes, but it will create duplicate sessions with different session IDs. The script doesn't detect duplicates - it processes all worktrees found in your home directory.

**Best practice:** Run migration once, then manually manage worktrees using the new structure.

---

### Q: How do I delete a session?

**A:** Sessions are just directories - you can delete them directly:

```bash
# Archive first (recommended)
./bin/archive-session.sh --push SESSION_ID

# Then remove session directory
rm -rf ~/sessions/SESSION_ID
```

**Warning:** Deleting a session removes all working files and artifacts. Archive first if you might need them later.

---

### Q: How much disk space do sessions use?

**A:** It depends on your working files and artifacts. Use the dashboard to check:

```bash
./bin/session-dashboard.sh
# Shows: Disk usage: ~125 MB (for all sessions combined)
```

**Typical sizes:**
- Manifest only: <1 KB
- With working files: 1-100 MB
- With large artifacts: 100 MB - 1 GB+

**Space-saving tips:**
- Archive and cleanup old sessions: `--push --cleanup`
- Store large files outside session directories
- Use `.gitignore` to exclude temporary files from archives

---

### Q: Can I use this with non-Claude Code projects?

**A:** Yes! The workspace management system works with any git repository. The "Claude Code session" concept is just organizational - the scripts work with standard git worktrees and YAML manifests.

**Use cases:**
- Organize multiple git worktrees by repository
- Track work-in-progress across branches
- Archive project states at milestones
- Manage multiple feature branches

---

### Q: What if my repository URL changes?

**A:** Update the manifest manually:

```bash
# Edit manifest
nano ~/sessions/SESSION_ID/manifest.yaml

# Update repository.url field
repository:
  url: https://github.com/new-user/new-repo.git

# Update worktree remote
cd ~/worktrees/github.com/old-user/old-repo/main
git remote set-url origin https://github.com/new-user/new-repo.git
```

---

### Q: Can I have multiple sessions for the same repository?

**A:** Yes! Each branch can have its own session:

```
~/sessions/github.com-user-repo-main/
~/sessions/github.com-user-repo-feature-branch/
~/sessions/github.com-user-repo-bugfix/
```

Session IDs include the branch name, so each branch gets a unique session.

---

### Q: What's the difference between "active" and "archived" sessions?

**A:**

| Status | Description | Files | Use case |
|--------|-------------|-------|----------|
| `active` | Work in progress | working/, artifacts/ present | Current or resumable sessions |
| `archived` | Committed to git | Optionally cleaned up | Completed work, historical reference |

**State transitions:**
```
active → (archive) → archived
```

Once archived, a session stays archived (no "unarchive" command, but you can manually change manifest).

---

### Q: How do I restore an archived session from remote?

**A:**

```bash
# Clone archived session from remote
git clone https://github.com/user/session-archive.git ~/sessions/SESSION_ID

# Check manifest
cat ~/sessions/SESSION_ID/manifest.yaml

# Restore worktree if needed
cd ~/worktrees/github.com/user/repo/branch
git checkout COMMIT_HASH_FROM_MANIFEST
```

---

### Q: Can I run multiple migrations in parallel?

**A:** No, the migration script should be run **one at a time**. It doesn't implement file locking, so running multiple instances could cause conflicts.

**Reason:** Documented in S7 retrospective - locking not implemented for simplicity.

---

### Q: What happens if the script crashes mid-migration?

**A:** The migration is **atomic per worktree** - either a worktree fully migrates or it doesn't. If the script crashes:

1. Already-migrated worktrees are complete and valid
2. In-progress worktree may be partially copied
3. Remaining worktrees not started

**Recovery:**
```bash
# Check what was migrated
./bin/resume-session.sh --list

# Remove partial session if any
rm -rf ~/sessions/incomplete-session-id

# Re-run migration (will process remaining worktrees)
./bin/migrate-workspace.sh
```

---

### Q: How do I customize the session ID format?

**A:** Session IDs are auto-generated from repository URLs:

**Format:** `{platform}-{user}-{repo}-{branch}`

**Example:** `github.com-vbonnet-engram-research-main`

**To customize:** You'd need to modify the `parse_git_url` and `build_session_id` functions in `lib/path-utils.sh`. This is not recommended as scripts expect the standard format.

---

### Q: How do I organize multiple project workspaces? (S9)

**A:** Use the `WORKSPACE_PROJECT_ROOT` environment variable to set a custom base directory for all workspace components.

**Example:**
```bash
# In ~/.bashrc:
export WORKSPACE_PROJECT_ROOT=~/my-project

# All workspace components now live under ~/my-project/:
# - ~/my-project/worktrees/
# - ~/my-project/sessions/
# - ~/my-project/src/
```

See [Custom Project Root](#custom-project-root-s9-enhancement) in Advanced Usage for complete documentation.

---

### Q: Can I have different workspace roots for different projects? (S9)

**A:** Yes! Use shell aliases to switch contexts:

```bash
# In ~/.bashrc:
alias work-mode='export WORKSPACE_PROJECT_ROOT=~/work'
alias personal-mode='export WORKSPACE_PROJECT_ROOT=~/personal'

# Switch to work projects:
$ work-mode
$ ./bin/session-dashboard.sh  # Shows work sessions

# Switch to personal projects:
$ personal-mode
$ ./bin/session-dashboard.sh  # Shows personal sessions
```

---

### Q: The WORKSPACE_PROJECT_ROOT environment variable isn't working

**A:** Common issues:

1. **Not reloaded shell:**
   ```bash
   # After adding to ~/.bashrc:
   source ~/.bashrc

   # Or restart your terminal
   ```

2. **Wrong shell config file:**
   - Bash: `~/.bashrc`
   - Zsh: `~/.zshrc`
   - Fish: `~/.config/fish/config.fish`

3. **Verify it's set:**
   ```bash
   echo $WORKSPACE_PROJECT_ROOT
   # Should show your custom path
   ```

4. **Check precedence:**
   Command-line flags override the environment variable:
   ```bash
   # This ignores WORKSPACE_PROJECT_ROOT:
   ./bin/resume-session.sh --sessions-base /custom/path
   ```

---

## Getting Help

**Documentation:**
- User Guide (this document)
- `S7-COMPLETE.md`: Migration script details
- `S8-PROGRESS-SESSION-CONTINUATION.md`: Session management implementation

**Command help:**
```bash
./bin/migrate-workspace.sh --help
./bin/resume-session.sh --help
./bin/archive-session.sh --help
./bin/session-dashboard.sh --help
```

**Issues:**
- Report bugs to repository: https://github.com/vbonnet/engram-research
- Include: script name, command run, error output, `git --version`, `bash --version`

**Tips for effective bug reports:**
1. Run with `set -x` for detailed trace: `bash -x bin/script-name.sh`
2. Check shellcheck output: `shellcheck bin/script-name.sh`
3. Verify git configuration: `git config --list`
4. Check disk space: `df -h ~`

---

**Document Version**: 1.0
**Last Updated**: 2025-12-03
**Status**: Production Ready
