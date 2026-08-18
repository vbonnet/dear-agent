# ci-escape-analysis Command Specification (EARS)

<!-- Last audited at: 2026-08-18 -->

**Version**: 2.0
**Status**: Active
**Scope**: `tools/ci-escape-analysis` — fact gathering, invocation, and issue mutation.

## Overview

This command is the plumbing around `pkg/cihealth`. It gathers facts from the
GitHub CLI, hands them to the classifier, and files the resulting incident brief.

**The classification contract and the ROI arithmetic are NOT specified here.**
They are owned by `pkg/cihealth/SPEC.md` and stated once. An earlier revision
restated them, creating two normative contracts for one behaviour — the sort of
pair that drifts the first time a class or a threshold moves.

## EARS Requirements

### Fact gathering

**CI-ESCAPE-01** When the system invokes the GitHub CLI, the system shall bound the call with a timeout and shall report an expired call as a failure, so a stalled network call or an interactive prompt cannot hang the workflow.

**CI-ESCAPE-02** When a fact lookup fails, the system shall report the failure on standard error and shall carry the fact into the brief as unknown, rather than substituting a zero, an empty list, or a fabricated check name.

**CI-ESCAPE-03** When the system determines the state of `main`, the system shall enumerate the repository's active workflows and query each workflow's own latest run, rather than taking a bounded slice of repository-wide runs.

**CI-ESCAPE-04** When the system judges whether a workflow is red, the system shall treat every terminal unsuccessful conclusion as red, shall treat a cancelled run as carrying no evidence and continue to the last run that concluded, and shall consider only runs from events that evaluate `main`.

**CI-ESCAPE-05** When a workflow's checked-out definition has no trigger reaching `main`, the system shall exclude it from the health picture; the system shall not otherwise expire health state on the ROI lookback window.

**CI-ESCAPE-06** When the system identifies the failing check for a red workflow, the system shall select a failed job belonging to that specific workflow run, and shall report the failure as workflow-level when the run produced no job.

**CI-ESCAPE-07** When the system determines whether a failing check could have run pre-merge, the system shall consider both the workflow's trigger block and the producing job's event guard including its comparison operator, and shall default to pre-merge capable when either is unknown.

**CI-ESCAPE-08** When the system identifies the pull request that introduced a commit on `main`, the system shall require a pull request that merged into `main` with that commit as its merge commit, and shall otherwise report no pull request.

**CI-ESCAPE-09** When the system reads the checks that reported on a pull request, the system shall select each attempt as it stood at the merge, shall report an attempt still running at the merge as pending rather than absent, and shall key attempts by producing app as well as by name.

**CI-ESCAPE-10** When the system counts escapes, the system shall count distinct failing commits across every conclusion it treats as red, and shall report the count as truncated when the query reaches the API page limit and as unmeasured when it fails.

**CI-ESCAPE-11** When the system measures prevention cost, the system shall report whether any qualifying pre-merge run was observed and whether the query reached the API page limit.

### Invocation

**CI-ESCAPE-12** When invoked without `-sweep`, the system shall render one brief to standard output as a pure function of its flags and the facts it looks up, shall mutate nothing, and shall accept pre-merge capability and scheduled-detection as explicit inputs.

**CI-ESCAPE-13** When a cost term is not supplied on the command line, the system shall mark that term as assumed rather than measured.

**CI-ESCAPE-14** When invoked in dry-run mode, the system shall render what it would file and shall not create, comment on, reopen, or close any issue.

### Issue mutation

**CI-ESCAPE-15** When the system files briefs, the system shall label them with a label owned by this command alone, so no other workflow closes or annotates them.

**CI-ESCAPE-16** When sweeping, the system shall file one brief per red workflow, shall comment on an existing open brief rather than opening a duplicate, and shall reopen and requeue a closed brief when the same check fails again.

**CI-ESCAPE-17** When sweeping, the system shall close a brief only when a later run of the same failing job succeeded, and shall skip reconciliation entirely when any workflow's run lookup failed, so a transient API failure cannot be read as recovery.

**CI-ESCAPE-18** When the system files a brief, the system shall mark it queued; when a sweep completes with every mutation successful, the system shall hand off at most one queued brief and remove its queue label, so simultaneous incidents are worked one at a time and a handoff is never recorded for a dispatch the caller will skip.

**CI-ESCAPE-19** When any brief mutation fails, the system shall continue processing the remaining workflows and shall exit non-zero, so a failed alert is never reported as a successful sweep.

## Test Traceability

- Command tests: `tools/ci-escape-analysis/triggers_test.go`
- Classification and ROI contract: `pkg/cihealth/SPEC.md`

## BDD Traceability

- Feature: `agm/test/bdd/features/ci_health_escape_analysis.feature`

## Non-Goals

- Classifying escapes or pricing prevention. Both belong to `pkg/cihealth`.
- Deciding when to run. The watchdog workflow owns the trigger.
