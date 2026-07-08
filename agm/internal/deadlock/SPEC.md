# AGM Deadlock Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/deadlock` diagnoses high-CPU Claude processes attached to tmux
sessions. It combines tmux pane lookup, process inspection, runtime parsing,
connection counts, human-readable formatting, and durable incident logging.

## Requirements

**AGM-DEADLOCK-01** When deadlock detection runs for a tmux session, the system shall find the tmux pane PID, locate a Claude or node child process, inspect process details, and return the populated process info.

**AGM-DEADLOCK-02** When process information shows CPU at or above 25 percent, runtime at or above 5 minutes, and running or runnable state, the system shall mark the process as deadlocked.

**AGM-DEADLOCK-03** When any deadlock criterion is not met, the system shall mark the process as not deadlocked while preserving the observed process details.

**AGM-DEADLOCK-04** When process runtime is parsed, the system shall support both `MM:SS.ss` and `HH:MM:SS` `ps` formats.

**AGM-DEADLOCK-05** When process connection counting fails, the system shall report zero connections instead of failing deadlock detection.

**AGM-DEADLOCK-06** When process information is formatted, the system shall include PID, CPU, runtime, state, wait channel, connection count, command, and per-criterion status.

**AGM-DEADLOCK-07** When a deadlock incident is logged, the system shall append an RFC3339 timestamped line to `~/deadlock-log.txt` with session, PID, CPU, runtime, state, and wait channel.

## BDD Traceability

- `agm/test/bdd/features/agm_runtime_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
