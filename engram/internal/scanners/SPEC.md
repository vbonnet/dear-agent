# Engram Project Scanner Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/scanners` detects project languages, frameworks, dependencies,
Git metadata, and recent conversation signals for metacontext analysis.

## EARS Requirements

**ESC-01** When file scanning runs, the system shall walk the working tree while respecting context cancellation.

**ESC-02** When file scanning encounters hidden directories, sensitive files, generated dependency trees, or oversized files, the system shall skip them before content analysis.

**ESC-03** When recognized file extensions or framework markers are present, the system shall emit bounded language and framework signals with source and confidence metadata.

**ESC-04** When dependency manifests are present, the system shall parse supported Node, Go, and Python dependency formats and emit corresponding framework or tool signals.

**ESC-05** When a dependency manifest is malformed, the system shall return scanner context for that manifest rather than fabricate signals.

**ESC-06** When Git metadata is available, the system shall emit repository signals; when the directory is not a Git repository, the system shall return no Git signal without failing analysis.

**ESC-07** When conversation scanning runs, the system shall inspect only the configured recent-turn window and match supported technologies case-insensitively.

**ESC-08** When repeated conversation mentions occur, the system shall increase confidence according to the documented bounded scoring rule.

**ESC-09** When scanners identify themselves, the system shall expose stable names and priorities for metacontext orchestration.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_reflection_storage_guardrails.feature`
- Package tests: `engram/internal/scanners/*_test.go`
