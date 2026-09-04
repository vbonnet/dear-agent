# Missing PR Checks Detection Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/nochecks` detects open pull requests whose head commit has no
required check runs and can retrigger CI through a safe empty commit. It uses
SafeGit's complete layered branch-policy owner and consumes every check-run
page before classification.

## Requirements

**NCK-01** When a pull request has one or more required check runs, the system shall not classify it as missing checks.

**NCK-02** When a pull request head has zero check runs for all configured required checks, the system shall classify it as needing a retrigger.

**NCK-03** When check-run state cannot be read, the system shall report the read error separately instead of classifying the pull request from incomplete evidence.

**NCK-04** When retriggering CI, the system shall target the pull request branch and surface command failures.

**NCK-05** When displaying a commit identifier, the system shall return a bounded short SHA without failing on short input.

**NCK-06** When effective required-check policy cannot be completely discovered or represented by check-run name, the system shall return a scan error before classifying or retriggering any pull request.

**NCK-07** When effective required-check policy is authoritatively empty, the system shall preserve the any-run fallback without conflating that state with a provider error.

**NCK-08** When reading check runs for a pull request head, the system shall treat the result as complete only after every provider page succeeds.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_supervision_recovery_guardrails.feature`
- Package tests: `agm/internal/nochecks/*_test.go`
