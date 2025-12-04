# D3: Approach Selection - Claude Session Resumption Tool

**Date**: 2025-12-03
**Phase**: Wayfinder D3 - Approach Selection
**Project**: Claude Session Resumption with Tmux Integration
**Status**: ✅ COMPLETE (Updated with CLI architecture)

---

## Executive Summary

**Objective**: Finalize technical decisions and detailed design based on D2 solutions search.

**Scope**: Select final approaches for all major components and create detailed implementation specifications.

**Outcome**: Ready-to-implement design with clear component interfaces, implementation sequence, and risk mitigation strategies.

**UPDATED**: Post-D3 user decision to adopt unified CLI architecture (see CLI-UNIFICATION-REVIEW.md for full rationale).

---

## 1. Final Approach Selections

Based on D2 analysis and user feedback, here are the final technical decisions:

### 1.1 JSON Parsing (history.jsonl)

**SELECTED**: Hybrid Approach (Python3 → Grep/Sed Fallback)

**Rationale**:
- **Primary**: Python3 provides robust JSON parsing (handles escaping, format changes)
- **Fallback**: Grep/sed for environments without python3
- **Risk mitigation**: Format validation before parsing
- **Performance**: Acceptable (50ms for 296 entries with python, 10ms with grep)

**Implementation**:
```bash
parse_history_jsonl() {
    local history_file="$1"

    # Validate file exists and has content
    if [[ ! -f "$history_file" ]] || [[ ! -s "$history_file" ]]; then
        log_error "history.jsonl not found or empty: $history_file"
        return 1
    fi

    # Try python3 first (most robust)
    if command -v python3 &>/dev/null; then
        python3 -c "
import json, sys
for line in sys.stdin:
    try:
        obj = json.loads(line)
        sid = obj.get('sessionId', '')
        proj = obj.get('project', '')
        ts = obj.get('timestamp', '')
        if sid:
            print(f'{sid}|{proj}|{ts}')
    except:
        pass
" < "$history_file" 2>/dev/null

        if [[ $? -eq 0 ]]; then
            return 0
        fi

        log_warn "Python parsing failed, falling back to grep/sed"
    fi

    # Fallback: Validate JSON Lines format
    local first_line=$(head -1 "$history_file")
    if [[ ! "$first_line" =~ ^\{.*\}$ ]]; then
        log_error "Invalid JSON Lines format in history.jsonl"
        return 1
    fi

    # Extract fields using grep/sed
    paste -d'|' \
        <(grep -o '"sessionId":"[^"]*"' "$history_file" | sed 's/"sessionId":"\([^"]*\)"/\1/') \
        <(grep -o '"project":"[^"]*"' "$history_file" | sed 's/"project":"\([^"]*\)"/\1/') \
        <(grep -o '"timestamp":[0-9]*' "$history_file" | sed 's/"timestamp"://')
}
```

**Review Condition Addressed**: ✅ Condition #4 (Format validation)

---

### 1.2 Tmux Control Mechanism

**SELECTED**: new-session with Command (User-Suggested)

**Rationale**:
- **Simplest**: One command creates session and starts Claude
- **Most reliable**: No timing issues, atomic operation
- **Best UX**: Automatic attach to session
- **User-validated**: User suggested this approach based on real-world usage

**Implementation**:
```bash
ensure_tmux_and_resume() {
    local session_name="$1"
    local worktree_path="$2"
    local claude_uuid="$3"

    # Check if session exists
    if ! tmux has-session -t "$session_name" 2>/dev/null; then
        # Create new session with Claude command
        log_info "Creating tmux session '$session_name' with Claude"
        tmux new-session -d -s "$session_name" \
            -c "$worktree_path" \
            "claude --resume $claude_uuid"
    else
        # Session exists - just attach
        log_info "Attaching to existing tmux session: $session_name"
    fi

    # Attach to the session (automatic per user request)
    tmux attach -t "$session_name"

    # Log action (Review Condition #6)
    log_resume_action "$session_name" "$claude_uuid" "attached"
}
```

**Review Conditions Addressed**:
- ✅ Condition #1 ELIMINATED (no sleep needed)
- ✅ Condition #6 (Resume action logging)

**Review Condition Deferred**: ⚠️ Condition #7 (Empty tmux detection - defer until proven needed)

---

### 1.3 Library Architecture

**SELECTED**: Two New Libraries + Extend Existing

**Rationale**:
- **Modularity**: Clear separation of concerns
- **Testability**: Each library can be tested independently
- **Consistency**: Follows existing workspace management patterns
- **Maintainability**: Easy to understand and extend

**Structure**:
```
lib/
├── claude-discovery.sh (NEW ~300 lines)
│   ├── discover_claude_sessions()
│   ├── parse_history_jsonl()
│   ├── find_manifest_by_claude_uuid()
│   ├── find_manifest_by_tmux_name()
│   ├── validate_claude_session_dirs()
│   └── match_sessions_to_manifests()
│
├── tmux-utils.sh (NEW ~200 lines)
│   ├── ensure_tmux_and_resume()
│   ├── get_unique_tmux_name()
│   ├── check_tmux_session_exists()
│   ├── list_tmux_sessions()
│   └── log_resume_action()
│
└── manifest-utils.sh (EXTEND +100 lines)
    ├── read_claude_session_id()
    ├── update_claude_metadata()
    ├── read_tmux_session_name()
    └── update_tmux_metadata()
```

**Total New Code**: ~600 lines of library code

---

### 1.4 CLI Architecture (Post-D3 Decision)

**SELECTED**: Unified CLI Tool

**Rationale**:
- **User concern**: "This is starting to be quite a few separate .sh scripts that I need to remember"
- **Review result**: 5/5 unanimous approval from all personas (9.1/10 confidence)
- **Benefits**: Better discoverability, consistency, tab completion, industry standard
- **ROI**: 1.3-11x in first year

**Architecture**:
```
session                    # Main CLI dispatcher (~100 lines)
├── lib/                   # Shared libraries (unchanged)
│   ├── common-utils.sh
│   ├── claude-discovery.sh
│   ├── tmux-utils.sh
│   └── manifest-utils.sh
├── commands/              # Command implementations
│   ├── migrate.sh        # Wraps migrate-workspace logic
│   ├── resume.sh         # Unified resume (workspace + Claude)
│   ├── archive.sh        # Wraps archive-session logic
│   ├── dashboard.sh      # Enhanced dashboard
│   ├── sync.sh           # Sync both workspace and Claude
│   └── list.sh           # List all sessions
└── completions/          # Shell completions
    ├── session.bash
    └── session.zsh
```

**User Interface**:
```bash
# Unified interface (smart detection)
session migrate ~/worktrees/...
session resume claude-1              # Auto-detects tmux name
session resume github.com-user-repo  # Auto-detects workspace ID
session resume c86ffd41-...          # Auto-detects Claude UUID
session archive github.com-user-repo
session dashboard
session sync                         # Sync both workspace and Claude
session list                         # List all sessions

# Global options
session --help
session --version
session resume --help
```

**Main Dispatcher Design**:
```bash
#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMANDS_DIR="$SCRIPT_DIR/commands"

show_help() {
    cat <<EOF
Usage: session <command> [options]

Commands:
  migrate     Migrate workspace to hierarchical structure
  resume      Resume a session (auto-detects workspace or Claude)
  archive     Archive a session
  dashboard   Interactive session dashboard
  sync        Sync Claude sessions with manifests
  list        List all sessions

Options:
  -h, --help     Show this help
  -v, --version  Show version
  --verbose      Verbose output

Run 'session <command> --help' for command-specific help.
EOF
}

# Dispatch to command
command="$1"
shift || true

case "$command" in
    migrate|resume|archive|dashboard|sync|list)
        if [[ -f "$COMMANDS_DIR/$command.sh" ]]; then
            exec bash "$COMMANDS_DIR/$command.sh" "$@"
        else
            echo "Error: Command implementation not found: $command" >&2
            exit 1
        fi
        ;;
    -h|--help|help)
        show_help
        ;;
    -v|--version)
        echo "session version 2.0.0"
        ;;
    "")
        show_help
        exit 1
        ;;
    *)
        echo "Error: Unknown command: $command" >&2
        echo "Run 'session help' for usage." >&2
        exit 1
        ;;
esac
```

**Implementation Overhead**: +2-3 hours (Phase 0)

**See**: CLI-UNIFICATION-REVIEW.md for complete multi-persona analysis

---

### 1.5 Manifest Schema Extension

**SELECTED**: YAML Extension (v2.0)

**Schema Addition**:
```yaml
# Existing manifest fields
session_id: github.com-user-repo-branch
repository:
  url: https://github.com/user/repo
  platform: github.com
worktree:
  path: /home/user/worktrees/github.com/user/repo/branch
  branch: branch
created_at: 2025-12-01T09:26:26Z
last_activity: 2025-12-03T17:30:00Z

# NEW: Claude integration (optional)
claude:
  session_id: c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2
  session_env_path: /home/user/.claude/session-env/c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2
  file_history_path: /home/user/.claude/file-history/c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2
  started_at: 2025-12-01T18:04:00Z
  last_activity: 2025-12-03T17:30:00Z

# NEW: Tmux integration (optional)
tmux:
  session_name: claude-1
  window_name: main
  created_at: 2025-12-01T09:26:26Z
```

**Backward Compatibility**: ✅ Both sections are optional
**Schema Version**: Will add `schema_version: "2.0"` field

---

### 1.6 Testing Strategy

**SELECTED**: Comprehensive BATS Testing

**Coverage Plan**:

**Unit Tests** (~550 lines, 45 tests):
- `claude-discovery.bats`: 20 tests (parsing, validation, discovery)
- `tmux-utils.bats`: 15 tests (session control, logging)
- `manifest-utils-extensions.bats`: 10 tests (field readers/writers)

**Integration Tests** (~500 lines, 45 tests):
- `session-cli.bats`: 8 tests (CLI dispatcher, help, unknown commands)
- `session-resume.bats`: 25 tests (end-to-end resume workflows)
- `session-sync.bats`: 12 tests (discovery and migration)

**Total**: ~80 tests (~1,050 lines)

**Coverage Goal**: 90%+ on critical paths

---

## 2. Component Design

### 2.1 claude-discovery.sh Library

**Purpose**: Parse history.jsonl and discover Claude sessions

**Public Functions**:

```bash
# Discover all Claude sessions from history.jsonl
# Returns: List of "uuid|project|timestamp" entries
discover_claude_sessions() {
    local history_file="${1:-$HOME/.claude/history.jsonl}"
    parse_history_jsonl "$history_file"
}

# Parse history.jsonl (hybrid approach)
# Returns: Pipe-separated values: sessionId|project|timestamp
parse_history_jsonl() {
    local history_file="$1"
    # Implementation from 1.1 above
}

# Validate Claude session directories exist and have content
# Args: $1=uuid
# Returns: 0 if valid, 1 if invalid
validate_claude_session_dirs() {
    local uuid="$1"

    local session_env="$HOME/.claude/session-env/$uuid"
    local file_history="$HOME/.claude/file-history/$uuid"

    # Check session-env exists and has content
    if [[ ! -d "$session_env" ]] || [[ -z "$(ls -A "$session_env" 2>/dev/null)" ]]; then
        log_warn "Missing or empty session-env: $session_env"
        return 1
    fi

    # file-history is optional (may not exist for new sessions)
    return 0
}

# Find manifest by Claude UUID
# Args: $1=uuid
# Returns: Path to manifest or empty string
find_manifest_by_claude_uuid() {
    local uuid="$1"
    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"

    # Search all manifests for matching Claude session_id
    grep -l "session_id: $uuid" "$sessions_dir"/*/manifest.yaml 2>/dev/null | head -1
}

# Find manifest by tmux session name
# Args: $1=tmux_name
# Returns: Path to manifest or empty string
find_manifest_by_tmux_name() {
    local tmux_name="$1"
    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"

    # Search all manifests for matching tmux session_name
    grep -l "session_name: $tmux_name" "$sessions_dir"/*/manifest.yaml 2>/dev/null | head -1
}

# Match Claude sessions to existing manifests by worktree path
# Args: stdin with "uuid|project|timestamp" entries
# Returns: Matched and orphaned sessions
match_sessions_to_manifests() {
    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"

    while IFS='|' read -r uuid project timestamp; do
        # Try to find manifest with matching worktree path
        local manifest=$(grep -l "path: $project" "$sessions_dir"/*/manifest.yaml 2>/dev/null | head -1)

        if [[ -n "$manifest" ]]; then
            echo "MATCHED|$uuid|$project|$manifest"
        else
            echo "ORPHAN|$uuid|$project|"
        fi
    done
}
```

**Review Condition Addressed**: ✅ Condition #3 (Validate Claude session directories)

---

### 2.2 tmux-utils.sh Library

**Purpose**: Control tmux sessions and log resume actions

**Public Functions**:

```bash
# Ensure tmux session exists and resume Claude
# Args: $1=session_name, $2=worktree_path, $3=claude_uuid
ensure_tmux_and_resume() {
    # Implementation from 1.2 above
}

# Get unique tmux session name (handle conflicts)
# Args: $1=desired_name
# Returns: Available name (may add -alt, -2, etc.)
get_unique_tmux_name() {
    local desired="$1"

    if ! tmux has-session -t "$desired" 2>/dev/null; then
        echo "$desired"
        return 0
    fi

    # Conflict: offer alternatives
    echo "Tmux session '$desired' already exists." >&2
    echo "Options:" >&2
    echo "  1. Use alternate name: ${desired}-alt" >&2
    echo "  2. Attach to existing session" >&2
    echo "  3. Cancel" >&2
    read -p "Select option (1-3): " -n 1 -r choice >&2
    echo "" >&2

    case "$choice" in
        1) echo "${desired}-alt" ;;
        2) echo "$desired" ;;
        3) return 1 ;;
        *) return 1 ;;
    esac
}

# Check if tmux session exists
# Args: $1=session_name
# Returns: 0 if exists, 1 if not
check_tmux_session_exists() {
    tmux has-session -t "$1" 2>/dev/null
}

# List all tmux sessions
# Returns: Session names, one per line
list_tmux_sessions() {
    tmux list-sessions -F "#{session_name}" 2>/dev/null || true
}

# Log resume action to audit trail
# Args: $1=session_id, $2=claude_uuid, $3=action, $4=details (optional)
log_resume_action() {
    local session_id="$1"
    local claude_uuid="$2"
    local action="$3"
    local details="${4:-}"

    local log_file="${RESUME_LOG:-$HOME/sessions/.resume-log}"
    local timestamp=$(date -Iseconds)

    # Create log file if doesn't exist
    if [[ ! -f "$log_file" ]]; then
        mkdir -p "$(dirname "$log_file")"
        echo "# Resume action log" > "$log_file"
        echo "# Format: timestamp | session_id | claude_uuid | action | details" >> "$log_file"
    fi

    # Append entry
    echo "$timestamp | $session_id | $claude_uuid | $action | $details" >> "$log_file"
}
```

**Review Condition Addressed**: ✅ Condition #6 (Resume action logging)

---

### 2.3 manifest-utils.sh Extensions

**Purpose**: Read/write Claude and tmux fields in manifests

**New Functions**:

```bash
# Read Claude session ID from manifest
# Args: $1=manifest_path
# Returns: Claude session UUID or empty string
read_claude_session_id() {
    local manifest="$1"
    grep "^  session_id:" "$manifest" 2>/dev/null | awk '{print $2}'
}

# Update Claude metadata in manifest
# Args: $1=manifest_path, $2=uuid, $3=started_at, $4=last_activity
update_claude_metadata() {
    local manifest="$1"
    local uuid="$2"
    local started_at="$3"
    local last_activity="$4"

    local session_env="$HOME/.claude/session-env/$uuid"
    local file_history="$HOME/.claude/file-history/$uuid"

    # Check if claude section exists
    if grep -q "^claude:" "$manifest"; then
        # Update existing
        sed -i "s|^  session_id:.*|  session_id: $uuid|" "$manifest"
        sed -i "s|^  last_activity:.*|  last_activity: $last_activity|" "$manifest"
    else
        # Add new section
        cat >> "$manifest" <<EOF

claude:
  session_id: $uuid
  session_env_path: $session_env
  file_history_path: $file_history
  started_at: $started_at
  last_activity: $last_activity
EOF
    fi
}

# Read tmux session name from manifest
# Args: $1=manifest_path
# Returns: Tmux session name or empty string
read_tmux_session_name() {
    local manifest="$1"
    grep "^  session_name:" "$manifest" 2>/dev/null | awk '{print $2}'
}

# Update tmux metadata in manifest
# Args: $1=manifest_path, $2=session_name, $3=window_name, $4=created_at
update_tmux_metadata() {
    local manifest="$1"
    local session_name="$2"
    local window_name="${3:-main}"
    local created_at="$4"

    # Check if tmux section exists
    if grep -q "^tmux:" "$manifest"; then
        # Update existing
        sed -i "s|^  session_name:.*|  session_name: $session_name|" "$manifest"
    else
        # Add new section
        cat >> "$manifest" <<EOF

tmux:
  session_name: $session_name
  window_name: $window_name
  created_at: $created_at
EOF
    fi
}
```

---

## 3. CLI Command Designs

### 3.1 session resume (commands/resume.sh)

**Purpose**: Resume any session by identifier (unified workspace + Claude)

**Usage**:
```bash
session resume claude-1                    # By tmux name (auto-detects Claude)
session resume github.com-user-repo-main   # By workspace ID (auto-detects workspace)
session resume c86ffd41-cbcc-4bfa-8b1f...  # By Claude UUID (auto-detects Claude)
```

**Flow**:
```
1. Parse arguments
2. Resolve identifier → find manifest
3. Read manifest (worktree path, Claude UUID, tmux name)
4. Health checks:
   - Validate Claude session directories exist
   - Check worktree exists (offer recovery if deleted)
   - Validate manifest format
5. Ensure tmux session (create or attach)
6. Update manifest last_activity timestamp
7. Log resume action
8. Attach to tmux (automatic)
```

**Detailed Algorithm**:

```bash
#!/bin/bash

set -euo pipefail

# Source libraries
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="$SCRIPT_DIR/../lib"

source "$LIB_DIR/common-utils.sh"
source "$LIB_DIR/path-utils.sh"
source "$LIB_DIR/manifest-utils.sh"
source "$LIB_DIR/claude-discovery.sh"
source "$LIB_DIR/tmux-utils.sh"

# Constants
SESSIONS_DIR="${SESSIONS_DIR:-$HOME/sessions}"

main() {
    local identifier="$1"

    # Step 1: Resolve identifier to manifest
    local manifest_path=$(resolve_session_identifier "$identifier")

    if [[ -z "$manifest_path" ]]; then
        log_error "Session not found: $identifier"

        # Review Condition #2: Offer auto-sync
        echo ""
        echo "Possible reasons:"
        echo "  - Session hasn't been discovered yet"
        echo "  - Session was created outside workspace management"
        echo "  - Manifest is out of sync"
        echo ""
        read -p "Run session-sync to discover sessions? (y/N): " -n 1 -r
        echo ""

        if [[ $REPLY =~ ^[Yy]$ ]]; then
            session sync

            # Retry resolution
            manifest_path=$(resolve_session_identifier "$identifier")
            if [[ -z "$manifest_path" ]]; then
                log_error "Session still not found after sync"
                return 1
            fi
        else
            echo "You can run: session sync"
            return 1
        fi
    fi

    # Step 2: Read manifest
    local session_id=$(basename "$(dirname "$manifest_path")")
    local worktree_path=$(read_manifest_field "$manifest_path" "worktree.path")
    local claude_uuid=$(read_claude_session_id "$manifest_path")
    local tmux_name=$(read_tmux_session_name "$manifest_path")

    if [[ -z "$claude_uuid" ]]; then
        log_error "No Claude session associated with: $session_id"
        return 1
    fi

    # Step 3: Health checks

    # Check Claude session directories (Review Condition #3)
    if ! validate_claude_session_dirs "$claude_uuid"; then
        log_error "Claude session directories invalid or missing"
        echo ""
        echo "Recovery options:"
        echo "  1. Session may be corrupted - try creating a new one"
        echo "  2. Session may have been cleaned up - archive this manifest"
        echo ""
        return 1
    fi

    # Check worktree exists (CWD deleted bug recovery)
    if [[ ! -d "$worktree_path" ]]; then
        log_warn "Worktree directory not found: $worktree_path"
        offer_cwd_recovery "$manifest_path" "$worktree_path"
        return $?
    fi

    # Check manifest corruption (Review Condition #8)
    if ! detect_manifest_corruption "$manifest_path"; then
        recover_corrupted_manifest "$manifest_path"
        return $?
    fi

    # Step 4: Ensure tmux session and resume
    ensure_tmux_and_resume "$tmux_name" "$worktree_path" "$claude_uuid"

    # Step 5: Update manifest last_activity
    update_claude_metadata "$manifest_path" "$claude_uuid" \
        "$(read_manifest_field "$manifest_path" "claude.started_at")" \
        "$(date -Iseconds)"

    log_success "Resumed Claude session: $session_id"
}

# Resolve identifier to manifest path
resolve_session_identifier() {
    local identifier="$1"

    # Try 1: Exact workspace session ID
    if [[ -d "$SESSIONS_DIR/$identifier" ]]; then
        echo "$SESSIONS_DIR/$identifier/manifest.yaml"
        return 0
    fi

    # Try 2: Claude UUID
    local manifest=$(find_manifest_by_claude_uuid "$identifier")
    if [[ -n "$manifest" ]]; then
        echo "$manifest"
        return 0
    fi

    # Try 3: Tmux session name
    manifest=$(find_manifest_by_tmux_name "$identifier")
    if [[ -n "$manifest" ]]; then
        echo "$manifest"
        return 0
    fi

    # Try 4: Partial match on session ID
    local matches=("$SESSIONS_DIR"/*"$identifier"*/manifest.yaml)
    if [[ ${#matches[@]} -eq 1 ]] && [[ -f "${matches[0]}" ]]; then
        echo "${matches[0]}"
        return 0
    elif [[ ${#matches[@]} -gt 1 ]]; then
        log_error "Ambiguous identifier '$identifier' matches multiple sessions"
        return 1
    fi

    # Not found
    return 1
}

# CWD deleted bug recovery
offer_cwd_recovery() {
    local manifest_path="$1"
    local worktree_path="$2"

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Worktree Directory Not Found"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "Directory: $worktree_path"
    echo ""
    echo "Recovery options:"
    echo "  1. Recreate worktree (if git worktree still registered)"
    echo "  2. Use fallback directory (~/sessions/.../working)"
    echo "  3. Archive session and start fresh"
    echo "  4. Cancel"
    echo ""
    read -p "Select option (1-4): " -n 1 -r
    echo ""

    case "$REPLY" in
        1)
            # Try to recreate worktree
            local repo_url=$(read_manifest_field "$manifest_path" "repository.url")
            local branch=$(read_manifest_field "$manifest_path" "worktree.branch")

            # Check if worktree is still registered
            if git -C "$(dirname "$worktree_path")" worktree list 2>/dev/null | grep -q "$worktree_path"; then
                git -C "$(dirname "$worktree_path")" worktree repair "$worktree_path"
            else
                mkdir -p "$worktree_path"
                # Note: Full recreation requires source repo - may need user guidance
                log_warn "Worktree recreation requires source repository"
            fi
            ;;
        2)
            # Use fallback directory
            local session_id=$(basename "$(dirname "$manifest_path")")
            local fallback="$SESSIONS_DIR/$session_id/working"
            mkdir -p "$fallback"
            log_info "Using fallback directory: $fallback"
            # Update worktree_path for this resume
            worktree_path="$fallback"
            ;;
        3)
            # Archive
            log_info "Run: session archive $(basename "$(dirname "$manifest_path")")"
            return 1
            ;;
        4)
            log_info "Cancelled"
            return 1
            ;;
    esac
}

# Manifest corruption detection and recovery (Review Condition #8)
detect_manifest_corruption() {
    local manifest="$1"

    # Try to parse YAML (basic check)
    if ! grep -q "^session_id:" "$manifest" 2>/dev/null; then
        return 1
    fi

    return 0
}

recover_corrupted_manifest() {
    local manifest_path="$1"
    local session_id=$(basename "$(dirname "$manifest_path")")

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Manifest Corruption Detected"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "Session: $session_id"
    echo "Manifest: $manifest_path"
    echo ""
    echo "Recovery options:"
    echo "  1. Backup and regenerate manifest (recommended)"
    echo "  2. Attempt manual repair"
    echo "  3. Cancel"
    echo ""
    read -p "Select option (1-3): " -n 1 -r
    echo ""

    case "$REPLY" in
        1)
            # Backup
            local backup="$manifest_path.corrupted.$(date +%s)"
            mv "$manifest_path" "$backup"
            log_info "Backed up to: $backup"

            log_warn "Manual regeneration required"
            log_info "Run: session sync"
            return 1
            ;;
        2)
            # Open in editor
            ${EDITOR:-nano} "$manifest_path"
            ;;
        3)
            log_info "Cancelled"
            return 1
            ;;
    esac
}

# Help text
show_help() {
    cat <<EOF
Usage: session resume <identifier>

Resume any session by identifier (auto-detects workspace or Claude).

Identifiers:
  - Tmux session name:  session resume claude-1
  - Workspace ID:       session resume github.com-user-repo-branch
  - Claude UUID:        session resume c86ffd41-cbcc-4bfa-8b1f...

The command will:
  1. Find the session manifest
  2. Validate session health
  3. Create/attach tmux session
  4. Resume Claude automatically
  5. Update activity timestamp

Examples:
  session resume claude-1
  session resume github.com-vbonnet-engram-research-main

Options:
  -h, --help    Show this help message
EOF
}

# Main entry point
if [[ $# -eq 0 ]] || [[ "$1" == "-h" ]] || [[ "$1" == "--help" ]]; then
    show_help
    exit 0
fi

main "$1"
```

**Estimated Lines**: ~350 lines

**Review Conditions Addressed**:
- ✅ Condition #2 (Auto-sync offer)
- ✅ Condition #3 (Validate Claude directories)
- ✅ Condition #8 (Corruption recovery)

---

### 3.2 session sync (commands/sync.sh)

**Purpose**: Discover Claude sessions and sync with manifests

**Usage**:
```bash
session sync          # Discover all sessions
session sync --auto   # Auto-map when unambiguous
```

**Flow**:
```
1. Parse history.jsonl
2. Discover all Claude sessions
3. Match to existing manifests by worktree path
4. Identify orphans:
   - Claude sessions without manifests
   - Manifests without Claude sessions
5. For each orphaned Claude session:
   - Show progress: "Mapping session 3/10"
   - Display session info (UUID, project, last activity)
   - Prompt: Map to workspace? (y/N/s=skip all)
   - Create manifest if confirmed
6. Summary report
```

**Detailed Algorithm**:

```bash
#!/bin/bash

set -euo pipefail

# Source libraries
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="$SCRIPT_DIR/../lib"

source "$LIB_DIR/common-utils.sh"
source "$LIB_DIR/claude-discovery.sh"
source "$LIB_DIR/manifest-utils.sh"

SESSIONS_DIR="${SESSIONS_DIR:-$HOME/sessions}"

main() {
    log_info "Discovering Claude sessions..."

    # Step 1: Discover all Claude sessions
    local sessions=$(discover_claude_sessions)

    if [[ -z "$sessions" ]]; then
        log_info "No Claude sessions found in history.jsonl"
        return 0
    fi

    # Step 2: Match sessions to manifests
    local matched=()
    local orphaned=()

    while IFS='|' read -r uuid project timestamp; do
        local manifest=$(find_manifest_by_claude_uuid "$uuid")

        if [[ -n "$manifest" ]]; then
            matched+=("$uuid|$project|$timestamp|$manifest")
        else
            # Try to find by worktree path
            manifest=$(grep -l "path: $project" "$SESSIONS_DIR"/*/manifest.yaml 2>/dev/null | head -1)

            if [[ -n "$manifest" ]]; then
                matched+=("$uuid|$project|$timestamp|$manifest")
            else
                orphaned+=("$uuid|$project|$timestamp")
            fi
        fi
    done <<< "$sessions"

    # Step 3: Report findings
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Session Discovery Report"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "Matched sessions: ${#matched[@]}"
    echo "Orphaned sessions: ${#orphaned[@]}"
    echo ""

    if [[ ${#orphaned[@]} -eq 0 ]]; then
        log_success "All sessions are mapped!"
        return 0
    fi

    # Step 4: Migrate orphaned sessions (Review Condition #5)
    migrate_orphaned_sessions "${orphaned[@]}"
}

# Migrate orphaned sessions with progress tracking
migrate_orphaned_sessions() {
    local orphan_sessions=("$@")
    local total=${#orphan_sessions[@]}
    local current=0

    echo "Found $total orphaned Claude sessions to map"
    echo ""

    for session_data in "${orphan_sessions[@]}"; do
        current=$((current + 1))

        # Parse session data
        IFS='|' read -r uuid project timestamp <<< "$session_data"

        # Progress indicator (Review Condition #5)
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "Mapping session $current/$total"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
        echo "Claude UUID: $uuid"
        echo "Project: $project"
        echo "Last activity: $(format_timestamp "$timestamp")"
        echo ""

        # Validate session directories
        if ! validate_claude_session_dirs "$uuid"; then
            log_warn "Skipping invalid session"
            echo ""
            continue
        fi

        # Prompt for mapping
        read -p "Map to workspace session? (y/N/s=skip all): " -n 1 -r
        echo ""

        if [[ $REPLY =~ ^[Ss]$ ]]; then
            echo "Skipping remaining sessions"
            break
        elif [[ $REPLY =~ ^[Yy]$ ]]; then
            map_session_to_workspace "$uuid" "$project" "$timestamp"
        else
            echo "Skipped"
        fi

        echo ""
    done

    echo "Migration complete: $current of $total processed"
}

# Map session to workspace
map_session_to_workspace() {
    local uuid="$1"
    local project="$2"
    local timestamp="$3"

    # Generate session ID from project path
    local session_id=$(generate_session_id_from_path "$project")

    echo "Creating manifest for: $session_id"

    # Create session directory
    local session_dir="$SESSIONS_DIR/$session_id"
    mkdir -p "$session_dir"

    # Create basic manifest
    cat > "$session_dir/manifest.yaml" <<EOF
session_id: $session_id
repository:
  url: unknown
  platform: unknown
worktree:
  path: $project
  branch: unknown
created_at: $(date -Iseconds)
last_activity: $(date -Iseconds)

claude:
  session_id: $uuid
  session_env_path: $HOME/.claude/session-env/$uuid
  file_history_path: $HOME/.claude/file-history/$uuid
  started_at: $(format_timestamp "$timestamp")
  last_activity: $(format_timestamp "$timestamp")
EOF

    log_success "Created: $session_dir/manifest.yaml"
}

# Format timestamp (milliseconds to ISO)
format_timestamp() {
    local ms="$1"
    date -d "@$((ms / 1000))" -Iseconds 2>/dev/null || echo "unknown"
}

# Generate session ID from path
generate_session_id_from_path() {
    local path="$1"
    basename "$path" | tr '/' '-'
}

# Help text
show_help() {
    cat <<EOF
Usage: session sync [options]

Discover Claude sessions and sync with workspace manifests.

The command will:
  1. Parse ~/.claude/history.jsonl
  2. Discover all Claude sessions
  3. Match to existing manifests
  4. Identify orphaned sessions
  5. Offer to create manifests for orphans

Options:
  -h, --help    Show this help message

Examples:
  session sync
EOF
}

if [[ "${1:-}" == "-h" ]] || [[ "${1:-}" == "--help" ]]; then
    show_help
    exit 0
fi

main
```

**Estimated Lines**: ~250 lines

**Review Condition Addressed**: ✅ Condition #5 (Migration progress tracking)

---

### 3.3 session list (commands/list.sh)

**Purpose**: Quick view of all sessions (unified workspace + Claude)

**Usage**:
```bash
session list
session list --active    # Only active sessions
session list --stale     # Only stale sessions
session list --claude    # Only Claude sessions
session list --workspace # Only workspace sessions
```

**Output**:
```
Claude Sessions

UUID (truncated)     | Workspace ID              | Tmux    | Last Activity
c86ffd41-cbcc-4bfa  | github.com-user-repo-main | claude-1| 2025-12-03 17:30
abc12345-def6-7890  | github.com-user-repo-feat | claude-2| 2025-12-02 10:15
...
```

**Estimated Lines**: ~150 lines

---

## 4. Implementation Sequence

### Phase 0: CLI Framework (2-3 hours) ← NEW

**Deliverables**:
1. Create main `session` dispatcher script
2. Create commands/ directory structure
3. Create command wrapper template
4. Basic shell completions (bash/zsh)

**Tasks**:
- [ ] Create `session` main script with dispatcher logic
- [ ] Implement help system (`session help`, `session <cmd> --help`)
- [ ] Create commands/ directory
- [ ] Write command wrapper template (sources libs, delegates to implementation)
- [ ] Create basic bash completion script
- [ ] Create basic zsh completion script
- [ ] Test: `session help`, `session --version`, unknown command handling

**Exit Criteria**:
- CLI dispatcher works correctly
- Help system functional
- Tab completion works in bash/zsh
- Unknown commands show helpful error
- All tests pass

---

### Phase 1: Foundation (3.5-4.5 hours)

**Deliverables**:
1. Extend manifest schema (add claude: and tmux: sections)
2. Create claude-discovery.sh library
3. Create basic resume-claude.sh (info-only mode)
4. BATS tests for parsing and validation

**Tasks**:
- [ ] Document manifest schema v2.0
- [ ] Implement `parse_history_jsonl()` with hybrid approach
- [ ] Implement `validate_claude_session_dirs()` (Condition #3)
- [ ] Implement format validation (Condition #4)
- [ ] Create resume-claude.sh skeleton (help text, arg parsing)
- [ ] Write 20 BATS tests for claude-discovery.sh

**Exit Criteria**:
- Can parse history.jsonl successfully
- Can validate Claude session directories
- resume-claude.sh shows session info (no resume yet)
- All tests pass

---

### Phase 2: Auto-Resume (2-3 hours)

**Deliverables**:
1. Create tmux-utils.sh library
2. Complete resume-claude.sh with tmux control
3. Implement resume action logging
4. BATS tests for tmux control

**Tasks**:
- [ ] Implement `ensure_tmux_and_resume()` (new-session approach)
- [ ] Implement `log_resume_action()` (Condition #6)
- [ ] Complete identifier resolution in resume-claude.sh
- [ ] Add health checks to resume-claude.sh
- [ ] Implement CWD recovery workflow
- [ ] Write 15 BATS tests for tmux-utils.sh
- [ ] Write 25 BATS tests for resume-claude.sh end-to-end

**Exit Criteria**:
- Can resume Claude by any identifier
- Tmux session created/attached automatically
- Resume actions logged to audit trail
- All tests pass

---

### Phase 3: Discovery & Migration (2.5-3.5 hours)

**Deliverables**:
1. Create session-sync.sh
2. Enhance session-dashboard.sh with Claude/tmux info
3. Create list-claude-sessions.sh
4. BATS tests for discovery and migration

**Tasks**:
- [ ] Implement session discovery in session-sync.sh
- [ ] Implement orphan detection
- [ ] Add migration progress tracking (Condition #5)
- [ ] Add auto-sync offer to resume-claude.sh (Condition #2)
- [ ] Enhance session-dashboard.sh to show Claude/tmux fields
- [ ] Create list-claude-sessions.sh
- [ ] Write 12 BATS tests for session-sync.sh

**Exit Criteria**:
- Can discover all Claude sessions
- Can map orphaned sessions to manifests
- Dashboard shows complete session state
- All tests pass

---

### Phase 4: Edge Cases & Polish (2.5-3.5 hours)

**Deliverables**:
1. CWD deleted bug recovery (complete)
2. Manifest corruption recovery (Condition #8)
3. Tmux name conflict handling
4. Cleanup utilities
5. Final testing

**Tasks**:
- [ ] Complete CWD recovery options (recreate/fallback/archive)
- [ ] Implement manifest corruption detection (Condition #8)
- [ ] Add recovery prompts for corrupted manifests
- [ ] Implement tmux name conflict resolution
- [ ] Test all edge cases (special chars, empty sessions, etc.)
- [ ] Performance testing (296 entries benchmark)
- [ ] Security review (no secrets in manifests)

**Exit Criteria**:
- All edge cases handled gracefully
- Recovery workflows tested
- No secrets exposure
- All tests pass

---

### Phase 5: Documentation (1-2 hours)

**Deliverables**:
1. User guide with examples
2. Migration guide for existing sessions
3. Integration documentation
4. Troubleshooting guide

**Tasks**:
- [ ] Write USER-GUIDE.md (usage, examples, troubleshooting)
- [ ] Write MIGRATION-GUIDE.md (how to migrate existing sessions)
- [ ] Update main README with Claude session features
- [ ] Document all review conditions and how they're addressed
- [ ] Create example workflows

**Exit Criteria**:
- Complete user documentation
- Clear migration path
- All features documented
- Examples tested

---

## 5. Risk Assessment (Final)

### Risk Matrix

| Risk | Impact | Likelihood | Mitigation | Status |
|------|--------|------------|------------|--------|
| history.jsonl format changes | HIGH | LOW | Hybrid parsing, format validation | ✅ Designed |
| Manifest-reality drift | MEDIUM | HIGH | Auto-sync offer, health checks | ✅ Designed |
| Tmux timing issues | NONE | NONE | ✅ Eliminated by new-session approach | ✅ Eliminated |
| Migration incompleteness | LOW | MEDIUM | Progress tracking, resumable | ✅ Designed |
| Session corruption | MEDIUM | LOW | Validation, recovery prompts | ✅ Designed |
| Session termination on exit | LOW | MEDIUM | Simple restart (one command) | ✅ Acceptable |

**Overall Risk Level**: **VERY LOW** ✅

---

## 6. Review Conditions Status

All conditions designed and ready for implementation:

| # | Condition | Design Status | Phase | Est. Time |
|---|-----------|--------------|-------|-----------|
| 1 | Sleep after tmux creation | ✅ **ELIMINATED** | N/A | 0 min |
| 2 | Auto-sync offer on failure | ✅ Complete | Phase 3 | 20 min |
| 3 | Validate Claude session dirs | ✅ Complete | Phase 1 | 20 min |
| 4 | Format validation for history.jsonl | ✅ Complete | Phase 1 | 30 min |
| 5 | Migration progress tracking | ✅ Complete | Phase 3 | 15 min |
| 6 | Resume action logging | ✅ Complete | Phase 2 | 20 min |
| 7 | Empty tmux detection | ⚠️ **DEFERRED** | (If needed) | 0 min |
| 8 | Corruption recovery prompts | ✅ Complete | Phase 4 | 15 min |

**Total Time**: 2 hours (as estimated)

---

## 7. Final Implementation Plan

### Total Effort Estimate: 13.5-19.5 hours (Updated with CLI)

**Breakdown**:
- Phase 0: CLI framework (2-3h) ← **NEW**
- Phase 1: Foundation + validations (3.5-4.5h)
- Phase 2: Auto-resume + logging (2-3h)
- Phase 3: Discovery + migration (2.5-3.5h)
- Phase 4: Edge cases + polish (2.5-3.5h)
- Phase 5: Documentation (1-2h)

**Code Estimates**:
- CLI: ~250 lines (dispatcher 100, completions 150)
- Commands: ~750 lines (resume 350, sync 250, list 150)
- Libraries: ~600 lines (claude-discovery 300, tmux-utils 200, manifest ext 100)
- Tests: ~1,050 lines (80 BATS tests)
- **Total**: ~2,650 lines

**CLI Overhead**: +2-3 hours, +350 lines
**ROI**: 1.3-11x in first year (from CLI review)

**Dependencies**:
- Existing workspace management (complete)
- Git, tmux, bash 4.0+
- Optional: python3 (for robust parsing)

---

## 8. D3 Exit Criteria

Before proceeding to D4, verify:

- ✅ All major technical decisions finalized
- ✅ Component interfaces defined
- ✅ Implementation sequence clear
- ✅ All review conditions addressed in design
- ✅ Risk mitigation strategies complete
- ✅ Effort estimate realistic (11.5-16.5 hours)
- ✅ No ambiguities in design
- ✅ Ready for detailed requirements (D4)

---

## 9. Next Phase: D4 - Implementation Requirements

**D4 Objectives**:
1. Define detailed function specifications
2. Document all error conditions and handling
3. Create comprehensive test plan
4. Define acceptance criteria for each component
5. Prepare for implementation (S5-S7)

**Expected D4 Duration**: 2-3 hours

---

## Summary

**D3 Completion Status**: ✅ COMPLETE (Updated with CLI architecture)

**Key Decisions Made**:
1. ✅ Hybrid JSON parsing (python3 → grep/sed)
2. ✅ new-session tmux control (user-suggested)
3. ✅ Two new libraries + extend existing
4. ✅ YAML manifest extension (v2.0)
5. ✅ **Unified CLI architecture** (post-D3 decision)
6. ✅ Comprehensive BATS testing (80 tests)
7. ✅ All 6 review conditions addressed

**Implementation Readiness**:
- Clear component boundaries ✅
- Well-defined interfaces ✅
- Realistic effort estimate (13.5-19.5 hours) ✅
- All risks mitigated ✅
- User feedback incorporated ✅
- CLI architecture designed ✅

**Confidence**: VERY HIGH (ready for D4)

**CLI Decision**:
- User approved unified CLI after multi-persona review
- 5/5 unanimous approval (9.1/10 confidence)
- See CLI-UNIFICATION-REVIEW.md for full analysis

---

**D3 Document Complete**: 2025-12-03 (Updated 2025-12-03 with CLI)
**Status**: Ready for D4 - Implementation Requirements
**Next**: D4 will include CLI framework detailed requirements
