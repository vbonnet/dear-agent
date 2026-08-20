# changed-paths Command Specification

<!-- Last audited at: 2026-08-20 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `cmd/changed-paths` — the repo's CI path taxonomy and the CI
Gateway skip audit. See [ADR-039](../../docs/adr/ADR-039-ci-path-scoping-and-gateway.md).

## Overview

`changed-paths` answers "what kind of change is this PR?" once, and every
workflow job consumes the answer as a job-level `if:` condition instead of
workflow-level `on.pull_request.paths`. It has two modes: change detection
(the default, invoked from the shared `changed-paths.yml` reusable workflow)
and the CI Gateway skip audit (`-audit`, invoked from `ci.yml`'s aggregate
job). Detection degrades to "everything is relevant" on any input it cannot
classify; the audit instead fails loudly, because a filter that wrongly
excludes a relevant job must turn CI red, not silently absent.

## EARS Requirements

**CHANGED-PATHS-01** When invoked without `-audit`, the system shall classify the diff between `BASE_SHA` and `HEAD_SHA` and emit one boolean per taxonomy key (`go`, `agm`, `engram`, `deps`, `docs`, `adr`, `global`) to `$GITHUB_OUTPUT`.

**CHANGED-PATHS-02** When the event is not `pull_request`, or `BASE_SHA`/`HEAD_SHA` is missing, or the diff cannot be computed, the system shall classify every key as true and shall emit a GitHub notice naming the reason.

**CHANGED-PATHS-03** When a changed path matches build metadata (`go.mod`, `go.sum`, `go.work`, `go.work.sum`, `Makefile`), `vendor/`, `.github/`, `.golangci.yml`, or `.dear-agent.yml`, the system shall classify every key as true, since such a change can invalidate what the selection itself means.

**CHANGED-PATHS-04** When a changed path's extension is not in the documentation extension set, the system shall classify `go` as true.

**CHANGED-PATHS-05** When a changed path has a documentation extension but sits under a `//go:embed`-discovered root or a hash/skill-verified root (`agm/agm-plugin/commands`, `skills`), the system shall still classify `go` as true.

**CHANGED-PATHS-06** When `$GITHUB_OUTPUT` is unset, the system shall skip writing outputs without error; when it is set to a non-absolute or non-clean path, the system shall return an error.

**CHANGED-PATHS-07** When invoked with `-audit`, the system shall parse `NEEDS_JSON` into per-job results and shall fail if any upstream job's result is `failure` or `cancelled`.

**CHANGED-PATHS-08** When the `changes` detector job's result is not `success`, the audit shall treat every taxonomy key as true for its own re-derived expectations, since a detector that did not succeed publishes no outputs.

**CHANGED-PATHS-09** When a job the audit expected to run reports `skipped` or is absent from `NEEDS_JSON`, the system shall record a violation naming that job and shall fail the audit.

**CHANGED-PATHS-10** When the audit completes with no failures and no violations, the system shall exit zero and report that every job either ran or was legitimately out of scope.

## BDD Traceability

- Feature: `agm/test/bdd/features/workflow_tooling_guardrails.feature`
  covers SPEC co-location for this command alongside the repo's other
  workflow-support tooling.
- `cmd/changed-paths/workflow_contract_test.go` asserts the four ADR-039
  rules directly against `.github/rulesets/main.json` and the workflow
  files, independent of this package's own unit tests.
