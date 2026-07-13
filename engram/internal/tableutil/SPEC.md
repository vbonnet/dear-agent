# Engram Table Formatting Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/tableutil` renders tabular results for terminals, Markdown,
CSV, and JSON consumers.

## EARS Requirements

**ETB-01** When a table has no rows, the system shall return empty rendered Markdown output.

**ETB-02** When a table has a title, headers, and rows, the system shall render a stable terminal table and plain Markdown table without changing cell order.

**ETB-03** When a title is absent, the system shall omit the title line while preserving the table structure.

**ETB-04** When CSV is formatted, the system shall use standard CSV quoting for delimiters, quotes, and structured cell values.

**ETB-05** When JSON is formatted, the system shall use the standard JSON encoder and return an error for unsupported values.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_cli_support_guardrails.feature`
- Package tests: `engram/internal/tableutil/table_test.go`
