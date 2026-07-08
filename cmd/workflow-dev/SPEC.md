# Workflow Dev Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/workflow-dev` provides an interactive development shell for workflow YAML.
It starts a fixture-backed session, optionally hot-reloads workflow inputs, and
delegates REPL verbs to `pkg/workflow/dev`.

## Requirements

**WORKFLOW-DEV-01** When no workflow path is provided, the system shall print usage and exit with code 2.

**WORKFLOW-DEV-02** When a workflow path is provided, the system shall create a development session for that workflow and its fixtures.

**WORKFLOW-DEV-03** When `--fixtures` is omitted, the system shall use the conventional fixtures path derived from the workflow file.

**WORKFLOW-DEV-04** When `--watch` is provided, the system shall hot-reload the workflow and fixtures files after the debounce interval.

**WORKFLOW-DEV-05** When reload fails during watch mode, the system shall print the reload failure without terminating the REPL.

**WORKFLOW-DEV-06** When SIGINT or SIGTERM is received, the system shall cancel the REPL context for clean shutdown.

**WORKFLOW-DEV-07** When startup or REPL execution fails, the system shall exit with code 1.

**WORKFLOW-DEV-08** When the REPL exits cleanly, the system shall exit with code 0.

## BDD Traceability

- `agm/test/bdd/features/workflow_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
