# Harness History Specification

<!-- Last audited at: 2026-07-21 -->

## Overview

`agm/internal/history` parses conversation history and resolves harness-specific
history locations for Claude Code, Codex CLI, AGY, OpenCode, Pi, and Gemini compatibility.

## Requirements

**HIS-01** When reading Claude history, the system shall tolerate empty, malformed, mixed-format, and null-byte-corrupted lines while retaining valid records.

**HIS-02** When selecting history by directory, UUID, or recency, the system shall return deterministic matches ordered by timestamp.

**HIS-03** When grouping conversation prompts, the system shall group by session ID, retain the most common project, enforce the requested entry limit, and order entries by timestamp.

**HIS-04** When resolving history for a supported harness, the system shall use that harness's canonical filesystem layout rather than assuming Claude paths.

**HIS-05** When path verification is requested, the system shall report missing history or conversation locations instead of returning an unverified path.

**HIS-06** When resolving AGY history, the system shall use the manifest's native conversation ID to return the Antigravity conversation database, compact transcript, and full transcript paths without entering Claude UUID discovery.

**HIS-07** When resolving Pi history, the system shall require persisted Pi metadata and verify that the exact JSONL transcript header matches the native session ID before returning the private session directory or transcript path.

**HIS-08** When Pi has not yet produced an assistant message and no native JSONL transcript exists, the system shall report history as unavailable and explain Pi's deferred-persistence requirement rather than claiming that launch or message submission alone creates the transcript.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_conversation_discovery_guardrails.feature`
- Package tests: `agm/internal/history/*_test.go`
