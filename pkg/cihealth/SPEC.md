# cihealth Package Specification

<!-- Last audited at: 2026-08-18 -->

**Version:** 2.0
**Status:** Baseline
**Scope:** `pkg/cihealth`.

This file is the single owner of the escape classification contract and the
prevention-vs-cure arithmetic. `tools/ci-escape-analysis/SPEC.md` specifies only
that command's fact gathering, invocation modes, and issue mutation, and defers
here for behaviour.

## Overview

`cihealth` answers two questions about a failure observed on `main`: how it got
past pre-merge, and whether moving the producing check pre-merge is worth
paying for. Both answers are pure functions over plain structs — fetching the
facts from GitHub lives in `tools/ci-escape-analysis`, so every judgement this
package makes is reachable from a table test.

The package also renders the incident brief the main-health watchdog files when
an escape is detected. The brief is not a completed DEAR retrospective: at
detection time nothing has been executed and there is no outcome to audit.

## EARS Requirements

### Classification

**CIHEALTH-01** When the failing check could not have run on a pull request, the system shall classify the failure as `post-merge-only` before evaluating any other condition, so a scheduled detector is never reported as a path-filter gap.

**CIHEALTH-02** When no pull request introduced the commit, the system shall classify the failure as `bypassed` and direct the reader at branch protection rather than at CI selection.

**CIHEALTH-03** When the failing check never reported on the pull request, the system shall classify the failure as `never-ran` and shall mark it filter-refinable.

**CIHEALTH-04** When the failing check reported `skipped` on the pull request, the system shall classify the failure as `selection-gap` and shall mark it filter-refinable.

**CIHEALTH-05** When the failing check reported `failure` on the pull request and is not a required context, the system shall classify the failure as `gating-gap` and shall attribute the escape to enforcement rather than selection.

**CIHEALTH-06** When the failing check reported `failure` on the pull request and is a required context, the system shall classify the failure as `gating-gap` and shall direct the reader at the bypass audit and ruleset history.

**CIHEALTH-07** When the failing check passed pre-merge and the check is diff-scoped, the system shall classify the failure as `scope-gap` and shall state that the narrower pre-merge scope is deliberate.

**CIHEALTH-08** When the failing check passed pre-merge at the same scope it runs at post-merge, the system shall classify the failure as `merge-skew`.

**CIHEALTH-09** When the system classifies a failure, the system shall mark `FilterRefinable` true only for `never-ran` and `selection-gap`, because those are the only classes path-filter refinement can fix.

**CIHEALTH-10** When the system classifies a failure, the system shall emit a summary and at least one suggested action naming the mechanism that addresses that class, and every mechanism named shall exist in this repository.

**CIHEALTH-19** When the failing check reported a conclusion on the pull request that is neither `success`, `failure`, nor `skipped`, the system shall classify the failure as `inconclusive` and shall not assert that the check passed pre-merge, because `scope-gap` and `merge-skew` both rest on a pre-merge pass.

**CIHEALTH-20** When the failure was observed on a scheduled or dispatched run, the system shall classify it as `post-merge-only` before consulting any pull request, because the commit at the head of `main` is not evidence of what caused a detection the clock triggered.

**CIHEALTH-21** When the required-context list could not be established, the system shall report the enforcement question as unresolved and shall not assert that the failing check is advisory.

### ROI pricing

**CIHEALTH-11** When prevention cost has not been measured, the system shall return a zero ratio and an insufficient-data verdict rather than inferring a placement, so a check with no pre-merge runs is never scored as infinitely worth blocking.

**CIHEALTH-12** When prevention cost is measured as zero and the cure-times-frequency product is non-zero, the system shall treat the ratio as unbounded, because a free check should always run.

**CIHEALTH-13** When prevention cost is measured as zero and the cure-times-frequency product is zero, the system shall return a zero ratio rather than an unbounded one.

**CIHEALTH-14** When prevention cost is measured and positive, the system shall compute the ratio as cure minutes multiplied by escapes, divided by prevention minutes.

**CIHEALTH-15** When the ratio exceeds ten to one, the system shall return an always-prevent verdict; above three to one, a usually-prevent verdict; above zero, a case-by-case verdict; and at zero, a no-signal verdict.

**CIHEALTH-16** When the system explains a ratio, the system shall render the arithmetic including both operands, so a retro shows its work rather than asserting a verdict.

**CIHEALTH-22** When the cure cost is a standing default rather than a measurement of the incident, or the prevention measurement was truncated at the API page limit, the system shall mark the verdict `PROVISIONAL` and shall name every term that is not evidence.

**CIHEALTH-23** When the system explains a ratio, the system shall state the provenance of each term — measured, assumed, unmeasured, or a lower bound — so a threshold crossing can be attributed to evidence or to an assumption.

**CIHEALTH-24** When the system reports an escape count, the system shall state the scope that count was taken over, because a count taken per workflow does not license a claim about an individual check.

### Retro rendering

**CIHEALTH-17** When a retro is rendered, the system shall produce a title and a body identifying the failing check, the main SHA, the class, and the ROI arithmetic.

**CIHEALTH-18** When required contexts are listed, the system shall return them in a deterministic sorted order so retro bodies do not churn between runs.

**CIHEALTH-25** When the system renders the incident brief, the system shall state that it is not a completed retrospective and shall emit empty `Execute` and `Audit` sections for the fixer to complete, because DEAR requires an executed fix and an audited outcome.

**CIHEALTH-26** When the class's remedy is not to move or widen the check pre-merge, the system shall omit the prevention-versus-cure verdict, because a placement verdict would contradict the finding it accompanies.

**CIHEALTH-28** When the system reports a prevention cost, the system shall state the scope that cost was measured over, because a figure timed across a whole workflow does not license a claim about one job.

**CIHEALTH-27** When the system decides whether a failing check is a required context, the system shall compare the producing app against the app the ruleset pins the context to, and shall treat an unknown producer as matching.

## BDD Traceability

- Feature: `agm/test/bdd/features/ci_health_escape_analysis.feature`

## Test Traceability

- Unit package: `pkg/cihealth`

## Non-Goals

- The package performs no network access; the caller supplies every fact.
- The package does not decide whether to file a retro; the watchdog workflow owns that trigger.
