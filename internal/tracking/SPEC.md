# Engram Access Tracking Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/tracking`.

## Overview

`internal/tracking` accumulates Engram retrieval metadata in memory and
persists it to frontmatter without coupling retrieval history to a harness or
model provider.

## EARS Requirements

**TRACKING-01** When an Engram is accessed, the system shall increment its pending count and retain the latest access timestamp safely across concurrent callers.

**TRACKING-02** When pending records are flushed, the system shall attempt every Engram update even if an earlier update fails.

**TRACKING-03** When an Engram update succeeds, the system shall remove only that successful record from the pending set.

**TRACKING-04** When an Engram update fails, the system shall retain the record for a later retry.

**TRACKING-05** When metadata is persisted, the system shall increment retrieval count, update last access time, and preserve Engram content.

**TRACKING-06** When legacy metadata omits creation time or encoding strength, the system shall populate stable fallback values during persistence.

**TRACKING-07** When metadata serialization succeeds, the system shall replace the Engram through a temporary file and preserve its permissions.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`

## Test Traceability

- Unit package: `internal/tracking`
