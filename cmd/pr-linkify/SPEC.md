# Pull Request Linkify Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/pr-linkify` exposes plain-text, summary, and hook-adapter entry points for
the shared pull-request reference parser.

## EARS Requirements

**PLC-01** When no repository is supplied, the command shall use the shared default repository unless `PR_LINKIFY_REPO` overrides it.

**PLC-02** When plain-text mode is selected, the command shall transform standard input with the shared linkifier.

**PLC-03** When summary mode is selected, the command shall print one deduplicated Markdown pull-request link per discovered reference.

**PLC-04** When hook-adapter input uses neutral top-level fields or Claude-compatible nested tool fields, the command shall select the first non-empty transcript, output, or message.

**PLC-05** When hook-adapter input contains pull-request references, the command shall emit both neutral additional context and Claude-compatible hook-specific context.

**PLC-06** When hook input is malformed or contains no references, the advisory command shall emit nothing rather than blocking the caller.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_maintenance_command_guardrails.feature`
- Package tests: `cmd/pr-linkify/*_test.go`
