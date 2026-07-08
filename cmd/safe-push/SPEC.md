# safe-push Command Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `cmd/safe-push`.

## Overview

`safe-push` is the sanctioned command-line wrapper for `git push`. It delegates
push policy to `internal/safegit`, supports repository selection with `-C`, and
rejects malformed timeout arguments before invoking git.

## EARS Requirements

**SAFE-PUSH-01** When help is requested, the system shall print usage and return success.

**SAFE-PUSH-02** When `-C` is provided without a repository argument, the system shall reject the command.

**SAFE-PUSH-03** When `--timeout` is provided without a duration, the system shall reject the command.

**SAFE-PUSH-04** When `--timeout` is not a valid duration, the system shall reject the command.

**SAFE-PUSH-05** When arguments are valid, the system shall call the safe push policy with the selected repository, push arguments, and timeout.

## BDD Traceability

- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- Unit package: `cmd/safe-push`
