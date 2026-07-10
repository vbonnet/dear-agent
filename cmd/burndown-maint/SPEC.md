# Burndown Maintenance Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/burndown-maint` maintains a bounded number of detached AGM burndown
workers without embedding a harness or model-family restriction.

## EARS Requirements

**BMC-01** When a maintenance tick starts, the command shall count non-archived sessions whose names use the burndown prefix.

**BMC-02** When the active worker count meets or exceeds the target, the command shall not spawn another worker.

**BMC-03** When the active count is below target, the command shall spawn at most one detached worker during that tick.

**BMC-04** When a worker is spawned, the command shall preserve the selected AGM harness, model, and workspace as shell-free arguments.

**BMC-05** When dry-run mode is enabled, the command shall report the selected harness and model without spawning a session.

**BMC-06** When session listing or spawning fails, the command shall return a contextual failure rather than reporting target compliance.

**BMC-07** When JSON output is selected, the command shall report active, target, spawned, spawned-session, and dry-run fields.

**BMC-08** When command-line arguments are invalid, the command shall use the documented usage exit code distinct from runtime failure.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_maintenance_command_guardrails.feature`
- Package tests: `cmd/burndown-maint/*_test.go`
