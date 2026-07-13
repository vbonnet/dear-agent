# Retrospective Audit Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/retro-audit` scans recent trace spans for retrospective findings and
appends a durable Markdown audit report.

## EARS Requirements

**RAC-01** When trace spans are read, the command shall inspect JSONL files beneath the configured trace directory.

**RAC-02** When a span predates the lookback threshold, the command shall exclude it from current findings.

**RAC-03** When malformed trace lines are encountered, the command shall preserve valid evidence and report actionable read failures according to the parser contract.

**RAC-04** When findings are generated, the command shall group them with timestamps, trace identifiers, summaries, and remediation context.

**RAC-05** When no findings exist, the command shall render an explicit clean report rather than fabricating issues.

**RAC-06** When dry-run mode is selected, the command shall print the report without modifying the output file.

**RAC-07** When persistence is selected, the command shall create parent directories and append rather than replace prior retrospective history.

**RAC-08** When traces originate from any harness or model family, the command shall analyze shared span attributes consistently.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_operations_command_guardrails.feature`
- Package tests: `cmd/retro-audit/*_test.go`
