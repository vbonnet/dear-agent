# Trace And Audit Sink Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/trace` maps events into audit records, fans them out to durable
backends, records lifecycle spans, and traces file provenance to AGM sessions.

## Requirements

**TRC-01** When an event is converted to a trace record, the system shall preserve event identity, time, type, session, and structured payload.

**TRC-02** When an audit sink handles a record, the system shall deliver it to every backend and continue delivery when one backend fails.

**TRC-03** When an audit sink closes, the system shall flush and close each backend once and reject subsequent records with `ErrSinkClosed`.

**TRC-04** When the JSONL backend records evidence, the system shall append valid JSONL without replacing prior records.

**TRC-05** When lifecycle span recording is enabled, the system shall attach maintained session, harness, model, duration, and exit attributes without requiring an exporter.

**TRC-06** When tracing file provenance, the system shall tolerate malformed history lines and report every matching session modification deterministically.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_diagnostics_package_guardrails.feature`
- Package tests: `agm/internal/trace/*_test.go`
