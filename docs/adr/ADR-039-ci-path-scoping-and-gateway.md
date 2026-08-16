# ADR-039: Job-level path scoping behind an aggregate CI gateway

Status: Proposed (2026-08-16)

## Context

Every pull request ran the full unit matrix on two runners, a CodeQL build, a
whole-tree Trivy scan, and two Go contract suites, regardless of what it
touched. A README fix paid the same ~30 minutes as a change to the scheduler.
Worse, `Vulnerability Scan` is whole-tree and gates on a live advisory feed, so
a CVE published against an untouched dependency turns every open PR red at once
and nothing merges until someone bumps the dep — pre-existing debt blocking
unrelated work.

The obvious fix, `on.pull_request.paths`, is a trap here. A workflow dropped by
a path filter never creates a check run, so a required context sits at
"Expected — Waiting for status to be reported" and the PR is unmergeable
forever. A job skipped by an `if:` condition does report, with conclusion
`skipped`, which GitHub accepts as satisfying a required check.

That asymmetry is also the hazard. Because `skipped` satisfies a required
check, a filter that wrongly excludes a relevant PR does not fail — it merges
green with the gate silently disabled.

## Decision

Path selection is expressed as job-level `if:` conditions fed by one reusable
workflow, `.github/workflows/changed-paths.yml`, which owns the path taxonomy
for the repo. Workflow-level `paths:` is prohibited for any job that produces a
required status context.

`changed-paths.yml` fails safe: every output defaults to `true`. A missing base
ref, a git failure, a non-`pull_request` event, or a change to a global input
(`go.mod`, `go.sum`, `Makefile`, `.github/**`, lint config) forces every
consumer on. This mirrors the forced-full fallback `cmd/test-affected` already
uses (ADR-028). Under-running is a hole in the gate; over-running costs runner
minutes.

Scoped jobs report through an aggregate `CI Gateway` job with `if: always()`,
which fails if any upstream job failed or was cancelled, **and** re-derives
which jobs should have run and fails if any of those reports `skipped`. The
second assertion is what keeps a mis-scoped filter from quietly disabling a
gate. `if: always()` is mandatory: under the default `success()` a single
skipped upstream would skip the gateway, and a gateway that skips is a gateway
that passes.

Enforcement is scoped to what a change can actually break, not to the whole
tree. `Vulnerability Scan` blocks a PR only when it touches a dependency
manifest or lockfile; the whole-tree blocking scan moves to push-to-main,
release, and the weekly schedule.

## Alternatives

Workflow-level `paths:` deadlocks required checks. GitHub's documented
dual-workflow workaround — a no-op twin with an inverse `paths-ignore` and an
identical job name — doubles the workflow count and drifts. Leaving everything
unconditional is what we had: correct, and expensive enough that people reach
for admin bypass, which is a worse failure than a slow PR.
`dorny/paths-filter` and `tj-actions/changed-files` are the common answers, but
this repo requires third-party actions to be SHA-pinned and already lists
`tj-actions/changed-files` as known-compromised; `git diff --name-only` against
the merge base carries no supply chain.

## Consequences

Selection correctness now depends on the path taxonomy in one file, which is
where drift is auditable rather than spread across workflows. Skipped jobs show
as grey in the PR checks list — that is the intended shape, not a missing run.

Manifest-scoping the vulnerability gate is narrower than true baseline
diffing: a PR that bumps one dependency is still blocked by an unrelated
pre-existing CRITICAL, because Trivy has no `--baseline-commit`. The
post-merge scans and the main-health watchdog carry that residual risk.

Moving whole-tree enforcement off the PR path means some debt is found after
merge instead of before. That is the deliberate trade, and it is only safe
because the post-merge side is watched: `main-health-watchdog.yml` files a DEAR
retro when main goes red and asks whether filter selection let it through.
