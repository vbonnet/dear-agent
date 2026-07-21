# Safe Unlock Specification

<!-- Last audited at: 2026-07-20 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/safeunlock`.

## Overview

`internal/safeunlock` is the general stale git-lock cleaner for worktrees and
arbitrary repositories. It scans known git lock locations, classifies each lock,
removes only locks that are old enough and unheld, and records decisions in a
JSONL audit log.

## EARS Requirements

**SAFEUNLOCK-01** When the target path is not a git repository, the system shall return an error before scanning locks.

**SAFEUNLOCK-02** When no lock files are present, the system shall return an empty result without error.

**SAFEUNLOCK-03** When a lock file is younger than the minimum age, the system shall mark it active and leave it in place.

**SAFEUNLOCK-04** When a process holds a lock file open, the system shall mark it active and leave it in place.

**SAFEUNLOCK-05** When dry run is enabled for a stale lock, the system shall report the stale lock without deleting it.

**SAFEUNLOCK-06** When a lock is stale and unheld, the system shall remove that lock file.

**SAFEUNLOCK-07** When include-worktrees is enabled, the system shall scan linked worktree lock locations.

**SAFEUNLOCK-08** When any locks are evaluated, the system shall append audit records for the decisions.

**SAFEUNLOCK-09** When one lock removal fails, the system shall continue evaluating remaining locks and return a joined error.

**SAFEUNLOCK-10** When a non-Git transaction guard is stored in a linked-worktree Git directory, the system shall exclude it from stale Git lock discovery regardless of its age or holder-probe availability.

## BDD Traceability

- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- Unit package: `internal/safeunlock`
