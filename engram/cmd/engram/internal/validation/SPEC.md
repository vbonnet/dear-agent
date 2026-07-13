# Engram Command Validation Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/cmd/engram/internal/validation` validates project, Engram, path, and
template arguments before command execution.

## EARS Requirements

**ECV-01** When a project name is empty or contains characters outside letters, digits, hyphens, and underscores, the system shall reject it.

**ECV-02** When an Engram name is empty or contains characters unsafe for a Markdown filename, the system shall reject it.

**ECV-03** When a project path is empty or cannot be resolved to a cleaned absolute path, the system shall reject it.

**ECV-04** When a template is empty or absent from the available template set, the system shall return an invalid-input error listing supported choices.

**ECV-05** When command validation succeeds, the system shall preserve the caller's value without silently rewriting it.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_cli_support_guardrails.feature`
- Package tests: `engram/cmd/engram/internal/validation/validator_test.go`
