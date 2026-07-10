# Burndown Worker Launch Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`internal/burndownmaint` builds the harness-neutral AGM launch contract used by
host burndown maintenance.

## EARS Requirements

**BWL-01** When worker launch arguments are built, the system shall invoke `session new` with a deterministic session name and detached mode.

**BWL-02** When a harness is selected, the system shall preserve its canonical AGM identifier in a `--harness` argument.

**BWL-03** When a model is selected, the system shall preserve its model or family alias in a `--model` argument for shared AGM validation.

**BWL-04** When a workspace is selected, the system shall preserve it in a `--workspace` argument.

**BWL-05** When route values are empty, the system shall omit their flags rather than emitting empty arguments.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_maintenance_command_guardrails.feature`
- Package tests: `internal/burndownmaint/*_test.go`
