# Workflow Logs Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/workflow-logs` prints the audit-event stream for one workflow run. It can
filter by node, limit event count, and emit either text tables or JSON.

## Requirements

**WORKFLOW-LOGS-01** When the run id argument is missing, the system shall print usage and exit with code 2.

**WORKFLOW-LOGS-02** When no database path is provided, the system shall read audit events from `runs.db`.

**WORKFLOW-LOGS-03** When `--node` is provided, the system shall return only audit events for that node id.

**WORKFLOW-LOGS-04** When `--limit` is greater than zero, the system shall bound the returned audit event count to that limit.

**WORKFLOW-LOGS-05** When `--json` is provided, the system shall emit indented JSON audit events.

**WORKFLOW-LOGS-06** When text output is selected, the system shall print time, node, state transition, actor, and reason columns.

**WORKFLOW-LOGS-07** When the run id is not found, the system shall exit with code 3.

**WORKFLOW-LOGS-08** When the configured database cannot be opened or queried, the system shall exit with code 1.

## BDD Traceability

- `agm/test/bdd/features/workflow_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
