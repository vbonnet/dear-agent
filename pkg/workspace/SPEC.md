# Workspace Resolution Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**WORKSPACE-01** When workspace configuration is loaded or saved, the system shall validate version, unique names, enabled entries, default identity, and private file permissions.

**WORKSPACE-02** When workspace paths are expanded, the system shall normalize home, environment, relative, and parent components into absolute paths.

**WORKSPACE-03** When workspace detection runs, the system shall prefer explicit selection, environment selection, path matching, configured default, and optional prompt in that order.

**WORKSPACE-04** When workspace settings are resolved, the system shall apply the documented seven-level precedence cascade.

**WORKSPACE-05** When workspace environment files are saved or displayed, the system shall preserve values on disk privately and mask sensitive values in output.

**WORKSPACE-06** When git identity is configured, the system shall generate scoped include files and diagnose mismatched workspace identity.

**WORKSPACE-07** When registry entries are added or removed, the system shall validate and persist the resulting registry without duplicate workspace names.

**WORKSPACE-08** While workspaces are selected by any supported harness and model family, the system shall preserve identical detection and setting precedence.

## BDD Traceability

- Feature: `agm/test/bdd/features/validation_workspace_parity.feature`

## Test Traceability

- Unit package: `pkg/workspace`
