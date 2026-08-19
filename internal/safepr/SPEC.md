# Safe PR Specification

<!-- Last audited at: 2026-07-20 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/safepr`.

## Overview

`internal/safepr` enforces the audited PR creation and closure path. It requires
an active Wayfinder session, rejects interactive or unstampable `gh pr` modes,
and stamps PR bodies or close comments with Wayfinder attribution so CI/review
cost and intent remain traceable.

## EARS Requirements

**SAFEPR-01** When no Wayfinder directory is provided by flag or environment, the system shall reject PR operations with escalation guidance.

**SAFEPR-02** When `WAYFINDER-STATUS.md` is missing, malformed, or incomplete under the complete canonical schema, the system shall reject PR operations.

**SAFEPR-03** When canonical Wayfinder V2 status is `planning` or `in-progress`, the system shall accept the project name as active PR attribution.

**SAFEPR-04** When a PR verb is not `create` or `close`, the system shall reject the request.

**SAFEPR-05** When `safe-pr create` lacks an explicit title, the system shall reject the request.

**SAFEPR-06** When `safe-pr create` uses interactive or unstamped body flags, the system shall reject the request.

**SAFEPR-07** When `safe-pr close` lacks an explicit comment, the system shall reject the request.

**SAFEPR-08** When rendering attribution, the system shall include the Wayfinder session and project.

**SAFEPR-09** When a Wayfinder bead is present, the system shall include a closing bead reference in the created PR body.

**SAFEPR-10** When Wayfinder status is `blocked`, `completed`, or `abandoned`, the system shall reject PR operations as inactive and direct the operator to `wayfinder session start <project-name>`.

**SAFEPR-11** When any supported harness or model family creates canonical Wayfinder V2 status, the system shall apply the same provider-neutral attribution policy.

**SAFEPR-12** When safe-pr creates a pull request from a linked worktree, the system shall hold a worktree lock across the complete preflight and GitHub mutation transaction.

**SAFEPR-13** When a linked worktree is already locked, the system shall preserve the existing lock and its exact reason after a successful or failed transaction.

**SAFEPR-14** When safe-pr acquires a worktree lock, the system shall release only that owned lock after a successful or failed transaction.

**SAFEPR-15** When safe-pr create runs from a primary checkout that cannot be worktree-locked, the system shall reject the operation before preflight or GitHub mutation.

**SAFEPR-16** When safe-pr invokes git to manage worktree protection, the system shall use a bounded context, isolated process group, group-wide cancellation, and bounded pipe-drain delay.

**SAFEPR-17** When safe-pr holds the per-worktree operating-system transaction lock and finds valid safe-pr ownership metadata left in Git, the system shall reclaim that provably stale Git lock before acquiring protection for the new transaction.

**SAFEPR-18** When another safe-pr process holds the per-worktree operating-system transaction lock, the system shall reject the overlapping transaction before it can inspect, reclaim, or replace the active owner's Git lock.

**SAFEPR-19** When Git returns linked-worktree or lock metadata with LF or CRLF line endings, the system shall parse the same canonical paths, lock state, and ownership reason.

**SAFEPR-20** When safe-pr releases an owned Git worktree lock, the system shall perform the ownership inspection and unlock while still holding the per-worktree operating-system transaction lock.

**SAFEPR-21** When safe-pr launches preflight or GitHub child commands, the system shall make those commands inherit the operating-system transaction lock so killing the parent cannot expose the worktree or mutation boundary while a child remains active.

**SAFEPR-22** When safe-pr persists its operating-system transaction guard in a linked-worktree Git directory, the system shall use a non-`.lock` name that stale Git-lock cleanup does not collect.

**SAFEPR-23** When safe-pr invokes Git to inspect, acquire, reclaim, or release worktree protection inside a transaction, the system shall make the Git helper inherit the operating-system transaction lock so killing the parent cannot expose an incomplete lock-management operation to a replacement owner.

**SAFEPR-24** When any pass-through GitHub argument contains a Claude Code session identifier or session permalink, the system shall refuse the mutation and shall report each matched reference with its line number.

## BDD Traceability

- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- Unit package: `internal/safepr`
