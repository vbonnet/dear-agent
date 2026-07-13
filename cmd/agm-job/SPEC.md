# AGM Job Locking Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`

<!-- Last audited at: 2026-07-03 -->

## Purpose

`cmd/agm-job` runs scheduled AGM maintenance jobs under a filesystem lock. The
lock prevents overlapping job executions while allowing recovery from stale lock
directories left behind by crashed processes.

## EARS Requirements

**AGMJ-01** When an AGM job acquires a fresh lock, the system shall create the lock directory and write the current process id into a `pid` file.

**AGMJ-02** When an AGM job attempts to acquire a lock held by a live process, the system shall reject the acquisition without modifying the existing lock directory.

**AGMJ-03** When an AGM job attempts to acquire a stale lock, the system shall remove the stale lock directory before creating a new lock directory.

**AGMJ-04** When an AGM job recovers a stale lock, the system shall write the current process id into the recovered lock's `pid` file before proceeding.

**AGMJ-05** When an AGM job releases a held lock, the system shall remove the lock directory so a later job can acquire it.
