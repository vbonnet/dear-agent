# A2A Protocol Message Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/a2a/protocol` defines the canonical markdown-friendly A2A message
shape and status vocabulary used by channel, JSON-RPC, Wayfinder, and messaging
packages.

## EARS Requirements

**A2A-PRO-01** When a protocol message is created, the system shall set agent ID, timestamp, status, and empty questions, blockers, and next-step lists.

**A2A-PRO-02** When a protocol message is validated, the system shall require agent ID, valid status, message number greater than zero, context, and proposal.

**A2A-PRO-03** When a status is validated, the system shall accept the fixed A2A statuses and `blocked-on-{reason}` statuses while rejecting the blocked prefix sentinel by itself.

**A2A-PRO-04** When formatting a protocol message, the system shall produce markdown sections for metadata, context, proposal or response, questions, blockers, and next steps.

**A2A-PRO-05** When optional questions, blockers, or next steps are empty, the system shall emit deterministic fallback text instead of omitting the section.

**A2A-PRO-06** When estimating tokens for a message, the system shall base the estimate on formatted message content using the package's approximate character-to-token ratio.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/a2a/protocol/message_test.go`
- Package tests: `agm/internal/a2a/protocol/status_test.go`
