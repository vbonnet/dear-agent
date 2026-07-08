# Workflow Lint Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/workflow-lint` validates workflow YAML files and surfaces migration
findings. It is quiet when files pass unless verbose output is requested.

## Requirements

**WORKFLOW-LINT-01** When no workflow files are provided, the system shall print usage and exit with code 2.

**WORKFLOW-LINT-02** When workflow files are provided, the system shall load each file and return one finding per load or validation problem.

**WORKFLOW-LINT-03** When `--check-deprecated-models` is provided, the system shall flag AI nodes whose `model` or `model_override` uses a deprecated model id.

**WORKFLOW-LINT-04** When `--deprecated-models` is provided, the system shall use that comma-separated model list instead of the built-in deprecated seed list.

**WORKFLOW-LINT-05** When an AI node declares `model` without `role`, the system shall emit a warning that role-based resolution is recommended.

**WORKFLOW-LINT-06** When a loop contains AI nodes, the system shall lint those nested AI nodes using `loop-id/node-id` references.

**WORKFLOW-LINT-07** When `--verbose` is provided and a file passes, the system shall print an `ok` line for that file.

**WORKFLOW-LINT-08** When at least one file has findings, the system shall exit with code 1.

**WORKFLOW-LINT-09** When every file passes, the system shall exit with code 0.

## BDD Traceability

- `agm/test/bdd/features/workflow_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
