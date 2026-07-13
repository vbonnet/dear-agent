# Dear Agent Search Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/dear-agent-search` queries the shared SQLite source index and annotates
results with workflow run context for any harness or model consumer.

## EARS Requirements

**DAS-01** When no query, work-item, cue, or time filter is supplied, the command shall return a usage error rather than running an unbounded search.

**DAS-02** When no source database is supplied, the command shall use the runs database so workflow annotations remain available.

**DAS-03** When multiple cue flags are supplied, the command shall preserve them as conjunctive source filters.

**DAS-04** When time filters use days, Go durations, dates, or RFC3339 timestamps, the command shall normalize them to UTC bounds.

**DAS-05** When a work item contains a run and node identifier, the command shall split and preserve both components.

**DAS-06** When source results map to workflow runs, the command shall annotate run state; when no run exists, it shall retain the source result without failing the query.

**DAS-07** When JSON output is selected, the command shall emit the same bounded result fields used by text rendering.

**DAS-08** When interrupted, the command shall propagate cancellation to source fetching and workflow annotation queries.

**DAS-09** When database initialization or fetching fails, the command shall return an adapter error without model-specific fallback behavior.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_intelligence_command_guardrails.feature`
- Package tests: `cmd/dear-agent-search/*_test.go`
