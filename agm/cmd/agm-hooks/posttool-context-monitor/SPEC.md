# PostTool Context Monitor Hook Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`posttool-context-monitor` extracts Claude Code context-token usage after tool
calls and reports meaningful changes back to the AGM session state.

## EARS Requirements

**PCM-01** When Claude emits token usage in a system reminder, the system shall parse the used and total token counts.

**PCM-02** When Claude emits token usage as JSON, the system shall parse total token usage and shall default the maximum context size when it is absent.

**PCM-03** When context usage is calculated, the system shall report a percentage rounded to one decimal place.

**PCM-04** When a session cache entry is newer than the minimum update interval, the system shall suppress redundant updates.

**PCM-05** When the context percentage changes by less than the configured minimum percentage delta, the system shall suppress redundant updates.

**PCM-06** When the hook cannot associate the Claude session with an AGM session, the system shall fail open without updating AGM state.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`
- Package tests: `agm/cmd/agm-hooks/posttool-context-monitor/main_test.go`

