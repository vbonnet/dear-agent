# A2A Messaging Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/a2a/messaging` defines structured A2A messages and an in-process
router for direct, broadcast, and role-based delivery with handler dispatch and
offline inbox queuing.

## EARS Requirements

**A2A-MSG-01** When creating requests or delegations, the system shall assign a unique ID, start a correlation chain, use direct routing, and default priority to normal.

**A2A-MSG-02** When creating a response, the system shall preserve the original correlation ID, address the original sender, and retain the original priority.

**A2A-MSG-03** When validating messages, the system shall require identity, sender, subject, body, recognized type, recognized routing mode, and route-specific destination fields.

**A2A-MSG-04** When sending an expired message, the system shall reject delivery before invoking handlers or queueing inbox entries.

**A2A-MSG-05** When direct delivery has no live handler, the system shall queue the message to the recipient inbox and report the recipient as delivered.

**A2A-MSG-06** When broadcasting a message, the system shall deliver best-effort to registered handlers other than the sender.

**A2A-MSG-07** When routing to a role, the system shall resolve targets through the model-card registry and queue messages for matching agents without live handlers.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/a2a/messaging/types_test.go`
- Package tests: `agm/internal/a2a/messaging/router_test.go`
