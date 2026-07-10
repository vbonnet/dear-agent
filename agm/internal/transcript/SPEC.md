# Transcript Context Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/transcript` extracts bounded display context from saved transcripts
and maintains the local mapping between conversation UUIDs and session names.

## Requirements

**TRN-01** When extracting transcript context, the system shall locate the saved conversation, decode supported user and assistant messages, and return the requested number of recent exchanges.

**TRN-02** When formatting transcript context, the system shall preserve speaker order and wrap text for bounded readable display.

**TRN-03** When a session name is set or deleted, the system shall persist the UUID-to-name mapping atomically.

**TRN-04** When multiple callers access the session map, the system shall serialize updates and preserve valid JSON state.

**TRN-05** When the session-map file does not exist, the system shall initialize an empty mapping instead of failing discovery.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_conversation_discovery_guardrails.feature`
- Package tests: `agm/internal/transcript/*_test.go`
