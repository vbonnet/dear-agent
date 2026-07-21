# cmd/ai-review — Fail-closed 5-dimension AI review gate

<!-- Last audited at: 2026-07-21 -->

## Overview

`cmd/ai-review` runs the REVIEW.md §2 five-dimension review against a pull
request diff and turns the synthesized outcome into a process exit code, so the
CI check is a real merge gate rather than an advisory comment.

## EARS Requirements

**AIREV-01** When the synthesized outcome is approved, the command shall exit successfully.

**AIREV-02** When the synthesized outcome is needs-work or rejected or needs-human-review, the command shall exit with a non-zero status unless a human override label is present.

**AIREV-03** When a human override label is present, the command shall post its review comment and exit successfully.

**AIREV-04** When the review cannot run because the API key is missing, a review dimension fails, synthesis fails, the outcome is unparseable, the pull request is from a fork, or the diff exceeds the size limit, the command shall exit with a non-zero status unless a human override label is present.

**AIREV-05** When the diff is empty, the command shall exit successfully.

**AIREV-06** When the review runs, the command shall submit the complete diff to each dimension and shall run the five dimensions concurrently as independent model calls.

## Enforcement wiring

- `.github/workflows/review.yml` invokes this command on every PR revision and
  on `merge_group`; the job name `5-Dimension AI Review` is the required-check
  context.
- `.github/rulesets/main.json` lists that context under
  `required_status_checks`.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- Package tests: `cmd/ai-review/*_test.go`
