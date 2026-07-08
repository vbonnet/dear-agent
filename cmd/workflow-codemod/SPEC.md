# Workflow Codemod Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/workflow-codemod` performs source transformations for workflow YAML. It
upgrades legacy workflow files and converts Wayfinder session YAML into
workflow files.

## Requirements

**WORKFLOW-CODEMOD-01** When no subcommand is provided, the system shall print usage and exit with code 2.

**WORKFLOW-CODEMOD-02** When `upgrade` is provided without `--write`, the system shall run in dry-run mode and leave the workflow file unchanged.

**WORKFLOW-CODEMOD-03** When `upgrade --write` is provided, the system shall overwrite the workflow file while preserving the original file permissions.

**WORKFLOW-CODEMOD-04** When `upgrade --add-budget` is provided, the system shall add missing budget fields supported by the workflow upgrader.

**WORKFLOW-CODEMOD-05** When `upgrade --drop-model` is provided, the system shall remove deprecated hard-coded model fields supported by the workflow upgrader.

**WORKFLOW-CODEMOD-06** When `from-wayfinder --out <file>` is provided, the system shall write a workflow YAML file converted from the Wayfinder session YAML.

**WORKFLOW-CODEMOD-07** When a transformation fails, the system shall exit with code 1.

**WORKFLOW-CODEMOD-08** When command usage is invalid, the system shall exit with code 2.

## BDD Traceability

- `agm/test/bdd/features/workflow_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
