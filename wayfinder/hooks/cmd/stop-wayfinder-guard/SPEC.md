# Wayfinder stop guard requirements specification

<!-- Last audited at: 2026-07-17 -->

**Status:** Active
**Scope:** Stop-hook behavior for active Wayfinder sessions.

## EARS requirements

**WFSTOP-01** When no `WAYFINDER-STATUS.md` exists, the system shall allow the unrelated session to stop.

**WFSTOP-02** When canonical status is valid and terminal, the system shall allow the session to stop.

**WFSTOP-03** When canonical status is in progress, the system shall warn that work remains without treating the lifecycle as terminal.

**WFSTOP-04** When status has a missing or unsupported schema version, the system shall block with a parse error.

**WFSTOP-05** When status cannot be read or parsed, the system shall block rather than assume completion.

**WFSTOP-06** When a completed project has no substantive retrospective artifact, the system shall block with remediation guidance.

## Traceability

- Tests: `wayfinder/hooks/cmd/stop-wayfinder-guard/main_test.go`
- BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
