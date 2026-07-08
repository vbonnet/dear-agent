# Source Adapter Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/source` defines the harness-neutral knowledge adapter contract used by
workflow durability, search, MCP source tools, and backend-specific knowledge
stores. Every backend implements the same `Adapter` interface so callers can
swap SQLite, markdown, plugin, or graph storage without changing workflow
logic.

## Requirements

**SOURCE-ADAPTER-01** When an adapter reports its name, the system shall return a stable backend identifier suitable for registry lookup and response metadata.

**SOURCE-ADAPTER-02** When an adapter health check is requested, the system shall return nil only when reads and writes can be served.

**SOURCE-ADAPTER-03** When a fetch query has `K` equal to zero, the system shall allow the adapter to apply its documented default result limit.

**SOURCE-ADAPTER-04** When a fetch query has a non-zero `K`, the system shall return no more than `K` sources.

**SOURCE-ADAPTER-05** When a fetch query includes multiple cue filters, the system shall require every requested cue to match.

**SOURCE-ADAPTER-06** When a fetch query includes a work item filter, the system shall match either the exact work item or a segment-aligned child work item.

**SOURCE-ADAPTER-07** When a fetch query includes `After` or `Before`, the system shall treat `After` as inclusive and `Before` as exclusive.

**SOURCE-ADAPTER-08** When a source is added with an existing URI, the system shall update that URI in place rather than creating a duplicate.

**SOURCE-ADAPTER-09** When a source is added successfully, the system shall return a reference containing the source URI, backend name, and indexed timestamp.

**SOURCE-ADAPTER-10** When a backend does not support reranking, the system shall ignore `Rerank` rather than failing the fetch.

## BDD Traceability

- `agm/test/bdd/features/source_knowledge_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
