# AGM Lock Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/lock` provides process-level file locks for AGM commands using
`flock`. Locks record the owning PID for diagnostics, expose stale-lock
inspection, and keep force removal explicit.

## Requirements

**AGM-LOCK-01** When a lock is created, the system shall create the lock directory with owner-only permissions and open the lock file with owner-only permissions.

**AGM-LOCK-02** When a non-blocking lock attempt finds another holder, the system shall return a `LockError` with recovery guidance.

**AGM-LOCK-03** When a lock is acquired, the system shall write the current process PID into the lock file.

**AGM-LOCK-04** When a lock is unlocked, the system shall release the flock, close the file, and make subsequent unlock calls no-ops.

**AGM-LOCK-05** When the default lock path is requested and `AGM_LOCK_PATH` is set, the system shall use that override.

**AGM-LOCK-06** When the default lock path is requested without an override, the system shall use `/tmp/agm-<uid>/agm.lock`.

**AGM-LOCK-07** When lock status is checked and the file is missing, the system shall report that no lock exists and that there is nothing to unlock.

**AGM-LOCK-08** When lock status is checked and the file is empty or contains an invalid PID, the system shall report the lock as stale and safe to unlock.

**AGM-LOCK-09** When lock status is checked and the recorded process no longer exists, the system shall report the lock as stale and safe to unlock.

**AGM-LOCK-10** When force unlock is requested, the system shall remove the lock file and ignore missing-file errors.

**AGM-LOCK-11** When a non-blocking flock attempt fails, the system shall classify only `EWOULDBLOCK` or `EAGAIN` as retryable contention and shall preserve any other operating-system error for immediate caller-visible failure.

## BDD Traceability

- `agm/test/bdd/features/agm_runtime_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
