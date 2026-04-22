#!/usr/bin/env bash
# Harness availability detection

declare -A HARNESS_CMDS=(
    [claude-code]="claude"
    [codex-cli]="codex"
    [gemini-cli]="gemini"
    [opencode-cli]="opencode"
)

harness_available() {
    local harness="$1"
    local cmd="${HARNESS_CMDS[$harness]:-}"
    if [[ -z "$cmd" ]]; then
        return 1
    fi
    command -v "$cmd" >/dev/null 2>&1
}

skip_if_no_harness() {
    local harness="$1"
    if ! harness_available "$harness"; then
        test_skip "harness '$harness' tests" "CLI not installed"
        return 1
    fi
    return 0
}

detect_all_harnesses() {
    printf "# Harness availability:\n"
    for harness in claude-code codex-cli gemini-cli opencode-cli; do
        if harness_available "$harness"; then
            printf "#   %s: available (%s)\n" "$harness" "$(command -v "${HARNESS_CMDS[$harness]}")"
        else
            printf "#   %s: NOT AVAILABLE\n" "$harness"
        fi
    done
}
