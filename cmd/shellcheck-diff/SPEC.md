# ShellCheck Diff Command Specification

<!-- Last audited at: 2026-08-19 -->

## Overview

`cmd/shellcheck-diff` decides which ShellCheck findings a change introduced.
It reads a unified diff and a ShellCheck JSON1 document and reports only the
findings anchored to lines the change added or rewrote, so a repository-wide
gate can run above the severity its pre-existing debt would otherwise pin it
to.

## EARS Requirements

**SHDIFF-01** When a finding sits on a line the diff added or rewrote and its severity is at or above the configured threshold, the command shall report it and exit non-zero.

**SHDIFF-02** When a finding sits on a line the diff did not touch, the command shall not report it, regardless of its severity.

**SHDIFF-03** When a finding's severity is below the configured threshold, the command shall not report it, regardless of whether the change touched its line.

**SHDIFF-04** When the findings document is absent, unreadable, or not a ShellCheck JSON1 document, the command shall fail with a diagnostic rather than report a clean result.

**SHDIFF-05** When the findings document is the legacy bare-array `-f json` format, the command shall reject it by name rather than silently report zero findings.

**SHDIFF-06** When a finding omits its file or a positive line number, or carries a severity level ShellCheck does not define, the command shall reject the document rather than skip that finding.

**SHDIFF-07** When a hunk header carries no parsable destination range, the command shall fail rather than attribute the hunk to no lines.

**SHDIFF-08** When the diff records a deletion whose destination is `/dev/null`, the command shall attribute no lines to that path.

**SHDIFF-09** When either `--diff` or `--findings` is omitted, the command shall fail with a usage error.

**SHDIFF-10** When no finding qualifies, the command shall report the number of findings inspected and exit zero.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
- Workflow: `.github/workflows/shell-lint.yml`
- Package tests: `cmd/shellcheck-diff/*_test.go`
- Pattern documentation: `docs/testing-non-go-sources.md`
