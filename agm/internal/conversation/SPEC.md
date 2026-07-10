# Conversation Format Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/conversation` defines AGM's harness-neutral JSONL conversation
format, content blocks, validation, conversion, and lightweight analysis.

## Requirements

**CONV-01** When parsing conversation JSONL, the system shall require a supported header schema and decode subsequent messages in order.

**CONV-02** When conversation content contains text, image, tool-use, or tool-result blocks, the system shall preserve each supported block's typed fields.

**CONV-03** When validating a conversation, the system shall reject unsupported schemas, missing required fields, invalid roles, empty content, and malformed content blocks.

**CONV-04** When writing a valid conversation, the system shall use an atomic replacement and preserve parse-write round-trip semantics.

**CONV-05** When converting supported Claude HTML exports, the system shall emit valid harness-neutral conversation JSONL.

**CONV-06** When classifying a session by message count, the system shall count valid message records and compare them consistently with the supplied trivial-session threshold.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_conversation_discovery_guardrails.feature`
- Package tests: `agm/internal/conversation/*_test.go`
