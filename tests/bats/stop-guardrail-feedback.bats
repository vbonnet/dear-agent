#!/usr/bin/env bats
# Tests for .claude/hooks/stop-guardrail-feedback — the WF-A stop-hook
# guardrail feedback loop (bead ce-vrux). Asserts the hook:
#   * blocks the stop with NL feedback when the guardrail bundle is RED,
#   * lets the agent stop (no decision) when the bundle is GREEN,
#   * stays silent on a clean tree, on opt-out, and on non-Stop events,
#   * bounds itself: yields control after MAX_ITERS attempts,
#   * resets its attempt budget once the bundle goes green,
#   * fails OPEN outside a git repo / with no bundle present.
#
# The bundle is driven through its GUARDRAIL_CMD override so these tests never
# depend on the Go toolchain — they exercise the loop mechanism, which is what
# WF-A owns. Lives under tests/bats/ (shell-tests.yml + shell-matrix.yml).

setup() {
    command -v jq >/dev/null 2>&1 || skip "hook requires jq, not installed here"
    command -v git >/dev/null 2>&1 || skip "hook requires git, not installed here"

    unset GIT_CONFIG_COUNT GIT_CONFIG_PARAMETERS GIT_TEMPLATE_DIR
    export GIT_CONFIG_GLOBAL=/dev/null
    export GIT_CONFIG_SYSTEM=/dev/null
    export GIT_TEMPLATE_DIR="$BATS_TEST_TMPDIR/git-template"
    mkdir -p "$GIT_TEMPLATE_DIR"

    REPO_ROOT="$(cd "$(dirname "$BATS_TEST_FILENAME")/../.." && pwd)"
    HOOK="$REPO_ROOT/.claude/hooks/stop-guardrail-feedback"

    # A throwaway git repo carrying a real copy of the guardrail bundle.
    REPO="$BATS_TEST_TMPDIR/repo"
    mkdir -p "$REPO/scripts"
    cp "$REPO_ROOT/scripts/guardrail-bundle.sh" "$REPO/scripts/guardrail-bundle.sh"
    chmod +x "$REPO/scripts/guardrail-bundle.sh"
    git -C "$REPO" init -q
    git -C "$REPO" config user.email t@t.t
    git -C "$REPO" config user.name t
    git -C "$REPO" add -A
    git -C "$REPO" commit -qm init

    # Isolate the attempt counter and start from a clean env every test.
    export CLAUDE_STATE_DIR="$BATS_TEST_TMPDIR/state"
    unset DEAR_GUARDRAIL_LOOP DEAR_GUARDRAIL_MAX_ITERS GUARDRAIL_CMD
}

# dirty — make the working tree non-clean so the hook engages.
dirty() { echo change > "$REPO/dirty.txt"; }

# payload <event> <session> — Stop-hook stdin JSON pointed at the temp repo.
payload() {
    jq -cn --arg e "$1" --arg s "$2" --arg c "$REPO" \
        '{hook_event_name:$e, session_id:$s, cwd:$c}'
}

# hook <json> — feed JSON on stdin; bats `run` exposes $output/$status.
hook() { run bash -c 'printf "%s" "$1" | "$2"' _ "$1" "$HOOK"; }

@test "RED bundle blocks the stop with NL feedback" {
    dirty
    export GUARDRAIL_CMD='echo BUNDLE_BOOM; exit 1'
    hook "$(payload Stop sess-red)"
    [ "$status" -eq 0 ]
    [ "$(printf '%s' "$output" | jq -r '.decision')" = "block" ]
    [[ "$(printf '%s' "$output" | jq -r '.reason')" == *"GUARDRAILS FAILING"* ]]
    [[ "$(printf '%s' "$output" | jq -r '.reason')" == *"BUNDLE_BOOM"* ]]
    [[ "$(printf '%s' "$output" | jq -r '.reason')" == *"attempt 1/3"* ]]
}

@test "GREEN bundle lets the agent stop (no decision)" {
    dirty
    export GUARDRAIL_CMD='exit 0'
    hook "$(payload Stop sess-green)"
    [ "$status" -eq 0 ]
    [ -z "$output" ]
}

@test "clean working tree is a no-op even with a RED bundle" {
    export GUARDRAIL_CMD='exit 1'   # would fail, but nothing changed
    hook "$(payload Stop sess-clean)"
    [ "$status" -eq 0 ]
    [ -z "$output" ]
}

@test "opt-out via DEAR_GUARDRAIL_LOOP=0 disables the hook" {
    dirty
    export DEAR_GUARDRAIL_LOOP=0 GUARDRAIL_CMD='exit 1'
    hook "$(payload Stop sess-optout)"
    [ "$status" -eq 0 ]
    [ -z "$output" ]
}

@test "ignores non-Stop events" {
    dirty
    export GUARDRAIL_CMD='exit 1'
    hook "$(payload PreToolUse sess-other)"
    [ "$status" -eq 0 ]
    [ -z "$output" ]
}

@test "SubagentStop is handled the same as Stop" {
    dirty
    export GUARDRAIL_CMD='exit 1'
    hook "$(payload SubagentStop sess-sub)"
    [ "$(printf '%s' "$output" | jq -r '.decision')" = "block" ]
}

@test "attempt budget is bounded — yields control after MAX_ITERS" {
    dirty
    export DEAR_GUARDRAIL_MAX_ITERS=2 GUARDRAIL_CMD='echo still_red; exit 1'
    hook "$(payload Stop sess-budget)"   # attempt 1 -> block
    [ "$(printf '%s' "$output" | jq -r '.decision')" = "block" ]
    hook "$(payload Stop sess-budget)"   # attempt 2 -> block
    [ "$(printf '%s' "$output" | jq -r '.decision')" = "block" ]
    hook "$(payload Stop sess-budget)"   # attempt 3 > MAX -> yield
    [ "$status" -eq 0 ]
    [ "$(printf '%s' "$output" | jq -r '.decision // "none"')" = "none" ]
    [[ "$(printf '%s' "$output" | jq -r '.hookSpecificOutput.additionalContext')" == *"yielding control"* ]]
}

@test "attempt budget resets after the bundle goes green" {
    dirty
    export GUARDRAIL_CMD='exit 1'
    hook "$(payload Stop sess-reset)"    # attempt 1 -> block, count=1
    [[ "$(printf '%s' "$output" | jq -r '.reason')" == *"attempt 1/3"* ]]
    GUARDRAIL_CMD='exit 0' hook "$(payload Stop sess-reset)"  # green -> reset
    export GUARDRAIL_CMD='exit 1'
    hook "$(payload Stop sess-reset)"    # back to attempt 1, not 2
    [[ "$(printf '%s' "$output" | jq -r '.reason')" == *"attempt 1/3"* ]]
}

@test "fails open outside a git repo" {
    export GUARDRAIL_CMD='exit 1'
    local json; json="$(jq -cn --arg c "$BATS_TEST_TMPDIR" '{hook_event_name:"Stop",session_id:"x",cwd:$c}')"
    hook "$json"
    [ "$status" -eq 0 ]
    [ -z "$output" ]
}

@test "fails open when the bundle is absent" {
    dirty
    rm -f "$REPO/scripts/guardrail-bundle.sh"
    export GUARDRAIL_CMD='exit 1'
    hook "$(payload Stop sess-nobundle)"
    [ "$status" -eq 0 ]
    [ -z "$output" ]
}

@test "untrackable counter + stop_hook_active yields (secondary brake)" {
    dirty
    # Point the state dir at a regular file so mkdir/write fail.
    printf x > "$BATS_TEST_TMPDIR/notadir"
    export CLAUDE_STATE_DIR="$BATS_TEST_TMPDIR/notadir"
    export GUARDRAIL_CMD='echo persist_fail; exit 1'
    local json; json="$(jq -cn --arg c "$REPO" \
        '{hook_event_name:"Stop",session_id:"s",cwd:$c,stop_hook_active:true}')"
    hook "$json"
    [ "$status" -eq 0 ]
    [ "$(printf '%s' "$output" | jq -r '.decision // "none"')" = "none" ]
    [[ "$(printf '%s' "$output" | jq -r '.hookSpecificOutput.additionalContext')" == *"cannot persist"* ]]
}

@test "untrackable counter without stop_hook_active still blocks once" {
    dirty
    printf x > "$BATS_TEST_TMPDIR/notadir"
    export CLAUDE_STATE_DIR="$BATS_TEST_TMPDIR/notadir"
    export GUARDRAIL_CMD='exit 1'
    local json; json="$(jq -cn --arg c "$REPO" \
        '{hook_event_name:"Stop",session_id:"s",cwd:$c,stop_hook_active:false}')"
    hook "$json"
    [ "$(printf '%s' "$output" | jq -r '.decision')" = "block" ]
}

@test "emitted block JSON is well-formed" {
    dirty
    export GUARDRAIL_CMD='exit 1'
    hook "$(payload Stop sess-json)"
    printf '%s' "$output" | jq -e '.decision == "block" and (.reason | type == "string")'
}
