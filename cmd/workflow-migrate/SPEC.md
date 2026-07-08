# Workflow Migrate Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/workflow-migrate` migrates legacy workflow FileState JSON snapshots into
the SQLite runs database. It preserves the source JSON file and writes
idempotent run and node records.

## Requirements

**WORKFLOW-MIGRATE-01** When the snapshot path argument is missing, the system shall print usage and exit with code 2.

**WORKFLOW-MIGRATE-02** When the snapshot lacks a workflow name and `--workflow` is omitted, the system shall exit with code 2.

**WORKFLOW-MIGRATE-03** When the snapshot lacks a run id, the system shall derive a deterministic run id from workflow name and started time.

**WORKFLOW-MIGRATE-04** When `--dry-run` is provided, the system shall print the migration plan summary without opening or writing the destination database.

**WORKFLOW-MIGRATE-05** When migration writes to SQLite, the system shall mark the run trigger as `migrate`.

**WORKFLOW-MIGRATE-06** When all migrated nodes are completed, the system shall finish the run as succeeded.

**WORKFLOW-MIGRATE-07** When at least one migrated node is not completed, the system shall keep the run in running state.

**WORKFLOW-MIGRATE-08** When a run row already exists for the migrated run id, the system shall treat the duplicate as idempotent and continue upserting node records.

**WORKFLOW-MIGRATE-09** When snapshot IO, parsing, or SQLite migration fails, the system shall exit with code 1.

## BDD Traceability

- `agm/test/bdd/features/workflow_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
