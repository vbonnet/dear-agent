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

**OLA-06** When a new authorization transaction crosses the privileged boundary, the command shall require a unique random authorization ID for every use and shall revalidate each use's exact subject against its matching root-owned grant before appending any record.

**OLA-07** When the NOPASSWD helper is invoked, the command shall require its launcher PID to be the first non-sudo ancestor, allow only root-owned `/usr/bin/sudo` intermediaries, inspect the live launcher code identity through the kernel, and require that identity to match the root-owned installed-launcher policy; on Linux it shall require an admin-only Yama ptrace policy, no live tracer, and no ELF interpreter so same-user debug and loader injection are impossible, and on macOS it shall require a valid hardened-runtime process that has never allowed debugging or invalid code.

**OLA-08** When the authenticated AGM launcher, or the separately attested co-installed AGM MCP companion, binds override proofs into a private handoff, the command shall issue a short-lived root-owned capability for the exact protocol, path, handoff digest, proofs, and accompanying spawn obligation; when an AGM-only executor presents that complete capability, the command shall compare it with the root-owned canonical bytes and atomically unlink it under an exclusive lock so concurrent or copied handoffs cannot consume it more than once.

**OLA-09** When launch capabilities are issued, the command shall serialize directory mutation through a root-only lock, remove expired canonical sidecars, reject unexpected directory entries, and refuse issuance beyond a fixed outstanding-capability limit so aborted launches cannot grow privileged runtime state without bound.

**OLA-10** When the privileged helper and its authenticated launchers are upgraded, installation shall stage and validate the complete helper, sudoers, identity-policy, AGM, and MCP-companion artifact set before replacing any live path; if any activation step fails, it shall restore every prior artifact or remove every newly created artifact before returning failure.

## BDD Traceability

- Feature: `agm/test/bdd/features/dangerous_override_governance.feature`
- Package tests: `cmd/override-ledger-append/main_test.go`
