# AGM Reaper Command Specification

<!-- Last audited at: 2026-08-27 -->

**Version:** 1.2
**Status:** Living
**Scope:** `agm-reaper` archived-session cleanup command.

## Overview

`agm-reaper` is the command-line wrapper around `agm/internal/reaper`. It stops a
named session and then completes archival through the shared AGM archive
operation. The command is intentionally small but remains a runtime surface
because detached archival, launchd, manual recovery, and cleanup automation
depend on its flags and exit behavior.

## EARS Requirements

**AGM-REAPER-01** When `agm-reaper` is invoked without `--session`, the system shall print flag usage and return an error.

**AGM-REAPER-02** When `--log-file` is provided, the system shall append structured log output to that file with private file permissions.

**AGM-REAPER-03** When `--sessions-dir` is provided, the system shall pass that directory to the reaper implementation.

**AGM-REAPER-04** When the reaper implementation returns an error, the system shall return a wrapped command error.

**AGM-REAPER-05** When the reaper implementation completes successfully, the system shall log successful completion.

**AGM-REAPER-06** When the command returns an error from `main`, the system shall write the error to stderr and exit nonzero.

**AGM-REAPER-07** When `--force` or `--keep-sandbox` is provided, the system shall pass the selected archive option to the reaper implementation unchanged; when an accepted `--outcome` is provided, the system shall pass its canonical typed value without rewriting the accepted wire spelling.

**AGM-REAPER-08** When no non-empty `--outcome` is provided, the system shall pass the legacy empty outcome to the shared archive operation, which shall default the archived session's terminal outcome to `completed`.

**AGM-REAPER-09** When AGM supplies an expected VCS revision for asynchronous archive execution, the system shall refuse to run lifecycle code unless the detached `agm-reaper` binary carries the same embedded revision and the same clean or dirty provenance.

**AGM-REAPER-10** When AGM supplies a startup acknowledgement descriptor, the system shall acknowledge readiness only after revision validation, required target validation, archive outcome validation, and durable log initialization succeed.

**AGM-REAPER-11** When the canonical root AGM install target is invoked, the system shall build and install both `agm` and `agm-reaper` with the same revision identity.

**AGM-REAPER-12** When asynchronous archive execution crosses into `agm-reaper`, the command shall require both `--session-id` with the stable AGM lifecycle identifier and `--session` with the resolved tmux target, and shall pass those identities to storage and pane-control paths without conflating them.

**AGM-REAPER-13** When a non-empty `--outcome` is provided, the system shall accept only `completed`, `crashed`, `killed`, or `gc-stale`, and shall reject any other text before durable log creation, startup acknowledgement, or reaper construction.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`

## Test Traceability

- Unit package: `agm/cmd/agm-reaper`
- Unit package: `agm/internal/reaper`
