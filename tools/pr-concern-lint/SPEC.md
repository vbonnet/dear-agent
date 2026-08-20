# PR Concern Lint Command Specification

<!-- Last audited at: 2026-08-18 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `tools/pr-concern-lint`.

## Overview

`pr-concern-lint` reports whether a pull request mixes a mechanical refactor
with net-new logic. It is the command wrapper over `internal/prconcern` and is
invoked by `.github/workflows/pr-size-scope.yml`, whose existing thresholds
react only to raw diff size.

The command is advisory by construction: a mixed verdict still exits
successfully, so this signal can never become the reason a pull request cannot
merge. Only usage and operational failures exit non-zero.

## EARS Requirements

**PRCONCERNLINT-01** When a base or head revision is not supplied, the command shall report the missing argument and exit with the usage status.

**PRCONCERNLINT-02** When the diff between the supplied revisions cannot be collected, the command shall report the failure and exit with the usage status.

**PRCONCERNLINT-03** When the diff is analyzed, the command shall exit successfully regardless of the verdict.

**PRCONCERNLINT-04** When GitHub output form is requested, the command shall emit the mixed-concern verdict and, for a mixed verdict, the reason in a terminated heredoc block.

**PRCONCERNLINT-05** When a rendered reason contains a line matching the heredoc delimiter, the command shall omit that line from the emitted block.

**PRCONCERNLINT-06** When prose output is requested for a single-concern diff, the command shall report the move-only count and the net-new logic total.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`

## Test Traceability

- Unit package: `tools/pr-concern-lint`
- Analysis library: `internal/prconcern`
- Consumer: `.github/workflows/pr-size-scope.yml`
