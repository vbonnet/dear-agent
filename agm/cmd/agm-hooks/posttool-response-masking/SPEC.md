# PostTool Response Masking Hook Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`posttool-response-masking` archives oversized tool output and returns a compact
summary so hook-enabled harnesses preserve context window budget.

## EARS Requirements

**PRM-01** When a tool name is listed in the skip configuration, the system shall leave that tool result unmodified.

**PRM-02** When a tool result length is at or below the masking threshold, the system shall leave the result unmodified.

**PRM-03** When a tool result exceeds the masking threshold, the system shall archive the full result under the session archive directory.

**PRM-04** When multiple results are archived for the same session and tool, the system shall assign the next monotonically increasing archive sequence number.

**PRM-05** When archive writing succeeds, the system shall emit a compact summary containing the archive path and size information.

**PRM-06** When archive writing fails, the system shall fail open and shall not block the tool result.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`
- Package tests: `agm/cmd/agm-hooks/posttool-response-masking/main_test.go`

