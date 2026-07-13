# Project Health Collector Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/aggregator/collectors`.

## Overview

`pkg/aggregator/collectors` gathers git activity, lint trend, test coverage,
dependency freshness, and vulnerability signals through bounded process and
file adapters.

## EARS Requirements

**COLLECT-01** When a collector invokes a host tool, the system shall pass arguments directly to a context-bound process without a command shell.

**COLLECT-02** When a required host tool is absent, the system shall return a typed missing-tool diagnostic.

**COLLECT-03** When git activity is collected, the system shall report recent commit and changed-line observations for the configured repository.

**COLLECT-04** When lint output is collected, the system shall parse supported golangci-lint JSON output and tolerate trailing summary records.

**COLLECT-05** When a coverage profile is collected, the system shall summarize statement coverage by package from valid profile rows.

**COLLECT-06** When dependency freshness is collected, the system shall count modules with available updates from structured Go module output.

**COLLECT-07** When vulnerability output is collected, the system shall parse govulncheck records into security-alert signals without treating malformed input as success.

## BDD Traceability

- Feature: `agm/test/bdd/features/session_protocol_guardrails.feature`

## Test Traceability

- Unit package: `pkg/aggregator/collectors`
