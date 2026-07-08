# Workflow Cancel Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/workflow-cancel` cancels a persisted workflow run in the SQLite runs
database and records the cancellation reason and actor through the workflow
state layer.

## Requirements

**WORKFLOW-CANCEL-01** When the run id argument is missing, the system shall print usage and exit with code 2.

**WORKFLOW-CANCEL-02** When a run id is provided, the system shall cancel that run in the configured runs database.

**WORKFLOW-CANCEL-03** When `--reason` is omitted, the system shall record `cancelled-via-cli` as the cancellation reason.

**WORKFLOW-CANCEL-04** When `--actor` is omitted, the system shall derive a `human:<username>` actor from the current OS user.

**WORKFLOW-CANCEL-05** When the configured database cannot be opened, the system shall exit with code 1.

**WORKFLOW-CANCEL-06** When the run id is not found, the system shall exit with code 3.

**WORKFLOW-CANCEL-07** When cancellation succeeds, the system shall print a cancellation confirmation and exit with code 0.

## BDD Traceability

- `agm/test/bdd/features/workflow_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
