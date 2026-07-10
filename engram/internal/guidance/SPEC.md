# Engram Guidance Search Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/guidance` discovers and ranks structured `.ai.md` guidance
without coupling retrieval to a particular agent harness or model provider.

## EARS Requirements

**EGS-01** When guidance search runs, the system shall discover `.ai.md` files recursively beneath the configured Engram path.

**EGS-02** When no guidance files exist or the guidance tree cannot be read, the system shall return an explicit discovery error.

**EGS-03** When a guidance file lacks valid delimited YAML frontmatter, the system shall skip that file and continue searching other files.

**EGS-04** When domain, type, or tag filters are supplied, the system shall include only frontmatter that satisfies every supplied filter.

**EGS-05** When matching a query, the system shall score case-insensitive title, description, and tag matches with title matches weighted highest.

**EGS-06** When a guidance file has a zero match score, the system shall omit it from results.

**EGS-07** When results are returned, the system shall sort them by descending score and express paths relative to the configured Engram root when possible.

**EGS-08** When a positive result limit is supplied, the system shall return no more than that number of highest-scoring results.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_governance_runtime_guardrails.feature`
- Package tests: `engram/internal/guidance/*_test.go`
