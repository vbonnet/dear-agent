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

**MSG-19** When simultaneous constructors receive a transient typed SQLite busy result before immediate transaction ownership, the system shall schedule retries only for that result while the shared 5000 ms retry deadline remains, honor caller cancellation between retries, and return a bounded error if ownership is not acquired; an in-flight driver connection or busy-handler call is outside the retry scheduler's interruption boundary.

**MSG-20** When the message queue is opened, before SQLite receives the database path, the system shall traverse the current user's home, `.config`, and AGM queue-storage root without following symbolic links; every existing component shall be a directory owned by the current user and shall not be group- or other-writable, every component created by this operation shall use mode `0700`, and an existing safe `.config` directory may retain narrower or read-and-search permissions while the AGM storage root shall be tightened to mode `0700`.

**MSG-21** When any of `message_queue.db`, its WAL, SHM, or rollback-journal sidecar already exists, the system shall admit all existing queue-owned leaves before changing any leaf mode or opening SQLite, require each leaf to be a non-symlink regular file owned by the current user with one link, and reject an orphan sidecar or any wrong-owner, linked, symbolic-link, or non-regular leaf without deleting, truncating, replacing, or otherwise mutating the rejected target.

**MSG-22** When queue-storage admission succeeds, before SQLite opens the database, the system shall atomically create a missing `message_queue.db` with mode `0600` or tighten the admitted database and any existing queue-owned sidecars to mode `0600`, preserve existing database identity and content, and open only the admitted database so SQLite-created WAL and SHM files remain mode `0600`; simultaneous first opens shall converge on the same private database without weakening these invariants.

**MSG-23** When SQLite queue initialization completes, the system shall verify without repair that the retained directory boundary and main-file identity still match and that every present main, WAL, SHM, or rollback-journal leaf satisfies its owner, type, link, and exact private-mode invariants; on failure it shall close the database and return an error preserving the unsafe-storage identity.

**MSG-24** When queue-storage trust cannot be established on Darwin or Linux, or when queue construction runs on another operating system, the system shall fail closed with a bounded error preserving `ErrUnsafeQueueStorage` without disclosing symlink targets, message content, session identifiers, row values, raw DDL, or raw SQLite diagnostics.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Test consequence: Deterministic package tests in `agm/internal/messages/queue_storage_unix_test.go` prove MSG-20 through MSG-24 with isolated homes and a permissive process umask: they observe the database plus live WAL and SHM modes, preserve admitted database identity and unrelated same-user AGM configuration while tightening modes, reject unsafe directory and leaf identities without touching their targets, exercise bounded verify-only post-initialization refusal, and cover simultaneous first opens; cross-compilation exercises the unsupported-platform fail-closed owner. This private POSIX storage-admission seam needs no additional Gherkin scenario.
- Package tests: `agm/internal/messages/priority_test.go`
- Package tests: `agm/internal/messages/queue_test.go`
- Package tests: `agm/internal/messages/queue_schema_test.go`
- Package tests: `agm/internal/messages/queue_schema_adversarial_test.go`
- Package tests: `agm/internal/messages/ack_test.go`
- Package tests: `agm/internal/messages/rate_limit_test.go`
