# AGM Usage Tracker Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/telemetry/usage` records AGM CLI command execution events as
JSONL. It mirrors the shared usage tracker contract under AGM's internal
module boundary so AGM-specific command tracking can be audited without
depending on an external telemetry service.

## EARS Requirements

**AUT-01** When a tracker is created with an explicit file path, the system shall use that path without creating the default Engram directory.

**AUT-02** When a tracker is created without an explicit file path, the system shall create `~/.engram` with private directory permissions and use `~/.engram/usage.jsonl`.

**AUT-03** When the home directory cannot be resolved for a default tracker, the system shall return an error.

**AUT-04** When asynchronous tracking is requested, the system shall record the event in a goroutine so AGM command execution is not blocked.

**AUT-05** When synchronous tracking is requested, the system shall append exactly one JSON event line before returning.

**AUT-06** When an event has no timestamp, the system shall stamp the current time before writing it.

**AUT-07** When the usage file is opened, the system shall create or append to it with private file permissions.

**AUT-08** When the usage file cannot be opened or the event cannot be encoded, the system shall return the tracking error to synchronous callers.

**AUT-09** When callers request the file path, the system shall return the configured path exactly.

## BDD Traceability

- Feature: `agm/test/bdd/features/quota_monitoring_guardrails.feature`
- Package tests: `agm/internal/telemetry/usage/tracker_test.go`

