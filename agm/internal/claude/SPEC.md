# Claude History Adapter Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/claude` reads Claude Code's local history and transcript files.
It is a Claude-specific adapter; harness-neutral conversation and session
behavior belongs in shared AGM packages.

## Requirements

**ACH-01** When Claude history contains valid entries, the system shall parse session IDs, projects, and timestamps while reporting parse statistics.

**ACH-02** When Claude history contains empty, malformed, or null-byte-corrupted lines, the system shall skip those lines and preserve valid entries.

**ACH-03** When parsed history exceeds the configured entry limit, the system shall return bounded partial results instead of allocating without limit.

**ACH-04** When history entries share a session ID, the system shall deduplicate them into one session ordered by most recent activity with aggregate activity statistics.

**ACH-05** When resolving a Claude transcript working directory, the system shall reject malformed UUIDs before filesystem matching and return the first non-empty transcript `cwd`.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_conversation_discovery_guardrails.feature`
- Package tests: `agm/internal/claude/*_test.go`
