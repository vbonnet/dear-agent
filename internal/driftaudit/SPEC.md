# Drift Audit Log Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`internal/driftaudit` appends deployment drift-check summaries to a durable
JSONL audit log under the user's state directory. Each line records whether a
drift check found actionable drift, even when the run was clean.

## Requirements

**DRIFT-AUDIT-01** When no home directory is provided, the system shall write audit records under `/tmp`.

**DRIFT-AUDIT-02** When appending a record, the system shall create `$HOME/.local/state/dear-agent` with directory mode `0700` if it does not exist.

**DRIFT-AUDIT-03** When opening the drift audit log, the system shall use append, create, and write-only flags with file mode `0600`.

**DRIFT-AUDIT-04** When a record is appended, the system shall encode it as one JSON object followed by one newline.

**DRIFT-AUDIT-05** When a record contains drift targets, the system shall preserve target name, deployed path, source path, status, and remediation text.

**DRIFT-AUDIT-06** When JSON marshaling fails, the system shall return an audit marshal error.

**DRIFT-AUDIT-07** When closing the audit log fails after a successful write, the system shall return the close error.

**DRIFT-AUDIT-08** When a write fails before close, the system shall return the write error instead of masking it with a later close error.

## BDD Traceability

- `agm/test/bdd/features/audit_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
