#!/usr/bin/env bash
# Harness availability detection

harness_command() {
    case "$1" in
        claude-code) printf '%s\n' "claude" ;;
        codex-cli) printf '%s\n' "codex" ;;
        gemini-cli) printf '%s\n' "gemini" ;;
        opencode-cli) printf '%s\n' "opencode" ;;
        *) return 1 ;;
    esac
}

harness_available() {
    local harness="$1"
    local cmd
    if ! cmd=$(harness_command "$harness"); then
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
    local cmd
    printf "# Harness availability:\n"
    for harness in claude-code codex-cli gemini-cli opencode-cli; do
        cmd=$(harness_command "$harness")
        if harness_available "$harness"; then
            printf "#   %s: available (%s)\n" "$harness" "$(command -v "$cmd")"
        else
            printf "#   %s: NOT AVAILABLE\n" "$harness"
        fi
    done
}
