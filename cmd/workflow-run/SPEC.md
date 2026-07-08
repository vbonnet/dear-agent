# Workflow Run Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/workflow-run` loads a workflow YAML file, resolves role-based AI execution,
optionally persists run state, and executes the workflow from the CLI.

## Requirements

**WORKFLOW-RUN-01** When `-file` is omitted, the system shall print usage and exit with code 2.

**WORKFLOW-RUN-02** When `-dry-run` is provided, the system shall load and validate the workflow without executing nodes or opening persistence.

**WORKFLOW-RUN-03** When `-input name=value` is provided, the system shall pass that input name and value into the workflow run.

**WORKFLOW-RUN-04** When an input flag does not contain `=`, the system shall reject it as usage error and exit with code 2.

**WORKFLOW-RUN-05** When `-db` is an empty string, the system shall disable SQLite persistence for the run.

**WORKFLOW-RUN-06** When `-db` is non-empty, the system shall persist run state in the configured SQLite database.

**WORKFLOW-RUN-07** When `-roles` is provided, the system shall use that role catalog for role-based AI executor selection.

**WORKFLOW-RUN-08** When `-trigger` is provided, the system shall record that trigger value in persisted run metadata.

**WORKFLOW-RUN-09** When SIGINT or SIGTERM is received, the system shall cancel the workflow execution context.

**WORKFLOW-RUN-10** When workflow loading, executor initialization, persistence initialization, or execution fails, the system shall exit with code 1.

## BDD Traceability

- `agm/test/bdd/features/workflow_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
