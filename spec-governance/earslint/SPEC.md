# EARS Linter Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `spec-governance/earslint`.

## Overview

`spec-governance/earslint` owns the repository's single EARS grammar
implementation. The public package is shared by the repository gates and the
distributable SPEC audit command so plugin execution does not depend on a
root-only internal package.

## EARS Requirements

**EARSLINT-01** When a linter is created without custom patterns, the system shall compile and apply the repository default EARS patterns.

**EARSLINT-02** When custom patterns are configured, the system shall apply valid overrides and reject invalid regular expressions.

**EARSLINT-03** When Markdown requirements include identifiers, list markers, emphasis, or inline code, the system shall normalize presentation syntax before matching the EARS grammar.

**EARSLINT-04** When text appears inside fenced code, the system shall exclude that text from requirement candidates.

**EARSLINT-05** When strict mode is enabled, the system shall fail on any nonconforming candidate requirement.

**EARSLINT-06** When a specification has no valid EARS requirements, the system shall report an error finding.

**EARSLINT-07** When linter configuration is missing or malformed, the system shall return an explicit configuration diagnostic.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_governance_tooling.feature`

## Test Traceability

- Unit package: `spec-governance/earslint`
