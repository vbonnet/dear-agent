# Hook Work-mode Pivot Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/hooks-bin/internal/pivot` detects a sustained shift from implementation
files to planning or documentation files while suppressing noisy alerts.

## EARS Requirements

**EHWP-01** When a file path has a recognized code or documentation extension or filename, the system shall classify it case-insensitively; otherwise it shall ignore it.

**EHWP-02** When file operations are recorded, the system shall retain a sliding window of the 10 most recent classified operations.

**EHWP-03** When the first three classified operations establish a strict majority, the system shall record that initial work kind.

**EHWP-04** When a code-started session has at least five recent operations, more than half are documentation operations, and at least one is a write, the system shall detect a pivot.

**EHWP-05** When documentation activity consists only of reads, the system shall not detect a work-mode pivot.

**EHWP-06** When an alert was emitted less than five minutes ago, the system shall mark a newly detected pivot as suppressed.

**EHWP-07** When persisted pivot state is missing or malformed, the system shall return empty state without blocking the hook.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks-bin/internal/pivot/*_test.go`
