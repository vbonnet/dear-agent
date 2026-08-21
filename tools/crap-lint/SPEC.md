# CRAP Lint Command Specification

<!-- Last audited at: 2026-08-20 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `tools/crap-lint`.

## Overview

`crap-lint` reports the changed functions whose complexity is not matched by
test coverage. It is the command wrapper over `internal/craplens` and is
invoked by `.github/workflows/pr-size-scope.yml`, alongside the existing size
and mixed-concern signals that share that workflow's comment.

The command is advisory by construction: a flagged diff still exits
successfully, so this signal can never become the reason a pull request cannot
merge. Only usage and operational failures exit non-zero.

## EARS Requirements

**CRAPLINT-01** When a base or head revision is not supplied, the command shall report the missing argument and exit with the usage status.

**CRAPLINT-02** When the diff between the supplied revisions cannot be read, the command shall report the failure and exit with the usage status without emitting a report.

**CRAPLINT-03** When the diff is analyzed, the command shall exit successfully regardless of the verdict.

**CRAPLINT-04** When GitHub output form is requested, the command shall emit the flagged verdict and, for a flagged verdict, the rendered report in a terminated heredoc block.

**CRAPLINT-05** When a rendered report contains a line matching the heredoc delimiter, the command shall omit that line from the emitted block.

**CRAPLINT-06** When GitHub output form is requested for an unflagged diff, the command shall emit an empty report value and no heredoc block.

**CRAPLINT-07** When prose output is requested for an unflagged diff, the command shall report the scored function count and how many met the agent-written target.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`

## Test Traceability

- Unit package: `tools/crap-lint`
- Analysis library: `internal/craplens`
- Consumer: `.github/workflows/pr-size-scope.yml`
