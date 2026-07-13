# Pre-push Act Validator Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/hooks-bin/cmd/prepush-act-validator` maps changed repository surfaces to
local GitHub Actions jobs and runs those jobs with `act` before a push.

## EARS Requirements

**EPAV-01** When pre-push input is empty, the command shall exit successfully without running validation.

**EPAV-02** When `act`, the repository root, or `.actrc` is unavailable, the command shall explain the skipped validation and shall not block the push.

**EPAV-03** When changed files affect known repository surfaces, the command shall select the corresponding test and lint jobs without duplicates.

**EPAV-04** When changed files cannot be determined, the command shall conservatively select the lint job.

**EPAV-05** When jobs are selected, the command shall return them in deterministic lexical order.

**EPAV-06** When any selected job fails, the command shall run the remaining jobs, summarize the failures, and exit unsuccessfully.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks-bin/cmd/prepush-act-validator/*_test.go`
