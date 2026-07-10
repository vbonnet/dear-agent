# Stop Retrospect Hook Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/hooks/cmd/stop-retrospect` is an advisory stop-hook adapter that scans
bounded conversation history for late promises and unretried tool errors. It
prefers the harness-provided transcript path and uses Claude project history
only as a compatibility fallback.

## EARS Requirements

**ESRH-01** When hook input is invalid, the command shall report the read failure and exit successfully so retrospective advice cannot block session shutdown.

**ESRH-02** When a transcript path is provided by the harness, the command shall analyze that path without model-family assumptions.

**ESRH-03** When no transcript path is provided, the Claude compatibility fallback shall accept only a path-separator-free session identifier.

**ESRH-04** When no conversation can be found, the hook shall report a skipped passing check.

**ESRH-05** When conversation JSONL is scanned, the system shall inspect at most 500 lines with a bounded scanner buffer and shall ignore malformed entries.

**ESRH-06** When assistant content is a string or an array of text blocks, the system shall extract text for promise analysis.

**ESRH-07** When potential late promises or tool errors are found, the hook shall report warnings with bounded examples and shall remain advisory.

**ESRH-08** When hook execution exceeds 10 seconds, the shared stop-hook runner shall terminate the operation according to its timeout contract.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks/cmd/stop-retrospect/*_test.go`
