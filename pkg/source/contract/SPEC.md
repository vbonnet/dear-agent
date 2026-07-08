# Source Contract Test Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/source/contract` provides the reusable conformance suite that every
`pkg/source.Adapter` backend must pass. It pins the shared behavior for
round-trips, cue filters, work-item filters, and health checks independently of
the backend implementation.

## Requirements

**SOURCE-CONTRACT-01** When the adapter contract suite runs, the system shall construct a fresh adapter for each named scenario.

**SOURCE-CONTRACT-02** When a source is added and fetched by query, the system shall preserve URI, title, cues, and work item metadata.

**SOURCE-CONTRACT-03** When a source is added, the system shall return a reference whose backend equals the adapter name.

**SOURCE-CONTRACT-04** When cue filters are applied, the system shall enforce AND semantics across requested cues.

**SOURCE-CONTRACT-05** When a work item filter is exact, the system shall return only sources with that exact work item.

**SOURCE-CONTRACT-06** When a work item filter names a parent run, the system shall return child node work items under that run.

**SOURCE-CONTRACT-07** When a fresh adapter is health checked, the system shall require the health check to succeed.

**SOURCE-CONTRACT-08** When an adapter contract scenario finishes, the system shall close the adapter best-effort.

## BDD Traceability

- `agm/test/bdd/features/source_knowledge_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
