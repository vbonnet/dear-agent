# Discord HITL Backend Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**DISCORD-HITL-01** When an approval is requested, the system shall render the request and correlate its approval identifier with the sent Discord message.

**DISCORD-HITL-02** When the sender is unavailable or send fails, the system shall return a contextual error without creating a false decision.

**DISCORD-HITL-03** When a correlated reply begins with an approval or rejection decision, the system shall return and persist the corresponding resolution.

**DISCORD-HITL-04** When allowed roles are configured, the system shall reject a recognized decision from a role outside the allowlist.

**DISCORD-HITL-05** When a reply is unrelated or lacks a decision, the system shall ignore it without resolving another request.

**DISCORD-HITL-06** When the wait context is cancelled, the system shall stop waiting and return the context error.

**DISCORD-HITL-07** While approvals originate from any supported harness and model family, the system shall preserve the same request and decision vocabulary.

## BDD Traceability

- Feature: `agm/test/bdd/features/evaluation_control_parity.feature`

## Test Traceability

- Unit package: `pkg/hitl/discord`
