#!/usr/bin/env bats
# Tests for .claude/hooks/pretool-spawn-routing — the dogfooding-routing nudge
# (Beads ce-qgf, AGENTS.md §Dogfooding). Asserts the hook nudges on raw session
# spawns, stays silent on the AGM path and ordinary commands, and NEVER emits a
# permissionDecision (so it can neither block nor auto-approve a call).
#
# Replaces the former pretool-spawn-routing.test.sh: that hand-rolled harness
# tripped the bash-20-line-limit language policy, and bats under tests/bats/ is
# the repo's sanctioned shell-test path (shell-tests.yml + shell-matrix.yml).

setup() {
    # The hook parses its JSON payload with jq; without it the hook fails open
    # (no-op) and there is nothing to assert. The shell-interpreter matrix runs
    # in minimal containers that omit jq, so skip there — full coverage still
    # runs under shell-tests.yml (ubuntu, jq preinstalled) and locally.
    command -v jq >/dev/null 2>&1 || skip "hook requires jq, not installed here"
    HOOK="$(cd "$(dirname "$BATS_TEST_FILENAME")/../.." && pwd)/.claude/hooks/pretool-spawn-routing"
}

# hook <json> — feed JSON on stdin; bats `run` sets $output/$status for the caller.
hook() {
    run bash -c 'printf "%s" "$1" | "$2"' _ "$1" "$HOOK"
}

# Invariant for every case: the hook must never make a permission decision.
assert_no_decision() {
    [[ "$output" != *permissionDecision* ]]
}

@test "nudges on a raw claude session spawn" {
    hook '{"tool_name":"Bash","tool_input":{"command":"claude -p \"do a thing\""}}'
    [[ "$output" == *additionalContext* ]]
    assert_no_decision
}

@test "nudges on a claude-code session spawn" {
    hook '{"tool_name":"Bash","tool_input":{"command":"claude-code --resume xyz"}}'
    [[ "$output" == *additionalContext* ]]
    assert_no_decision
}

@test "nudges on a cowork session spawn" {
    hook '{"tool_name":"Bash","tool_input":{"command":"cowork run task"}}'
    [[ "$output" == *additionalContext* ]]
    assert_no_decision
}

@test "nudges on a spawn after &&" {
    hook '{"tool_name":"Bash","tool_input":{"command":"cd /tmp && claude -p hi"}}'
    [[ "$output" == *additionalContext* ]]
    assert_no_decision
}

@test "nudges on a spawn behind an env-var prefix" {
    hook '{"tool_name":"Bash","tool_input":{"command":"FOO=bar claude -p hi"}}'
    [[ "$output" == *additionalContext* ]]
    assert_no_decision
}

@test "nudges on a Cowork scheduled-task mcp call" {
    hook '{"tool_name":"mcp__scheduled-tasks__create_scheduled_task","tool_input":{}}'
    [[ "$output" == *additionalContext* ]]
    assert_no_decision
}

@test "silent on the agm path (it is the right way)" {
    hook '{"tool_name":"Bash","tool_input":{"command":"agm new worker && agm send worker hi"}}'
    [ -z "$output" ]
    assert_no_decision
}

@test "silent on read-only claude mcp subcommand" {
    hook '{"tool_name":"Bash","tool_input":{"command":"claude mcp list"}}'
    [ -z "$output" ]
    assert_no_decision
}

@test "silent on claude --version" {
    hook '{"tool_name":"Bash","tool_input":{"command":"claude --version"}}'
    [ -z "$output" ]
    assert_no_decision
}

@test "silent on an ordinary command" {
    hook '{"tool_name":"Bash","tool_input":{"command":"go test ./..."}}'
    [ -z "$output" ]
    assert_no_decision
}

@test "silent when 'claude' only appears as an argument" {
    hook '{"tool_name":"Bash","tool_input":{"command":"git log --author=claude"}}'
    [ -z "$output" ]
    assert_no_decision
}

@test "silent on the in-session Agent tool (never escapes the mesh)" {
    hook '{"tool_name":"Agent","tool_input":{"description":"x"}}'
    [ -z "$output" ]
    assert_no_decision
}

@test "silent (fails open) on empty/garbage input" {
    hook 'not json at all'
    [ -z "$output" ]
    assert_no_decision
}
