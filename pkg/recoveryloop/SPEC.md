# Recovery Loop Specification

<!-- Last audited at: 2026-09-05 -->
<!-- Audit scope: pkg/recoveryloop (domain) + cmd/recovery-loop (CLI), both
     present at this revision. cmd/recovery-loop/SPEC.owner delegates here.

     Domain (pkg/recoveryloop/*_test.go):
       RL-01 through RL-10, RL-18 through RL-22.

     CLI (cmd/recovery-loop/main_test.go):
       RL-11 through RL-17. -->

**Status:** Production-ready
**Scope:** Host critical job self-healing loop and escalation engine

## Purpose

`cmd/recovery-loop` is the M2 mechanism of the absence-blindness architecture
(ce-23lf7, ce-a1uqr): a host-level launchd tick that consumes the absence-alarm
escalation journal plus a registry of critical jobs and attempts self-healing
recovery instead of only alarming.

Prior to this mechanism, every alerting path terminated in a notification banner,
an escalation log, or a human-needed journal line. Dead or wedged processes
remained down until a human noticed and intervened manually. The recovery loop
closes this gap by executing bounded, observable remediation actions:
reinstalling missing binaries, bootstrapping unloaded launchd plists, and
kickstarting wedged or alarming jobs.

Policy guard (the mergeloop lesson): an operator who deliberately disables a
critical job must snooze it with an explicit expiry in the shared snooze file.
Unexpired snoozes are respected; an unloaded or failing job without an
unexpired snooze is treated as a defect and recovered.

Every recovery attempt and outcome is journaled to ensure that recovery actions
themselves remain observable and cannot fail silently.

## Applicability

The recovery loop executes on the host environment as a launchd agent or
standalone binary. It manages critical launchd services and binaries for the
`dear-agent` fleet.

## EARS Requirements

**RL-01** When a critical job's binary does not exist at its configured path and an install command is configured, the system shall attempt to reinstall the binary.

**RL-02** When a critical job's launchd service is not loaded in launchctl and the job is not snoozed, the system shall attempt to bootstrap the service.

**RL-03** When a critical job's launchd service is loaded but reports exit code 78, the system shall attempt to bootout and re-bootstrap the service.

**RL-04** When a critical job's pulse is reported absent or undetermined in the absence-alarm journal and its service is loaded, the system shall attempt to restart the job via launchctl kickstart.

**RL-05** When a critical job is covered by an unexpired snooze entry in the snooze configuration, the system shall classify the job as SNOOZED and the system shall not attempt any recovery action for that job.

**RL-06** When an unexpired snooze does not cover an unloaded critical job, the system shall attempt recovery regardless of manual disabled state.

**RL-07** When a recovery attempt succeeds, the system shall reset the consecutive failure count for that job to 0.

**RL-08** When a recovery attempt fails, the system shall increment the consecutive failure count for that job by 1.

**RL-09** When a job's consecutive failure count reaches 2 or more, the system shall dispatch an escalation notification and record that human intervention is needed.

**RL-10** The system shall append a structured record to the recovery journal for every recovery attempt, recording the job name, action taken, outcome status, consecutive attempt count, whether human intervention is needed, and any error message.

**RL-11** When all critical jobs are healthy, snoozed, or successfully recovered, the system shall exit 0.

**RL-12** When at least one recovery attempt fails or remains in human-needed escalation, the system shall exit 1.

**RL-13** When configuration loading fails or invalid arguments are supplied, the system shall exit 2 with a usage error.

**RL-14** While dry-run mode is set, the system shall plan and report recovery actions on stdout without executing commands, writing state, updating heartbeats, or appending to the journal.

**RL-15** When JSON mode is set, the system shall emit the recovery report as a single JSON object on stdout.

**RL-16** When a tick completes and dry-run mode is not set, the system shall write a heartbeat file recording the tick time and recovery results.

**RL-17** If dispatching an escalation notification fails, then the system shall report the error on stderr and the system shall not change its exit code.

**RL-18** If the recovery state file cannot be read, then the system shall treat the state as empty and the system shall log a warning to stderr.

**RL-19** When the critical jobs configuration cannot be loaded or contains duplicate job names, the system shall exit 2 with a usage error.

**RL-20** When the snooze configuration has an invalid entry or an expiry beyond the maximum snooze horizon (14 days), the system shall exit 2 with a usage error.

**RL-21** When a recovery action is executed, the system shall bound execution with a timeout so a hung command cannot block the recovery loop indefinitely.

**RL-22** When a snooze entry has expired, the system shall treat the job as active and eligible for recovery.

## BDD Traceability

- Feature: `agm/test/bdd/features/observability_package_guardrails.feature`
- Package tests: `pkg/recoveryloop/loop_test.go` (RL-01..RL-10, RL-18..RL-22)
- CLI tests: `cmd/recovery-loop/main_test.go` (RL-11..RL-17)

