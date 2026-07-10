# Engram Memory Consolidation Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/consolidation` defines provider-neutral working, session,
long-term, and artifact memory contracts plus configuration and safe purging.

## EARS Requirements

**ECO-01** When consolidation configuration is discovered, the system shall prefer the nearest project `.engram/config.yaml`, then global configuration, then built-in simple-provider defaults.

**ECO-02** When a selected configuration file is malformed, the system shall return its parse error rather than constructing a provider from invalid data.

**ECO-03** When no provider type is configured, the system shall select the simple provider and pass its provider-specific options unchanged.

**ECO-04** When a provider factory is registered and loaded by name, the system shall construct that provider; when the name is unknown, it shall return the provider-not-found error.

**ECO-05** When providers are listed, the system shall return the names of every registered memory provider so custom backends remain discoverable.

**ECO-06** When memory is serialized, the system shall preserve its identifier, type, namespace, content, metadata, provenance, importance, timestamps, and artifact references.

**ECO-07** When context is purged, the system shall remove only old tool-result events outside the most-recent event window and preserve structural events.

**ECO-08** When context is purged, the system shall redact configured PII and secret patterns from retained event and memory string content and report redaction statistics.

**ECO-09** When a memory lifecycle event is created, the system shall use the harness-neutral memory topic, publisher, timestamp, and supplied data.

**ECO-10** When an event bus or telemetry recorder is absent, the system shall make memory event reporting a no-op; when present, it shall emit outcome metadata without changing the memory operation result.

**ECO-11** When event bus and telemetry dependencies are attached to a context, the system shall retrieve only the typed dependency associated with that context key.

**ECO-12** When a provider evaluates a memory query, the system shall combine type, namespace, importance, time-range, text, and limit fields according to the provider-neutral query contract.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_analysis_configuration_guardrails.feature`
- Package tests: `engram/internal/consolidation/*_test.go`
