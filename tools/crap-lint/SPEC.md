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

**CRAPLINT-01** When the CRAP lens is invoked without both revisions that bound the diff, the command shall name the missing revision flag and exit with the usage status rather than score an empty range.

**CRAPLINT-02** When the diff between the supplied revisions cannot be read, the command shall report the failure and exit with the usage status without emitting a report.

**CRAPLINT-03** When changed functions have been scored, the command shall exit successfully whether or not any function exceeded the threshold, so this signal can never be the reason a pull request fails.

**CRAPLINT-04** When GitHub output form is requested, the command shall emit the flagged verdict and, for a flagged verdict, the rendered report in a terminated heredoc block.

**CRAPLINT-05** When a rendered report contains a line matching the heredoc delimiter, the command shall omit that line from the emitted block.

**CRAPLINT-06** When GitHub output form is requested for an unflagged diff, the command shall emit an empty report value and no heredoc block.

**CRAPLINT-07** When prose output is requested for an unflagged diff, the command shall report the scored function count and how many met the agent-written target, and shall report a diff where nothing could be measured as unmeasured rather than as clean.

**CRAPLINT-08** When a rendered report is emitted in a heredoc block, the block shall contain no trailing blank line before its delimiter.

**CRAPLINT-09** When GitHub output form is requested for an unflagged diff, the command shall emit the prose summary as a single-line output value, so a consumer can disclose why nothing is flagged even when the diff is not measured.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- No BDD change, with reason: this command is a thin argument-and-output wrapper over `internal/craplens`, mirroring how `tools/pr-concern-lint` wraps `internal/prconcern`, and its behavior is proven by `tools/crap-lint/main_test.go`. The analysis it wraps carries its own contract and tests, so a scenario here would assert the wrapper twice and the analysis not at all.

## Test Traceability

- Unit package: `tools/crap-lint`
- Analysis library: `internal/craplens`
- Consumer: `.github/workflows/pr-size-scope.yml`
