# Workflow Codemod Package Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/workflow/codemod` transforms workflow YAML across schema eras and converts
Wayfinder session YAML into workflow YAML. It preserves operator-authored
content unless a requested transformation has a defensible mechanical mapping.

## Requirements

**WORKFLOW-CODEMOD-PKG-01** When upgrading workflow YAML, the system shall parse the document as YAML and reject non-mapping top-level documents.

**WORKFLOW-CODEMOD-PKG-02** When an upgraded workflow lacks `schema_version`, the system shall add `schema_version: "1"`.

**WORKFLOW-CODEMOD-PKG-03** When an AI node uses a model in the model-to-role mapping and lacks `role`, the system shall add the mapped role.

**WORKFLOW-CODEMOD-PKG-04** When `DropModelOnRolePromotion` is true and a model is promoted to a role, the system shall remove the original model field.

**WORKFLOW-CODEMOD-PKG-05** When `AddDefaultBudget` is true and an AI node lacks a budget block, the system shall add the default budget block.

**WORKFLOW-CODEMOD-PKG-06** When no transformations are needed, the system shall return the original bytes unchanged.

**WORKFLOW-CODEMOD-PKG-07** When upgrading loop nodes, the system shall apply AI node transformations inside loop bodies.

**WORKFLOW-CODEMOD-PKG-08** When converting a Wayfinder session with roadmap phases, the system shall synthesize one workflow node per phase with linear dependencies.

**WORKFLOW-CODEMOD-PKG-09** When a Wayfinder session lacks roadmap phases, the system shall fall back to waypoint history entries.

**WORKFLOW-CODEMOD-PKG-10** When a Wayfinder session has no inferable workflow content, the system shall return a conversion error.

## BDD Traceability

- `agm/test/bdd/features/workflow_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
