# Sandbox Garbage Collection Safety Specification

<!-- Last audited at: 2026-08-14 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm/internal/sandboxgc` — provably-safe reaping of session sandbox directories under `~/.agm/sandboxes`

## Overview

`agm/internal/sandboxgc` owns the safety gates that decide whether a sandbox
directory may be deleted, and the reap sequence that deletes it. It exists
because on 2026-07-03 `~/.agm/sandboxes` grew to 2.3T across 541 directories
and crashed the host twice (ce-uxju), and because sandboxes use overlay-style
`merged/` + `upper/` layouts where deleting through a live mount can destroy
the underlying source repository (`~/.agm/cleanup-runbook.md`).

Consumers: the archive-time reap in `agm/internal/ops` (session archive /
session gc / orphaned-sandbox cleanup) and the periodic `agm sandbox gc`
sweep scheduled by `deploy/launchd/com.dear-agent.sandbox-gc.plist`.

## EARS Requirements

### Path Validation

**SGC-01** When the reaper evaluates a candidate path, the system shall refuse any path that is not directly under the sandbox base directory as exactly one clean, absolute path component.

**SGC-02** When the reaper is constructed with a base directory that does not end in `.agm/sandboxes`, the system shall refuse every reap against that base.

### Live-Session Gate

**SGC-03** When a live-session source is configured and a non-archived session references the sandbox name, the system shall refuse the reap.

**SGC-04** When the live-session source returns an error or the session store returns zero sessions, the system shall fail closed and refuse the reap.

**SGC-05** When the archive flow reaps the sandbox of a session it has just archived, the system shall allow the live-session gate to be omitted while keeping all other gates mandatory.

**SGC-14** When the periodic sandbox sweep builds its live-session source from the configured workspace registry and one configured Dolt database does not exist, the system shall record a visible warning and continue with the remaining reachable configured stores; if no configured store is reachable or all reachable stores return zero sessions, the system shall fail closed and refuse the sweep.

**SGC-16** When `agm sandbox gc --reap` runs and any configured workspace store was skipped, the system shall downgrade the run to a scan and delete nothing, because a skipped store contributes none of its live session IDs and its sandboxes would otherwise pass the live-session gate for the sole reason that the store proving them live could not be read.

**SGC-17** When a requested reap is downgraded to a scan under SGC-16, the system shall report the refusal to the caller as an explicit `reap_refused` reason in the machine-readable result, in addition to the warning, so that an automated caller cannot read the refusal as an ordinary preview run nor read the resulting would-reap count as sandboxes deleted.

### Live-Process Gate

**SGC-06** When any process holds a working directory or an open file descriptor at or under the sandbox, the system shall refuse the reap before attempting any unmount.

**SGC-07** When the process table cannot be enumerated or the enumeration times out, the system shall fail closed and refuse the reap.

### Mount Hard Gate

**SGC-08** When a reap proceeds past the liveness gates, the system shall attempt a best-effort unmount of the sandbox overlay and then re-read the mount table before removal.

**SGC-09** When the re-read mount table shows any mount point at or under the sandbox, the system shall refuse the removal.

**SGC-10** When the mount table cannot be read, the system shall fail closed and refuse the removal.

### Probe-Failure Reporting

**SGC-15** When a fail-closed refusal (SGC-04, SGC-07, SGC-10) is raised because a safety check could not run, rather than because it positively found the sandbox live, mounted, or referenced, the system shall mark the `RefusalError` as a probe failure so that callers can distinguish "not evaluated" from "evaluated and kept" instead of accreting silently into a heartbeat that then reads as healthy.

### Sweep Behaviour

**SGC-11** When `agm sandbox gc` runs without the `--reap` flag, the system shall report reap decisions without deleting anything.

**SGC-12** When the sweep encounters a sandbox entry younger than the configured minimum age, the system shall keep the entry untouched.

**SGC-13** When the sweep encounters non-git, partial, or corrupt sandbox content, the system shall treat it as ordinary reapable content rather than an error.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_coverage.feature` (changed-package SPEC coverage gate)
- Unit evidence: `agm/internal/sandboxgc/sandboxgc_test.go` (table-driven gate tests with fakes: mount-survives-unmount, live fd/cwd, store-down, path escapes), `agm/internal/ops/sandbox_gc_test.go` (sweep dry-run default, age gate, fail-closed storage), and `agm/cmd/agm/sandbox_gc_test.go` (configured workspace missing-database degradation).
- SGC-16 evidence: `agm/cmd/agm/sandbox_gc_test.go::TestEffectiveSandboxGCReapRefusesPartialInventory` (a requested reap with any skipped workspace yields scan-only plus a notice; a complete inventory reaps as asked).
- SGC-17 evidence: `agm/internal/ops/sandbox_gc_test.go::TestSandboxGCResultPublishesTheRefusalOnTheWire` and `::TestSandboxGCResultOmitsTheRefusalWhenItReaped` (the refusal, and only a real refusal, reaches the caller as `reap_refused`); the reciprocal consumer check is `cmd/disk-watchdog/reaper_liveness_honesty_test.go::TestSweepMergedWorktrees_RefusedReapIsARemediationFailure`, which asserts the watchdog treats it as failed remediation rather than a completed sweep.
