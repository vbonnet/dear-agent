# AGM Reaper Command Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm-reaper` archived-session cleanup command.

## Overview

`agm-reaper` is the command-line wrapper around `agm/internal/reaper`. It archives
a named session using the configured sessions directory and optional log file.
The command is intentionally small but remains a runtime surface because launchd,
manual recovery, and cleanup automation depend on its flags and exit behavior.

## EARS Requirements

**AGM-REAPER-01** When `agm-reaper` is invoked without `--session`, the system shall print flag usage and return an error.

**AGM-REAPER-02** When `--log-file` is provided, the system shall append structured log output to that file with private file permissions.

**AGM-REAPER-03** When `--sessions-dir` is provided, the system shall pass that directory to the reaper implementation.

**AGM-REAPER-04** When the reaper implementation returns an error, the system shall return a wrapped command error.

**AGM-REAPER-05** When the reaper implementation completes successfully, the system shall log successful completion.

**AGM-REAPER-06** When the command returns an error from `main`, the system shall write the error to stderr and exit nonzero.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`

## Test Traceability

- Unit package: `agm/cmd/agm-reaper`
- Unit package: `agm/internal/reaper`
