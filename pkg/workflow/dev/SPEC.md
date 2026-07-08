# Workflow Dev Package Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/workflow/dev` powers the interactive workflow development shell. It
provides fixture-backed AI responses, reloadable workflow sessions, node retry,
diffing, and file watching without coupling dev-mode behavior into the runner.

## Requirements

**WORKFLOW-DEV-PKG-01** When a fixture file is missing, the system shall return an empty fixture set without error.

**WORKFLOW-DEV-PKG-02** When fixture YAML includes `_default`, the system shall use it as the fallback response for missing node-specific fixtures.

**WORKFLOW-DEV-PKG-03** When fixture YAML is reloaded, the system shall replace responses and default text atomically.

**WORKFLOW-DEV-PKG-04** When the mock AI executor has no matching fixture and `FailOnGap` is false, the system shall return a placeholder response.

**WORKFLOW-DEV-PKG-05** When the mock AI executor has no matching fixture and `FailOnGap` is true, the system shall return a fixture gap error.

**WORKFLOW-DEV-PKG-06** When a session is created without an explicit fixtures path, the system shall use the conventional `<workflow>.fixtures.yaml` companion path.

**WORKFLOW-DEV-PKG-07** When a session runs with `live=false`, the system shall execute with the fixture-backed mock AI executor.

**WORKFLOW-DEV-PKG-08** When a session runs with `live=true` and no live AI executor is configured, the system shall return an error.

**WORKFLOW-DEV-PKG-09** When retrying a node, the system shall execute a single-node workflow seeded with outputs from the last successful run.

**WORKFLOW-DEV-PKG-10** When diffing a node with fewer than two runs, the system shall return an explicit error.

**WORKFLOW-DEV-PKG-11** When hot reload is started with no paths, the system shall return an error.

**WORKFLOW-DEV-PKG-12** When watched workflow or fixture files change, the system shall debounce events before invoking the change callback.

## BDD Traceability

- `agm/test/bdd/features/workflow_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
