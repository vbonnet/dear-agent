#!/usr/bin/env bats
# Tests for .claude/hooks/pretool-pr-guard — the wayfinder-only PR lifecycle
# gate (Beads ce-p17s, CLAUDE.md §PR Lifecycle). Asserts the hook denies raw
# `gh pr create|close|reopen` with exit 2 + safe-pr guidance, allows read-only
# pr verbs / merge / safe-pr itself, and fails open on unparseable input.

setup() {
    command -v jq >/dev/null 2>&1 || skip "hook requires jq, not installed here"
    HOOK="$(cd "$(dirname "$BATS_TEST_FILENAME")/../.." && pwd)/.claude/hooks/pretool-pr-guard"
}

# hook <json> — feed JSON on stdin; bats `run` sets $output/$status.
hook() {
    run bash -c 'printf "%s" "$1" | "$2" 2>&1' _ "$1" "$HOOK"
}

bash_cmd() { # $1 = command string -> hook payload
    hook "$(jq -cn --arg c "$1" '{tool_name:"Bash",tool_input:{command:$c}}')"
}

assert_blocked() {
    [ "$status" -eq 2 ]
    [[ "$output" == *safe-pr* ]]
    [[ "$output" == *PERMISSION_ESCALATION* ]]
}

assert_allowed() {
    [ "$status" -eq 0 ]
    [ -z "$output" ]
}

@test "blocks gh pr create" {
    bash_cmd 'gh pr create --title "x" --body "y"'
    assert_blocked
}

@test "blocks gh pr close" {
    bash_cmd 'gh pr close 123 --comment done'
    assert_blocked
}

@test "blocks gh pr reopen" {
    bash_cmd 'gh pr reopen 123'
    assert_blocked
}

@test "blocks create behind -R repo flag" {
    bash_cmd 'gh -R vbonnet/dear-agent pr create --title t'
    assert_blocked
}

@test "blocks create behind --repo= flag" {
    bash_cmd 'gh --repo=vbonnet/dear-agent pr create --title t'
    assert_blocked
}

@test "blocks create after && chain" {
    bash_cmd 'git push origin x && gh pr create --title t'
    assert_blocked
}

@test "blocks create behind env-var prefix" {
    bash_cmd 'GH_TOKEN=x gh pr create --title t'
    assert_blocked
}

@test "blocks create via absolute gh path" {
    bash_cmd '/opt/homebrew/bin/gh pr create --title t'
    assert_blocked
}

@test "allows gh pr view" {
    bash_cmd 'gh pr view 123 --json state'
    assert_allowed
}

@test "allows gh pr list and checks" {
    bash_cmd 'gh pr list --state open; gh pr checks 42 --watch'
    assert_allowed
}

@test "allows gh pr merge (governed elsewhere)" {
    bash_cmd 'gh pr merge 42 --squash --delete-branch'
    assert_allowed
}

@test "allows safe-pr create (the sanctioned path)" {
    bash_cmd 'safe-pr create --wayfinder /x --title t --body b'
    assert_allowed
}

@test "allows quoted mention inside another command" {
    bash_cmd 'echo "gh pr create is blocked here"'
    assert_allowed
}

@test "allows unrelated gh verbs" {
    bash_cmd 'gh api repos/x/y/pulls --jq length; gh auth status'
    assert_allowed
}

@test "ignores non-Bash tools" {
    hook '{"tool_name":"Write","tool_input":{"file_path":"/tmp/x","content":"gh pr create"}}'
    assert_allowed
}

@test "fails open on garbage input" {
    hook 'this is not json'
    assert_allowed
}

@test "fails open on empty input" {
    hook ''
    assert_allowed
}

@test "blocks create behind gtimeout prefix (repo-taught habit)" {
    bash_cmd 'gtimeout 30 gh pr create --title t'
    assert_blocked
}

@test "blocks create behind env/command/sudo launchers" {
    bash_cmd 'env gh pr create --title t'
    assert_blocked
    bash_cmd 'command gh pr create --title t'
    assert_blocked
    bash_cmd 'sudo gh pr close 1'
    assert_blocked
}

@test "blocks create split across backslash continuations" {
    bash_cmd $'gh \\\npr create --title t'
    assert_blocked
}

@test "allows gtimeout-prefixed read-only pr verbs" {
    bash_cmd 'gtimeout 30 gh pr view 123'
    assert_allowed
}

@test "reopen denial points at a human path, not safe-pr" {
    bash_cmd 'gh pr reopen 123'
    [ "$status" -eq 2 ]
    [[ "$output" == *"human decision"* ]]
    [[ "$output" != *"safe-pr reopen"* ]]
}
