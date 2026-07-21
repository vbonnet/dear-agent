# cmd/ai-review — Fail-closed 5-dimension AI review gate

<!-- Last audited at: 2026-07-21 -->

`ai-review` runs the REVIEW.md §2 five-dimension review against a PR diff and
turns the synthesized outcome into a process exit code, so the CI check is a
real merge gate rather than an advisory comment.

## EARS Requirements

**AIREV-01** When the synthesized outcome is `approved`, the command shall exit 0.

**AIREV-02** When the synthesized outcome is `needs-work`, `rejected`, or
`needs-human-review` and no override label is present, the command shall exit
non-zero.

**AIREV-03** When the `ai-review:override` label is present, the command shall
post its comment and exit 0 (the verified human fallback).

**AIREV-04** If `ANTHROPIC_API_KEY` is unset, a review dimension fails, synthesis
fails, the outcome is unparseable, the PR is from a fork, or the diff exceeds the
size limit, then the command shall exit non-zero (fail closed), absent an
override.

**AIREV-05** When the diff is empty, the command shall exit 0.

**AIREV-06** The command shall submit the complete diff to each dimension and
shall run the five dimensions concurrently as independent model calls.

## Enforcement wiring

- `.github/workflows/review.yml` invokes this command on every PR revision and
  on `merge_group`; the job name `5-Dimension AI Review` is the required-check
  context.
- `.github/rulesets/main.json` lists that context under
  `required_status_checks`.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
  (co-located SPEC coverage for `cmd/` directories).
- Unit tests: `cmd/ai-review/outcome_test.go`, `cmd/ai-review/main_test.go`.
