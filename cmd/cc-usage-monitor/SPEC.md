# Claude Code Usage Monitor Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/cc-usage-monitor` scans local Claude Code transcript JSONL files,
aggregates token and estimated cost usage by session type and model, emits
OpenTelemetry gauges when configured, and appends a VROOM decision-trail alert
when projected 24-hour burn exceeds the configured threshold.

## EARS Requirements

**CUM-01** When session type is classified, the system shall prefer supervisor markers over worker markers and shall return unknown for unrecognized names.

**CUM-02** When a transcript line is blank, malformed JSON, or lacks token usage, the system shall skip the line.

**CUM-03** When a transcript line has token usage, the system shall normalize model, session type, timestamp, input tokens, output tokens, cache creation tokens, cache read tokens, and USD cost into an entry.

**CUM-04** When a transcript line contains `costUSD`, the system shall use that cost instead of recomputing from token counts.

**CUM-05** When a transcript line omits `costUSD`, the system shall estimate cost from token counts using the shared `pkg/costtrack` pricing table.

**CUM-06** When transcripts are scanned from a directory, the system shall walk `*.jsonl` files, use the parent directory as a classification hint, skip unreadable files or subtrees, and count only successfully scanned files.

**CUM-07** When usage entries are aggregated, the system shall accumulate token counts, cost, message count, earliest timestamp, and latest timestamp by session type, by model, and in total.

**CUM-08** When projected daily cost is calculated, the system shall use only entries inside the trailing window and extrapolate the window cost to 24 hours.

**CUM-09** When the trailing window is non-positive, the system shall report zero projected daily cost.

**CUM-10** When metric emission fails, the system shall report the emission error on stderr and continue scanning and alerting.

**CUM-11** When projected 24-hour cost exceeds the threshold, the system shall log a burn-rate warning, append a best-effort `quota.burn.alert` decision-trail record when a trail path is configured, and return the alert exit code.

**CUM-12** When JSON output is requested, the system shall emit an indented JSON payload containing report, projected daily cost, threshold, window, and alert status.

**CUM-13** When human output is requested, the system shall print total usage, per-session-type usage in deterministic order, projected burn rate, and alert status.

## BDD Traceability

- Feature: `agm/test/bdd/features/quota_monitoring_guardrails.feature`
- Package tests: `cmd/cc-usage-monitor/scan_test.go`

