# Workflow Inspector Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/workflow-inspector` serves a read-only HTML UI over a workflow runs
database. It exposes run lists, run details, audit timelines, and a health
endpoint without providing state-changing controls.

## Requirements

**WORKFLOW-INSPECTOR-01** When no address is provided, the system shall bind to `127.0.0.1:8080`.

**WORKFLOW-INSPECTOR-02** When the runs database cannot be opened, the system shall exit with code 1.

**WORKFLOW-INSPECTOR-03** When the server starts, the system shall print the database path and HTTP address it is serving.

**WORKFLOW-INSPECTOR-04** When `GET /` or `GET /runs` is requested, the system shall render a read-only HTML list of workflow runs.

**WORKFLOW-INSPECTOR-05** When a `state` query parameter is provided on the list route, the system shall filter listed runs by that workflow state, and when WFLOW-08 rejects that filter the system shall return HTTP 400 rather than an internal error.

**WORKFLOW-INSPECTOR-06** When `GET /run/<id>` is requested for an existing run, the system shall render run status, node status, and audit events.

**WORKFLOW-INSPECTOR-07** When `GET /run/<id>` is requested for a missing run, the system shall return HTTP 404.

**WORKFLOW-INSPECTOR-08** When `GET /healthz` is requested, the system shall return HTTP 200 only when the database is reachable.

**WORKFLOW-INSPECTOR-09** When SIGINT or SIGTERM is received, the system shall shut down the HTTP server cleanly.

## BDD Traceability

- `agm/test/bdd/features/workflow_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
