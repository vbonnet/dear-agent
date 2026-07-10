# Sentinel Work Intake Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/sentinel/intake` parses, validates, watches, and transitions
versioned sentinel work items.

## Requirements

**INT-01** When a work item is parsed, the system shall require the supported schema version, identity, title, creation time, priority, scope, and status values.

**INT-02** When JSONL contains multiple work items, the system shall skip empty lines, preserve item order, and reject malformed records.

**INT-03** When a status change is requested, the system shall permit only declared lifecycle transitions.

**INT-04** When the intake watcher observes a stable eligible file, the system shall submit it to the configured processor once.

**INT-05** When processing succeeds or fails, the system shall move or retain the work item according to the configured intake directories without losing source evidence.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_supervision_recovery_guardrails.feature`
- Package tests: `agm/internal/sentinel/intake/*_test.go`
