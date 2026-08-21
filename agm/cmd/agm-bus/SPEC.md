# AGM Bus Command Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm-bus` broker daemon and command-line status helpers.

## Overview

`agm-bus` is the local Unix-socket broker used by AGM sessions, supervisors,
permission prompts, offline queues, and optional external chat adapters. The
command must remain safe as a globally runnable daemon: it scopes state under
the user's AGM directories, degrades optional adapters when configuration is
missing, and reports broker status without treating "not running" as an error.

## EARS Requirements

**AGMBUS-01** When `agm-bus` is invoked without a supported subcommand, the system shall print usage and exit with command-line error status.

**AGMBUS-02** When `agm-bus socket` is invoked, the system shall print the effective bus socket path.

**AGMBUS-03** When `agm-bus status` is invoked and the socket file is absent, the system shall report that the broker is not running and return success.

**AGMBUS-04** When `agm-bus status` is invoked and the socket accepts Unix connections, the system shall report that the broker is listening.

**AGMBUS-05** When `agm-bus serve` starts successfully, the system shall bind the configured Unix socket until the process receives a shutdown signal.

**AGMBUS-06** When `agm-bus serve` shuts down cleanly, the system shall remove the bound socket file.

**AGMBUS-07** When offline queue support is not disabled, the system shall initialize the queue from the configured queue directory.

**AGMBUS-08** When ACL support is not disabled, the system shall load a reloadable ACL from the configured ACL path.

**AGMBUS-09** When the process receives SIGHUP and a reloadable ACL is active, the system shall reload the ACL without restarting the broker.

**AGMBUS-10** When the Discord adapter is enabled without a token, the system shall disable Discord routing without stopping the broker.

**AGMBUS-11** When the Matrix adapter is enabled without required connection settings, the system shall disable Matrix routing without stopping the broker.

**AGMBUS-12** When supervisor heartbeat watching is not disabled, the system shall publish stale-supervisor signals using the configured supervisors directory and thresholds.

**AGMBUS-13** When `agm-bus discord-reset` is invoked without confirmation, the system shall perform a dry run instead of deleting messages.

**AGMBUS-14** When `agm-bus discord-reset` is asked to act as an agent absent from the portal config, the system shall fail rather than fall back to another configured bot.

**AGMBUS-15** When the serve command is assembled, the system shall separate flag parsing, broker construction, and adapter startup into independently callable seams so the daemon's setup is verifiable in-process rather than only by running the built binary.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`

## Test Traceability

- Unit package: `agm/cmd/agm-bus`
- In-process serve seams: `agm/cmd/agm-bus/serve_test.go`
- In-process subcommands: `agm/cmd/agm-bus/subcommands_test.go`
- End-to-end binary behavior: `agm/cmd/agm-bus/main_test.go`
