# AGM Database Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm/internal/db`.

## Overview

`agm/internal/db` owns SQLite persistence for AGM sessions, conversation
messages, escalation events, hierarchy queries, and FTS5-backed session search.
It is a harness-neutral storage layer: callers persist shared manifest fields
such as harness, model, context, tmux metadata, Claude resume metadata, and
Engram metadata without creating a Claude-only database contract.

## EARS Requirements

**AGM-DB-01** When a database is opened, the system shall apply the embedded SQLite schema before returning the database handle.

**AGM-DB-02** When a session create, read, update, delete, hierarchy, message, or escalation operation receives an empty session identifier, the system shall reject the operation.

**AGM-DB-03** When a session is created, the system shall persist harness-neutral manifest metadata including harness, model, context tags, tmux session name, Claude UUID, and Engram metadata.

**AGM-DB-04** When a session is retrieved, the system shall reconstruct JSON-backed context tags and Engram identifiers into manifest fields.

**AGM-DB-05** When a session update or delete affects no rows, the system shall return a not-found error.

**AGM-DB-06** When a session is deleted, the system shall cascade-delete messages and escalations through SQLite foreign-key constraints.

**AGM-DB-07** When messages are retrieved for a session, the system shall return them in chronological order and apply role, harness, time, limit, and offset filters when provided.

**AGM-DB-08** When an escalation is created, the system shall store it unresolved and return the inserted escalation identifier.

**AGM-DB-09** When unresolved escalations are queried, the system shall return only rows whose resolved flag is false ordered by newest detection time first.

**AGM-DB-10** When a session hierarchy is queried, the system shall return direct children in creation order, root parent as nil, and reject circular parent references.

**AGM-DB-11** When session search receives an empty query or unbalanced quotes, the system shall reject the query before executing FTS5 SQL.

**AGM-DB-12** When session search runs, the system shall use the FTS5 session index with parameterized filters for lifecycle, harness, creation time, parent session, escalation presence, limit, and offset.

## BDD Traceability

- Feature: `agm/test/bdd/features/db_persistence_guardrails.feature`

## Test Traceability

- Unit package: `agm/internal/db`
