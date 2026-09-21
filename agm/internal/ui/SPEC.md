# AGM Operator UI Specification

<!-- Last audited at: 2026-08-14 -->

## Overview

`agm/internal/ui` provides accessible operator forms, pickers, confirmations,
themes, hierarchy views, tables, JSON output, and cleanup selection.

## Requirements

**UI-01** When UI configuration is absent, the system shall use complete defaults and merge partial configuration without clearing omitted values.

**UI-02** When no-color or screen-reader mode is enabled, the system shall remove color dependence and provide textual status equivalents.

**UI-03** When rendering session tables or hierarchies, the system shall preserve status grouping, parent-child structure, stable ordering, and bounded column widths.

**UI-04** When JSON output is requested, the system shall emit the maintained session schema and represent an empty result consistently.

**UI-05** When an operator form or picker has no valid choices, the system shall return an explicit error instead of presenting an unusable interaction.

**UI-06** When cleanup options have duplicate names or other display collisions, the system shall keep each visible option and action bound to its distinct stable session ID and return only the selected archive and delete targets.

**UI-07** When confirmation is disabled by configuration, the system shall follow the configured non-interactive default deterministically.

**UI-08** When an operator confirms batch cleanup, the system shall show every selected target with its stable session ID and the irreversible-delete warning in both standard and accessible terminal modes before accepting input; if that list cannot be displayed, the system shall not confirm.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_product_surface_guardrails.feature`
- Package tests: `agm/internal/ui/*_test.go`
- Shared configuration/default tests: `agm/internal/config/config_strict_test.go`
- Root projection test: `agm/cmd/agm/new_sandbox_test.go`
