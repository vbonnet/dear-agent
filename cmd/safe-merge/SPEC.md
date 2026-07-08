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

## BDD Traceability

- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- Unit package: `cmd/safe-merge`
