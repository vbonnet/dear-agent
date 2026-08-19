#!/usr/bin/env bash
# guardrail-bundle.sh — the single deterministic guardrail bundle the WF-A
# stop-hook loop runs (.claude/hooks/stop-guardrail-feedback, bead ce-vrux).
#
# This is the extension SEAM: WF-B (Semgrep) and WF-C (architectural unit
# tests) append `run_step` lines below. WF-A owns the loop, not the checks.
#
# Steps here are chosen from evidence, not taste: a check earns its place by
# catching a mistake the automated PR reviewer has already had to catch by
# hand, repeatedly. Catching it here -- before the agent stops -- costs a
# second; catching it in review costs a round trip.
#   * preflight  -- vet + build + lint parity tier.
#   * docref     -- living documents naming files that do not exist. Mined
#                   from the review corpus: 32 comments across 12 PRs.
#
# Exits non-zero if ANY step fails (so the hook blocks the stop). Set
# GUARDRAIL_CMD to override the whole bundle for ad-hoc runs and tests.
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[ -n "${GUARDRAIL_CMD:-}" ] && exec bash -c "$GUARDRAIL_CMD"
rc=0
run_step() { printf '\n=== guardrail: %s ===\n' "$1"; shift; "$@" || rc=1; }
run_step "preflight (vet+build+lint)" "$ROOT/scripts/preflight.sh" --fast
run_step "docref (living docs name real files)" go -C "$ROOT" run ./tools/docref-lint
# WF-B: run_step "semgrep" semgrep --error --config "$ROOT/semgrep" "$ROOT"
# WF-C: run_step "arch-tests" go -C "$ROOT" test ./... -run TestArch
exit "$rc"
