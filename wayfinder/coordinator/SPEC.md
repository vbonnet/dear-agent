# Wayfinder Coordinator Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Provider-neutral project coordination, sandbox reuse, monitoring, and events.

## EARS Requirements

**WAYFINDER-COORD-01** When a coordinator is created, the system shall validate concurrency, polling, timeout, and log configuration.

**WAYFINDER-COORD-02** When project execution needs isolation, the system shall reuse an existing sandbox or create one through the supplied `SandboxManager` contract.

**WAYFINDER-COORD-03** When the preferred sandbox cannot be created, the system shall apply the configured fallback without selecting an agent harness or model provider.

**WAYFINDER-COORD-04** While projects execute concurrently, the system shall enforce the configured concurrency limit.

**WAYFINDER-COORD-05** When status output is monitored, the system shall update a project only after complete canonical schema 2.0 validation; a missing file may report a waiting default, while invalid status shall not be exposed.

**WAYFINDER-COORD-06** When events are emitted, the system shall deliver them to registered listeners without deadlocking shutdown.

## Test Traceability

- Package tests: `wayfinder/coordinator/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
