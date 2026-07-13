# Dear Agent Bumblebee Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/dear-agent-bumblebee` installs a pinned scanner binary, runs catalog scans,
and manages an optional macOS launch agent.

## EARS Requirements

**DAB-01** When a binary release is installed, the command shall select a supported operating-system and architecture artifact for the pinned version.

**DAB-02** When a downloaded artifact checksum differs from the pinned checksum, the command shall reject installation and shall not replace the current binary.

**DAB-03** When installation succeeds, the command shall stage and atomically replace the executable at the selected destination.

**DAB-04** When a scan runs, the command shall prefer an explicit binary and catalog path and shall report missing dependencies.

**DAB-05** When scanner output is persisted, the command shall create the output directory and use a deterministic dated filename unless overridden.

**DAB-06** When the scanner process fails, the command shall propagate its failure rather than publishing a successful report.

**DAB-07** When launch-agent installation is requested on a non-macOS platform, the command shall reject the unsupported operation.

**DAB-08** When a launch agent is installed or removed, the command shall render the pinned template, update launchd idempotently, and surface bootstrap failures.

**DAB-09** When status is requested, the command shall distinguish loaded, unloaded, and query-failure states.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_operations_command_guardrails.feature`
- Package tests: `cmd/dear-agent-bumblebee/*_test.go`
