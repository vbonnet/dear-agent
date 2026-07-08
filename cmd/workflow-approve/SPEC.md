# Workflow Approve Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/workflow-approve` is the human-in-the-loop decision CLI for persisted
workflow runs. It lists pending approval requests and records approve or reject
decisions through the workflow audit trail.

## Requirements

**WORKFLOW-APPROVE-01** When no subcommand is provided, the system shall print usage and exit with code 2.

**WORKFLOW-APPROVE-02** When the `list` subcommand is provided, the system shall read pending HITL requests from the configured runs database.

**WORKFLOW-APPROVE-03** When `list --json` is provided, the system shall emit indented JSON for pending HITL requests.

**WORKFLOW-APPROVE-04** When `approve <approval-id>` is provided, the system shall record an approval decision for the requested approval id.

**WORKFLOW-APPROVE-05** When `reject <approval-id>` is provided, the system shall record a rejection decision for the requested approval id.

**WORKFLOW-APPROVE-06** When `--actor` is omitted for a decision, the system shall derive a human actor from the current OS user.

**WORKFLOW-APPROVE-07** When `--actor` starts with `human:`, the system shall strip that prefix before passing the approver to workflow decision recording.

**WORKFLOW-APPROVE-08** When the approval id is not found, the system shall exit with code 3.

**WORKFLOW-APPROVE-09** When the approval id is already resolved, the system shall exit with code 4.

**WORKFLOW-APPROVE-10** When the provided approver role does not match the required role, the system shall exit with code 5.

## BDD Traceability

- `agm/test/bdd/features/workflow_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
