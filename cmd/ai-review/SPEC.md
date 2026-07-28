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

**AIREV-07** When the changed paths touch agent permissions, pre or post tool hooks, security boundaries, expensive-to-reverse infrastructure, or CI and CD pipeline definitions, the command shall force the outcome to needs-human-review regardless of the synthesized outcome.

**AIREV-08** When the pull request body or a commit message contains the explicit human review required marker, the command shall force the outcome to needs-human-review.

**AIREV-09** When the synthesis output does not contain an exact canonical outcome token, or the approval token is negated, the command shall treat the outcome as needs-human-review.

## Enforcement wiring

- `.github/workflows/review.yml` invokes this command from trusted
  `pull_request_target` PR revisions, publishing its result on the reviewed
  head under the check context `5-Dimension AI Review`.
- **Paused 2026-07-27**: that context is **not** currently listed under
  `required_status_checks` in `.github/rulesets/main.json`, so it does not
  block merges. The workflow also no longer invokes this command at all when
  `ANTHROPIC_API_KEY` is unset and no `ai-review:override` label is active — it
  publishes a `neutral` check instead. Every requirement above still holds
  whenever the command *is* invoked; AIREV-04's fail-closed-on-missing-key
  behavior is unchanged and still applies on the override path. Re-enable by
  setting the secret, and re-add the context to `main.json` per
  `docs/branch-protection.md` to make it required again.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- Package tests: `cmd/ai-review/*_test.go`
