# SPEC governance launcher specification

## EARS Requirements

**SPEC-GOV-LAUNCH-01** When the installed audit launcher runs, the system shall execute the collector from its own resolved distribution root.

**SPEC-GOV-LAUNCH-02** When the installed audit launcher runs, the system shall disable inherited Go workspaces, flags, environment files, and toolchain selection before executing the collector.

**SPEC-GOV-LAUNCH-03** When an audit command writes an artifact, the system shall leave the destination selection to the caller's standard-output redirection.

**SPEC-GOV-LAUNCH-04** If the launcher cannot resolve an absolute Go executable selected from the caller's `PATH` at a stable three-component version 1.26.5 or newer, then the system shall stop before compilation with a diagnostic naming the required and detected versions.

**SPEC-GOV-LAUNCH-05** When the installed audit launcher runs with fresh empty module and build caches, the system shall execute its standard-library-only collector with module lookup disabled and without network access.

**SPEC-GOV-LAUNCH-06** When the installed audit launcher dispatches Go, the system shall neutralize inherited workspace, build-flag, environment-file, toolchain, platform, experiment, proxy, checksum, VCS, CGO compiler, and external cache-program settings that could change code selection or trigger external execution.

**SPEC-GOV-LAUNCH-07** When the installed audit launcher dispatches Go, it shall compile only the exact declared non-test Go source inventory for the isolated distribution and shall stop before compilation if that inventory contains an undeclared or stale source file.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_governance_tooling.feature`
