# Engram Simple Memory Provider Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/providers/simple` implements provider-neutral memory,
working-context, session-history, and artifact operations with private JSON files.

## EARS Requirements

**ESP-01** When the simple provider is created, the system shall require a non-empty string storage path and register the provider under the `simple` name.

**ESP-02** When memory is stored, updated, retrieved, or deleted, the system shall reject unsafe namespace components and memory identifiers and keep paths beneath the storage root.

**ESP-03** When memory is stored without a schema version, the system shall assign version `1.0` and write private JSON beneath its namespace.

**ESP-04** When memories are retrieved, the system shall skip unreadable or malformed records, combine supported query filters, sort newest first, and apply a positive limit.

**ESP-05** When an update targets missing memory, the system shall return not-found; when append content targets non-string content, it shall return an update error.

**ESP-06** When artifact operations receive an unsafe identifier, the system shall reject it and keep all artifact paths beneath the private `_artifacts` directory.

**ESP-07** When working context or session history is persisted, the system shall flush a private temporary file before atomically renaming it over the destination.

**ESP-08** When session events are appended, the system shall preserve append order, initialize the session start time, and atomically persist private history JSON.

**ESP-09** When a session is persisted, the system shall require existing history, record its completion time, and publish a harness-neutral session-persisted event when a bus is present.

**ESP-10** When concurrent context, session, or artifact operations use one provider instance, the system shall serialize access to persisted filesystem state.

**ESP-11** When memory and artifact operations complete or fail, the system shall emit available telemetry without requiring a recorder or changing the operation result.

**ESP-12** When memory mutations succeed and an event bus is configured, the system shall publish the corresponding harness-neutral lifecycle event.

**ESP-13** When provider health and close operations run, the system shall remain safe for repeated lifecycle calls.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_reflection_storage_guardrails.feature`
- Package tests: `engram/internal/providers/simple/*_test.go`
