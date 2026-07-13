# Agent Log Rotation Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/log-rotate` bounds append-only AGM, VROOM, and dear-agent state logs by
size, age, count, and optional compression.

## EARS Requirements

**LRC-01** When no targets are supplied, the command shall sweep the default VROOM and dear-agent state log patterns.

**LRC-02** When explicit target patterns are supplied, the command shall expand home references, globs, regular files, deduplication, and lexical order deterministically.

**LRC-03** When a live log exceeds the configured size, the command shall rotate it through the shared log-rotation implementation.

**LRC-04** When rotated siblings exceed age or count limits, the command shall prune them according to the configured policy.

**LRC-05** When dry-run mode is selected, the command shall report intended rotation and pruning without changing files.

**LRC-06** When no files match scheduled patterns, the command shall report no matches and exit successfully.

**LRC-07** When one target fails, the command shall continue processing other files and directories and shall return the runtime-failure exit code.

**LRC-08** When size, duration, or glob syntax is invalid, the command shall return the usage exit code without rotating files.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_safety_command_guardrails.feature`
- Package tests: `cmd/log-rotate/*_test.go`
