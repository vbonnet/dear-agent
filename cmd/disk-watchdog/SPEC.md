# Disk Watchdog Specification

<!-- Last audited at: 2026-07-03 -->

## Purpose

`cmd/disk-watchdog` is the host-level backstop of the 2026-07-03 disk-full P0
(ce-6fel): a launchd-driven tick that samples disk free space and inode usage,
alarms on the same thresholds the VROOM Overseer classifies in-process
(`supervisor.DiskAlertThresholds.Classify` — one shared classifier, so the two
layers can never disagree), records the alarm to the decision trail, and
remediates through the existing safe hook `agm worktree sweep --execute`
(provably-merged clean worktree husks only). It runs independently of the VROOM
mesh because a full disk is exactly the failure state that takes the mesh down.

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
