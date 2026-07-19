# Backlog Suggestion Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/backlog-suggest` exposes deterministic parsing, eligibility, and ranking
for explicitly supplied Markdown snapshots. Beads owns Dear Agent's live work;
this command is a read-only inspection utility, not a dispatcher.

## EARS Requirements

**BSC-01** When no source files are supplied, the command shall return a usage error without reading repository defaults.

**BSC-02** When custom files are supplied, the command shall trim comma-separated paths and ignore empty entries.

**BSC-03** When list mode is selected, the command shall render every parsed item's status, identifier, priority, effort, and title or encode the same data as JSON.

**BSC-04** When suggest mode is selected, the command shall apply phase, capacity, and maximum-effort constraints through the shared deterministic suggester.

**BSC-05** When an effort flag is unknown or empty, the command shall treat it as no effort cap rather than inventing a size.

**BSC-06** The command shall not expose a VROOM dispatch-ledger emission option.

**BSC-07** When no suggestions exist, the command shall return an empty result without manufacturing a work item.

**BSC-08** When source loading, ranking, or encoding fails, the command shall return a runtime error distinct from usage failure.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_intelligence_command_guardrails.feature`
- Package tests: `cmd/backlog-suggest/*_test.go`
