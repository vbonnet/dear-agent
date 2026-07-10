# Conversation Search Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/search` searches saved conversation text and joins matches with
AGM session and workspace metadata.

## Requirements

**SRCH-01** When searching conversations, the system shall require a Dolt adapter and map saved conversation IDs to AGM session and workspace metadata.

**SRCH-02** When a workspace filter is supplied, the system shall exclude conversations outside that workspace.

**SRCH-03** When regex mode is selected, the system shall compile the query with the requested case sensitivity and reject invalid patterns.

**SRCH-04** When literal mode is selected, the system shall count all occurrences with the requested case sensitivity.

**SRCH-05** When malformed records or unreadable conversation files are encountered, the system shall continue searching other conversations and return valid matches.

**SRCH-06** When a conversation matches, the system shall return the match count and a normalized bounded snippet from the first matching text block.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_conversation_discovery_guardrails.feature`
- Package tests: `agm/internal/search/*_test.go`
