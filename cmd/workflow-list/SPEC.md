# Workflow List Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/workflow-list` lists recent persisted workflow runs from a runs database.
It supports state filtering, result limits, and JSON output.

## Requirements

**WORKFLOW-LIST-01** When no database path is provided, the system shall read runs from `runs.db`.

**WORKFLOW-LIST-02** When `--state` is provided, the system shall filter runs by the requested workflow state.

**WORKFLOW-LIST-03** When `--limit` is provided, the system shall bound the number of returned runs to that limit.

**WORKFLOW-LIST-04** When `--json` is provided, the system shall emit indented JSON run summaries.

**WORKFLOW-LIST-05** When text output is selected, the system shall print run id, workflow name, state, start time, and duration columns.

**WORKFLOW-LIST-06** When no runs match, the system shall print `no runs match` to stderr and still exit with code 0.

**WORKFLOW-LIST-07** When the configured database cannot be opened or queried, the system shall exit with code 1.

## BDD Traceability

- `agm/test/bdd/features/workflow_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
