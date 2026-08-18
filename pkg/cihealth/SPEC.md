# cihealth Package Specification

<!-- Last audited at: 2026-08-18 -->

**Version:** 2.0
**Status:** Baseline
**Scope:** `pkg/cihealth`.

## Overview

`cihealth` answers two questions about a failure observed on `main`: how it got
past pre-merge, and whether moving the producing check pre-merge is worth
paying for. Both answers are pure functions over plain structs — fetching the
facts from GitHub lives in `tools/ci-escape-analysis`, so every judgement this
package makes is reachable from a table test.

This file is the single owner of the escape classification contract and the
prevention-versus-cure arithmetic. `tools/ci-escape-analysis/SPEC.md` specifies
only that command's fact gathering, invocation, and issue mutation, and defers
here for behaviour.

The package also renders the incident brief the main-health watchdog files. The
brief is not a completed DEAR retrospective: at detection time nothing has been
executed and there is no outcome to audit.

## EARS Requirements

### Classification precedence

The order matters. Each requirement is evaluated before the ones after it, so an
unresolved fact can never be dressed up as a finding about selection.

**CIHEALTH-01** When the checks that reported on the pull request are not known, the system shall classify the failure as `unknown` before evaluating any other condition, because every other class asserts something about checks that were never read.

**CIHEALTH-02** When the failing check could not have run on a pull request, the system shall classify the failure as `post-merge-only`, so a scheduled detector is never reported as a path-filter gap.

**CIHEALTH-03** When the failure was observed on a scheduled or dispatched run, the system shall classify it as `post-merge-only`, because a detection the clock triggered is not attributable to the commit at the head of `main`.

**CIHEALTH-04** When the run failed before producing any job, the system shall classify the failure as `inconclusive` and attribute it to the workflow definition rather than to selection.

**CIHEALTH-05** When no pull request introduced the commit, the system shall classify the failure as `bypassed` and direct the reader at branch protection rather than at CI selection.

**CIHEALTH-06** When the failing check never reported on the pull request, the system shall classify the failure as `never-ran` and shall mark it filter-refinable.

**CIHEALTH-07** When the failing check reported `skipped`, the system shall classify the failure as `selection-gap` and shall mark it filter-refinable.

**CIHEALTH-08** When the failing check reported `failure`, the system shall classify the failure as `gating-gap`, shall distinguish a required context from an advisory one, and shall report the enforcement question as unresolved when the required-context list could not be established.

**CIHEALTH-09** When the failing check reported any other non-success conclusion, including a check still pending at merge time, the system shall classify the failure as `inconclusive`, because `scope-gap` and `merge-skew` both rest on a pre-merge pass.

**CIHEALTH-10** When the failing check passed pre-merge and is diff-scoped, the system shall classify the failure as `scope-gap` and state that the narrower pre-merge scope is deliberate; when it passed at the same scope, the system shall classify the failure as `merge-skew`.

### Classification output

**CIHEALTH-11** When the system classifies a failure, the system shall emit a summary and at least one suggested action naming the mechanism that addresses that class; every mechanism named shall exist in this repository, and `FilterRefinable` shall be true for no class other than `never-ran` and `selection-gap`.

**CIHEALTH-12** When the system decides whether a failing check is a required context, the system shall compare the producing app against the app the ruleset pins the context to, and shall treat an unknown producer as matching.

### ROI pricing

**CIHEALTH-13** When prevention cost or the escape count has not been measured, the system shall return a zero ratio and an insufficient-data verdict rather than inferring a placement; when prevention cost is measured as zero, the system shall treat the ratio as unbounded if the cure-times-frequency product is non-zero and as zero otherwise, because a free check should always run but a free check that has caught nothing is no evidence.

**CIHEALTH-14** When prevention cost and the escape count are both measured, the system shall compute the ratio as cure minutes multiplied by escapes divided by prevention minutes, and shall return an always-prevent verdict above ten to one, usually-prevent above three to one, case-by-case above zero, and no-signal at zero.

**CIHEALTH-15** When the system explains a ratio, the system shall render the arithmetic, shall state each term's provenance as measured, assumed, unmeasured, or a lower bound, shall state the scope each term was measured over, and shall mark the verdict provisional naming every term that is not evidence rather than rendering a prescriptive band.

### Rendering

**CIHEALTH-16** When a brief is rendered, the system shall identify the failing check, the main SHA, and the class; shall list required contexts in a deterministic sorted order labelled as the ruleset read at analysis time; and shall report required contexts as not established when the ruleset could not be read.

**CIHEALTH-17** When a brief is rendered, the system shall state that it is not a completed retrospective and shall emit empty `Execute` and `Audit` sections for the fixer to complete, because DEAR requires an executed fix and an audited outcome.

**CIHEALTH-18** When the class's remedy is not to move or widen the check pre-merge, the system shall omit the prevention-versus-cure verdict and shall state in that class's own terms why placement is not the decision, because a placement verdict would contradict the finding it accompanies.

## BDD Traceability

- Feature: `agm/test/bdd/features/ci_health_escape_analysis.feature`

## Test Traceability

- Unit package: `pkg/cihealth`

## Non-Goals

- The package performs no network access; the caller supplies every fact.
- The package does not decide whether to file a brief; the watchdog workflow owns that trigger.
