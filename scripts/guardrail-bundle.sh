#!/bin/bash
# guardrail-bundle.sh — the single deterministic guardrail bundle the WF-A
# stop-hook loop runs (.claude/hooks/stop-guardrail-feedback, bead ce-vrux).
#
# This is the extension SEAM: WF-B (Semgrep) and WF-C (architectural unit
# tests) append `run_step` lines below. WF-A owns the loop, not the checks.
# Today the bundle is the fast preflight parity tier (vet + build + lint).
#
# Exits non-zero if ANY step fails (so the hook blocks the stop). Set
# GUARDRAIL_CMD to override the whole bundle for ad-hoc runs and tests.
set -uo pipefail
ROOT="${PREFLIGHT_PROJECT_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
if [ -n "${AGM_CODEX_HOOK_ROOT:-}" ]; then
  PREFLIGHT="${AGM_CODEX_HOOK_ROOT:-.}/scripts/preflight.sh"
else
  PREFLIGHT="$ROOT/scripts/preflight.sh"
fi
[ -n "${GUARDRAIL_CMD:-}" ] && exec /bin/bash -c "$GUARDRAIL_CMD"
rc=0
run_step() { printf '\n=== guardrail: %s ===\n' "$1"; shift; "$@" || rc=1; }
run_step "preflight (vet+build+lint)" /usr/bin/env PREFLIGHT_PROJECT_ROOT="$ROOT" /bin/bash "$PREFLIGHT" --fast
# WF-B: run_step "semgrep" semgrep --error --config "$ROOT/semgrep" "$ROOT"
# WF-C: run_step "arch-tests" go -C "$ROOT" test ./... -run TestArch
exit "$rc"
