# AGM Usage Accounting Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/usage` parses Claude Code JSONL transcripts and estimates token
usage, cost, burn rate, and worker/supervisor attribution for quota visibility.
It is an after-the-fact accounting source, not authoritative billing data.

## EARS Requirements

**USE-01** When pricing is requested for a known Claude model tier, the system shall return that tier's per-million-token estimate.

**USE-02** When pricing is requested for an unknown or synthetic model, the system shall return zero pricing so unknown usage cannot inflate cost.

**USE-03** When token cost is computed, the system shall include input, output, cache-read, and cache-write categories with their corresponding prices.

**USE-04** When usage collection walks transcript files, the system shall only collect assistant usage entries at or after the requested timestamp.

**USE-05** When usage collection sees unreadable, malformed, or stale files, the system shall skip the unusable data without aborting the entire collection.

**USE-06** When duplicate request and message identifiers appear across transcripts, the system shall count the entry once.

**USE-07** When a usage entry cwd is under `.agm/sandboxes`, the system shall classify it as worker usage; otherwise it shall classify it as supervisor usage.

## BDD Traceability

- Feature: `agm/test/bdd/features/quota_parity.feature`
- Package tests: `agm/internal/usage/usage_test.go`
