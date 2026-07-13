# Agent Log Rotation Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/logrotate`.

## Overview

`internal/logrotate` bounds append-only agent and session logs while preserving
recent diagnostics. Rotation policy is shared across harnesses and does not
interpret model-specific log content.

## EARS Requirements

**LOGROTATE-01** When a rotator is created with omitted limits, the system shall apply the repository default size, age, and count limits.

**LOGROTATE-02** When a live log is missing or does not exceed its size threshold, the system shall leave the filesystem unchanged.

**LOGROTATE-03** When an uncompressed live log exceeds its threshold, the system shall rotate it to a timestamped sibling and recreate the live path with its original permissions.

**LOGROTATE-04** When compression is enabled, the system shall preserve rotated content in gzip form and truncate the live file in place.

**LOGROTATE-05** When rotated logs exceed age or count limits, the system shall prune oldest files independently for each live-log base name.

**LOGROTATE-06** When dry-run mode is enabled, the system shall report planned rotation and pruning without changing files.

**LOGROTATE-07** When a directory contains unrelated files, the system shall exclude them from rotated-log pruning.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`

## Test Traceability

- Unit package: `internal/logrotate`
