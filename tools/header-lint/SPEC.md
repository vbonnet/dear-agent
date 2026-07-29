# Header Lint Command Specification

<!-- Last audited at: 2026-07-27 -->

## Overview

`tools/header-lint` is the thin command adapter for `pkg/headerlint`. It
checks a tracked repository, one Markdown file, or one or more directories
for the single-line bold metadata "header block" anti-pattern documented in
[`docs/doc-header-format.md`](../../docs/doc-header-format.md).

## EARS Requirements

**HEADERLINT-CMD-01** When no validation mode is selected, the system shall print usage and exit with code 2.

**HEADERLINT-CMD-02** When more than one of repository, file, or directory mode is selected, the system shall print usage and exit with code 2.

**HEADERLINT-CMD-03** When `-repo <root>` is provided, the system shall call `pkg/headerlint.CheckRepository` for that root.

**HEADERLINT-CMD-04** When `-file <path>` is provided, the system shall call `pkg/headerlint.CheckFile` only for that path.

**HEADERLINT-CMD-05** When directory arguments are provided, the system shall call `pkg/headerlint.CheckDir` for each directory.

**HEADERLINT-CMD-06** When validation returns an operational error, the system shall print the error to stderr and exit with code 2.

**HEADERLINT-CMD-07** When validation returns content violations, the system shall print each violation to stderr and exit with code 1.

**HEADERLINT-CMD-08** When validation returns no violations, the system shall exit with code 0.

## BDD Traceability

- Feature: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
- Command tests: `tools/header-lint/main_test.go`
