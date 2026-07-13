# Stop Hook Utility Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**STOPHOOK-01** When Claude Code, Codex, Antigravity, or OpenCode hook JSON is read, the system shall preserve the explicit harness and normalize supported session, transcript, reason, and workspace aliases.

**STOPHOOK-02** When hook input is malformed JSON or cannot be read, the system shall return a contextual error.

**STOPHOOK-03** When a hook function completes before its deadline, the system shall return the function exit code.

**STOPHOOK-04** When a hook function exceeds its deadline, the system shall fail open with exit code zero.

**STOPHOOK-05** When a hook function panics, the system shall recover and return a failing exit code.

**STOPHOOK-06** When findings contain a blocking severity, the system shall return hook exit code two.

**STOPHOOK-07** When findings contain only pass or warning severities, the system shall return hook exit code zero.

**STOPHOOK-08** When repository state is inspected, the system shall report git status, unpushed commits, extra branches, and worktrees without requiring a harness-specific process.

## BDD Traceability

- Feature: `agm/test/bdd/features/agent_utility_parity.feature`

## Test Traceability

- Unit package: `pkg/stophook`
