# Wayfinder Workspace Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`wayfinder/cmd/wayfinder-session/internal/workspace` detects workspace
ownership for Wayfinder projects and lists Wayfinder projects inside a workspace
root. It prefers explicit environment state, falls back to AGM session metadata,
then infers workspace names from supported path layouts.

## EARS Requirements

**WAYFINDER-WORKSPACE-01** When `WORKSPACE` is set to a valid workspace name, the system shall return it before consulting AGM metadata or path patterns.

**WAYFINDER-WORKSPACE-02** When `WORKSPACE` is empty or invalid, the system shall attempt to read the current AGM or Claude session manifest for a workspace value.

**WAYFINDER-WORKSPACE-03** When a current-session manifest is JSON and declares a valid workspace, the system shall return that workspace.

**WAYFINDER-WORKSPACE-04** When no explicit or manifest workspace is available, the system shall detect production paths shaped as `ws/<workspace>/wf/<project>`.

**WAYFINDER-WORKSPACE-05** When no production path is available, the system shall detect test paths shaped as `<workspace>/wf/<project>`.

**WAYFINDER-WORKSPACE-06** When a workspace name is empty or contains characters outside letters, digits, hyphen, and underscore, the system shall reject it.

**WAYFINDER-WORKSPACE-07** When a workspace root does not exist, the system shall return an empty project list rather than an error.

**WAYFINDER-WORKSPACE-08** When projects are listed, the system shall walk for `WAYFINDER-STATUS.md`, load valid status files, skip unreadable or invalid entries, and include project path, session ID, status, current phase, and detected workspace.

**WAYFINDER-WORKSPACE-09** When workspace isolation is validated, the system shall compare the detected workspace for the project path to the expected workspace exactly.

**WAYFINDER-WORKSPACE-10** When the workspace root is requested from a project path, the system shall return the path through `ws/<workspace>/wf` only for valid production layouts.

## BDD Traceability

- Feature: `agm/test/bdd/features/wayfinder_lifecycle_guardrails.feature`
- Package tests: `wayfinder/cmd/wayfinder-session/internal/workspace/workspace_detection_test.go`
- Package tests: `wayfinder/cmd/wayfinder-session/internal/workspace/workspace_isolation_test.go`
- Package tests: `wayfinder/cmd/wayfinder-session/internal/workspace/testdata_generator_test.go`

