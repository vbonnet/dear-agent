# AGM Message Queue Specification

<!-- Last audited at: 2026-08-26 -->

## Overview

`agm/internal/messages` provides the SQLite-backed queue and acknowledgement
manager for non-disruptive cross-session messages. It owns the typed queue
priority and state vocabularies and preserves priority order, delivery status,
retry state, and acknowledgement timeouts.

## EARS Requirements

**MSG-01** When the message queue is opened, the system shall create the AGM config directory and initialize the owned SQLite database behind one private storage seam with an escaped file URI, WAL journaling, a 5000 ms busy timeout, CHECK enforcement, and an immediate schema transaction.

**MSG-02** When a message is enqueued, the system shall persist message identity, sender, recipient, body, priority, queued timestamp, and queued status.

**MSG-03** When pending messages are listed for one session or all sessions, the system shall order them by priority from critical to low and then by oldest queue time.

**MSG-04** When a message is marked delivered, failed, permanently failed, acknowledged, or timed out, the system shall update only the matching message ID and shall report an error if no row matches.

**MSG-05** When waiting for an acknowledgement, the system shall register a pending acknowledgement and return when either the acknowledgement arrives or the timeout expires.

**MSG-06** When acknowledgement timeout expires, the system shall remove the pending wait, mark timeout state in the queue, and return a timeout error.

**MSG-07** When dead-letter messages are requested without a queue, the system shall return a queue-not-configured error.

**MSG-08** When a raw message priority is parsed, the system shall return a typed priority only for the exact values `CRITICAL`, `HIGH`, `MEDIUM`, and `LOW`.

**MSG-09** When a queue entry has an undeclared priority, the system shall reject the enqueue before attempting a SQLite write.

**MSG-10** When a persisted queue row is read, the system shall decode priority and state through one validation seam and return an error for any undeclared value without exposing the message body.

**MSG-11** When a pending-query scan encounters a persisted state outside `queued`, `delivered`, or `failed`, the system shall surface that validation error rather than silently omitting the row, including for corruption written by an older or external connection.

**MSG-12** When the queue-list CLI receives a non-empty status filter, the system shall parse it once into `QueueState` before calling the message repository, and the repository shall reject any invalid typed value before querying SQLite.

**MSG-13** When an empty queue database is initialized, the system shall create the constrained current table and all owned indexes in one transaction, and SQLite shall reject inserts or updates outside the declared priority and state vocabularies.

**MSG-14** When the stored queue table exactly matches the owned current fingerprint, the system shall retain the table and rows without a rebuild, restore only missing owned indexes, and reject conflicting or unknown user-owned schema objects.

**MSG-15** When the stored queue table exactly matches one of the reachable historical acknowledgement-column orderings, all rows have declared priority and state values, and the complete autoincrement inventory is valid, the system shall atomically rebuild it into the constrained current schema while preserving every row, present acknowledgement value, absent-column default, and queue sequence high-water mark.

**MSG-16** If a database has invalid legacy domain values, an unknown table lineage, an unsupported user-owned table, view, trigger, or index, conflicting owned metadata, invalid or aliased b-tree root pages, or invalid or non-owned autoincrement metadata, then the system shall refuse to open it without changing its schema, rows, or sequence inventory.

**MSG-17** When SQLite rejects or cannot classify a stored schema, the queue constructor shall return a bounded error that identifies only the owned invariant category and shall not expose message content, session identifiers, unknown object names, collation text, or raw driver diagnostics derived from stored SQL.

**MSG-18** When standard SQLite analysis has created exact engine-owned statistics tables, the system shall treat those objects as advisory metadata rather than user schema; a current-schema open shall retain them, while a legacy rebuild may invalidate queue statistics for later analysis.

**MSG-19** When simultaneous constructors receive a transient typed SQLite busy result before immediate transaction ownership, the system shall retry only that result within the same 5000 ms contention budget, honor caller cancellation between retries, and return a bounded error if ownership is not acquired.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/messages/priority_test.go`
- Package tests: `agm/internal/messages/queue_test.go`
- Package tests: `agm/internal/messages/queue_schema_test.go`
- Package tests: `agm/internal/messages/queue_schema_adversarial_test.go`
- Package tests: `agm/internal/messages/ack_test.go`
- Package tests: `agm/internal/messages/rate_limit_test.go`
