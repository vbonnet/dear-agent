#!/usr/bin/env bats
# Tests for .claude/hooks/pretool-bead-close-guard — the Definition-of-Done gate
# on bead closure (AGENTS.md §6, bead ce-7p6s).
#
# The 2026-06-17 daily ops audit (retro R.1) found ce-rpet/ce-11fi closed while
# their PRs were still open, despite this guard shipping in PR #464. Root cause:
# the hook's command parser only recognised a narrow set of close forms — it
# missed `--status=closed` (equals form) and any form where the bead id followed
# a value-taking flag (`close --reason X <id>`, `update --status closed <id>`),
# silently failing open. These tests pin every close path to the guard.

setup() {
    # The hook parses its JSON payload with jq; without it the hook fails open
    # (no-op) and there is nothing to assert. The shell-interpreter matrix runs
    # in minimal containers that omit jq, so skip there.
    command -v jq >/dev/null 2>&1 || skip "hook requires jq, not installed here"
    HOOK="$(cd "$(dirname "$BATS_TEST_FILENAME")/../.." && pwd)/.claude/hooks/pretool-bead-close-guard"

    # Stub bead-close-guard on PATH: record the args it was invoked with, then
    # exit with GUARD_EXIT (default 0) so we can drive the deny path too.
    STUBDIR="$BATS_TEST_TMPDIR/bin"
    GUARD_LOG="$BATS_TEST_TMPDIR/guard.log"
    mkdir -p "$STUBDIR"
    cat > "$STUBDIR/bead-close-guard" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$GUARD_LOG"
echo "${GUARD_MSG:-ok}"
exit "${GUARD_EXIT:-0}"
STUB
    chmod +x "$STUBDIR/bead-close-guard"
    export GUARD_LOG
    PATH="$STUBDIR:$PATH"
}

# hook <json> — feed JSON on stdin; bats `run` sets $output/$status.
hook() {
    run bash -c 'printf "%s" "$1" | "$2"' _ "$1" "$HOOK"
}

# bd <command-string> — wrap a bd command in the Bash-tool JSON envelope.
bd() {
    hook "$(jq -cn --arg c "$1" '{tool_name:"Bash",tool_input:{command:$c}}')"
}

guard_calls() { [ -f "$GUARD_LOG" ] && cat "$GUARD_LOG" || true; }

# --- canonical forms (already worked before the fix) ---

@test "close <id>" {
    bd "bd --db ~/beads/x close ce-rpet"
    [[ "$(guard_calls)" == *"--bead ce-rpet"* ]]
}

@test "update <id> --status closed" {
    bd "bd --db ~/beads/x update ce-rpet --status closed"
    [[ "$(guard_calls)" == *"--bead ce-rpet"* ]]
}

@test "close <id> --reason (reason after id)" {
    bd "bd close ce-rpet --reason done"
    [[ "$(guard_calls)" == *"--bead ce-rpet"* ]]
}

# --- forms that PREVIOUSLY bypassed the guard (the bug) ---

@test "update <id> --status=closed (equals form)" {
    bd "bd --db ~/beads/x update ce-rpet --status=closed"
    [[ "$(guard_calls)" == *"--bead ce-rpet"* ]]
}

@test "close --reason X <id> (id after a value-flag)" {
    bd "bd --db ~/beads/x close --reason hello ce-rpet"
    [[ "$(guard_calls)" == *"--bead ce-rpet"* ]]
}

@test "update --status closed <id> (id after flags)" {
    bd "bd --db ~/beads/x update --status closed ce-rpet"
    [[ "$(guard_calls)" == *"--bead ce-rpet"* ]]
}

@test "update --status=closed <id> (equals + id after flags)" {
    bd "bd update --status=closed ce-rpet"
    [[ "$(guard_calls)" == *"--bead ce-rpet"* ]]
}

# --- beads-dir propagation (~ expanded) ---

@test "--db path is passed through to the guard with ~ expanded" {
    bd "bd --db ~/beads/x close ce-rpet"
    [[ "$(guard_calls)" == *"--beads-dir $HOME/beads/x"* ]]
}

# --- deny / pass-through behaviour ---

@test "blocks (deny) when the guard reports an unmerged PR" {
    GUARD_EXIT=2 GUARD_MSG="BLOCKED: PR #516 not merged" \
        bd "bd close ce-rpet"
    [[ "$output" == *'"permissionDecision":"deny"'* ]]
    [[ "$output" == *"BLOCKED"* ]]
    [ "$status" -eq 2 ]
}

@test "fails CLOSED when a close is detected but no bead id parses" {
    bd "bd close"
    [[ "$output" == *'"permissionDecision":"deny"'* ]]
    [ "$status" -eq 2 ]
    [ -z "$(guard_calls)" ]
}

@test "allows abandonment via --force without invoking the guard" {
    bd "bd close ce-rpet --force"
    [ "$status" -eq 0 ]
    [ -z "$output" ]
    [ -z "$(guard_calls)" ]
}

@test "Codex hook-trust bypass blocks force-close before invoking user-authenticated CLIs" {
    local codex_hook test_hook jq_path
    codex_hook="$(cd "$(dirname "$BATS_TEST_FILENAME")/../.." && pwd)/.codex/hooks/pretool-bead-close-guard"
    test_hook="$BATS_TEST_TMPDIR/pretool-bead-close-guard"
    jq_path="$(command -v jq)"
    sed \
        -e "s|/usr/local/libexec/dear-agent-codex-hook-json|$jq_path|g" \
        -e "s|/usr/local/libexec/dear-agent-bead-close-guard|$STUBDIR/bead-close-guard|g" \
        "$codex_hook" > "$test_hook"
    chmod +x "$test_hook"
    HOOK="$test_hook"

    AGM_CODEX_HOOK_ROOT="$BATS_TEST_TMPDIR/immutable-hooks" \
        bd "bd close ce-rpet --force"

    [ "$status" -eq 2 ]
    [[ "$output" == *'"permissionDecision":"deny"'* ]]
    [[ "$output" == *"ordinary reviewed session"* ]]
    [ -z "$(guard_calls)" ]
}

@test "silent on a read: bd list --status closed" {
    bd "bd list --status closed"
    [ "$status" -eq 0 ]
    [ -z "$output" ]
    [ -z "$(guard_calls)" ]
}

@test "silent on bd close --help" {
    bd "bd close --help"
    [ "$status" -eq 0 ]
    [ -z "$(guard_calls)" ]
}

@test "silent on an unrelated bd command" {
    bd "bd show ce-rpet"
    [ "$status" -eq 0 ]
    [ -z "$output" ]
    [ -z "$(guard_calls)" ]
}

@test "fails open when the guard binary is not installed" {
    PATH="/usr/bin:/bin" run bash -c \
        'printf "%s" "$1" | "$2"' _ \
        "$(jq -cn '{tool_name:"Bash",tool_input:{command:"bd close ce-rpet"}}')" "$HOOK"
    [ "$status" -eq 0 ]
}

@test "silent on a non-Bash tool" {
    hook '{"tool_name":"Agent","tool_input":{"description":"x"}}'
    [ "$status" -eq 0 ]
    [ -z "$output" ]
}
