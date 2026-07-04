# PostTool Cost Guard Hook Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`posttool-cost-guard` samples Claude session JSONL cost after tool calls and
warns when cost thresholds are crossed.

## EARS Requirements

**PCG-01** When hook input is unreadable or invalid JSON, the system shall fail open.

**PCG-02** When the hook runs, the system shall bound execution with a five-second timeout.

**PCG-03** When a traceparent is present in hook input or environment, the system shall propagate that trace context to the hook span.

**PCG-04** When a session has not reached the sampling interval, the system shall skip cost computation.

**PCG-05** When the session JSONL file cannot be located, the system shall fail open.

**PCG-06** When computed session cost reaches warning or critical thresholds, the system shall emit the corresponding warning to stderr.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`
- Package tests: `agm/hooks/cmd/posttool-cost-guard/main_test.go`

