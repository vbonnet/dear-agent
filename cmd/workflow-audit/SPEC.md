# Workflow Audit Command Specification

<!-- Last audited at: 2026-08-02 -->

## Overview

`cmd/workflow-audit` is the CLI front end for workflow audit checks. It loads
audit configuration, registers built-in checks, and delegates run, list, show,
acknowledge, and resolve operations to the audit package.

## Requirements

**WORKFLOW-AUDIT-01** When no subcommand is provided, the system shall print usage and exit with a usage error.

**WORKFLOW-AUDIT-02** When the `run` subcommand is provided, the system shall evaluate configured audit checks against workflow audit storage.

**WORKFLOW-AUDIT-03** When the `list` subcommand is provided, the system shall list stored audit findings from the configured audit database.

**WORKFLOW-AUDIT-04** When the `show` subcommand is provided, the system shall render one stored audit finding by id.

**WORKFLOW-AUDIT-05** When the `ack` subcommand is provided, the system shall mark the selected audit finding acknowledged.

**WORKFLOW-AUDIT-06** When the `resolve` subcommand is provided, the system shall mark the selected audit finding resolved.

**WORKFLOW-AUDIT-07** When no database path is provided, the system shall use `.dear-agent/audit.db` as the audit database.

**WORKFLOW-AUDIT-08** When audit configuration is requested, the system shall load audit config before running checks or mutating finding state.

**WORKFLOW-AUDIT-09** When the `run` subcommand records remediation suggestions, the system shall not expose a flag that implies those suggestions are dispatched by this command.

## BDD Traceability

- `agm/test/bdd/features/workflow_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
