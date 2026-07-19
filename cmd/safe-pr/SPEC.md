# safe-pr Command Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `cmd/safe-pr`.

## Overview

`safe-pr` is the sanctioned CLI for audited PR creation and closure. It wraps
GitHub CLI operations with Wayfinder attribution, remote-url preflight, denied
interactive modes, timeout control, and optional CI verification after PR
creation.

## EARS Requirements

**SAFE-PR-01** When help is requested, the system shall print usage without treating it as an error.

**SAFE-PR-02** When no verb is provided, the system shall reject the command.

**SAFE-PR-03** When a PR command runs from a git repository, the system shall require the origin remote URL to target the sanctioned GitHub organization before invoking GitHub CLI.

**SAFE-PR-04** When `create` is requested without `--skip-preflight`, the system shall run `make preflight-full` before invoking GitHub CLI.

**SAFE-PR-05** When `--skip-preflight` is provided, the system shall not run `make preflight-full` while retaining the Wayfinder and remote-url guards.

**SAFE-PR-06** When CI verification is requested after PR creation, the system shall check that the created PR has check runs.

**SAFE-PR-07** When a PR URL is returned, the system shall parse owner, repository, and PR number from GitHub URLs.

**SAFE-PR-08** When the canonical Wayfinder V2 writer creates a `planning` session with `project_name`, the system shall accept that trace without requiring a legacy `session_id`.

**SAFE-PR-09** When safe-pr control flow is tested, the system shall replace the GitHub mutation boundary so unit tests cannot create real pull requests.

**SAFE-PR-10** When safe-pr runs the repository full preflight, the system shall allow at least twenty minutes before terminating the gate.

**SAFE-PR-11** When `create --draft` succeeds, the system shall leave the pull request unarmed for a human to advance.

**SAFE-PR-12** When non-draft PR creation succeeds, the system shall attempt to arm squash auto-merge.

## BDD Traceability

- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- Unit package: `cmd/safe-pr`
