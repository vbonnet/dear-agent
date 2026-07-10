# GitHub HITL Backend Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**GITHUB-HITL-01** When an approval is requested, the system shall post a PR comment containing the request fields and a machine-readable approval identifier.

**GITHUB-HITL-02** When the comment client is unavailable or posting fails, the system shall return a contextual error.

**GITHUB-HITL-03** When waiting for a decision, the system shall poll only comments after the request comment at the configured bounded interval.

**GITHUB-HITL-04** When a later comment begins with approval or rejection vocabulary, the system shall return the corresponding resolution.

**GITHUB-HITL-05** When comments do not contain decision vocabulary, the system shall continue waiting without inventing a resolution.

**GITHUB-HITL-06** When the wait context is cancelled, the system shall stop polling and return the context error.

**GITHUB-HITL-07** While approvals originate from any supported harness and model family, the system shall preserve the same request and decision semantics as other HITL backends.

## BDD Traceability

- Feature: `agm/test/bdd/features/evaluation_control_parity.feature`

## Test Traceability

- Unit package: `pkg/hitl/github`
