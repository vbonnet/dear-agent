# Session UUID Discovery Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/uuid` discovers Claude conversation UUIDs from manifests, rename
history, activity windows, and saved transcript files.

## Requirements

**UUID-01** When a session manifest already contains a conversation UUID, the system shall return that UUID before consulting fallback discovery sources.

**UUID-02** When rename history contains a matching session name, the system shall return the most recent matching conversation UUID.

**UUID-03** When timestamp discovery is requested, the system shall select the nearest eligible history entry inside the configured time window.

**UUID-04** When saved transcript discovery is used, the system shall return the most recently modified matching JSONL conversation.

**UUID-05** When no discovery source yields a UUID, the system shall return an explicit error instead of inventing an identifier.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_conversation_discovery_guardrails.feature`
- Package tests: `agm/internal/uuid/*_test.go`
