# Benchmark Baseline Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/benchmark-baseline` captures structural, operational, and trace metrics as
a reproducible baseline for later regression comparisons.

## EARS Requirements

**BBC-01** When baseline collection starts, the command shall record repository identity, timestamp, environment, and selected suite metadata.

**BBC-02** When structural benchmarks run, the command shall execute the shared benchmark suite under a bounded context.

**BBC-03** When operational data is available, the command shall preserve session, worktree, and repository health counts without harness-specific field loss.

**BBC-04** When trace input is configured, the command shall aggregate supported span metrics and report malformed trace data as an error.

**BBC-05** When output is a file, the command shall create parent directories and write valid formatted JSON.

**BBC-06** When stdout mode is selected, the command shall emit the same baseline schema without writing a file.

**BBC-07** When collection succeeds, the command shall print a concise summary derived from the recorded baseline.

**BBC-08** When a required collection stage fails, the command shall return a runtime failure without writing a fabricated successful baseline.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_operations_command_guardrails.feature`
- Package tests: `cmd/benchmark-baseline/*_test.go`
