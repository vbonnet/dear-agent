# ci-escape-analysis Command Specification (EARS)

<!-- Last audited at: 2026-08-18 -->

**Version**: 2.0
**Status**: Active
**Scope**: `tools/ci-escape-analysis` — fact gathering, invocation modes, and issue mutation.

## Overview

This command is the plumbing around `pkg/cihealth`. It gathers facts from the
GitHub CLI, hands them to the classifier, and files the resulting retro.

**The classification contract and the ROI arithmetic are NOT specified here.**
They are owned by `pkg/cihealth/SPEC.md` (`CIHEALTH-01`..`CIHEALTH-24`) and
stated once. An earlier revision of this file restated them as `CI-ESCAPE-01`
..`CI-ESCAPE-07`, which created two normative contracts for one behaviour —
the sort of pair that drifts the first time a class or a threshold moves.
Requirements below cover only what this command does that the package cannot:
talk to GitHub, and mutate issues.

## EARS Requirements

### Fact gathering

**CI-ESCAPE-01** When the system invokes the GitHub CLI, the system shall bound the call with a timeout and shall report an expired call as a failure, so a stalled network call or an interactive prompt cannot hang the workflow.

**CI-ESCAPE-02** When a fact lookup fails, the system shall report the failure on standard error and shall carry the fact into the retro as unknown rather than substituting a zero value or an empty list.

**CI-ESCAPE-03** When the system determines the current state of `main`, the system shall enumerate the repository's active workflows and query each workflow's own latest run, rather than taking a bounded slice of repository-wide runs.

**CI-ESCAPE-04** When the system evaluates whether a workflow is red, the system shall treat every terminal unsuccessful conclusion as red, and shall treat in-flight and successful runs as not red.

**CI-ESCAPE-05** When the system identifies the failing check for a red workflow, the system shall select a failed job belonging to that specific workflow run.

**CI-ESCAPE-06** When the system determines whether a failing check could have run pre-merge, the system shall consider both the workflow's trigger block and the producing job's event guard, and shall default to pre-merge capable when either is unknown.

**CI-ESCAPE-07** When the system counts escapes, the system shall count distinct failing commits rather than failing runs, so repeated runs against one unchanged commit are one incident.

**CI-ESCAPE-08** When the system measures prevention cost, the system shall report whether any qualifying pre-merge run was observed and whether the query reached the API page limit.

### Invocation

**CI-ESCAPE-09** When invoked without `-sweep`, the system shall render one retrospective to standard output as a pure function of its flags and the facts it looks up, and shall mutate nothing.

**CI-ESCAPE-10** When invoked without `-sweep`, the system shall accept pre-merge capability and scheduled-detection as explicit inputs, because there is no sweep context to derive them from.

**CI-ESCAPE-11** When a cost term is not supplied on the command line, the system shall mark that term as assumed rather than measured.

**CI-ESCAPE-12** When invoked in dry-run mode, the system shall render what it would file and shall not create, comment on, or close any issue.

### Issue mutation

**CI-ESCAPE-13** When the system files retrospectives, the system shall label them with a label owned by this command alone, so no other workflow closes or annotates them.

**CI-ESCAPE-14** When sweeping, the system shall file one retrospective per red workflow and shall comment on an existing open retrospective rather than opening a duplicate.

**CI-ESCAPE-15** When sweeping, the system shall close every retrospective carrying its label that the current sweep did not re-file, which covers both a recovered workflow and a workflow whose failing check has changed.

**CI-ESCAPE-16** When sweeping, the system shall report which retrospectives it opened for the first time, so the caller can distinguish a new incident from a recurrence.

**CI-ESCAPE-17** When any retrospective mutation fails, the system shall continue processing the remaining workflows and shall exit non-zero, so a failed alert is never reported as a successful sweep.

## Test Traceability

- Command tests: `tools/ci-escape-analysis/triggers_test.go`
- Classification and ROI contract: `pkg/cihealth/SPEC.md`

## BDD Traceability

- Feature: `agm/test/bdd/features/ci_health_escape_analysis.feature`

## Non-Goals

- Classifying escapes or pricing prevention. Both belong to `pkg/cihealth`.
- Deciding when to run. The watchdog workflow owns the trigger.
