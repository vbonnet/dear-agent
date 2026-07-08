# AGM CLI Project Directory Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/cli` owns process-local project directory state for AGM commands.
It lets commands resolve `.agm` files and session data without changing the
process working directory.

## Requirements

**AGM-CLI-01** When a command stores the project directory, the system shall update the shared project directory state under a write lock.

**AGM-CLI-02** When a command reads the project directory, the system shall read the shared project directory state under a read lock.

**AGM-CLI-03** When no project directory has been stored, the system shall return `.` for backward compatibility.

**AGM-CLI-04** When a project directory is stored, the system shall not change the process working directory.

**AGM-CLI-05** When concurrent commands read and write the project directory, the system shall avoid data races through synchronization.

## BDD Traceability

- `agm/test/bdd/features/agm_control_surface_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
