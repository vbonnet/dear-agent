# Disk Watchdog Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`

<!-- Last audited at: 2026-07-21 -->

## Purpose

`cmd/disk-watchdog` is the host-level backstop of the 2026-07-03 disk-full P0
(ce-6fel): a launchd-driven tick that samples disk free space and inode usage,
alarms on the same thresholds the VROOM Overseer classifies in-process
(`supervisor.DiskAlertThresholds.Classify` — one shared classifier, so the two
layers can never disagree), records the alarm to the decision trail, and
remediates through the existing safe hook `agm worktree sweep --execute`
(provably-merged clean worktree husks only). It runs independently of the VROOM
mesh because a full disk is exactly the failure state that takes the mesh down.

Since ce-93lw.18 it also drives the cross-process **admission brake**
(`pkg/vroom/admission`). Alarming was never the gap: on 2026-07-18 this watchdog
was in ALARM with root at 96.2% used and
`agm worktree sweep --execute: signal: killed` in every remediation slot — the
remediation path being killed by the exhaustion it existed to relieve — and
nothing consumed that fact, so the mesh kept spawning until the host had to be
power-cycled. The brake is the consumer: a TTL'd latch every spawn path reads.

## EARS Requirements

**DW-01** When free disk space on the measured filesystem falls below the critical floor (default 5 GiB), the system shall classify the condition as CRITICAL.

**DW-02** When free disk space on the measured filesystem falls below the warn floor (default 20 GiB) but not the critical floor, the system shall classify the condition as WARN.

**DW-03** When inode usage on the measured filesystem exceeds the critical ceiling (default 95%), the system shall classify the condition as CRITICAL.

**DW-04** When inode usage on the measured filesystem exceeds the warn ceiling (default 90%) but not the critical ceiling, the system shall classify the condition as WARN.

**DW-05** When the probe cannot measure the filesystem (a snapshot with zero free bytes and a zero used fraction), the system shall not raise an alarm.

**DW-06** When any threshold is breached, the system shall append one `watchdog.disk.alarm` record carrying the snapshot, reasons, thresholds, and remediation outcome to the decision trail.

**DW-07** When any threshold is breached and dry-run mode is not set, the system shall remediate by invoking `agm worktree sweep --execute` with JSON output.

**DW-08** When remediation or the trail append fails, the system shall report the failure and still exit with the breach exit code 1.

**DW-09** When no threshold is breached, the system shall exit 0 without invoking any remediation.

**DW-10** While dry-run mode is set, the system shall detect and log breaches but the system shall not remove any worktree.

**DW-11** When any threshold is breached and remediation returns an error, the system shall engage the admission brake with a reason carrying the remediation error.

**DW-12** When no threshold is breached, the system shall release the admission brake only if it engaged that brake itself.

**DW-13** If the filesystem snapshot cannot be taken, then the system shall engage the admission brake before reporting the error.

**DW-14** When any threshold is breached and remediation succeeds, the system shall leave any existing admission brake in place.

**DW-15** While dry-run mode is set, the system shall not write or remove the admission brake file.

**DW-16** If engaging or releasing the admission brake fails, then the system shall report the failure on stderr and the system shall not change its exit code.
