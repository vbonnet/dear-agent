# Process Reaper Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm/internal/procreaper`.

## Overview

`procreaper` safely reclaims unsupervised high-resource or idle processes while
protecting AGM-owned session process trees. It discovers supervised root PIDs,
expands protection through child-process subtrees, applies resource filters, and
terminates only unprotected candidates with SIGTERM before SIGKILL escalation.

## EARS Requirements

**PROCREAPER-01** When the reaper is constructed without required adapters, the system shall return an error.

**PROCREAPER-02** When supervised session PID discovery fails, the system shall fail closed and avoid terminating processes.

**PROCREAPER-03** When a process belongs to a supervised session subtree, the system shall include that process in the protected set.

**PROCREAPER-04** When a process matches a protected role name, the system shall include that process in the protected set.

**PROCREAPER-05** When a process does not exceed the configured RSS threshold, the system shall exclude that process from reapable candidates.

**PROCREAPER-06** When a process does not exceed the configured idle threshold, the system shall exclude that process from reapable candidates.

**PROCREAPER-07** When dry run is enabled, the system shall compute reapable candidates without sending signals.

**PROCREAPER-08** When terminating a reapable process, the system shall send SIGTERM before SIGKILL.

**PROCREAPER-09** When a process exits during the grace window after SIGTERM, the system shall not send SIGKILL.

**PROCREAPER-10** When SIGKILL fails, the system shall record the error without aborting the entire reap result.

**PROCREAPER-11** When process data is read from `ps`, the system shall parse PID, parent PID, RSS, idle time, command, and arguments into process records.

**PROCREAPER-12** When AGM session supervisor output is parsed, the system shall return supervised PIDs keyed by session name.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`

## Test Traceability

- Unit package: `agm/internal/procreaper`
