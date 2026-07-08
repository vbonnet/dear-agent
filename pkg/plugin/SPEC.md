# Plugin Package Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/plugin` defines first-party plugin manifests, filesystem discovery, and
runtime composition for workflow hooks, audit checks, and audit verifiers. It
keeps declared plugin capabilities aligned with the Go interfaces a plugin
actually implements.

## Requirements

**PLUGIN-PKG-01** When a manifest is validated, the system shall accept only API version `dear-agent.io/v1`.

**PLUGIN-PKG-02** When a manifest is validated, the system shall accept only kind `Plugin`.

**PLUGIN-PKG-03** When a manifest name is empty, contains control characters, or has surrounding whitespace, the system shall reject the manifest.

**PLUGIN-PKG-04** When a manifest declares an unknown capability, the system shall reject the manifest.

**PLUGIN-PKG-05** When manifest filesystem permissions contain absolute read or write paths, the system shall reject the manifest.

**PLUGIN-PKG-06** When filesystem discovery scans a missing directory, the system shall return no manifests and no errors.

**PLUGIN-PKG-07** When filesystem discovery scans a directory, the system shall support both top-level YAML manifests and per-plugin `plugin.yaml` subdirectories.

**PLUGIN-PKG-08** When filesystem discovery encounters hidden entries, the system shall skip names starting with `.` or `_`.

**PLUGIN-PKG-09** When filesystem discovery encounters one malformed manifest, the system shall return sibling valid manifests and the per-file error.

**PLUGIN-PKG-10** When a plugin is registered, the system shall reject capability declarations that do not match implemented provider interfaces.

**PLUGIN-PKG-11** When registered plugins are listed for presentation, the system shall return a snapshot sorted by manifest name.

**PLUGIN-PKG-12** When hook providers are composed, the system shall run hooks in registration order.

**PLUGIN-PKG-13** When composed `OnDefine` or `OnEnforce` hooks return an error, the system shall short-circuit remaining providers.

**PLUGIN-PKG-14** When composed `OnAudit` or `OnResolve` hooks return errors, the system shall run every provider and join returned errors.

**PLUGIN-PKG-15** When applying plugin audit checks or verifiers, the system shall delegate duplicate and metadata validation to the target audit registry.

## BDD Traceability

- `agm/test/bdd/features/plugin_skill_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
