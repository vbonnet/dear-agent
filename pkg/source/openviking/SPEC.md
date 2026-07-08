# OpenViking Source Adapter Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/source/openviking` is the compile-time placeholder for a future graph
database source adapter. It exposes the configuration and adapter shape now,
while returning a stable not-implemented error until the real backend lands.

## Requirements

**SOURCE-OPENVIKING-01** When an OpenViking adapter is opened, the system shall accept URL, user, password, and database configuration without requiring a live graph backend.

**SOURCE-OPENVIKING-02** When an OpenViking adapter reports its name, the system shall return `openviking`.

**SOURCE-OPENVIKING-03** When an OpenViking adapter is health checked before implementation, the system shall return `ErrNotImplemented`.

**SOURCE-OPENVIKING-04** When an OpenViking adapter is asked to fetch sources before implementation, the system shall return `ErrNotImplemented`.

**SOURCE-OPENVIKING-05** When an OpenViking adapter is asked to add a source before implementation, the system shall return `ErrNotImplemented`.

**SOURCE-OPENVIKING-06** When an OpenViking adapter is closed before implementation, the system shall return nil.

**SOURCE-OPENVIKING-07** When callers inspect OpenViking configuration, the system shall return the configuration supplied at open time.

## BDD Traceability

- `agm/test/bdd/features/source_knowledge_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
