# Engram Wayfinder Analytics Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/analytics` parses Wayfinder telemetry, reconstructs sessions
and phases, computes activity metrics, and renders stable reports.

## EARS Requirements

**EAN-01** When analytics parses telemetry, the system shall read JSONL records and group valid Wayfinder event-bus publications by non-empty session identifier.

**EAN-02** When telemetry contains empty, malformed, non-Wayfinder, or structurally invalid records, the system shall skip those records and continue parsing subsequent records.

**EAN-03** When the telemetry file is missing or cannot be read, the system shall return an error identifying the telemetry source.

**EAN-04** When a requested session has no parsed events, the system shall return a session-not-found error.

**EAN-05** When aggregating one session, the system shall order events chronologically and require a session-start event.

**EAN-06** When no session-completion event exists, the system shall mark the session incomplete and measure it through the current time.

**EAN-07** When phase start and completion events exist, the system shall pair them by phase, preserve completion metadata, and order phases by start time.

**EAN-08** When phase gaps exceed the configured wait threshold, the system shall classify those gaps as wait time; otherwise, it shall classify non-negative gaps as AI transition time.

**EAN-09** When multiple sessions are aggregated, the system shall continue past invalid sessions and sort valid sessions newest first.

**EAN-10** When summary metrics are computed, the system shall aggregate status counts, duration, AI time, wait time, and cost and compute averages over all sessions.

**EAN-11** When reports are requested, the system shall render session data in Markdown, JSON, or CSV without changing the underlying metrics.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_analysis_configuration_guardrails.feature`
- Package tests: `engram/internal/analytics/*_test.go`
