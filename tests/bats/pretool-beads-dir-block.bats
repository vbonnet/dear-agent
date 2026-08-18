#!/usr/bin/env bats
# Tests for .codex/hooks/pretool-beads-dir-block — blocks BEADS_DIR env var
# usage and redirects agents to `bd --db` or `bd -C` instead.
#
# BEADS_DIR is not a supported bd environment variable; setting it has no
# effect on bd. This hook catches the two mistake forms:
#   BEADS_DIR=/path bd ...        (assignment)
#   echo "$BEADS_DIR"             (dereference)
# and blocks them with a helpful redirect message.

setup() {
    command -v jq >/dev/null 2>&1 || skip "hook requires jq"
    command -v python3 >/dev/null 2>&1 || skip "test helper requires python3"
    HOOK="$(cd "$(dirname "$BATS_TEST_FILENAME")/../.." && pwd)/.codex/hooks/pretool-beads-dir-block"
    [ -x "$HOOK" ] || skip "hook not found or not executable: $HOOK"
}

# hook <json> — feed JSON on stdin; bats `run` captures $output/$status.
hook() {
    run bash -c 'printf "%s" "$1" | "$2" 2>&1' _ "$1" "$HOOK"
}

bash_cmd() { # $1 = command string -> hook payload
    hook "$(printf '{"tool_name":"Bash","tool_input":{"command":"%s"}}' "$(printf '%s' "$1" | python3 -c 'import sys,json; print(json.dumps(sys.stdin.read())[1:-1])')")"
}

# --- Bash assignment forms ---

@test "blocks BEADS_DIR= assignment before bd" {
    bash_cmd 'BEADS_DIR=/path/to/beads bd list'
    [ "$status" -eq 2 ]
    [[ "$output" == *"bd --db"* ]]
}

@test "blocks export BEADS_DIR=..." {
    bash_cmd 'export BEADS_DIR=/path/to/beads'
    [ "$status" -eq 2 ]
}

@test "blocks BEADS_DIR= in env prefix form" {
    bash_cmd 'BEADS_DIR=/x bd -C /y show ce-123'
    [ "$status" -eq 2 ]
}

# --- Bash dereference forms ---

@test "blocks \$BEADS_DIR dereference" {
    bash_cmd 'bd --db $BEADS_DIR list'
    [ "$status" -eq 2 ]
}

@test "blocks \${BEADS_DIR} dereference" {
    bash_cmd 'bd --db ${BEADS_DIR} list'
    [ "$status" -eq 2 ]
}

@test "blocks \$BEADS_DIR in echo" {
    bash_cmd 'echo $BEADS_DIR'
    [ "$status" -eq 2 ]
}

# --- Negative lookbehind: must NOT match MY_BEADS_DIR ---

@test "allows MY_BEADS_DIR (longer var name)" {
    bash_cmd 'MY_BEADS_DIR=/x bd list'
    [ "$status" -eq 0 ]
}

# --- Non-matching Bash commands pass through ---

@test "allows bd --db with explicit path" {
    bash_cmd 'bd --db /path/to/.beads/embeddeddolt list'
    [ "$status" -eq 0 ]
}

@test "allows bd -C with dir" {
    bash_cmd 'bd -C /path/to/project list'
    [ "$status" -eq 0 ]
}

@test "allows plain bd list" {
    bash_cmd 'bd list --status=open'
    [ "$status" -eq 0 ]
}

# --- Write/Edit/apply_patch on shell files ---

@test "blocks BEADS_DIR= in Write to .sh file" {
    payload='{"tool_name":"Write","tool_input":{"file_path":"/tmp/setup.sh","content":"export BEADS_DIR=/x\n"}}'
    hook "$payload"
    [ "$status" -eq 2 ]
}

@test "blocks BEADS_DIR= in Edit new_string on .env file" {
    payload='{"tool_name":"Edit","tool_input":{"file_path":"/home/user/.env","old_string":"","new_string":"BEADS_DIR=/x\n"}}'
    hook "$payload"
    [ "$status" -eq 2 ]
}

@test "allows Write to non-shell file even with BEADS_DIR text" {
    payload='{"tool_name":"Write","tool_input":{"file_path":"/tmp/notes.md","content":"Do not use BEADS_DIR=..."}}'
    hook "$payload"
    [ "$status" -eq 0 ]
}

@test "blocks BEADS_DIR= in apply_patch to .sh file" {
    payload="$(jq -cn --arg patch '*** Begin Patch
*** Update File: /tmp/setup.sh
@@
+export BEADS_DIR=/x
*** End Patch' '{tool_name:"apply_patch",tool_input:{patch:$patch}}')"
    hook "$payload"
    [ "$status" -eq 2 ]
}

@test "allows apply_patch to non-shell file even with BEADS_DIR text" {
    payload="$(jq -cn --arg patch '*** Begin Patch
*** Update File: /tmp/notes.md
@@
+Do not use BEADS_DIR=...
*** End Patch' '{tool_name:"apply_patch",tool_input:{patch:$patch}}')"
    hook "$payload"
    [ "$status" -eq 0 ]
}

# --- Fail-open on malformed input ---

@test "fails open on empty input" {
    hook ""
    [ "$status" -eq 0 ]
}

@test "fails open on non-JSON" {
    hook "not json at all"
    [ "$status" -eq 0 ]
}

@test "fails open on wrong tool name" {
    hook '{"tool_name":"Read","tool_input":{"file_path":"/tmp/f"}}'
    [ "$status" -eq 0 ]
}

# --- Message content ---

@test "error message mentions bd --db" {
    bash_cmd 'BEADS_DIR=/x bd list'
    [[ "$output" == *"bd --db"* ]]
}

@test "error message does not suggest non-canonical bd -C" {
    bash_cmd 'BEADS_DIR=/x bd list'
    [[ "$output" != *"bd -C"* ]]
}
