# Override Ledger Append Command Specification

<!-- Last audited at: 2026-07-29 -->

## Overview

`cmd/override-ledger-append` is the fixed privileged Unix boundary for
dangerous-override audit records. Its installed path may receive an exact
NOPASSWD sudoers rule because it accepts neither an operator-selected command
nor an operator-selected destination.

## EARS Requirements

**OLA-01** When the command receives arguments, multiple JSON values, non-canonical JSONL, unknown fields, or an oversized record, the command shall reject the request without writing.

**OLA-02** When a canonical record is appended at the privileged boundary, the command shall require root execution, a matching active root-owned grant, and a timestamp within the bounded append window.

**OLA-03** When the fixed ledger or its parent is inspected, the command shall reject symlinks, non-regular files, non-root ownership, and group-writable or other-writable modes.

**OLA-04** When concurrent or repeated records are appended, the command shall serialize the rate-limit, size-check, and append under an exclusive lock, shall enforce a bounded per-kind window with automatic recovery, and shall refuse growth beyond the fixed ledger cap.

**OLA-05** When a valid record is accepted, the command shall append only to `/var/log/dear-agent-overrides.jsonl`, set the ledger to a non-writable-by-users mode, and durably synchronize it before returning success.

## BDD Traceability

- Feature: `agm/test/bdd/features/dangerous_override_governance.feature`
- Package tests: `cmd/override-ledger-append/main_test.go`
