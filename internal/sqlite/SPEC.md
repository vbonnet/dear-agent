# Shared SQLite Opener Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/sqlite`.

## Overview

`internal/sqlite` centralizes the baseline SQLite connection policy used by
dear-agent persistence surfaces.

## EARS Requirements

**SQLITE-01** When a SQLite database is opened, the system shall configure a bounded busy timeout for concurrent access.

**SQLITE-02** When a SQLite database is opened, the system shall enable foreign-key enforcement for every connection.

**SQLITE-03** When a file path or in-memory path is supplied, the system shall return a standard database handle without creating model-specific persistence behavior.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`

## Test Traceability

- Unit package: `internal/sqlite`
