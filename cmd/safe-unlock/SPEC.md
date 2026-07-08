# safe-unlock Command Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `cmd/safe-unlock`.

## Overview

`safe-unlock` is the sanctioned CLI for clearing stale git lock files from a
repository or worktree. It delegates lock discovery and removal policy to
`internal/safeunlock` and uses exit codes to distinguish cleaned/no-op, active
locks, and command errors.

## EARS Requirements

**SAFE-UNLOCK-01** When help is requested, the system shall print usage and exit zero.

**SAFE-UNLOCK-02** When too many positional arguments are provided, the system shall reject the command.

**SAFE-UNLOCK-03** When the target is not a repository, the system shall exit with command-error status.

**SAFE-UNLOCK-04** When only stale locks are cleaned or no locks exist, the system shall exit zero.

**SAFE-UNLOCK-05** When dry run is enabled, the system shall leave stale locks in place and exit zero.

**SAFE-UNLOCK-06** When active locks remain, the system shall exit with active-lock status.

**SAFE-UNLOCK-07** When include-worktrees is set, the system shall ask the cleaner to scan linked worktree locks.

## BDD Traceability

- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- Unit package: `cmd/safe-unlock`
