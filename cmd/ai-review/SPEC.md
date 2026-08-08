# cmd/ai-review — Fail-closed 5-dimension AI review gate

<!-- Last audited at: 2026-07-21 -->

## Overview

`cmd/ai-review` runs the REVIEW.md §2 five-dimension review against a pull
request diff and turns the synthesized outcome into a process exit code, so the
CI result can become a merge gate rather than an advisory comment after the
separate provider-enforcement rollout is verified.

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

**AIREV-10** When a pull request changes a file whose exact basename is `SPEC.md`, including an addition, deletion, or either side of a rename, the command shall build an authenticated versioned review plan before accessing a reviewer credential or model.

**AIREV-11** When a changed SPEC lacks either exact reciprocal BDD feature evidence parsed from the bounded canonical template form or established labeled form in visible Markdown, the exact canonical `- No BDD change, with reason: ...` declaration, or an established deterministic schema/unit/integration test consequence in its canonical traceability section, links a feature that is not valid scenario-bearing Gherkin with runnable steps and at least one executable case for every Scenario or Scenario Outline, links a Scenario Outline without a nonempty structurally valid Examples body, exceeds the bounded review capacity for executable cases or inherited and scenario step instances, fails strict EARS validation over the same line-preserving visible-Markdown view used for requirement extraction, reuses a changed requirement identifier or normalized promise elsewhere in the HEAD SPEC corpus, claims normative ownership under a registered harness root, deletes the whole SPEC, requires a truncated or incomplete owner search, exceeds an input bound, or the SPEC reviewer is unavailable, ambiguous, or cannot authenticate its evidence to the reviewed revisions and paths, the command shall report needs-human-review and shall not approve the change.

**AIREV-12** When a changed SPEC plan is reviewable, the command shall classify every authenticated unchanged-SPEC owner shard with one strict versioned JSON call per shard, at no more than four concurrent calls, and shall require one ordered result for every candidate; only a complete all-`distinct` classification may proceed to one final SPEC contract review call, while `possible-owner`, `uncertain`, an incomplete response, or a provider failure shall require human review, and the final blocking verdict shall not be upgraded by five-dimension synthesis.

**AIREV-13** When a changed SPEC plan is reviewable, the command shall use `docs/spec-authoring.md` from the authenticated current protected-base revision as its sole substantive prompt policy and shall supply complete bounded current changed-SPEC contracts, applicable parsed and bounded authenticated Gherkin evidence or declared deterministic/no-BDD consequence evidence, a path-sorted bounded owner index that projects every unchanged HEAD SPEC exactly once through its ordinal, path, complete whitespace-normalized visible contract text, normalized requirement identifiers, exact feature pointers and backlink/lexical signals, canonical SHA-256 index and shard digests, and the complete active-member applicability grid for every current promise in each added or modified SPEC; the review contract shall require human review rather than an invented owner or confirmed defect when the semantic evidence is empty, low-confidence, or incomplete.

**AIREV-14** When a modified SPEC deletes a stable requirement, the command shall include the deleted identifier and prior promise in authenticated review evidence and shall require one strict structured reviewer disposition for that deletion or report needs-human-review.

**AIREV-15** When the command invokes Git to inspect a pull request revision, the command shall enforce time and output limits, shall expose only an allowlisted non-credential environment to the subprocess, and shall read exact committed object bytes without applying revision-controlled export transformations.

**AIREV-16** When a pull request changes the canonical SPEC authoring entry point, template, write or audit workflow and reference files, active harness registry, trusted review workflow or ruleset, review implementation, deterministic Markdown or EARS parsers, Go build manifests, workspace manifests, or vendored dependencies used by that implementation, the command shall require maintainer review rather than allow the revision to approve its own enforcement change.

**AIREV-17** When a changed SPEC or protected enforcement owner is reviewed from a head that does not contain the current protected base, the command shall require the branch to be updated and shall not bind policy or corpus evidence to the stale merge base.

**AIREV-18** When the trusted source workflow runs, it shall attempt to publish a uniquely named `SPEC Contract Review` semantic verdict for the reviewed pull-request head and shall fail its distinct native `AI review orchestration` job when creation or publication is unavailable; without a reviewer credential, only plans with no changed SPEC, protected owner, binary or gitlink evidence, or deterministic AIREV-07 or AIREV-08 escalation may publish neutral, while every other relevant plan shall remain fail closed pending a revision-bound maintainer override. Neither source result is yet a provider-required merge gate.

**AIREV-19** When a changed SPEC plan is reviewable, the command shall parse the exact package-level `activeHarnesses` string-slice literal from `agm/internal/agent/harnesses.go` at the authenticated protected-base revision and shall require one final `supported`, `adapted`, `unsupported`, or `not-applicable` reviewer disposition for every active member and every current promise in each added or modified SPEC.

**AIREV-20** When a pull request adds or modifies a normative `SPEC.md` beneath a registered dotted configuration root, a plugin registration root, an explicit `harness/` or `harnesses/` grouping, or a top-level authenticated active-harness alias, the command shall reject the local owner without relying on peer similarity. An `internal/` or `cmd/` ancestor shall not conceal an intrinsic registration root; native variation shall remain an applicability-scoped requirement in one shared product or domain owner, while legitimate logical owners under `internal/` and `cmd/` without such a registration root shall remain eligible for semantic review.

**AIREV-21** When a pull request adds, modifies, deletes, or renames a file whose exact basename is `SPEC.owner`, the command shall record the authenticated changed path and status as a normative ownership-edge change and require a revision-bound maintainer review rather than allow the pointer to approve its own reassignment.

**AIREV-22** When a changed SPEC plan is reviewable, the command shall prove before credential access that every owner-classification prompt, the canonical minimum-field-value and maximum-field-value complete shard verdicts, and the minimum complete final versioned verdict fit their accepted bounds, shall use dedicated bounded owner-classification and final SPEC output budgets, and shall reject any model response whose stop reason is not `end_turn`.

## Enforcement wiring

- `.github/workflows/review.yml` invokes this command from trusted
  `pull_request_target` revisions and publishes its result on the reviewed
  head under the unique `SPEC Contract Review` check context. The workflow's
  native `AI review orchestration` job has a distinct name and fails if
  creation or publication of that semantic check fails.
- This source workflow is not evidence that either context is provider-required
  or that an LLM reviewed a pull request. Provider enforcement, credentials,
  and an exact-head canary belong to a separate reviewed infrastructure
  rollout. `pull_request_target` attaches its native job to the protected-base
  revision, so that job cannot be required on the pull-request head. The
  semantic context must not be made provider-required until that later rollout
  supplies and canaries a trusted head-attached transport mechanism, or removes
  mutable same-head inputs so an earlier verdict remains valid.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- Package tests: `cmd/ai-review/*_test.go`
