# cmd/ai-review — Fail-closed 5-dimension AI review gate

<!-- Last audited at: 2026-07-21 -->

## Overview

`cmd/ai-review` runs the REVIEW.md §2 five-dimension review against a pull
request diff and turns the synthesized outcome into a process exit code, so the
CI result can become a merge gate rather than an advisory comment after the
separate provider-enforcement rollout is verified.

## EARS Requirements

**AIREV-01** When the synthesized outcome is approved, the command shall exit successfully.

**AIREV-02** When the synthesized outcome is needs-work or rejected or needs-human-review, the command shall exit with a non-zero status unless the trusted workflow supplies an authenticated revision-bound human override.

**AIREV-03** When the trusted workflow supplies an authenticated revision-bound human override, the command shall post its review comment and exit successfully.

**AIREV-04** When the review cannot run because the API key is missing, a review dimension fails, synthesis fails, the outcome is unparseable, the pull request is from a fork, or the diff exceeds the size limit, the command shall exit with a non-zero status unless the trusted workflow supplies an authenticated revision-bound human override.

**AIREV-05** When the diff is empty, the command shall exit successfully.

**AIREV-06** When the review runs, the command shall submit the complete diff to each dimension and shall run the five dimensions concurrently as independent model calls.

**AIREV-07** When the changed paths touch agent permissions, pre or post tool hooks, security boundaries, expensive-to-reverse infrastructure, or CI and CD pipeline definitions, the command shall force the outcome to needs-human-review regardless of the synthesized outcome.

**AIREV-08** When the pull request body or a commit message contains the explicit human review required marker, the command shall force the outcome to needs-human-review.

**AIREV-09** When the synthesis output does not contain an exact canonical outcome token, or the approval token is negated, the command shall treat the outcome as needs-human-review.

**AIREV-10** When a pull request changes a file whose exact basename is `SPEC.md`, including an addition, deletion, or either side of a rename, the command shall build an authenticated versioned review plan before accessing a reviewer credential or model.

**AIREV-11** When a changed SPEC lacks either exact reciprocal BDD feature evidence parsed from the bounded canonical template form or established labeled form in visible Markdown, the exact canonical `- No BDD change, with reason: ...` declaration, or an established deterministic schema/unit/integration test consequence in its canonical traceability section, links a feature that is not valid scenario-bearing Gherkin with runnable steps and at least one executable case for every Scenario or Scenario Outline, links a Scenario Outline without a nonempty structurally valid Examples body, exceeds the bounded review capacity for executable cases or inherited and scenario step instances, fails strict EARS validation over the same line-preserving visible-Markdown view used for requirement extraction, reuses a changed requirement identifier or normalized promise elsewhere in the HEAD SPEC corpus, claims normative ownership under a registered harness root, deletes the whole SPEC, requires a truncated or incomplete owner search, exceeds an input bound, or the SPEC reviewer is unavailable, ambiguous, or cannot authenticate its evidence to the reviewed revisions and paths, the command shall report needs-human-review and shall not approve the change.

**AIREV-12** When a changed SPEC plan is reviewable, the command shall classify every authenticated owner shard with one strict versioned JSON call per shard, at no more than four concurrent calls, and shall require one ordered result for every candidate; the owner index shall include every unchanged HEAD SPEC and, when more than one live SPEC changes, every changed contract as a peer candidate compared with every other changed contract but never itself, so only a complete all-`distinct` classification proves both unchanged-owner uniqueness and every unordered changed-contract pair distinct enough to proceed to one final SPEC contract review call, while `possible-owner`, `uncertain`, an incomplete response, or a provider failure shall require human review, and the final blocking verdict shall not be upgraded by five-dimension synthesis.

**AIREV-13** When a changed SPEC plan is reviewable, the command shall use `docs/spec-authoring.md` from the authenticated current protected-base revision as its sole substantive prompt policy and shall supply complete bounded current changed-SPEC contracts, an exact path and visible-contract-digest certification item with the complete stable-requirement identifier set for every added or modified contract, applicable parsed and bounded authenticated Gherkin evidence or declared deterministic/no-BDD consequence evidence, a path-sorted bounded owner index that projects every unchanged HEAD SPEC exactly once and every required changed peer through its ordinal, path, changed-peer marker, complete whitespace-normalized visible contract text, normalized requirement identifiers, exact feature pointers and authenticated signals, canonical SHA-256 index and shard digests, and the complete active-member applicability grid for every stable current promise; applicability construction shall append each contract's complete requirement-by-harness grid atomically, stop before the first contract that would exceed the authenticated limit, append neither that partial grid nor any later grid, and require human review; the strict final verdict shall accept approval only when every ordered contract certification is `complete`, shall route an observable prose promise outside the stable requirement set to `needs-conversion`, shall route uncertain completeness to human review, and shall require human review rather than an invented owner or confirmed defect when semantic evidence is empty, low-confidence, or incomplete.

**AIREV-14** When a modified SPEC deletes a stable requirement, the command shall include the deleted identifier and prior promise in authenticated review evidence and shall require one strict structured reviewer disposition for that deletion or report needs-human-review.

**AIREV-15** When the command invokes Git to inspect a pull request revision, the command shall enforce time and output limits, shall expose only an allowlisted non-credential environment to the subprocess, and shall read exact committed object bytes without applying revision-controlled export transformations.

**AIREV-16** When a pull request changes the canonical SPEC authoring entry point, template, write or audit workflow and reference files, active harness registry, trusted review workflow or ruleset, review implementation, deterministic Markdown or EARS parsers, Go build manifests, workspace manifests, or vendored dependencies used by that implementation, the command shall record a maintainer-review requirement rather than allow the revision to approve its own enforcement change, while it shall distinguish reviewer dependency inputs from SPEC contract changes and only the trusted workflow may convert the exact AIREV-24 dependency-automation case to neutral.

**AIREV-17** When a changed SPEC or protected enforcement owner is reviewed from a head that does not contain the current protected base, the command shall require the branch to be updated and shall not bind policy or corpus evidence to the stale merge base.

**AIREV-18** When the trusted source workflow runs, it shall attempt to publish a uniquely named `SPEC Contract Review` semantic verdict for the reviewed pull-request head and shall fail its distinct native `AI review orchestration` job when creation or publication is unavailable; without a reviewer credential, plans with no changed SPEC, protected owner, binary or gitlink evidence, or deterministic AIREV-07 or AIREV-08 escalation, plus the authenticated AIREV-24 dependency-automation case, may publish neutral, and a same-repository non-override plan whose gate exits with the explicit AIREV-26 cannot-run status shall publish a neutral-with-warning verdict that claims no approval and records that human review is recommended, while every other failure shall remain fail closed pending a revision-bound maintainer override. Neither source result is yet a provider-required merge gate.

**AIREV-19** When a changed SPEC plan is reviewable, the command shall parse the exact package-level `activeHarnesses` string-slice literal from `agm/internal/agent/harnesses.go` at the authenticated protected-base revision and shall require one final `supported`, `adapted`, `unsupported`, or `not-applicable` reviewer disposition for every active member and every stable current promise in each added or modified SPEC, while approval shall require an explicit semantic-reviewer certification that no observable mixed-format prose bypasses that grid.

**AIREV-20** When a pull request adds or modifies a normative `SPEC.md` beneath a registered dotted configuration root, a plugin registration root, an explicit `harness/` or `harnesses/` grouping, or a top-level authenticated active-harness alias, the command shall reject the local owner without relying on peer similarity. An `internal/` or `cmd/` ancestor shall not conceal an intrinsic registration root; native variation shall remain an applicability-scoped requirement in one shared product or domain owner, while legitimate logical owners under `internal/` and `cmd/` without such a registration root shall remain eligible for semantic review.

**AIREV-21** When a pull request adds, modifies, deletes, or renames a file whose exact basename is `SPEC.owner`, the command shall record the authenticated changed path and status as a normative ownership-edge change and require a revision-bound maintainer review rather than allow the pointer to approve its own reassignment.

**AIREV-22** When a changed SPEC plan is reviewable, the command shall prove before credential access that every owner-classification prompt, the canonical minimum-field-value and maximum-field-value complete shard verdicts, and both the minimum-field-value and worst-JSON-expanding maximum-field-value complete final versioned verdict fit their accepted wire bounds, including every parser-permitted finding; the maximum unescaped final verdict shall also fit a separate conservative visible-output byte ceiling no larger than half the final SPEC `max_tokens` budget so adaptive thinking retains a proven reserve, and the command shall reject any model response whose stop reason is not `end_turn`.

**AIREV-23** When the review runs, the workflow and command shall bound every setup action, pre-publication network or Git operation, model request, owner-search wave, final SPEC review, concurrent dimension batch, synthesis stage, and total command invocation with explicit deadlines whose worst-case execution fits the earlier of a command-local budget and a trusted absolute workflow cutoff exported by the first workflow step; that cutoff shall leave a bounded publication reserve below the trusted job timeout, and a missing, malformed, expired, or reached deadline or provider failure shall remain fail closed without an unbounded retry or partial approval.

**AIREV-24** When the authenticated Git plan proves that a current-base pull request modifies only `go.mod` or `go.mod` and `go.sum`, both module files parse, at least one existing required-module version changes, only requirements marked indirect may be added or removed, retained requirements may be reclassified between direct and indirect, membership of every policy-annotated require block and all other requirement and require-block annotations remain unchanged, module identity, Go and toolchain directives, all non-require syntax, and all exclude and retract directives remain unchanged, no replace directive exists, and no SPEC, other protected owner, binary, gitlink, or deterministic escalation evidence exists, the plan shall mark a dependency-automation candidate. The trusted workflow shall publish a non-blocking neutral verdict without a model call only when GitHub's trusted APIs also identify the immutable Dependabot app bot and the same numeric repository, bind the exact current head to the canonical Dependabot and GitHub commit identities with exactly the current protected base as its parent, and show either that the original head has not been replaced or that an exact-current-head force push was made by Dependabot or by a principal with existing maintain or admin authority. After the reviewed revision has been resolved and its pending verdict created, any missing, malformed, additional, fork-controlled, stale-head, unauthenticated-actor, API-failure, or otherwise uncertain candidate or provenance evidence shall preserve the ordinary fail-closed review path without preventing verdict publication; failures that prevent revision resolution or pending-verdict creation remain governed by AIREV-18 and AIREV-25.

**AIREV-25** When the trusted workflow starts from a pull-request event, it shall resolve and validate the current pull-request and protected-main snapshots from GitHub's trusted API before choosing the reviewed revision, bind the exact head, live protected-base, author, repository, body, labels, fork status, policy plan, identity checks, and published verdict to those snapshots, and fetch the resolved head by immutable object ID rather than a mutable pull-request ref. A stale event or embedded base, concurrent ref movement, closed or wrong-base pull request, malformed or oversized snapshot, missing object, or API failure shall not pair evidence from different revisions or reuse stale payload fields as current evidence. A human override shall activate only during an `ai-review:override` labeled event whose event head equals the resolved head and whose event actor GitHub currently verifies has maintain or admin permission; synchronize and every other event shall ignore retained labels and bot-authored markers, while a synchronize run shall not remove the cosmetic label.

**AIREV-26** When a same-repository, non-override run is blocked solely because no reviewer credential is configured — an otherwise-reviewable changed SPEC, a deterministic REVIEW.md §3 escalation whose accompanying model review cannot run, or the credential preflight itself — the command shall exit with the distinct blocking cannot-run status 78 only after its needs-human-review evidence comment has been recorded on the pull request or no pull-request context exists to post to, and only the trusted workflow shall translate exactly that status into a neutral-with-warning published verdict; a failed evidence comment, an oversize or uncomputable diff, a conclusive SPEC-governance human-review verdict that requires no model (ownership edge, reviewer dependency, traceability, or stale-base evidence), a fork pull request, a plan construction error, an expired deadline, and an override audit-comment failure shall each keep an undifferentiated non-zero status and remain fail closed.

## Enforcement wiring

- `.github/workflows/review.yml` invokes this command from trusted
  `pull_request_target` revisions and publishes its result on the reviewed
  head under the unique `SPEC Contract Review` check context. The workflow's
  native `AI review orchestration` job has a distinct name and fails if
  creation or publication of that semantic check fails.
- The command classifies only the Git shape of a possible
  dependency-version-led module update. The trusted workflow separately
  resolves the current PR revision from GitHub's API and authenticates
  Dependabot's immutable bot identity, numeric head-repository identity, exact
  current commit and parent, and current-head timeline
  provenance before it may skip the model and publish neutral; neither branch
  names nor labels authorize it. The maintainer timeline arm grants no new
  authority: only `maintain` or `admin`, which can already apply the audited
  revision override, may attest a Dependabot rebase that GitHub records under
  the requesting maintainer rather than the bot.
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
