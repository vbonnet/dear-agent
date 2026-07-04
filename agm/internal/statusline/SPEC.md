# AGM Status Line Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`agm/internal/statusline` renders tmux status-line text from AGM session
status data. It validates templates, rejects missing data, and provides shared
default templates for state, context, cost, branch, and harness display.

## EARS Requirements

**SLINE-01** When a formatter is created, the system shall reject an empty template string.

**SLINE-02** When a formatter is created, the system shall parse the template and return an error for invalid template syntax.

**SLINE-03** When formatting status-line data, the system shall reject nil status-line data before executing the template.

**SLINE-04** When formatting succeeds, the system shall render the configured template against session status-line data.

**SLINE-05** When default templates are requested, the system shall provide templates for default, minimal, compact, multi-agent, and full status-line layouts.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
