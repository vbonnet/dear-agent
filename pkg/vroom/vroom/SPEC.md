# VROOM Event Emitter Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Typed, harness-neutral VROOM decision events and handoffs.

## EARS Requirements

**VROOM-EVENT-01** When a dispatch decision is emitted, the system shall publish it on the dispatch topic with its typed payload.

**VROOM-EVENT-02** When an escalation decision is emitted, the system shall publish it on the escalation topic with its typed payload.

**VROOM-EVENT-03** When an evaluation decision is emitted, the system shall publish it on the evaluation topic with its typed payload.

**VROOM-EVENT-04** When a gate decision is emitted, the system shall publish it on the gate topic with its typed payload.

**VROOM-EVENT-05** When a context handoff is emitted, the system shall publish sender, receiver, confidence, rationale, and optional gaps on the handoff topic.

**VROOM-EVENT-06** When any decision event is emitted, the system shall add a unique event identifier, the VROOM role, and a UTC timestamp.

**VROOM-EVENT-07** When an event publisher returns an error, the system shall preserve fire-and-forget caller behavior.

**VROOM-EVENT-08** When event conversion or publication panics, the system shall recover inside the asynchronous emitter without crashing the caller.

**VROOM-EVENT-09** When a decision topic is selected, the system shall use the canonical `vroom.decision.*` topic names without a harness or model-family prefix.

## Test Traceability

- Package tests: `pkg/vroom/vroom/emitter_test.go`
- Package tests: `pkg/vroom/vroom/handoff_test.go`
- BDD: `agm/test/bdd/features/vroom_runtime_guardrails.feature`
