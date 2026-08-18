# AGM Capacity Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/capacity` estimates safe concurrent Claude session capacity from
RAM, CPU, process count, and explicit operator overrides. The calculator is
deterministic for a supplied `SystemInfo`, while the detector isolates the
platform-dependent memory and process probes.

## Requirements

**AGM-CAPACITY-01** When system information is calculated, the system shall derive used RAM and RAM usage percentage from total and available RAM.

**AGM-CAPACITY-02** When RAM totals are unavailable or zero, the system shall report zero RAM usage percentage instead of dividing by zero.

**AGM-CAPACITY-03** When capacity is calculated, the system shall compute RAM-based max, CPU-based max, hard cap, current sessions, available slots, and RAM usage zone.

**AGM-CAPACITY-04** When available RAM is less than or equal to reserved RAM, the system shall set RAM-based capacity to zero.

**AGM-CAPACITY-05** When `AGM_MAX_SESSIONS` is set to a non-negative integer, the system shall use it as the maximum session count and record the override value.

**AGM-CAPACITY-06** When available slots would be negative, the system shall floor available slots at zero.

**AGM-CAPACITY-07** When RAM usage is classified, the system shall report green below 60 percent, yellow from 60 through 80 percent, red above 80 through 90 percent, and critical above 90 percent.

**AGM-CAPACITY-08** When process counting fails during detection, the system shall treat the current Claude process count as zero and still return memory and CPU information.

**AGM-CAPACITY-09** When `/proc/meminfo` is parsed, the system shall require both `MemTotal` and `MemAvailable` and convert kB values to bytes.

**AGM-CAPACITY-10** When system information is detected on macOS, the system shall obtain total RAM from `hw.memsize` and available RAM from the platform `memory_pressure -Q` percentage.

**AGM-CAPACITY-11** When a native memory probe fails, returns zero total RAM, or reports a percentage outside zero through 100, the system shall return an explicit error instead of inventing capacity.

## BDD Traceability

- `agm/test/bdd/features/agm_runtime_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
- `agm/test/bdd/features/agm_capacity_platform.feature` exercises the native detector on the current Linux or macOS host.
