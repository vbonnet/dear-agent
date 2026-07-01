# pkg/vroom/supervisor — Requirements Specification (EARS)

<!-- Last audited at: NEEDS-AUDIT -->

**Version**: 1.0
**Last Updated**: 2026-07-01
**Status**: Baseline for VROOM supervisor queue dispatch behavior
**Scope**: VROOM supervisor task queues, with emphasis on AGM-backed worker dispatch.

---

## Overview

The supervisor package coordinates VROOM task dispatch. The AGM-backed queue
adapts accepted tasks into detached `agm session new` worker sessions. Because
that handoff crosses a CLI boundary, the generated argument list must stay in
sync with AGM's registered `session new` flags.

---

## EARS Requirements

### Queue Semantics

**VROOM-SUP-01** When a task is enqueued with an empty ID, the system shall reject the task.

**VROOM-SUP-02** When a task is enqueued with an ID already pending in the queue, the system shall reject the duplicate task.

**VROOM-SUP-03** When a task is enqueued without `EnqueuedAt`, the system shall set `EnqueuedAt` before storing the task.

**VROOM-SUP-04** When pending tasks are requested, the system shall return a snapshot rather than exposing the queue's internal slice.

### AGM Worker Dispatch

**VROOM-SUP-05** When AGMQueue dispatches a pending task, the system shall remove the task from the pending list only after `agm session new` succeeds.

**VROOM-SUP-06** When AGMQueue builds worker dispatch arguments, the system shall pass the worker session name as the positional `agm session new` argument.

**VROOM-SUP-07** When AGMQueue builds worker dispatch arguments, the system shall use AGM's registered `--detached` flag and shall not use the obsolete `--detach` spelling.

**VROOM-SUP-08** When AGMQueue builds worker dispatch arguments, the system shall pass the initial worker instruction with AGM's registered `--prompt` flag.

**VROOM-SUP-09** When AGMQueue invokes AGM and the command fails, the system shall return an error containing the failed worker session name and AGM output.

### CLI Contract Drift

**VROOM-SUP-10** When VROOM worker dispatch arguments are changed, the system shall verify that they parse against AGM's actual `session new` flag set.
