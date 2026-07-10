# Engram Episodic Memory Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/memory` maintains the project decision log and integrates
episodic memory with Wayfinder phase and session completion.

## EARS Requirements

**EME-01** When episodic memory is initialized without a decision log, the system shall create `DECISION_LOG.md` with private file permissions.

**EME-02** When a memory entry is appended, the system shall serialize its session, event, summary, optional details, and metadata as Markdown and update token usage.

**EME-03** When current session tokens reach at least eighty percent of the configured maximum, the system shall report that a molt is required.

**EME-04** When a decision, error, or molt is recorded, the system shall append an appropriately typed episodic entry.

**EME-05** When a Wayfinder phase completes, the system shall record its outcome, quality signals, decisions, and lessons in episodic memory.

**EME-06** When a Wayfinder session completes above the molt threshold, the system shall append a session molt; otherwise, it shall append a normal completion decision.

**EME-07** When an event bus is configured, the system shall publish harness-neutral Wayfinder phase and session completion events without requiring the bus for memory persistence.

**EME-08** When concurrent Wayfinder callbacks append memory, the system shall serialize writes so the decision log remains coherent.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_core_context_guardrails.feature`
- Package tests: `engram/internal/memory/*_test.go`
