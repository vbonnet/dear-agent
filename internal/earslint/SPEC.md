# EARS Linter Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/earslint`.

## Overview

`internal/earslint` validates requirement statements against the repository's
EARS grammar. It is the deterministic SPEC gate used by local development and
CI regardless of the authoring harness or model family.

## EARS Requirements

**EARSLINT-01** When a linter is created without custom patterns, the system shall compile and apply the repository default EARS patterns.

**EARSLINT-02** When custom patterns are configured, the system shall apply valid overrides and reject invalid regular expressions.

**EARSLINT-03** When Markdown requirements include identifiers, list markers, emphasis, or inline code, the system shall normalize presentation syntax before matching the EARS grammar.

**EARSLINT-04** When text appears inside fenced code, the system shall exclude that text from requirement candidates.

**EARSLINT-05** When strict mode is enabled, the system shall fail on any nonconforming candidate requirement.

**EARSLINT-06** When a specification has no valid EARS requirements, the system shall report an error finding.

**EARSLINT-07** When linter configuration is missing or malformed, the system shall return an explicit configuration diagnostic.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`

## Test Traceability

- Unit package: `internal/earslint`
