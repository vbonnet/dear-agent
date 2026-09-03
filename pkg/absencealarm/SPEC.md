# Absence Alarm Specification

<!-- Last audited at: 2026-09-01 -->
<!-- Audit scope: the domain helpers pkg/absencealarm ships at this revision,
     and only those. Audited and covered by pkg/absencealarm/*_test.go:
     AA-01 through AA-06 (pulse classification), AA-09 (journal record),
     AA-11 (re-dispatch interval decision), AA-14 (snooze rejection),
     AA-18 (unreadable alarm state), and AA-19 (configuration loading).

     NOT audited, because nothing at this revision implements them: AA-07 and
     AA-08 (exit-code selection), AA-10, AA-12 and AA-17 (notification
     dispatch and its failure reporting), AA-13 and AA-15 (applying a snooze
     to produce SNOOZED or an expired-snooze reason -- StatusSnoozed is
     declared here but never assigned), AA-16 (heartbeat on tick completion),
     AA-20 (dry-run enforcement), AA-21 and AA-22 (report emission and JSON
     output), and AA-23 (the system-level response to a persistence failure).
     Those behaviors belong to cmd/absence-alarm, which does not exist at this
     revision and is audited by the slice that introduces it. -->

## Purpose

`cmd/absence-alarm` is the fleet-wide generalization of the disk-watchdog's
reaper-liveness check (DW-17): a launchd-driven tick that alarms on the
ABSENCE of expected positive events, not on the presence of errors.

Every silent multi-week outage in the 2026-07/08 window shared one shape: a
mechanism stopped producing its positive event and nothing was watching for
that event, so the gap was invisible until a human tripped over it.
Concretely: `com.dear-agent.mergeloop` was `launchctl disable`d around
2026-07-22 and stayed dark for 41 days; OTel span production stopped for ~46
days while a daily check kept reporting success; the VROOM supervisors were
down ~44 days; the sandbox GC failed on every hourly tick for a month into an
unread log (the DW-17 lineage); the disk-watchdog itself logged thousands of
identical failed remediations that nothing consumed. Error-presence
monitoring cannot see any of these, because a dead process emits no errors.

The absence alarm inverts the question. It carries a declarative set of
**pulses** - expected positive events with a maximum silence window - and
alarms LOUDLY when a pulse has not been observed inside its window: a merge
into main, fresh OTel traces, a supervisor heartbeat, a completed sweep,
a critical launchd job still loaded. It runs as a host-level launchd tick,
independent of the mesh, Dispatch, and any agent session, because "the
watcher lives inside the thing being watched" is the exact anti-pattern that
produced the 46-day OTel gap (the prior health check degraded into a
sandboxed reminder addressed to an offline Dispatch).

The OTel restore sharpened the diagnosis this tool answers. The fleet did
not lack absence detectors: `cmd/jaeger-health` already implemented exactly
the right check (alive but no traces in the lookback window reports
degraded, exit 1) while OTel sat dark for 46 days - because nothing ran it,
and no sink consumed its exit code. What was missing is generic: a registry
of liveness checks, a scheduler that guarantees each one runs, and an alarm
sink that escalates each failure to a human even when the mesh is down.
That is this tool's decomposition:

- **Registry**: the pulse config. Each entry declares one expected positive
  event and how to probe it.
- **Scheduler**: the launchd tick. One job runs every registered check; a
  check can no longer exist without being run.
- **Sink**: the alarm ladder - desktop notification with backoff, the
  escalation journal, the exit code - one shared, tested path instead of a
  seventh bespoke notify implementation.

Probe checks themselves follow the `jaeger-health` pattern: small,
spec-pinned, no-side-effect binaries sharing the exit contract 0 healthy /
1 degraded / 2 down / 3 usage, registered here as `command` pulses.
`jaeger-health` is the first registered check; `merge-health` (the
merge-pipeline absence probe, EARS MH-01..MH-08) is its first deliberate
sibling; bead-close and sweep-success checks follow the same shape rather
than becoming bespoke one-liners inside this config.

Three design rules carried over from the disk-watchdog retros:

- **Undetermined is not OK** (DW-28 lineage): a probe that cannot be
  evaluated alarms exactly like an absent pulse. "Could not check" must never
  print as health.
- **Silence may not be purchased permanently** (the mergeloop lesson): a
  pulse can be snoozed only with an explicit expiry, and an over-long or
  expiry-less snooze is a usage error, so "disabled temporarily" cannot
  silently become "disabled forever".
- **Degrade toward louder, never toward quieter**: losing the dedup state
  file re-notifies; a notification dispatch failure never masks the alarm
  exit code; a config error refuses to run a subset rather than silently
  unmonitoring part of the fleet (DW-31 lineage).

The alarm ladder is transition-plus-backoff: one desktop notification when a
pulse goes absent, re-notifications at escalating intervals while it stays
absent, one recovery notification when it returns. A standing alarm can
neither fall silent nor become notification noise that trains the operator to
ignore it - both failure modes are attested in the retro record.

Every tick writes its own heartbeat file so an independent watcher (the
disk-watchdog, or a second trivial launchd job) can alarm on the absence of
the absence alarm itself.

## EARS Requirements

**AA-01** When a file-mtime pulse's file has a modification time older than the pulse window, the system shall classify the pulse as ABSENT.

**AA-02** When a file-mtime pulse's file does not exist, the system shall classify the pulse as ABSENT with a reason recording that the file is missing.

**AA-03** When a launchd pulse's label does not appear in the launchd job listing, the system shall classify the pulse as ABSENT.

**AA-04** When a command pulse's command exits non-zero, the system shall classify the pulse as ABSENT with the exit status in the reason.

**AA-05** If a pulse's probe cannot be evaluated (the launchd listing cannot be obtained, a command cannot be started, or a file's metadata cannot be read for a reason other than absence), then the system shall classify the pulse as UNDETERMINED and the system shall alarm exactly as for an absent pulse.

**AA-06** When a pulse's evidence timestamp is more than the clock-skew tolerance (5 minutes) in the future, the system shall classify the pulse as UNDETERMINED.

**AA-07** When at least one pulse is ABSENT or UNDETERMINED, the system shall exit 1.

**AA-08** When every pulse is PRESENT or SNOOZED, the system shall exit 0.

**AA-09** When a pulse is ABSENT or UNDETERMINED, the system shall append one alarm record carrying the pulse name, status, reason, window, evidence timestamp, and consecutive-miss count to the escalation journal.

**AA-10** When a pulse transitions from healthy to ABSENT or UNDETERMINED, the system shall dispatch a notification naming the pulse and the expected event.

**AA-11** While a pulse remains ABSENT or UNDETERMINED, the system shall re-dispatch a notification only when the alarm age crosses an escalation point (1h, 6h, 24h, then every 24h), so a standing alarm neither falls silent nor becomes noise.

**AA-12** When a pulse transitions from ABSENT or UNDETERMINED back to PRESENT, the system shall dispatch one recovery notification and the system shall clear that pulse's alarm state.

**AA-13** When a snooze entry names a pulse and carries an expiry in the future within the maximum snooze horizon (14 days), the system shall classify the pulse as SNOOZED and the system shall not notify for it.

**AA-14** When a snooze entry has no expiry or an expiry beyond the maximum snooze horizon, the system shall reject the configuration with a usage error and exit 2.

**AA-15** When a snooze entry's expiry has passed, the system shall evaluate the pulse normally and the system shall include the expired snooze in any alarm reason for that pulse.

**AA-16** When a tick completes and dry-run mode is not set, the system shall write a heartbeat file recording the tick time and every pulse's status, so an independent watcher can alarm on the absence of the absence alarm itself.

**AA-17** If dispatching a notification fails, then the system shall report the failure on stderr and the system shall not change its exit code.

**AA-18** If the alarm-state file cannot be read, then the system shall treat every alarming pulse as newly transitioned and notify for it, so losing dedup state degrades toward louder rather than toward silent.

**AA-19** When the pulse configuration cannot be loaded, contains no pulses, or contains an invalid pulse (unknown type, missing required field, non-positive window on a file-mtime pulse, or a duplicate name), the system shall exit 2 with a usage error rather than evaluate a subset.

**AA-20** While dry-run mode is set, the system shall evaluate and report pulses but the system shall not notify, shall not write alarm state, shall not write the heartbeat, and shall not append to the escalation journal.

**AA-21** The system shall include every configured pulse exactly once in its report, with the pulse's status, reason, window, and evidence timestamp.

**AA-22** When JSON output mode is set, the system shall emit the full report as a single JSON object on stdout.

**AA-23** If appending to the escalation journal or writing the heartbeat or state file fails, then the system shall report the failure on stderr and the system shall not change its exit code.

## BDD Traceability

- Feature: `agm/test/bdd/features/observability_package_guardrails.feature`
- Package tests: `pkg/absencealarm/*_test.go` (domain)

The CLI's executable coverage (`cmd/absence-alarm/*_test.go`) lands with the
CLI slice. It is deliberately not listed as current evidence: `cmd/absence-alarm`
does not exist at this revision, so citing it here would let the audit stamp
certify paths that cannot have been checked.
