# A2A Message Broker Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/a2a/broker` composes model-card discovery and message routing into
the in-process A2A broker interface used by agents that register, discover,
send, receive, and update runtime status.

## EARS Requirements

**A2A-BRK-01** When an agent registers with the broker, the system shall validate and publish its model card through the shared model-card registry.

**A2A-BRK-02** When an agent unregisters, the system shall remove both its model card and its message handler.

**A2A-BRK-03** When a message is sent through the broker, the system shall delegate validation, routing, and inbox queuing to the shared message router.

**A2A-BRK-04** When discovery queries run through the broker, the system shall return agents by role, capability, identity, or active runtime status from the same registry.

**A2A-BRK-05** When an agent status changes, the system shall update the registered model card timestamp and report an error for unknown agents.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/a2a/broker/broker_test.go`
