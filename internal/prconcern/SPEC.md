# Mixed-Concern Pull Request Detection Specification

<!-- Last audited at: 2026-08-18 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/prconcern`.

## Overview

`internal/prconcern` detects a pull request that mixes a mechanical refactor
with net-new logic. It supplies the shape-based half of the deterministic
split-suggestion signal in `.github/workflows/pr-size-scope.yml`, which
otherwise reacts only to raw diff size and therefore misses a small PR that
renames files and adds a feature on top of the new names.

A diff is mixed only when a move-only record and substantial net-new source
logic both appear. Requiring both keeps an ordinary rename quiet, because the
call-site fix-ups a rename drags along cannot reach the new-logic threshold.

## EARS Requirements

**PRCONCERN-01** When a diff record is a rename or copy of a source file whose content did not change, the system shall classify that record as move-only.

**PRCONCERN-09** When a rename or copy record targets a non-source file, the system shall exclude it from the move-only classification.

**PRCONCERN-02** When a diff contains at least one move-only record and its added non-test source lines outside any rename record reach the configured threshold, the system shall report the diff as mixed-concern.

**PRCONCERN-03** When a diff contains no move-only record, or its added non-test source lines are below the configured threshold, the system shall report the diff as single-concern and shall emit no reason.

**PRCONCERN-04** When added lines belong to a test file, a testdata directory, or a non-source file type, the system shall exclude them from the net-new logic total.

**PRCONCERN-05** When a mixed-concern diff is reported, the system shall render a reason naming the move-only count, the net-new logic total, and a bounded preview of both path sets.

**PRCONCERN-06** When numstat output contains a rename record, the system shall pair its pre-image and post-image paths.

**PRCONCERN-07** When numstat output is malformed or a rename record is truncated, the system shall return a parse error rather than a partial classification.

**PRCONCERN-08** When a diff record reports binary content, the system shall exclude it from the net-new logic total.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`

## Test Traceability

- Unit package: `internal/prconcern`
- Command: `tools/pr-concern-lint`
- Consumer: `.github/workflows/pr-size-scope.yml`
