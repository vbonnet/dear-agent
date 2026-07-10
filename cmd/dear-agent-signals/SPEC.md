# Dear Agent Signals Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/dear-agent-signals` collects, scores, filters, and reports shared
operational signals for cross-harness supervision.

## EARS Requirements

**DASG-01** When no subcommand or an unknown subcommand is supplied, the command shall print usage and return the usage code.

**DASG-02** When collection runs, the command shall gather supported repository and runtime signal sources into the shared signal store.

**DASG-03** When report filters select a signal kind, the command shall reject unknown kinds and preserve supported kinds.

**DASG-04** When a report is empty, the command shall render an explicit empty state rather than failing.

**DASG-05** When signals are reported, the command shall preserve timestamps, kinds, sources, scores, and summaries in text and JSON forms.

**DASG-06** When salience input is empty, the command shall return the documented empty-input behavior without manufacturing a score.

**DASG-07** When bypass markers are invalid, the command shall reject them rather than weakening salience filtering.

**DASG-08** When noise retention is disabled, the command shall filter low-salience noise according to the shared scoring policy.

**DASG-09** When noise retention is enabled, the command shall preserve low-salience records while still reporting their scores.

**DASG-10** When signals originate from any active harness or model family, the command shall apply the same storage and salience contracts.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_intelligence_command_guardrails.feature`
- Package tests: `cmd/dear-agent-signals/*_test.go`
