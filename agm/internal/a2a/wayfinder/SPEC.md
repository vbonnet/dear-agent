# A2A Wayfinder Message Specification

<!-- Last audited at: 2026-07-20 -->

## Overview

`agm/internal/a2a/wayfinder` converts Wayfinder status, waypoint transitions,
task updates, handoffs, and blockers into canonical A2A protocol messages.

## EARS Requirements

**A2A-WF-01** When Wayfinder status is converted to an A2A message, the system shall include project name, current waypoint, status, waypoint progress, and next steps.

**A2A-WF-02** When Wayfinder status is completed, the system shall mark the A2A message as consensus reached.

**A2A-WF-03** When a waypoint transition is converted, the system shall produce an awaiting-response message that asks for approval to proceed to the new waypoint.

**A2A-WF-04** When a task update is completed, blocked, or in-progress, the system shall map it to consensus-reached, blocked-on-task-dependency, or pending status respectively.

**A2A-WF-05** When a handoff is converted, the system shall ask the receiving agent to accept the handoff and shall include deliverables and blockers.

**A2A-WF-06** When a blocker is converted, the system shall emit a blocked-on status derived from blocker type and include blocked-by context.

## BDD Traceability

- Feature: `agm/test/bdd/features/wayfinder_parity.feature`
- Package tests: `agm/internal/a2a/wayfinder/wayfinder_test.go`
