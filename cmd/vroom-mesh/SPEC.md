# VROOM Mesh Harness Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`

<!-- Last audited at: 2026-07-03 -->

## Purpose

`cmd/vroom-mesh` runs the canonical 3-supervisor VROOM mesh (Meta-Orchestrator,
Orchestrator, Overseer) in a single process. By default every substrate is
in-memory so the mesh can be exercised without touching the host; flags opt
individual adapters onto real state (`--sys-probe` for OS metrics, `--beads-db`
for the roadmap, `--agm-dispatch` for real worker sessions). The invariant the
harness must uphold: adapters that mutate or page live host state are armed
only on the real-probe path, never against simulated snapshots.

## EARS Requirements

**VM-01** The system shall run the Meta-Orchestrator, Orchestrator, and Overseer on a shared tick cadence in one process.

**VM-02** The system shall write every decision-trail record to stdout as one JSON object per line.

**VM-03** When the `--sys-probe` flag is set, the system shall arm the Overseer's resource remediation, session hygiene, session inbox alert, memory-pressure alert, and disk-free/inode alert against the real OS probe.

**VM-04** While the in-memory probe is in use, the system shall not arm adapters that mutate or page live host state (reclaimer, session gardener, inbox counter, memory-alert notifier, disk-alert notifier).

**VM-05** When the `--trail` flag names a file, the system shall append the decision trail to that file in addition to stdout.

**VM-06** When the `--duration` deadline elapses or a termination signal arrives, the system shall shut the mesh down cleanly and exit with code 0.

**VM-07** When the configuration is invalid, the system shall exit with code 1.
