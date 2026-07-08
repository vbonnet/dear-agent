# AGM Failure Tracking Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/tracking` persists task failure counts so AGM can auto-skip
repeatedly failing tasks while allowing successful work to reset the counter.
The tracker is mutex-protected for in-process callers and JSON-backed for
durability across command invocations.

## Requirements

**AGM-TRACKING-01** When a failure tracker is created without a path, the system shall use `~/.agm/failure-tracking.json`.

**AGM-TRACKING-02** When the tracking file does not exist, the system shall initialize an empty tracker without error.

**AGM-TRACKING-03** When a failure is recorded for a task, the system shall increment that task's count, update its last failure timestamp, and persist the tracking file.

**AGM-TRACKING-04** When skip status is checked, the system shall return true only when the task failure count is greater than or equal to the supplied maximum.

**AGM-TRACKING-05** When a task has no failure record, the system shall report zero failures and not skip it.

**AGM-TRACKING-06** When a task is reset, the system shall remove that task's failure record and persist the updated tracking file.

**AGM-TRACKING-07** When tracking data is saved, the system shall create the parent directory and write JSON with owner-only file permissions.

## BDD Traceability

- `agm/test/bdd/features/agm_runtime_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
