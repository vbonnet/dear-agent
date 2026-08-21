# Usage Tracker Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`internal/telemetry/usage` records CLI command execution events as JSONL. It is
a lightweight telemetry boundary: tracking failures must not break the command
being observed, while synchronous tracking remains available for tests and
callers that need durability before exit.

This package is the single owner of the usage-tracker contract for every CLI in
the module, AGM included. An `agm/internal/telemetry/usage` copy carried a
byte-identical tracker under a parallel `AUT-01..09` requirement set; it was
retired in favor of this owner so the two cannot drift.

## EARS Requirements

**UTR-01** When a tracker is created with an explicit file path, the system shall use that path without creating the default Engram directory.

**UTR-02** When a tracker is created without an explicit file path, the system shall create `~/.engram` with private directory permissions and use `~/.engram/usage.jsonl`.

**UTR-03** When the home directory cannot be resolved for a default tracker, the system shall return an error.

**UTR-04** When asynchronous tracking is requested, the system shall record the event in a goroutine so CLI execution is not blocked.

**UTR-05** When synchronous tracking is requested, the system shall append exactly one JSON event line before returning.

**UTR-06** When an event has no timestamp, the system shall stamp the current time before writing it.

**UTR-07** When the usage file is opened, the system shall create or append to it with private file permissions.

**UTR-08** When the usage file cannot be opened or the event cannot be encoded, the system shall return the tracking error to synchronous callers.

**UTR-09** When callers request the file path, the system shall return the configured path exactly.

## BDD Traceability

- Feature: `agm/test/bdd/features/quota_monitoring_guardrails.feature`
- Package tests: `internal/telemetry/usage/tracker_test.go`

