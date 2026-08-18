# Wayfinder session requirements specification

<!-- Last audited at: 2026-07-17 -->

**Status:** Active
**Scope:** Session orchestration behind the `wayfinder session` command tree.

## EARS requirements

**WFSESSION-01** When `start` succeeds, the system shall create canonical schema 2.0 status with validated project type and risk.

**WFSESSION-02** When an existing session is detected, the system shall resume safely unless a justified force operation is requested.

**WFSESSION-03** When any normal lifecycle command reads state, the system shall parse only canonical status.

**WFSESSION-04** When `next-phase` runs, the system shall return the next non-skipped phase from the named sequence.

**WFSESSION-05** When `start-phase` runs, the system shall enforce ordering and phase-specific start gates.

**WFSESSION-06** When `complete-phase` runs, the system shall enforce artifact, content, git, code, review, and child-project gates that apply.

**WFSESSION-07** When `rewind-to` runs, the system shall require a prior completed target and persist the reason.

**WFSESSION-08** When `status` runs, the system shall render project, lifecycle, history, and remaining named phases.

**WFSESSION-09** When `end` runs, the system shall validate the terminal state, persist it, and publish the completion event.

**WFSESSION-10** When task commands run, the system shall mutate only the canonical roadmap in the current status file.

**WFSESSION-11** When a lifecycle state requires diagnostic context, the system shall reject a transition that omits that context.

**WFSESSION-12** When a write fails, the system shall preserve the prior status file.

## Traceability

- Package tests: `commands/*_test.go`, `internal/status/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
- Strictness BDD: `agm/test/bdd/features/legacy_spec_strictness_guardrails.feature`
