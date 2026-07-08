# AGM-Aware Reaper Command Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm-aware-reaper` short-lived supervised process reaper.

## Overview

`agm-aware-reaper` is the command-line adapter around `agm/internal/procreaper`.
It protects AGM-supervised process trees, selects unsupervised high-resource or
idle candidates, and emits either a human summary or a stable JSON contract for
supervisor automation. The command must be safe to invoke from pressure handlers:
dry-run mode computes the kill set without sending signals, and parse/help paths
must fail cleanly.

## EARS Requirements

**AWARE-REAPER-01** When `agm-aware-reaper` receives invalid flags, the system shall return a command-line error without panicking.

**AWARE-REAPER-02** When `agm-aware-reaper` receives help flags, the system shall return the flag help path without panicking.

**AWARE-REAPER-03** When `--dry-run` is set, the system shall compute candidate and protected process sets without terminating processes.

**AWARE-REAPER-04** When `--json` is set, the system shall emit a machine-readable reaping result.

**AWARE-REAPER-05** When `--json` is not set, the system shall emit a human-readable summary of candidates, protected processes, reaped processes, and errors.

**AWARE-REAPER-06** When an AGM binary override is provided, the system shall use that binary to discover protected AGM sessions.

**AWARE-REAPER-07** When a process belongs to a live AGM session subtree, the system shall treat that process as protected.

**AWARE-REAPER-08** When a process exceeds the requested resource or idle thresholds and is not protected, the system shall treat that process as reapable.

**AWARE-REAPER-09** When termination is required, the system shall use the configured grace window before escalation.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`

## Test Traceability

- Unit package: `agm/cmd/agm-aware-reaper`
- Unit package: `agm/internal/procreaper`
