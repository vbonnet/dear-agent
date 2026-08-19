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

**SAFE-MERGE-07** When every other gate has passed and the PR head is behind its base branch, the system shall advance the head to the base tip and block the merge attempt, so that the merge is only ever executed against checks that ran on the base the PR will land on.

**SAFE-MERGE-08** When the PR head is behind its base branch and `--dry-run` is set, the system shall report the staleness without pushing to the branch.

**SAFE-MERGE-09** When the PR conflicts with its base branch, the system shall reject the merge without modifying the branch.

## BDD Traceability

- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- Unit package: `cmd/safe-merge`
