# Recovery Loop Specification

<!-- Verification map: pkg/recoveryloop (domain) + cmd/recovery-loop (CLI), both
     present at this revision. cmd/recovery-loop/SPEC.owner delegates here.

     Domain (pkg/recoveryloop/*_test.go):
       RL-01 through RL-06; RL-10; RL-18, RL-19, RL-21 through RL-29;
       RL-32, RL-33, RL-39, RL-42, RL-43, RL-45, RL-47.

     CLI (cmd/recovery-loop/*_test.go):
       RL-07 through RL-17; RL-20; RL-24, RL-26, RL-27;
       RL-29 through RL-32; RL-34 through RL-41; RL-44 through RL-60. -->

**Status:** Proposed
**Scope:** Host critical job self-healing loop and escalation engine

## Purpose

`cmd/recovery-loop` is the M2 mechanism of the absence-blindness architecture
(ce-23lf7, ce-a1uqr): a host-level launchd tick that reads pulse truth from the
absence-alarm heartbeat plus a registry of critical jobs, attempts self-healing
recovery instead of only alarming, and verifies that the condition cleared
before recording a recovery.

The absence-alarm escalation journal is an output of this mechanism, not an
input: it receives `recovery.human_needed` records. It is append-only and
records absences alone, so it cannot express that a pulse came back (RL-23).

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

Recovery is an observed post-condition, never an executed command. RL-23
through RL-60 require clearable current pulse truth, explicit present evidence,
and post-action observation before recovery is recorded.

## Applicability

The recovery loop executes on the host environment as a launchd agent or
standalone binary. It manages critical launchd services and binaries for the
`dear-agent` fleet.

## EARS Requirements

**RL-01** When a critical job's binary does not exist at its configured path and an install command is configured, the system shall attempt to reinstall the binary.

**RL-02** When a critical job's launchd service is not loaded in launchctl and the job is not snoozed, the system shall attempt to bootstrap the service.

**RL-03** When a critical job's launchd service is loaded but reports exit code 78 or -9, the system shall attempt to bootout and re-bootstrap the service.

**RL-04** When a critical job's ACTIVITY pulse is reported absent or undetermined in the current pulse truth (RL-23) and its service is loaded, the system shall attempt to restart the job via launchctl kickstart. A structural pulse is governed by RL-52 instead.

**RL-05** When a critical job is covered by an unexpired snooze entry in the snooze configuration, the system shall classify the job as SNOOZED and the system shall not attempt any recovery action for that job.

**RL-06** When an unexpired snooze does not cover an unloaded critical job, the system shall attempt recovery regardless of manual disabled state.

**RL-07** When the condition that triggered a recovery action is observed to have cleared, the system shall reset the consecutive failure count for that job to 0.

**RL-08** When a recovery attempt fails, the system shall increment the consecutive failure count for that job by 1.

**RL-09** When a job's consecutive failure count reaches the configured escalation threshold, the system shall record that human intervention is needed, append the required durable escalation record, and dispatch a desktop notification when the re-escalation interval is due.

**RL-10** The system shall append a structured record to the recovery journal for every recovery attempt, recording the job name, action taken, outcome status, consecutive attempt count, whether human intervention is needed, and any error message.

**RL-11** When all critical jobs are healthy, snoozed, or successfully recovered, with no recovery awaiting verification, the system shall exit 0.

**RL-12** When at least one recovery attempt fails, awaits verification, lacks a required observation, or remains in human-needed escalation, the system shall exit 1.

**RL-13** When configuration loading fails or invalid arguments are supplied, the system shall exit 2 with a usage error.

**RL-14** While dry-run mode is set, the system shall report the recovery actions it would attempt on stdout without executing those commands.

**RL-15** When JSON mode is set, the system shall emit the recovery report as a single JSON object on stdout.

**RL-16** When a tick completes and dry-run mode is not set, the system shall write a heartbeat file recording the tick time and recovery results.

**RL-17** If dispatching an escalation notification fails, then the system shall report the error on stderr and the system shall not change its exit code.

**RL-18** If the recovery state file cannot be read, then the system shall treat the state as empty and the system shall log a warning to stderr.

**RL-19** When the critical jobs configuration cannot be loaded or contains duplicate job names, the system shall exit 2 with a usage error.

**RL-20** When the shared snooze configuration is rejected under AA-14, the system shall exit 2 with a usage error.

**RL-21** When a recovery action is executed, the system shall bound execution with a timeout so a hung command cannot block the recovery loop indefinitely.

**RL-22** When a snooze entry has expired, the system shall treat the job as active and eligible for recovery.

**RL-23** The system shall read current pulse truth from the absence-alarm heartbeat, which reports presence as well as absence, and the system shall not derive pulse health from the append-only escalation journal alone.

**RL-24** When no structural failure under RL-01 through RL-03 is observed and a job's ACTIVITY pulse is present with evidence admissible under RL-47, the system shall classify the job as HEALTHY regardless of any other nonzero periodic-job exit status. A pulse that only proves the service is loaded is not an activity pulse and is governed by RL-43 and RL-45 instead.

**RL-25** When planning a recovery action, the system shall classify the job as UNHEALTHY and the system shall not classify any job as RECOVERED before an action has been executed and verified.

**RL-26** When a recovery action's command succeeds and the job's pulse has not returned, the system shall classify the job as PENDING VERIFICATION and the system shall not reset the consecutive failure count.

**RL-27** When a job's ACTIVITY pulse is observed present after a recovery action with evidence admissible under RL-39 and RL-47, the system shall classify the recovery as RECOVERED and the system shall reset the consecutive failure count to 0.

**RL-28** When a structural condition (missing binary, unloaded launchd job, or status 78/-9) persists after a recovery action, the system shall classify the recovery as FAILED.

**RL-29** When a job's ACTIVITY pulse was not observed by any evidence source, the system shall not classify the recovery as RECOVERED.

**RL-30** When a tick observes that the verification grace period has expired and the job's pulse has not returned, the system shall increment the consecutive failure count, and when that count reaches the escalation threshold the system shall append a `recovery.human_needed` record to the absence-alarm journal naming the pulse and how long it has been absent.

**RL-31** When a job's consecutive failure count reaches the give-up threshold, the system shall suppress further remediation for that job and the system shall continue to report it as requiring human intervention.

**RL-32** When the absence-alarm heartbeat is older than the maximum heartbeat age, or carries no tick time at all, the system shall treat all downstream pulse truth as unavailable rather than acting on stale evidence. RL-54 separately governs the source owner's liveness classification.

**RL-33** The system shall treat only an explicitly present pulse status as evidence of life, and the system shall not derive presence from the absence of an alarm.

**RL-34** When a job declares an ACTIVITY pulse and no pulse truth is available for it, the system shall classify the job as unverifiable, and the system shall not classify it as HEALTHY or reset its consecutive failure count.

**RL-35** When a job is covered by an unexpired snooze, the system shall honour the snooze before settling any pending verification, and the system shall not escalate that job.

**RL-36** When a job requires human intervention and a further remediation is pending verification, the system shall continue to report that job as requiring human intervention.

**RL-37** When a job's consecutive failure count has reached the give-up threshold and that job was escalated within the re-escalation interval, the system shall continue to report the job while not appending a duplicate escalation record, except that a durable journal delivery pending under RL-59 shall still be retried.

**RL-38** While dry-run mode is set, the system shall not write recovery state, update the heartbeat, append recovery or escalation journal records, or dispatch notifications on any code path, including settlement of a verification opened by an earlier tick.

**RL-39** When verifying a recovery action, the system shall treat a pulse as proof that the action worked only when the pulse was observed after the action ran, and the system shall classify an action confirmed only by earlier evidence as PENDING VERIFICATION.

**RL-40** When no escalation sink accepts a human-needed alert, the system shall not advance the re-escalation timestamp, so the next tick attempts delivery again. Acceptance by a non-durable notification sink does not satisfy the durable-journal obligation in RL-59.

**RL-41** When the post-action launchd listing cannot be obtained, pulse evidence does not independently settle the outcome, and no launchd-independent structural predicate establishes failure under RL-60, the system shall preserve the recovery as PENDING VERIFICATION, report the required observation as unavailable, and exit nonzero rather than verifying against the pre-action snapshot.

**RL-42** When a job declares no pulse and its recovery was triggered by a stopped service with a nonzero exit status, the system shall require that condition to have cleared before classifying the recovery as RECOVERED.

**RL-43** When a job's pulse only proves its service is loaded rather than that its scheduled work succeeds, the system shall not let that pulse override the exit-status evaluation.

**RL-44** When a dry run plans one or more remediations, the system shall report that action is needed rather than summarising the tick as OK.

**RL-45** When a job's pulse only proves its service is loaded, the system shall not accept that pulse as verification of a recovery, and the system shall verify such a job against its exit status.

**RL-46** When a deployed job configuration predates a safety property the built-in registry asserts for the same pulse, the system shall apply that property rather than operating without it.

**RL-47** When pulse evidence is missing its observation timestamp or is materially future-dated relative to the recovery observation, the system shall not use that evidence to classify current health, verify a recovery, or clear an existing failure state.

**RL-48** When initial launchd state cannot be observed, the system shall report the observation as unavailable and exit nonzero without executing remediation or changing the job's attempt or failure state.

**RL-49** When pulse evidence first proves recovery after the latest non-superseded verification deadline, the system shall classify the current condition as recovered and report that the evidence arrived after the deadline.

**RL-50** When a pending deadline lies more than one verification grace period in the future, the system shall bound the remaining wait to one grace period without accepting evidence from before the original action as recovery proof. When that bounded attempt settles and its action timestamp still lies in the future, the system shall quarantine the damaged timestamp and require subsequent recovery evidence to post-date the settlement observation.

**RL-51** When a recovery action executes, the system shall use the action's actual execution time as the boundary for subsequent recovery evidence. When a tick takes no recovery action, including suppression at the give-up threshold, the system shall not advance that boundary.

**RL-52** When a job's pulse only duplicates a structural launchd predicate, the system shall decide current health from the direct launchd observation and shall not let older, absent, or unavailable pulse truth override that observation. A current healthy launchd observation may clear a prior structural failure.

**RL-53** When the last successful escalation timestamp lies in the future relative to the current tick, the system shall treat escalation as due immediately and shall replace that timestamp only after an escalation sink accepts the new delivery.

**RL-54** When the canonical pulse-truth source heartbeat is missing or stale, the system shall infer source-unavailability remediation only for the job that owns that heartbeat, shall treat downstream pulse facts as unavailable, and shall require fresh post-action source evidence before recording the owner as recovered. If the source remains missing or stale after the verification grace period, the system shall count the owner's recovery attempt as failed. Independent direct structural predicates remain actionable for every job. When the source is malformed, unreadable, or future-dated, the system shall treat even the owner's source condition as unavailable and shall not remediate from that observation.

**RL-55** While any recovery is awaiting verification, the system shall expose that pending state in its report and exit nonzero.

**RL-56** When give-up policy suppresses a newly proposed remediation, the system shall distinguish that suppressed proposal from the last remediation that actually executed in reports, durable escalation records, and notifications.

**RL-57** When the system escalates an unhealthy job, it shall preserve the observed pulse status in both the machine-readable record and operator narrative, and it shall classify an unobserved or pulse-less condition as undetermined rather than fabricating an absence.

**RL-58** While a job has a standing human-needed state and no recovery has been verified, the system shall preserve that state across failed, pending, and observation-unavailable transitions, including when the configured escalation threshold changes. When recovery is verified, the system shall clear that incident's human-needed state, pending notification, and notification rate-limit window even if historical durable delivery debt remains.

**RL-59** When appending a required durable human-needed journal record fails, the system shall retain every exact rejected record in order and retry that delivery queue on the next tick where both the current job and each record's original pulse are not snoozed, without repeating a rate-limited notification. When notification dispatch fails, the system shall retain that exact current-incident narrative independently of durable delivery debt and retry it only after current observation conclusively reports the incident as failed and still human-needed. Historical delivery debt shall not suppress or replace the durable record or notification for a fresh incident. A removed job's durable debt shall still retry, but its notification shall remain deferred without a current observation. While a snooze is active, RL-35 shall defer the affected record and its ordered suffix without discarding them.

**RL-60** When a launchd-independent structural predicate proves that a recovery action failed, the system shall settle that attempt as FAILED even if launchd state cannot be observed.

## BDD Traceability

- Test consequence: Deterministic Go unit and CLI integration tests listed
  below exercise RL-01 through RL-60; the existing guardrail feature checks
  only co-located SPEC presence and RL-21 wording.
- Guardrail feature: `agm/test/bdd/features/observability_package_guardrails.feature`
  verifies that this co-located specification exists and that RL-21 declares
  bounded execution. It does not execute recovery-loop outcomes.
- BDD disposition for RL-47 through RL-60: no scenario change. These are
  deterministic evidence-timing, state-transition, reporting, and exit-code
  boundaries exercised directly by the Go tests below.
- Planning and storage tests: `pkg/recoveryloop/loop_test.go`
  (RL-01..RL-06, RL-10, RL-18, RL-19, RL-21, RL-22, RL-52, RL-54)
- Pulse-truth and verification tests: `pkg/recoveryloop/verify_test.go`
  (RL-23..RL-29, RL-32, RL-33, RL-39, RL-42, RL-43, RL-45, RL-47, RL-54)
- CLI tests: `cmd/recovery-loop/main_test.go`
  (RL-09, RL-11..RL-17, RL-46)
- Multi-tick verified-recovery CLI tests:
  `cmd/recovery-loop/verified_test.go` and
  `cmd/recovery-loop/observation_test.go`, plus focused review regressions in
  `cmd/recovery-loop/review_regression_test.go` and
  `cmd/recovery-loop/escalation_queue_test.go`
  (RL-07, RL-08, RL-10, RL-12, RL-20, RL-24, RL-26, RL-27,
  RL-29..RL-32, RL-34..RL-41, RL-44, RL-45, RL-47..RL-60)
