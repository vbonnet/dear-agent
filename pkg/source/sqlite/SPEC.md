# SQLite Source Adapter Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/source/sqlite` is the default source adapter. It stores source metadata in
SQLite, mirrors searchable fields into FTS5, and applies structured filters in
SQL so search, MCP source tools, and workflow durability share one local
knowledge index.

## Requirements

**SOURCE-SQLITE-01** When a SQLite adapter opens a path, the system shall create or migrate the embedded source schema before returning the adapter.

**SOURCE-SQLITE-02** When a SQLite adapter wraps an existing database handle, the system shall apply the source schema without taking ownership of the handle.

**SOURCE-SQLITE-03** When a source is added without a URI, the system shall reject the add request.

**SOURCE-SQLITE-04** When a source is added without an indexed timestamp, the system shall assign the current UTC timestamp.

**SOURCE-SQLITE-05** When source custom metadata is present, the system shall persist it as JSON.

**SOURCE-SQLITE-06** When a source is added with an existing URI, the system shall update the row and refresh the FTS mirror atomically.

**SOURCE-SQLITE-07** When a text query is present, the system shall search through FTS5 and order by rank before indexed timestamp.

**SOURCE-SQLITE-08** When no text query is present, the system shall order results by descending indexed timestamp.

**SOURCE-SQLITE-09** When cue filters are applied, the system shall match exact cue tokens rather than substrings inside other cues.

**SOURCE-SQLITE-10** When a URI lookup misses, the system shall return `source.ErrNotFound`.

**SOURCE-SQLITE-11** When closing an adapter that owns its database handle, the system shall close the handle and clear it.

## BDD Traceability

- `agm/test/bdd/features/source_knowledge_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
