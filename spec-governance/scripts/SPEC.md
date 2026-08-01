# SPEC governance launcher specification

## EARS Requirements

**SPEC-GOV-LAUNCH-01** When the installed audit launcher runs, the system shall execute the collector from its own authenticated distribution root.

**SPEC-GOV-LAUNCH-02** When the installed audit launcher runs, the system shall disable inherited Go workspaces, flags, environment files, and toolchain selection before executing the collector.

**SPEC-GOV-LAUNCH-03** When an audit command writes an artifact, the system shall leave the destination selection to the caller's standard-output redirection.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_governance_tooling.feature`
