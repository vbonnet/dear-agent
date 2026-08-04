# Codex Hook Guard Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`.codex/hooks` contains Codex CLI hook scripts that enforce repository guardrails
before tool execution. These hooks are part of cross-harness parity: Codex must
receive the same safety and workflow guidance that Claude Code, AGY, and
OpenCode receive through their native hook or bridge surfaces.

## EARS Requirements

**CODEX-HOOK-01** When a Bash tool command assigns `BEADS_DIR`, the system shall block the tool use and explain that `bd --db` or `bd -C` must be used instead.

**CODEX-HOOK-02** When a Bash tool command dereferences `BEADS_DIR`, the system shall block the tool use and explain that `bd --db` or `bd -C` must be used instead.

**CODEX-HOOK-03** When a Write, Edit, MultiEdit, or apply_patch tool writes `BEADS_DIR` usage to a shell-like file, the system shall block the tool use and print the same Beads guidance.

**CODEX-HOOK-04** When a Write, Edit, MultiEdit, or apply_patch tool writes `BEADS_DIR` text to a non-shell file, the system shall allow the tool use.

**CODEX-HOOK-05** When hook input is malformed, incomplete, or uses an unrelated tool name, the system shall fail open with exit code 0.

**CODEX-HOOK-06** When a variable name merely contains `BEADS_DIR` as a suffix or prefix, the system shall not treat it as unsupported `BEADS_DIR` usage.

**CODEX-HOOK-07** When an attested unattended AGM hook-trust session presents a directly recognizable Bead close or force-close command to the PreToolUse hook, the system shall deny that immediate tool request before invoking the close guard or its user-authenticated CLI dependencies. This input-inspection hook does not claim to mediate descendant process execution or authorize the Beads store.

## BDD Traceability

- `agm/test/bdd/features/hook_parity.feature`

## Test Traceability

- `tests/bats/pretool-beads-dir-block.bats`
- `tests/bats/pretool-bead-close-guard.bats`
