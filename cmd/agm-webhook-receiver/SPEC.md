# AGM Webhook Receiver Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/agm-webhook-receiver` verifies GitHub webhook requests and appends bounded
pull-request events for AGM ingestion.

## EARS Requirements

**AWR-01** When a webhook request is received, the server shall bound request-body size before decoding it.

**AWR-02** When a webhook secret is configured, the server shall require a valid SHA-256 GitHub signature using constant-time comparison.

**AWR-03** When a signature is missing, malformed, or invalid, the server shall reject the request without appending an event.

**AWR-04** When a supported pull-request event is decoded, the server shall preserve event, action, repository, pull request, branch, and sender metadata.

**AWR-05** When an unsupported event is received, the server shall acknowledge it without manufacturing a pull-request event.

**AWR-06** When an event is persisted, the server shall append one private JSONL record without truncating prior events.

**AWR-07** When the sink cannot be created or written, the server shall return an error response rather than claiming durable receipt.

**AWR-08** When the server starts, the command shall honor configured listen and output paths and use bounded HTTP server timeouts.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_operations_command_guardrails.feature`
- Package tests: `cmd/agm-webhook-receiver/*_test.go`
