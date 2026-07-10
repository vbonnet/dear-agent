# Engram Metacontext Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/metacontext` safely analyzes project signals, bounds the
resulting context, caches stable data, and protects the repository constitution.

## EARS Requirements

**EMC-01** When analysis receives a working directory, the system shall reject traversal patterns, inaccessible paths, and paths that do not identify a directory.

**EMC-02** When project scanners run, the system shall execute them concurrently, recover scanner panics, and preserve failures as warnings instead of aborting all analysis.

**EMC-03** When scanner signals are aggregated, the system shall group, score, deduplicate, sort, and truncate them to the configured per-type limits.

**EMC-04** When stable project metacontext is cached, the system shall key it by working directory and keep conversation-derived signals uncached.

**EMC-05** When a cache entry is read, the system shall reject corrupted entries, nil signal slices, invalid confidence values, oversized collections, and token-budget violations.

**EMC-06** When the unified cache exceeds capacity, the system shall evict least-recently-used entries and remain safe under concurrent access.

**EMC-07** When metacontext is returned to a caller, the system shall return independent data so caller mutation cannot alter cached state.

**EMC-08** When both `AGENTS.md` and a harness-specific instruction file exist, the system shall load `AGENTS.md` as the canonical constitution.

**EMC-09** When `AGENTS.md` is absent and `CLAUDE.md` exists, the system shall allow `CLAUDE.md` only as a backward-compatible constitution source.

**EMC-10** When the loaded constitution is modified or deleted at runtime, the system shall fail integrity validation and require the governed improvement workflow.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_core_context_guardrails.feature`
- Package tests: `engram/internal/metacontext/*_test.go`
