# Hook Git State Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/hooks-bin/internal/git` extracts branch, working-directory, and changed
file state for advisory hook output.

## EARS Requirements

**EHGS-01** When git is unavailable or the current directory is not a repository, the system shall return a safe default state and a contextual error.

**EHGS-02** When individual git metadata calls fail, the system shall preserve other available state and use documented fallback values.

**EHGS-03** When porcelain status reports staged or unstaged changes, the system shall classify modified, created, deleted, and renamed files without duplicate entries.

**EHGS-04** When a branch abbreviation is requested, the system shall retain at most the final five characters.

**EHGS-05** When a file list exceeds its display limit, the system shall emit only the bounded entries and report the omitted count.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks-bin/internal/git/*_test.go`
