#!/usr/bin/env bats
# Tests for the cross-harness wayfinder-only PR lifecycle guard (Beads ce-p17s,
# ce-1hu9.81, AGENTS.md §PR Lifecycle). Asserts raw create/close/reopen and
# merge are denied through their sanctioned paths, read-only PR verbs and
# safe-pr remain allowed, and unparseable input fails open.

setup() {
    if ! command -v jq >/dev/null 2>&1; then
        [ "${CI:-}" != "true" ] || return 1
        skip "hook requires jq, not installed here"
    fi
    REPO_ROOT="$(cd "$(dirname "$BATS_TEST_FILENAME")/../.." && pwd)"
    HOOK="$REPO_ROOT/.claude/hooks/pretool-pr-guard"
    HOOK_PATHS=(
        "$HOOK"
        "$REPO_ROOT/.agents/hooks/pretool-pr-guard"
        "$REPO_ROOT/.codex/hooks/pretool-pr-guard"
        "$REPO_ROOT/.opencode/hooks/pretool-pr-guard"
        "$REPO_ROOT/.pi/guardrails/pretool-pr-guard"
    )
}

# hook_at <path> <json> — feed JSON on stdin; bats `run` sets output/status.
hook_at() {
    run bash -c 'unset AGM_CODEX_HOOK_ROOT; export CLAUDE_PROJECT_DIR="$3" PI_PROJECT_DIR="$3"; printf "%s" "$1" | "$2" 2>&1' _ "$2" "$1" "$REPO_ROOT"
}

hook() { hook_at "$HOOK" "$1"; }

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

@test "Codex immutable projection matches the canonical guard core" {
    canonical="$(sed -n '/^# BEGIN CANONICAL PR GUARD CORE$/,/^# END CANONICAL PR GUARD CORE$/p' "$REPO_ROOT/scripts/pretool-pr-guard")"
    codex="$(sed -n '/^# BEGIN CANONICAL PR GUARD CORE$/,/^# END CANONICAL PR GUARD CORE$/p' "$REPO_ROOT/.codex/hooks/pretool-pr-guard")"
    [ -n "$canonical" ]
    [ "$codex" = "$canonical" ]
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

@test "allows gh pr view across tracked harness adapters" {
    payload="$(jq -cn --arg c 'gh pr --repo vbonnet/dear-agent view 123 --json state' '{tool_name:"Bash",tool_input:{command:$c}}')"
    for hook_path in "${HOOK_PATHS[@]}"; do
        hook_at "$hook_path" "$payload"
        assert_allowed
    done
}

@test "allows gh pr list and checks" {
    bash_cmd 'gh pr list --state open; gh pr checks 42 --watch'
    assert_allowed
}

@test "blocks gh pr merge across adapters and global flag orderings" {
    for command_line in \
        'gh pr merge 42 --squash --delete-branch' \
        'gh pr --repo vbonnet/dear-agent merge 42 --squash' \
        'gh -Rvbonnet/dear-agent pr merge 42 --squash' \
        'env -u GH_TOKEN gh pr merge 42 --squash' \
        'env -u gh gh pr merge 42 --squash' \
        'env --argv0 gh gh pr merge 42 --squash' \
        'env -u command -v gh pr merge 42 --squash' \
        "FOO='value with spaces' gh pr merge 42 --squash" \
        'FOO+=bar gh pr merge 42 --squash' \
        'FOO[x]=bar gh pr merge 42 --squash' \
        'FOO=x\ y gh pr merge 42 --squash' \
        "env FOO='value with spaces' gh pr merge 42 --squash" \
        'sudo env gh pr merge 42 --squash' \
        'env sudo gh pr merge 42 --squash' \
        'sudo FOO=x gh pr merge 42 --squash' \
        "sudo FOO='value with spaces' gh pr merge 42 --squash" \
        'sudo -u root FOO=x gh pr merge 42 --squash' \
        'sudo -u root gh pr merge 42 --squash' \
        'sudo -k gh pr merge 42 --squash' \
        'env -P /opt/homebrew/bin gh pr merge 42 --squash' \
        "env -S 'gh pr merge 42 --squash'" \
        "env -S'gh pr merge 42 --squash'" \
        "env -S'gh\_pr\_merge\_42\_--squash'" \
        "env -ivS 'gh pr merge 42 --squash'" \
        "env -iS'gh pr merge 42 --squash'" \
        "sudo --preserve-env=FOO env -S'gh pr merge 42 --squash'" \
        "gtimeout -v 0.5s env -S'gh pr merge 42 --squash'" \
        'gtimeout 0.5s gh pr merge 42 --squash' \
        'time -o gh gh pr merge 42 --squash' \
        'time FOO=x gh pr merge 42 --squash' \
        "time FOO='value with spaces' gh pr merge 42 --squash" \
        'time -p FOO=x gh pr merge 42 --squash' \
        'time ! gh pr merge 42 --squash' \
        'time -p ! gh pr merge 42 --squash' \
        'printf x | xargs -J % gh pr merge 42 --body % --squash' \
        'command gh pr merge 42 --body -v --squash' \
        'exec gh pr merge 42 --squash' \
        'builtin exec gh pr merge 42 --squash' \
        'coproc PR_GUARD_JOB { gh pr merge 42 --squash; }' \
        'coproc PR_GUARD_JOB if gh pr merge 42 --squash; then :; fi' \
        'if gh pr merge 42 --squash; then true; fi' \
        '{ gh pr merge 42 --squash; }' \
        'gh</dev/null pr merge 42 --squash' \
        'gh 2>/dev/null pr merge 42 --squash' \
        'gh pr merge>/dev/null 42 --squash' \
        'env gh</dev/null pr merge 42 --squash' \
        'gh {guard_fd}>/dev/null pr merge 42 --squash' \
        'gh pr merge 42 --body --help --squash' \
        "gh pr merge 42 --body 'please --help me' --squash" \
        'gh pr merge 42 --squash # --help' \
        'echo "${PR_GUARD_UNSET:-default}"; gh pr merge 42 --squash' \
        'echo "${#PATH}"; gh pr merge 42 --squash' \
        'gh --help=false pr merge 42 --squash' \
        'gh pr merge --help=false 42 --squash'
    do
        payload="$(jq -cn --arg c "$command_line" '{tool_name:"Bash",tool_input:{command:$c}}')"
        for hook_path in "${HOOK_PATHS[@]}"; do
            hook_at "$hook_path" "$payload"
            [ "$status" -eq 2 ]
            [[ "$output" == *pr-blockers* ]]
            [[ "$output" == *safe-merge* ]]
            [[ "$output" == *PERMISSION_ESCALATION* ]]
        done
    done
}

@test "blocks help-looking option values across tracked harness adapters" {
    for command_line in \
        'gh pr create --title --help --body actual' \
        'command -p gh pr create --title -V --body actual' \
        "gh pr create --title 'document --help behavior' --body actual" \
        'gh pr create --title x --body y # --help' \
        'gh pr create -th --body actual'
    do
        payload="$(jq -cn --arg c "$command_line" '{tool_name:"Bash",tool_input:{command:$c}}')"
        for hook_path in "${HOOK_PATHS[@]}"; do
            hook_at "$hook_path" "$payload"
            assert_blocked
        done
    done
}

@test "allows merge help across tracked harness adapters" {
    for command_line in \
        'gh pr merge --help' \
        'gh pr --help merge' \
        'gh pr merge 42 --squash --help' \
        'gh pr merge -dh' \
        'gh pr create -dh' \
        'gh pr close -dh' \
        'gh pr merge --help=true' \
        'gh pr merge -h=true' \
        'gh pr merge -h=false'
    do
        payload="$(jq -cn --arg c "$command_line" '{tool_name:"Bash",tool_input:{command:$c}}')"
        for hook_path in "${HOOK_PATHS[@]}"; do
            hook_at "$hook_path" "$payload"
            assert_allowed
        done
    done
}

@test "blocks quoted assignments, redirections, and control prefixes across adapters" {
    for command_line in \
        "env FOO='value with spaces' gh pr create --title t --body b" \
        "sudo FOO='value with spaces' gh pr close 42" \
        "time FOO='value with spaces' gh pr reopen 42" \
        '! gh pr create --title t --body b' \
        '</dev/null gh pr create --title t --body b' \
        '2>/dev/null gh pr close 42'
    do
        payload="$(jq -cn --arg c "$command_line" '{tool_name:"Bash",tool_input:{command:$c}}')"
        for hook_path in "${HOOK_PATHS[@]}"; do
            hook_at "$hook_path" "$payload"
            [ "$status" -eq 2 ]
            [[ "$output" == *PERMISSION_ESCALATION* ]]
        done
    done
}

@test "blocks env split-string escapes and inherited expansion across adapters" {
    export PR_GUARD_TEST_CMD=gh
    export PR_GUARD_EXPANDED_COMMAND='gh pr merge 42 --squash'
    export PR_GUARD_ASSIGNMENT_VALUE='alpha beta'
    export PR_GUARD_REDIRECTION_VALUE='input value'
    unset PR_GUARD_TEST_UNSET
    for command_line in \
        "env -S'gh\_pr\_create\_--title\_t\_--body\_b'" \
        "sudo --preserve-env=FOO env -S'gh pr close 42'" \
        "PR_GUARD_TEST_CMD=gh env -S '\${PR_GUARD_TEST_CMD} pr reopen 42'" \
        "env -S '\${PR_GUARD_TEST_CMD} pr create --title t --body b'" \
        "PR_GUARD_TEST_CMD=echo env -S \"\${PR_GUARD_TEST_CMD} pr merge 42 --squash\"" \
        "env -S '\${PR_GUARD_TEST_UNSET} gh pr close 42'" \
        "PR_GUARD_TEST_EMPTY= env -S '\${PR_GUARD_TEST_EMPTY} gh pr create --title t --body b'" \
        "\${PR_GUARD_EXPANDED_COMMAND}" \
        "FOO=\${PR_GUARD_ASSIGNMENT_VALUE} \${PR_GUARD_EXPANDED_COMMAND}" \
        "<<< \${PR_GUARD_REDIRECTION_VALUE} gh pr merge 42 --squash"
    do
        payload="$(jq -cn --arg c "$command_line" '{tool_name:"Bash",tool_input:{command:$c}}')"
        for hook_path in "${HOOK_PATHS[@]}"; do
            hook_at "$hook_path" "$payload"
            [ "$status" -eq 2 ]
            [[ "$output" == *PERMISSION_ESCALATION* ]]
        done
    done
}

@test "allows inert quoted separators and split-string utilities across adapters" {
    for command_line in \
        'echo "foo; gh pr merge 42"' \
        "printf '%s\\n' 'x | gh pr create --title t'" \
        "echo 'safe && gh pr close 42'" \
        "env -S'echo; gh pr merge 42'" \
        "env -S'echo gh pr merge 42'" \
        "env -S='gh pr merge 42'" \
        "gh '{guard_fd}'>/dev/null pr merge 42" \
        'gh "2">/dev/null pr merge 42'
    do
        payload="$(jq -cn --arg c "$command_line" '{tool_name:"Bash",tool_input:{command:$c}}')"
        for hook_path in "${HOOK_PATHS[@]}"; do
            hook_at "$hook_path" "$payload"
            assert_allowed
        done
    done
}

@test "distinguishes heredoc bodies from the command that owns them" {
    inert=$'cat <<\'EOF\'\ngh pr merge 42 --squash\nEOF'
    mutating=$'gh pr merge 42 --squash <<EOF\nbody only\nEOF'
    arithmetic=$'(( guard_value << 1 ))\ngh pr merge 42 --squash'
    arithmetic_matching_line=$'(( guard_value << 1 ))\ngh pr merge 42 --squash\n1'
    arithmetic_expansion=$'echo $((1 << 2))\ngh pr merge 42 --squash\n2'
    export PR_GUARD_HD=EOF
    literal_delimiter=$'cat <<${PR_GUARD_HD}\nbody\n${PR_GUARD_HD}\ngh pr merge 42 --squash'
    ansi_delimiter=$'cat <<$\'EOF\'\nbody\nEOF\ngh pr merge 42 --squash'
    ansi_escaped_inert=$'cat <<$\'E\\x4fF\' >/dev/null\ngh pr merge 42 --squash\nEOF'
    ansi_octal=$'cat <<$\'E\\117F\' >/dev/null\nbody\nEOF\ngh pr merge 42 --squash\nE117F'
    ansi_unicode=$'cat <<$\'E\\u004fF\' >/dev/null\nbody\nEOF\ngh pr merge 42 --squash\nEu004fF'
    ansi_unknown=$'cat <<$\'\\q\' >/dev/null\nbody\n\\q\ngh pr merge 42 --squash\nq'
    for hook_path in "${HOOK_PATHS[@]}"; do
        payload="$(jq -cn --arg c "$inert" '{tool_name:"Bash",tool_input:{command:$c}}')"
        hook_at "$hook_path" "$payload"
        assert_allowed

        payload="$(jq -cn --arg c "$ansi_escaped_inert" '{tool_name:"Bash",tool_input:{command:$c}}')"
        hook_at "$hook_path" "$payload"
        assert_allowed

        payload="$(jq -cn --arg c "$mutating" '{tool_name:"Bash",tool_input:{command:$c}}')"
        hook_at "$hook_path" "$payload"
        [ "$status" -eq 2 ]
        [[ "$output" == *safe-merge* ]]

        for command_line in "$arithmetic" "$arithmetic_matching_line" "$arithmetic_expansion" "$literal_delimiter" "$ansi_delimiter" "$ansi_octal" "$ansi_unicode" "$ansi_unknown"; do
            payload="$(jq -cn --arg c "$command_line" '{tool_name:"Bash",tool_input:{command:$c}}')"
            hook_at "$hook_path" "$payload"
            [ "$status" -eq 2 ]
            [[ "$output" == *safe-merge* ]]
        done
    done
}

@test "allows unrelated launcher operands across tracked harness adapters" {
    for command_line in \
        'env echo gh pr merge 42' \
        'sudo -u root echo gh pr merge 42' \
        'env -P /usr/bin echo gh pr merge 42' \
        'env -iv echo gh pr merge 42' \
        'env -ivu FOO echo gh pr merge 42' \
        'env -iuFOO echo gh pr merge 42' \
        'coproc echo gh pr merge 42' \
        'printf x | xargs -J % echo gh pr merge %' \
        'gtimeout --signal TERM 0.5s echo gh pr merge 42'
    do
        payload="$(jq -cn --arg c "$command_line" '{tool_name:"Bash",tool_input:{command:$c}}')"
        for hook_path in "${HOOK_PATHS[@]}"; do
            hook_at "$hook_path" "$payload"
            assert_allowed
        done
    done
}

@test "allows non-executing launcher modes across tracked harness adapters" {
    for command_line in 'command -v gh pr merge' 'env --help gh pr merge' 'sudo -l gh pr merge' 'sudo -V gh pr merge'; do
        payload="$(jq -cn --arg c "$command_line" '{tool_name:"Bash",tool_input:{command:$c}}')"
        for hook_path in "${HOOK_PATHS[@]}"; do
            hook_at "$hook_path" "$payload"
            assert_allowed
        done
    done
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
