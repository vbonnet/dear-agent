# Engram Reflection Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/reflection` captures session learning, classifies failures,
parses retrospectives, and synthesizes recurring patterns through multiple lenses.

## EARS Requirements

**ERF-01** When reflection context reports errors, the system shall classify an incomplete session as failed and a completed session as partially successful.

**ERF-02** When no explicit errors exist but failure language is present, the system shall classify the outcome as partial; otherwise, it shall classify it as successful.

**ERF-03** When a failed or partial outcome is classified, the system shall apply syntax, permission, timeout, tool-misuse, and fallback categories in deterministic priority order.

**ERF-04** When a lesson is supplied, the system shall preserve it; otherwise, it shall derive a bounded lesson from the error or category.

**ERF-05** When a reflection is saved, the system shall reject nil reflections and unsafe or too-short session identifiers before constructing the output path.

**ERF-06** When a valid reflection is saved, the system shall write private `.ai.md` content with retrieval frontmatter, trigger, learning, outcome details, and session metrics.

**ERF-07** When a retrospective is parsed, the system shall extract technical challenges and improvement bullets across supported heading and list styles.

**ERF-08** When retrospective learnings are converted, the system shall create reflection records associated with the supplied session.

**ERF-09** When multi-lens synthesis runs, the system shall cluster related statements and produce root-cause, recurrence, remediation, and classification findings deterministically.

**ERF-10** When recurrence or severity crosses the documented thresholds, the system shall classify the pattern as systemic; otherwise, it shall retain one-off classification.

**ERF-11** When a synthesis report is rendered, the system shall include lens summaries and source artifact traceability.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_reflection_storage_guardrails.feature`
- Package tests: `engram/internal/reflection/*_test.go`
