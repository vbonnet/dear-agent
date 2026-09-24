# safe-merge Command Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `cmd/safe-merge`.

## Overview

`safe-merge` is the sanctioned CLI for squash-merging PRs after CI, review,
soak, and optional repository policy gates pass. It exposes dry-run and watch
modes, loads repository identity from flags or environment, and keeps
break-glass merge behind an explicit TTY-only subcommand.

## EARS Requirements

**SAFE-MERGE-01** When `--pr` is missing, the system shall reject the merge command.

**SAFE-MERGE-02** When `--repo` is missing and no repository environment fallback exists, the system shall reject the merge command.

**SAFE-MERGE-03** When `--skip-review-check` is absent, the system shall leave the review-thread gate enabled.

**SAFE-MERGE-04** When `--skip-review-check` is present, the system shall pass the audited review-skip request to the merge policy.

**SAFE-MERGE-05** When break-glass mode is requested without a TTY, the system shall reject the request.

**SAFE-MERGE-06** When break-glass mode is requested without a PR argument, the system shall reject the request.

**SAFE-MERGE-07** When every other gate has passed, the system shall report base freshness only after proving that the gated PR head contains the then-live target base tip.

**SAFE-MERGE-08** When the PR head does not contain the live base tip and `--dry-run` is set, the system shall report the staleness without pushing to the branch.

**SAFE-MERGE-09** When the PR conflicts with its base branch, the system shall reject the merge without modifying the branch.

**SAFE-MERGE-10** When the target base state cannot be established or changes across the client-side freshness proof, the system shall reject the merge attempt.

**SAFE-MERGE-11** When the PR head does not contain the live base tip and the provider reports `BEHIND`, `CLEAN`, or `UNSTABLE`, the system shall request a head-anchored branch update and block until a later attempt re-runs every gate against the updated head.

## BDD Traceability

- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- Unit package: `cmd/safe-merge`
