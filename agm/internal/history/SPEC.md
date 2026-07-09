# Harness History Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/history` parses conversation history and resolves harness-specific
history locations for Claude Code, Codex CLI, Gemini compatibility, and OpenCode.

## Requirements

**HIS-01** When reading Claude history, the system shall tolerate empty, malformed, mixed-format, and null-byte-corrupted lines while retaining valid records.

**HIS-02** When selecting history by directory, UUID, or recency, the system shall return deterministic matches ordered by timestamp.

**HIS-03** When grouping conversation prompts, the system shall group by session ID, retain the most common project, enforce the requested entry limit, and order entries by timestamp.

**HIS-04** When resolving history for a supported harness, the system shall use that harness's canonical filesystem layout rather than assuming Claude paths.

**HIS-05** When path verification is requested, the system shall report missing history or conversation locations instead of returning an unverified path.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_conversation_discovery_guardrails.feature`
- Package tests: `agm/internal/history/*_test.go`
