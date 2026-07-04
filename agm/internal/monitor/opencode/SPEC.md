# OpenCode Monitor Specification

<!-- Last audited at: 2026-07-03 -->

## Purpose

`agm/internal/monitor/opencode` converts OpenCode server-sent events into AGM
state transitions and publishes lifecycle observations into AGM monitoring
pipelines. The monitor keeps OpenCode's client/server event stream compatible
with AGM's harness-neutral state model.

## EARS Requirements

**OCMON-01** When an OpenCode event is parsed, the system shall validate that the event type and timestamp are present.

**OCMON-02** When OpenCode asks for permission, the system shall map the event to AGM's awaiting-permission state and preserve permission metadata.

**OCMON-03** When OpenCode tool execution starts or finishes, the system shall map the event to AGM working or idle state respectively.

**OCMON-04** When an unknown OpenCode event type is received, the system shall use a safe working-state default instead of treating the session as idle.

**OCMON-05** When OpenCode lifecycle events are published, the system shall preserve session identifiers and event metadata for downstream state writers.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
