# Backlog Suggestion Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/backlog-suggest` exposes deterministic backlog parsing, eligibility, and
ranking for human and VROOM dispatch consumers.

## EARS Requirements

**BSC-01** When no source files are supplied, the command shall use an empty source and shall not read temporal files from dear-agent.

**BSC-02** When custom files are supplied, the command shall trim comma-separated paths and ignore empty entries.

**BSC-03** When list mode is selected, the command shall render every parsed item's status, identifier, priority, effort, and title or encode the same data as JSON.

**BSC-04** When suggest mode is selected, the command shall apply phase, capacity, and maximum-effort constraints through the shared deterministic suggester.

**BSC-05** When an effort flag is unknown or empty, the command shall treat it as no effort cap rather than inventing a size.

**BSC-06** When VROOM emission is selected and suggestions exist, the command shall append one private JSONL dispatch event for the top-ranked item.

**BSC-07** When no suggestions exist, the command shall not emit a dispatch event.

**BSC-08** When source loading, ranking, encoding, or event persistence fails, the command shall return a runtime error distinct from usage failure.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_intelligence_command_guardrails.feature`
- Package tests: `cmd/backlog-suggest/*_test.go`
