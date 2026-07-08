# Safe Source Recovery Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/safesrc`.

## Overview

`internal/safesrc` contains the narrow recovery operations allowed against
golden `~/src` repositories. Its lock-unlock path removes only a provably stale
`index.lock`, after validating repository boundaries, age, and live file
holders, so agents do not use raw filesystem deletion in source checkouts.

## EARS Requirements

**SAFESRC-01** When a repository path is outside the allowed source boundary, the system shall reject recovery.

**SAFESRC-02** When a repository does not contain a git directory, the system shall reject unlock.

**SAFESRC-03** When no `index.lock` exists, the system shall return success without removal.

**SAFESRC-04** When an `index.lock` is younger than the minimum age, the system shall refuse removal.

**SAFESRC-05** When a process holds `index.lock` open, the system shall refuse removal.

**SAFESRC-06** When dry run is enabled for a stale lock, the system shall report the removal without deleting the lock.

**SAFESRC-07** When a lock is stale and unheld, the system shall remove only the computed `index.lock` path.

**SAFESRC-08** When unlock runs, the system shall write a human-readable audit decision to the configured log.

## BDD Traceability

- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- Unit package: `internal/safesrc`
