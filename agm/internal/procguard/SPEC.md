# Process Guard Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm/internal/procguard`.

## Overview

`procguard` centralizes process-spawn limits for AGM supervisors and tools. It
tracks active spawned work, validates depth and fanout limits before new process
creation, exposes process-group attributes for child processes, and applies
platform-specific process-count limits where supported.

## EARS Requirements

**PROCGUARD-01** When the active spawn counter is incremented, the system shall increase the active count by one.

**PROCGUARD-02** When the active spawn counter is decremented, the system shall decrease the active count by one.

**PROCGUARD-03** When the active spawn counter is reset, the system shall report zero active spawns.

**PROCGUARD-04** When default limits are requested, the system shall return nonzero depth, child, and total-active spawn limits.

**PROCGUARD-05** When current depth exceeds the configured maximum depth, the system shall reject the spawn.

**PROCGUARD-06** When current child count exceeds the configured maximum children, the system shall reject the spawn.

**PROCGUARD-07** When total active count exceeds the configured maximum active processes, the system shall reject the spawn.

**PROCGUARD-08** When multiple spawn limits are exceeded, the system shall return the highest-priority limit violation.

**PROCGUARD-09** When process group attributes are requested, the system shall return attributes that isolate spawned children into a process group.

**PROCGUARD-10** When a process-count limit is applied to an invalid PID, the system shall return an error.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`

## Test Traceability

- Unit package: `agm/internal/procguard`
