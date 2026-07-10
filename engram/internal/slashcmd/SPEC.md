# Engram Slash Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/slashcmd` parses enhanced slash-command metadata, validates
required parameters, and provides bounded autocomplete choices.

## EARS Requirements

**ESL-01** When a command file begins with delimited YAML frontmatter, the system shall parse command metadata and preserve the following Markdown body.

**ESL-02** When frontmatter is absent, the system shall treat the complete file as the body with empty command metadata.

**ESL-03** When frontmatter is unclosed or malformed, the system shall return a parse error rather than executing partial metadata.

**ESL-04** When a parameter declares static choices, the system shall return those values without invoking a process.

**ESL-05** When dynamic autocomplete is requested, the system shall execute only allowlisted program names as direct argv without shell interpretation and with a bounded timeout.

**ESL-06** When an autocomplete program is empty, path-qualified, or absent from the allowlist, the system shall reject it before execution.

**ESL-07** When autocomplete succeeds, the system shall trim output and return non-empty lines as choices.

**ESL-08** When required command parameters are absent, the system shall return one validation error per missing parameter.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_cli_support_guardrails.feature`
- Package tests: `engram/internal/slashcmd/parser_test.go`
