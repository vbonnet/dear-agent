# Missing PR Checks Detection Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/nochecks` detects open pull requests whose head commit has no
required check runs and can retrigger CI through a safe empty commit.

## Requirements

**NCK-01** When a pull request has one or more required check runs, the system shall not classify it as missing checks.

**NCK-02** When a pull request head has zero check runs for all configured required checks, the system shall classify it as needing a retrigger.

**NCK-03** When check-run state cannot be read, the system shall report the read error separately instead of classifying the pull request from incomplete evidence.

**NCK-04** When retriggering CI, the system shall target the pull request branch and surface command failures.

**NCK-05** When displaying a commit identifier, the system shall return a bounded short SHA without failing on short input.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_supervision_recovery_guardrails.feature`
- Package tests: `agm/internal/nochecks/*_test.go`
