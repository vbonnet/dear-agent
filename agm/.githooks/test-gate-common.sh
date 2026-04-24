#!/bin/bash
# test-gate-common.sh — Shared enforcement logic for pre-commit and pre-merge-commit hooks.
# Sources this file and calls: check_test_gate "$hook_name" with changed files on stdin.
#
# Environment:
#   AGM_SKIP_TEST_GATE=1  — bypass all checks (for infrastructure-only changes)

set -euo pipefail

# Infrastructure file patterns that don't require tests
INFRA_PATTERNS='\.md$|\.sh$|\.txt$|\.yml$|\.yaml$|\.json$|\.toml$|^Makefile$|^\.github/|^\.githooks/|^\.gitignore$|^go\.mod$|^go\.sum$|^docs/|^\.claude|^\.wayfinder/|^scripts/|^skills/|^hooks/|^testdata/|^vendor/|^\.goreleaser|^LICENSE|^\.golangci'

check_test_gate() {
    local hook_name="${1:-pre-commit}"
    local changed_files
    changed_files=$(cat)

    # Skip if override is set
    if [ "${AGM_SKIP_TEST_GATE:-0}" = "1" ]; then
        echo "⚠️  [$hook_name] AGM_SKIP_TEST_GATE=1 — skipping test enforcement"
        return 0
    fi

    # If no files changed, nothing to check
    if [ -z "$changed_files" ]; then
        return 0
    fi

    # Separate Go code files from test files
    local go_code_files
    local go_test_files
    local non_infra_go_files

    go_code_files=$(echo "$changed_files" | grep '\.go$' | grep -v '_test\.go$' | grep -v '^vendor/' || true)
    go_test_files=$(echo "$changed_files" | grep '_test\.go$' || true)

    # Filter out infrastructure-only Go files (generated, tools, etc.)
    non_infra_go_files=$(echo "$go_code_files" | grep -vE "$INFRA_PATTERNS" || true)

    # If no non-infrastructure Go code files changed, allow
    if [ -z "$non_infra_go_files" ]; then
        return 0
    fi

    # GATE: Go code changed but no test files changed
    if [ -z "$go_test_files" ]; then
        echo ""
        echo "╔══════════════════════════════════════════════════════════════╗"
        echo "║  ❌  TEST ENFORCEMENT GATE — $hook_name blocked            ║"
        echo "╠══════════════════════════════════════════════════════════════╣"
        echo "║                                                            ║"
        echo "║  Go source files were changed without any test files.      ║"
        echo "║  TDD is mandatory — every code change needs tests.         ║"
        echo "║                                                            ║"
        echo "╚══════════════════════════════════════════════════════════════╝"
        echo ""
        echo "Changed Go files without tests:"
        echo "$non_infra_go_files" | while read -r f; do echo "  - $f"; done
        echo ""
        echo "To fix: add or modify *_test.go files covering your changes."
        echo "To override (infrastructure-only): AGM_SKIP_TEST_GATE=1 git commit ..."
        echo ""
        return 1
    fi

    # WARN: Check for t.Skip in new test code
    local skip_warnings=""
    if [ -n "$go_test_files" ]; then
        while IFS= read -r test_file; do
            if [ -f "$test_file" ]; then
                local skips
                skips=$(grep -n 't\.Skip' "$test_file" 2>/dev/null || true)
                if [ -n "$skips" ]; then
                    skip_warnings="${skip_warnings}\n  ⚠️  t.Skip found in $test_file:\n"
                    skip_warnings="${skip_warnings}$(echo "$skips" | while read -r line; do echo "      $line"; done)\n"
                fi
            fi
        done <<< "$go_test_files"
    fi

    # WARN: Check for TODO/deferred test patterns in code
    local todo_warnings=""
    if [ -n "$non_infra_go_files" ]; then
        while IFS= read -r code_file; do
            if [ -f "$code_file" ]; then
                local todos
                todos=$(grep -inE '(TODO|FIXME|HACK).*test|deferred.*test|skip.*test' "$code_file" 2>/dev/null || true)
                if [ -n "$todos" ]; then
                    todo_warnings="${todo_warnings}\n  ⚠️  Deferred test pattern in $code_file:\n"
                    todo_warnings="${todo_warnings}$(echo "$todos" | while read -r line; do echo "      $line"; done)\n"
                fi
            fi
        done <<< "$non_infra_go_files"
    fi

    # WARN: Check for AGM_SKIP_TEST_GATE references in Go code (workaround bias)
    local bypass_warnings=""
    if [ -n "$non_infra_go_files" ]; then
        while IFS= read -r code_file; do
            if [ -f "$code_file" ]; then
                local bypasses
                bypasses=$(grep -n 'AGM_SKIP_TEST_GATE' "$code_file" 2>/dev/null || true)
                if [ -n "$bypasses" ]; then
                    bypass_warnings="${bypass_warnings}\n  ⚠️  AGM_SKIP_TEST_GATE reference in $code_file:\n"
                    bypass_warnings="${bypass_warnings}$(echo "$bypasses" | while read -r line; do echo "      $line"; done)\n"
                fi
            fi
        done <<< "$non_infra_go_files"
    fi

    # WARN: Check for --force bypass suggestions in Go code (workaround bias)
    local force_warnings=""
    if [ -n "$non_infra_go_files" ]; then
        while IFS= read -r code_file; do
            if [ -f "$code_file" ]; then
                local forces
                forces=$(grep -n '\-\-force\s\+\(to\s\+\)\?\(override\|bypass\|skip\)' "$code_file" 2>/dev/null || true)
                if [ -n "$forces" ]; then
                    force_warnings="${force_warnings}\n  ⚠️  --force bypass suggestion in $code_file:\n"
                    force_warnings="${force_warnings}$(echo "$forces" | while read -r line; do echo "      $line"; done)\n"
                fi
            fi
        done <<< "$non_infra_go_files"
    fi

    # Print warnings if any
    if [ -n "$skip_warnings" ] || [ -n "$todo_warnings" ] || [ -n "$bypass_warnings" ] || [ -n "$force_warnings" ]; then
        echo ""
        echo "┌──────────────────────────────────────────────────────────────┐"
        echo "│  ⚠️   TEST ENFORCEMENT — warnings                          │"
        echo "└──────────────────────────────────────────────────────────────┘"
        if [ -n "$skip_warnings" ]; then
            echo -e "$skip_warnings"
        fi
        if [ -n "$todo_warnings" ]; then
            echo -e "$todo_warnings"
        fi
        if [ -n "$bypass_warnings" ]; then
            echo -e "$bypass_warnings"
        fi
        if [ -n "$force_warnings" ]; then
            echo -e "$force_warnings"
        fi
        echo "  Tests are present but please address the warnings above."
        echo ""
    fi

    return 0
}
