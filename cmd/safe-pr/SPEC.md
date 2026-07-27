# safe-pr Command Specification

<!-- Last audited at: 2026-07-20 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `cmd/safe-pr`.

## Overview

`safe-pr` is the sanctioned CLI for audited PR creation, closure, and reopening. It wraps
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

**SAFE-PR-08** When the canonical Wayfinder writer creates a schema 2.0 `planning` session with `project_name`, the system shall accept that trace as the sole attribution identity.

**SAFE-PR-09** When safe-pr control flow is tested, the system shall replace the GitHub mutation boundary so unit tests cannot create real pull requests.

**SAFE-PR-10** When safe-pr runs the repository full preflight, the system shall allow at least forty-five minutes before terminating the gate.

**SAFE-PR-11** When `create --draft` succeeds, the system shall leave the pull request unarmed for a human to advance.

**SAFE-PR-12** When non-draft PR creation succeeds, the system shall attempt to arm squash auto-merge.

**SAFE-PR-13** When draft detection scans GitHub CLI arguments, the system shall treat `-R` and every other value-taking shorthand as consuming its repository value rather than interpreting that value as Boolean flags.

**SAFE-PR-14** When pull request creation runs, the system shall protect the linked worktree across both the full preflight and GitHub mutation boundaries.

**SAFE-PR-15** When pull request creation manages safe-pr lock ownership, the system shall hold a per-worktree operating-system serialization lock across the transaction so process termination releases liveness ownership without relying on reusable numeric process IDs.

**SAFE-PR-16** When pull request creation launches preflight, GitHub mutation, auto-merge, or CI-discovery commands, the system shall attach the active worktree transaction guard so child process lifetime remains part of the protected transaction after abrupt parent termination. For preflight, the guard runner shall require its argument to match its current working directory and shall close the inherited descriptor before it launches nested build or test processes, so detached test helpers cannot retain the transaction after the guard exits.

**SAFE-PR-17** When a protected safe-pr child command is canceled or times out, the system shall terminate its isolated process group and bound pipe draining before the parent releases Git worktree ownership.

**SAFE-PR-18** When an attributed safe-pr transaction finishes, the system shall append exactly one audit record whose exit code and error reflect the final acquisition, GitHub mutation, and worktree release outcome.

**SAFE-PR-19** When `reopen` is requested, the system shall require an explicit reason and stamp the active Wayfinder trace into the reopening comment before invoking GitHub CLI.

## BDD Traceability

- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- Unit package: `cmd/safe-pr`
