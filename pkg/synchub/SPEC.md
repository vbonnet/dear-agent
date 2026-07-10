# Synchronization Hub Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**SYNCHUB-01** When a hub is created, the system shall require a harness-neutral session identifier and apply bounded lock, round, and tombstone defaults.

**SYNCHUB-02** When multiple surfaces answer the same open question, the system shall accept only the first answer and preserve its winning surface.

**SYNCHUB-03** When an answered, cancelled, or expired round receives a late answer, the system shall return the corresponding typed closure error.

**SYNCHUB-04** When a lock is acquired, the system shall issue a monotonic fence token and reject reentry by the same holder.

**SYNCHUB-05** When a lock deadline expires, the system shall reclaim the lock without allowing a stale handle to release a newer holder.

**SYNCHUB-06** When hub state changes, the system shall publish best-effort lifecycle events without making bus delivery a correctness dependency.

**SYNCHUB-07** When the HTTP synchronization server starts, the system shall reject public binds, require bearer authentication, and store token material privately.

**SYNCHUB-08** When Claude Code, Codex, Antigravity, or OpenCode creates a synchronization session, the system shall apply the same Q&A, locking, and lifecycle semantics for every model family.

## BDD Traceability

- Feature: `agm/test/bdd/features/agent_utility_parity.feature`

## Test Traceability

- Unit package: `pkg/synchub`
