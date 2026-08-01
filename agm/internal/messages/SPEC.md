# AGM Message Queue Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/messages` provides the SQLite-backed queue and acknowledgement
manager for non-disruptive cross-session messages. It owns the typed queue
priority and state vocabularies and preserves priority order, delivery status,
retry state, and acknowledgement timeouts.

## EARS Requirements

**MSG-01** When the message queue is opened, the system shall create the AGM config directory, enable WAL mode, create the base schema, and apply idempotent acknowledgement-column migrations.

**MSG-02** When a message is enqueued, the system shall persist message identity, sender, recipient, body, priority, queued timestamp, and queued status.

**MSG-03** When pending messages are listed for one session or all sessions, the system shall order them by priority from critical to low and then by oldest queue time.

**MSG-04** When a message is marked delivered, failed, permanently failed, acknowledged, or timed out, the system shall update only the matching message ID and shall report an error if no row matches.

**MSG-05** When waiting for an acknowledgement, the system shall register a pending acknowledgement and return when either the acknowledgement arrives or the timeout expires.

**MSG-06** When acknowledgement timeout expires, the system shall remove the pending wait, mark timeout state in the queue, and return a timeout error.

**MSG-07** When dead-letter messages are requested without a queue, the system shall return a queue-not-configured error.

**MSG-08** When a raw message priority is parsed, the system shall return a typed priority only for the exact values `CRITICAL`, `HIGH`, `MEDIUM`, and `LOW`.

**MSG-09** When a queue entry has an undeclared priority, the system shall reject the enqueue before attempting a SQLite write.

**MSG-10** When a persisted queue row is read, the system shall decode priority and state through one validation seam and return an error for any undeclared value without exposing the message body.

**MSG-11** When a pending-query scan encounters a persisted state outside `queued`, `delivered`, or `failed`, the system shall surface that validation error rather than silently omitting the row. Existing databases are not migrated or rewritten by this policy.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/messages/priority_test.go`
- Package tests: `agm/internal/messages/queue_test.go`
- Package tests: `agm/internal/messages/ack_test.go`
- Package tests: `agm/internal/messages/rate_limit_test.go`
