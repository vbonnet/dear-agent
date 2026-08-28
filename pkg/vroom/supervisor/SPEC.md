# pkg/vroom/supervisor — Requirements Specification (EARS)

<!-- Last audited at: 2026-07-17 -->

**Version**: 1.3
**Last Updated**: 2026-08-28
**Status**: Active
**Scope**: VROOM supervisor task queues, with emphasis on AGM-backed worker dispatch.

---

## Overview

The supervisor package coordinates VROOM task dispatch. The AGM-backed queue
adapts accepted tasks into detached `agm session new` worker sessions. Because
that handoff crosses a CLI boundary, the generated argument list must stay in
sync with AGM's registered `session new` flags. The overseer also classifies
host pressure signals and routes memory, disk, and inode alerts to supervisors
that can pause or reshape work before resource exhaustion causes data loss.

---

## EARS Requirements

### Queue Semantics

**VROOM-SUP-01** When a task is enqueued with an empty ID, the system shall reject the task.

**VROOM-SUP-02** When a task is enqueued with an ID already pending in the queue, the system shall reject the duplicate task.

**VROOM-SUP-03** When a task is enqueued without `EnqueuedAt`, the system shall set `EnqueuedAt` before storing the task.

**VROOM-SUP-04** When pending tasks are requested, the system shall return a snapshot rather than exposing the queue's internal slice.

### AGM Worker Dispatch

**VROOM-SUP-05** When AGMQueue dispatches a pending task, the system shall prevent concurrent dispatch of the same task, remove it from pending only after `agm session new` succeeds, and preserve it in pending if dispatch fails.

**VROOM-SUP-06** When AGMQueue builds worker dispatch arguments, the system shall pass the worker session name as the positional `agm session new` argument.

**VROOM-SUP-07** When AGMQueue builds worker dispatch arguments, the system shall use AGM's registered `--detached` flag and shall not use the obsolete `--detach` spelling.

**VROOM-SUP-08** When AGMQueue builds worker dispatch arguments, the system shall pass the initial worker instruction with AGM's registered `--prompt` flag.

**VROOM-SUP-09** When AGMQueue invokes AGM and the command fails, the system shall return an error containing the failed worker session name and AGM output.

### CLI Contract Drift

**VROOM-SUP-10** When VROOM worker dispatch arguments are changed, the system shall verify that they parse against AGM's actual `session new` flag set.

### Peer Liveness (ce-axsr)

**VROOM-SUP-11** When a peer liveness check combines heartbeat freshness with a harness-process probe, the system shall report the peer as blocked (DEAD, with a zombie-heartbeat reason) if the probe proves no harness process is running, even when the heartbeat is fresh.

**VROOM-SUP-12** When the harness-process probe returns an error, the peer liveness check shall fail open to the heartbeat check alone and shall not mark the peer dead on an unverifiable probe.

### Disk and Inode Pressure (ce-6fel)

**VROOM-SUP-13** When disk-alert thresholds are zero-valued, the system shall apply the default free-space and inode thresholds before classification.

**VROOM-SUP-14** When measured free disk space falls below the warning or critical floors, the system shall classify the snapshot at the matching pressure level and include a free-space reason.

**VROOM-SUP-15** When measured inode usage rises above the warning or critical thresholds, the system shall classify the snapshot at the matching pressure level and include an inode reason.

**VROOM-SUP-16** When both free-space and inode pressure are present, the system shall report the highest pressure level warranted by either signal.

**VROOM-SUP-17** When disk alerting is not explicitly wired into the overseer, the system shall leave disk-pressure snapshots quiet.

**VROOM-SUP-18** When a warning disk alert is emitted, the system shall route it to the Meta-Orchestrator.

**VROOM-SUP-19** When a critical disk alert is emitted, the system shall route it to both the Meta-Orchestrator and Orchestrator.

**VROOM-SUP-20** When a disk alert notifier fails for one supervisor role, the system shall record the error in the decision trail and shall continue notifying remaining roles.

### Harness and Model Routing

**VROOM-SUP-21** When AGM worker dispatch has no explicit model route, the system shall omit `--model` so AGM and the active harness or provider select the model.

**VROOM-SUP-22** When AGM worker dispatch has an explicit model route, the system shall preserve the route without restricting its model family.

**VROOM-SUP-23** When a burndown policy has no model list, the system shall default to one provider-selected empty model route.

**VROOM-SUP-24** When a burndown policy has multiple explicit model routes, the system shall assign those routes to workers in round-robin order.

**VROOM-SUP-25** When VROOM records a worker session with an empty model route, the system shall preserve the empty value as provider-selected rather than substituting an Anthropic model.

**VROOM-SUP-26** While VROOM runs under any supported harness, the system shall use the same model-neutral dispatch contract.

### Queue Storage Hygiene

**VROOM-SUP-27** When a pending task is removed, the system shall clear the vacated backing-array slot before shrinking the queue so removed task data is not retained.

### Canonical Supervisor Topology

**VROOM-SUP-28** The VROOM supervisor topology shall contain exactly the Meta-Orchestrator, Orchestrator, and Overseer canonical members.

**VROOM-SUP-29** When a caller supplies a canonical supervisor ID, compact alias, or role name, the system shall resolve it to the same immutable topology member.

**VROOM-SUP-30** The VROOM supervisor topology shall assign each member distinct canonical Primary and Tertiary peers and shall cover the complete cyclic peer graph defined by ADR-002.

**VROOM-SUP-31** When callers request all topology members, the system shall return a copy that cannot mutate the canonical topology.

### Authoritative Supervisor Heartbeat Observation

**VROOM-SUP-32** When a VROOM component observes supervisor heartbeat freshness, the system shall use the authoritative AGM supervisor record addressed by canonical supervisor ID.

**VROOM-SUP-33** When the authoritative AGM supervisor record is missing, unreadable, or has no heartbeat timestamp, the system shall not infer heartbeat freshness from a legacy mirror.

**VROOM-SUP-34** When supervisor heartbeat persistence receives an empty identifier or an identifier that is not one path component, the system shall reject the operation before accessing a heartbeat record.

**VROOM-SUP-35** When an authoritative supervisor heartbeat record's embedded identity differs from the identity used to address it, the system shall reject the record as invalid rather than infer heartbeat freshness from it.

## BDD traceability

- No BDD change, with reason: an independent existing-AGM JSON fixture plus deterministic Store, AGM command, and Dispatch adapter tests exercise the established file protocol, invalid-identifier rejection, bounded read diagnostics, and classification precedence without process orchestration.

## Test Traceability

- Package tests: `pkg/vroom/supervisor/disk_alert_test.go`
- Package tests: `pkg/vroom/supervisor/check_test.go`
- Package tests: `pkg/vroom/supervisor/queue_test.go`
- Package tests: `pkg/vroom/supervisor/topology_test.go`
- Authoritative heartbeat store: `internal/supervisorheartbeat/store_test.go`
- Authoritative heartbeat adapters: `agm/cmd/agm/supervisor_heartbeat_store_test.go`, `agm/cmd/agm/supervisor_test.go`, `agm/internal/bus/heartbeat_watcher_test.go`, `cmd/vroom-dispatch/coverage_test.go`
- BDD: `agm/test/bdd/features/vroom_runtime_guardrails.feature`
