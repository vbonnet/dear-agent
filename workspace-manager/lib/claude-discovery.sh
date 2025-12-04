#!/bin/bash

# claude-discovery.sh - Parse history.jsonl and discover Claude sessions
# Part of unified session management CLI

# discover_claude_sessions([history_file])
# Parse history.jsonl and return all Claude sessions
# Returns: Tab-separated values: sessionId|project|timestamp
discover_claude_sessions() {
    local history_file="${1:-$HOME/.claude/history.jsonl}"

    if ! parse_history_jsonl "$history_file"; then
        return $?
    fi
}

# get_session_stats(uuid, [history_file])
# Get statistics for a specific session
# Returns: message_count|duration_hours|first_timestamp|last_timestamp
get_session_stats() {
    local uuid="$1"
    local history_file="${2:-$HOME/.claude/history.jsonl}"

    if command -v python3 &>/dev/null; then
        python3 -c "
import json, sys

target_uuid = '$uuid'
messages = []

with open('$history_file', 'r') as f:
    for line in f:
        try:
            obj = json.loads(line.strip())
            sid = obj.get('sessionId')
            ts = obj.get('timestamp', 0) / 1000

            if sid == target_uuid:
                messages.append(ts)
        except: pass

if messages:
    first_ts = min(messages)
    last_ts = max(messages)
    duration_h = (last_ts - first_ts) / 3600
    print(f'{len(messages)}|{duration_h:.1f}|{int(first_ts * 1000)}|{int(last_ts * 1000)}')
else:
    print('0|0|0|0')
" 2>/dev/null
    else
        echo "0|0|0|0"
    fi
}

# parse_history_jsonl(history_file)
# Parse JSON Lines format history file
# Returns: sessionId|project|timestamp
# Review Condition #4: Format validation
parse_history_jsonl() {
    local history_file="$1"

    # Validate file exists and has content (Condition #4)
    if [[ ! -f "$history_file" ]]; then
        log_error "history.jsonl not found: $history_file"
        return 1
    fi

    if [[ ! -s "$history_file" ]]; then
        log_warn "history.jsonl is empty: $history_file"
        return 0  # Empty is valid, just no sessions
    fi

    # Format validation (Condition #4)
    local first_line
    first_line=$(head -1 "$history_file")
    if [[ ! "$first_line" =~ ^\{.*\}$ ]]; then
        log_error "Invalid JSON Lines format in history.jsonl"
        log_error "Expected: {...} on each line"
        log_error "Got: $first_line"
        return 2
    fi

    # Try python3 first (most robust)
    if command -v python3 &>/dev/null; then
        local result
        result=$(python3 -c "
import json, sys
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        obj = json.loads(line)
        sid = obj.get('sessionId', '')
        proj = obj.get('project', '')
        ts = obj.get('timestamp', '')
        if sid:
            print(f'{sid}|{proj}|{ts}')
    except json.JSONDecodeError:
        # Skip malformed lines
        pass
" < "$history_file" 2>/dev/null)

        if [[ $? -eq 0 ]]; then
            echo "$result"
            return 0
        fi

        log_warn "Python parsing failed, falling back to grep/sed"
    fi

    # Fallback: grep/sed (less robust but no dependencies)
    # Use || true to prevent grep from failing the script if no matches
    paste -d'|' \
        <(grep -o '"sessionId":"[^"]*"' "$history_file" | sed 's/"sessionId":"\([^"]*\)"/\1/' || true) \
        <(grep -o '"project":"[^"]*"' "$history_file" | sed 's/"project":"\([^"]*\)"/\1/' || true) \
        <(grep -o '"timestamp":[0-9]*' "$history_file" | sed 's/"timestamp"://' || true)
}

# validate_claude_session_dirs(uuid)
# Validate that Claude session directories exist and have content
# Review Condition #3: Validate Claude session dirs
validate_claude_session_dirs() {
    local uuid="$1"

    if [[ -z "$uuid" ]]; then
        log_error "validate_claude_session_dirs: UUID required"
        return 1
    fi

    local session_env="$HOME/.claude/session-env/$uuid"
    local file_history="$HOME/.claude/file-history/$uuid"

    # Check session-env exists and has content (Condition #3)
    if [[ ! -d "$session_env" ]]; then
        log_warn "Claude session-env directory not found: $session_env"
        return 1
    fi

    if [[ -z "$(ls -A "$session_env" 2>/dev/null)" ]]; then
        log_warn "Claude session-env directory is empty: $session_env"
        return 1
    fi

    # file-history is optional (may not exist for new sessions)
    if [[ -d "$file_history" ]]; then
        log_debug "Claude file-history exists: $file_history"
    else
        log_debug "Claude file-history not found (OK for new sessions): $file_history"
    fi

    return 0
}

# find_manifest_by_claude_uuid(uuid)
# Find manifest.yaml that contains a Claude session UUID
# Returns: Path to manifest.yaml or empty string
find_manifest_by_claude_uuid() {
    local uuid="$1"
    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"

    if [[ ! -d "$sessions_dir" ]]; then
        return 0  # No sessions directory, no manifests
    fi

    # Search for manifest with matching Claude session_id
    # Use || true to prevent grep exit code 1 from failing script
    grep -l "session_id: $uuid" "$sessions_dir"/*/manifest.yaml 2>/dev/null | head -1 || true
}

# find_manifest_by_tmux_name(tmux_name)
# Find manifest.yaml that contains a tmux session name
# Returns: Path to manifest.yaml or empty string
find_manifest_by_tmux_name() {
    local tmux_name="$1"
    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"

    if [[ ! -d "$sessions_dir" ]]; then
        return 0
    fi

    # Search for manifest with matching tmux session_name
    # Use || true to prevent grep exit code 1 from failing script
    grep -l "session_name: $tmux_name" "$sessions_dir"/*/manifest.yaml 2>/dev/null | head -1 || true
}
