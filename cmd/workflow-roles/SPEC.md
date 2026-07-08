# Workflow Roles Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/workflow-roles` inspects and validates workflow role catalogs. It supports
built-in role definitions, explicit files, and the standard local/user discovery
paths used by workflow execution.

## Requirements

**WORKFLOW-ROLES-01** When no subcommand is provided, the system shall print usage and exit with code 2.

**WORKFLOW-ROLES-02** When `--file` is provided, the system shall load roles from that file.

**WORKFLOW-ROLES-03** When `--file` is omitted and `DEAR_AGENT_ROLES` is set, the system shall load roles from the environment-provided path.

**WORKFLOW-ROLES-04** When no explicit or environment path is available, the system shall try `./.dear-agent/roles.yaml` before user-level role config.

**WORKFLOW-ROLES-05** When no configured role file exists, the system shall fall back to built-in roles.

**WORKFLOW-ROLES-06** When `list` is provided, the system shall list available workflow roles.

**WORKFLOW-ROLES-07** When `describe <role>` is provided, the system shall render that role definition.

**WORKFLOW-ROLES-08** When `describe --json <role>` is provided, the system shall render that role definition as JSON.

**WORKFLOW-ROLES-09** When `validate` is provided, the system shall parse and validate the selected roles catalog.

## BDD Traceability

- `agm/test/bdd/features/workflow_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
