# Workflow Status Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/workflow-status` renders persisted status for one workflow run. It supports
human-readable and JSON output and can re-render status on a watch interval.

## Requirements

**WORKFLOW-STATUS-01** When the run id argument is missing, the system shall print usage and exit with code 2.

**WORKFLOW-STATUS-02** When no database path is provided, the system shall read status from `runs.db`.

**WORKFLOW-STATUS-03** When `--json` is provided, the system shall emit indented JSON status for the run.

**WORKFLOW-STATUS-04** When text output is selected, the system shall render run state, workflow metadata, node states, and cost fields.

**WORKFLOW-STATUS-05** When `--watch` is provided, the system shall repeatedly clear the terminal and re-render status at the requested interval until signalled.

**WORKFLOW-STATUS-06** When the run id is not found, the system shall exit with code 3.

**WORKFLOW-STATUS-07** When the configured database cannot be opened or queried, the system shall exit with code 1.

## BDD Traceability

- `agm/test/bdd/features/workflow_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
