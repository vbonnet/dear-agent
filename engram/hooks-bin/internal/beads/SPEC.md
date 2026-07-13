# Hook Beads Client Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/hooks-bin/internal/beads` is the hook-facing adapter for Beads lifecycle
operations. It invokes `bd` directly with structured arguments and decodes
machine-readable list responses.

## EARS Requirements

**EHBC-01** When the `bd` executable is absent, the client shall return an explicit unavailable error rather than invoking an empty command.

**EHBC-02** When a session UUID is queried, the client shall pass it as a `uuid:<value>` label argument without shell evaluation.

**EHBC-03** When a Beads list response is valid JSON, the client shall decode typed summaries and return an empty result when no bead matches.

**EHBC-04** When an automatic bead is created, the client shall attach the auto-created, session UUID, and session-end labels.

**EHBC-05** When `bd` output or execution is invalid, the client shall return contextual errors without manufacturing successful results.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks-bin/internal/beads/*_test.go`
