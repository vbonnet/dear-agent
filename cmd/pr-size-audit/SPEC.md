# PR Size Audit Specification

<!-- Last audited at: 2026-08-19 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `cmd/pr-size-audit`.

## Overview

`pr-size-audit` sweeps recently merged changes on a base branch and reports the
ones that exceeded the repository PR size budget or mixed a mechanical refactor
with net-new logic. It reads merged history rather than open pull requests, so
it measures outcomes rather than intentions.

It is the audit leg of the PR-size DEAR cycle. The open-PR gate flags an
oversized change and the split-request job asks for a split; neither records
what happened afterwards, which is how a 100% split-decline rate stayed
invisible for two days. Following `cmd/merge-audit`, this command detects and
reports only — it never reverts, blocks, or mutates a pull request.

## EARS Requirements

**PRSIZEAUDIT-01** When the sweep completes, the command shall exit successfully regardless of how many offenders it found.

**PRSIZEAUDIT-02** When a swept merge exceeds either the changed-line budget or the changed-file budget, the command shall report that merge as an offender.

**PRSIZEAUDIT-03** When a swept merge is within both budgets and is not mixed-concern, the command shall exclude it from the offender list.

**PRSIZEAUDIT-04** When a swept merge mixes a mechanical refactor with net-new logic, the command shall report that merge as an offender.

**PRSIZEAUDIT-05** When the report is rendered, the command shall include the swept count, the over-budget count and share, the mixed-concern count, and the median, ninetieth-percentile, and maximum changed-line counts.

**PRSIZEAUDIT-06** When no swept merge is an offender, the command shall state that the window is clean.

**PRSIZEAUDIT-07** When a merge subject ends with a pull-request reference, the command shall report that pull-request number in place of the commit identifier.

**PRSIZEAUDIT-08** When a merge subject contains a Markdown table delimiter, the command shall escape it in the rendered report.

**PRSIZEAUDIT-09** When the requested limit is not positive or the requested format is unknown, the command shall report the invalid argument and exit with the usage status.

**PRSIZEAUDIT-10** When the base branch history cannot be listed, the command shall report the failure and exit with the usage status.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- Package tests: `cmd/pr-size-audit/*_test.go`

## Test Traceability

- Unit package: `cmd/pr-size-audit`
- Classification library: `internal/prconcern`
- Consumer: `.github/workflows/pr-size-audit.yml`
