# Hook Heartbeat State Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/hooks-bin/internal/heartbeat` records bounded file-operation batches,
long-running operations, and task activity for hook status reporting.

## EARS Requirements

**EHHS-01** When heartbeat state is missing, unreadable, or malformed, the system shall return empty state without blocking the hook.

**EHHS-02** When heartbeat state is saved, the system shall create private parent directories and write a private JSON file.

**EHHS-03** When consecutive file tools are recorded, the system shall increment the batch and retain at most the 20 most recent file paths.

**EHHS-04** When a non-file tool is recorded, the system shall reset the current file batch.

**EHHS-05** When at least three consecutive file operations are recorded, the system shall identify the operation as a batch.

**EHHS-06** When a long-running operation is recorded, the system shall cap its command preview at 80 bytes and expose its elapsed duration.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks-bin/internal/heartbeat/*_test.go`
