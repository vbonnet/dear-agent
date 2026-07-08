# AGM Workflow Registry Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/workflow` defines AGM's named workflow abstraction and registry.
It keeps workflow lookup, harness compatibility, and result/artifact contracts
separate from individual workflow implementations.

## Requirements

**AGM-WORKFLOW-01** When a workflow implementation is registered, the system shall store it by workflow name and replace any previous implementation with the same name.

**AGM-WORKFLOW-02** When a workflow is requested by name, the system shall return the registered workflow and a found flag without mutating registry state.

**AGM-WORKFLOW-03** When workflows are listed, the system shall return them sorted by workflow name.

**AGM-WORKFLOW-04** When workflows are listed for a harness, the system shall include only workflows whose supported harness list contains that harness name.

**AGM-WORKFLOW-05** When compatibility is validated for an unknown workflow, the system shall return an error that names available workflows.

**AGM-WORKFLOW-06** When compatibility is validated for an unsupported harness, the system shall return an error that names the workflow's supported harnesses.

**AGM-WORKFLOW-07** When compatibility is validated for a supported workflow and harness pair, the system shall return no error.

**AGM-WORKFLOW-08** When workflows execute, the system shall pass harness, session id, prompt, working directory, output path, and environment through the workflow context.

**AGM-WORKFLOW-09** When workflows complete, the system shall report success, artifacts, summary, log path, metadata, and execution time through the workflow result.

## BDD Traceability

- `agm/test/bdd/features/workflow_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
