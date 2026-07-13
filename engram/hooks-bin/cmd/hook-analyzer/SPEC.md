# Hook Analyzer Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/hooks-bin/cmd/hook-analyzer` is the command adapter for analyzing hook
denial logs. It keeps Claude transcript correlation optional while exposing
text and JSON reports that can be consumed independently of a model family.

## EARS Requirements

**EHAC-01** When a log path starts with `~/`, the command shall expand it against the current user's home directory.

**EHAC-02** When `--since` is invalid, the command shall report the invalid value and exit unsuccessfully.

**EHAC-03** When transcript correlation is disabled, the command shall classify each parsed denial as unknown without reading transcripts.

**EHAC-04** When a pattern filter is supplied, the command shall retain only case-insensitive matching pattern analyses.

**EHAC-05** When JSON output is requested, the command shall emit the structured report; when text output is requested, it shall honor the top-pattern limit.

**EHAC-06** When an unsupported output format is requested, the command shall report the accepted formats and exit unsuccessfully.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks-bin/cmd/hook-analyzer/*_test.go`
