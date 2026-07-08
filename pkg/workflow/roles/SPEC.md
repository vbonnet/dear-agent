# Workflow Roles Package Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/workflow/roles` maps workflow AI roles to concrete model tiers. It loads
role registries, validates tier structure, and resolves the first model that
satisfies override, capability, capacity, and budget constraints.

## Requirements

**WORKFLOW-ROLES-PKG-01** When a roles file is loaded, the system shall parse YAML and validate the registry before returning it.

**WORKFLOW-ROLES-PKG-02** When auto-loading roles, the system shall prefer the explicit environment path before the working-directory role file.

**WORKFLOW-ROLES-PKG-03** When no explicit or local role file exists, the system shall prefer the user-level role file before built-in roles.

**WORKFLOW-ROLES-PKG-04** When no role files are found, the system shall return the built-in registry and source marker `<builtin>`.

**WORKFLOW-ROLES-PKG-05** When a registry role has no primary, secondary, or tertiary tier, the system shall reject the registry.

**WORKFLOW-ROLES-PKG-06** When a registry tier lacks a model id, the system shall reject the registry.

**WORKFLOW-ROLES-PKG-07** When role names are listed, the system shall return them in deterministic alphabetical order.

**WORKFLOW-ROLES-PKG-08** When a model override is provided in a resolution request, the system shall return that model with tier name `override`.

**WORKFLOW-ROLES-PKG-09** When no role is provided and a legacy model is provided, the system shall return that model with tier name `model`.

**WORKFLOW-ROLES-PKG-10** When a role is provided, the system shall evaluate tiers in primary, secondary, tertiary order.

**WORKFLOW-ROLES-PKG-11** When required capabilities are provided, the system shall skip tiers whose role and tier capabilities do not cover every requirement.

**WORKFLOW-ROLES-PKG-12** When a capacity checker rejects a tier model, the system shall skip that tier.

**WORKFLOW-ROLES-PKG-13** When `MaxDollars` is positive and a tier input cost exceeds it, the system shall skip that tier.

**WORKFLOW-ROLES-PKG-14** When no tier passes filters, the system shall return `ErrNoModelAvailable`.

## BDD Traceability

- `agm/test/bdd/features/workflow_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
