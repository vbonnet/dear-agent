# Local GitHub Actions Executor Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/ci/act`.

## Overview

`internal/ci/act` implements the shared pipeline contract through `nektos/act`.
It validates host dependencies, protects temporary secrets, distinguishes
pipeline outcomes from executor failures, and coordinates multiple workflows.

## EARS Requirements

**ACT-01** When an act pipeline is validated, the system shall require an executable act binary, an existing workflow, and an available Docker runtime.

**ACT-02** When an act command is built, the system shall pass workflow inputs as process arguments without interpolating them through a command shell.

**ACT-03** When pipeline secrets are supplied, the system shall write them to a private temporary file and remove that file after execution.

**ACT-04** When the act process exits with a workflow failure, the system shall return an unsuccessful pipeline result instead of classifying the outcome as an infrastructure error.

**ACT-05** When execution is cancelled or exceeds its deadline, the system shall terminate the process and return the corresponding infrastructure diagnostic.

**ACT-06** When an output callback is configured, the system shall emit start, output, and end events for the pipeline.

**ACT-07** When workflows run sequentially, the system shall preserve order and skip remaining workflows after a required failure.

**ACT-08** When workflows have dependencies, the system shall run independent workflows concurrently and skip dependents whose prerequisites failed.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`

## Test Traceability

- Unit package: `internal/ci/act`
