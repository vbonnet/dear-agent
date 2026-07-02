# SPEC and BDD Coverage Specification

<!-- Last audited at: 2026-07-01 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** Parity-critical SPEC.md and BDD feature coverage.

## Overview

`internal/speccoverage` owns the repository governance matrix that links
parity-critical implementation surfaces to both a co-located `SPEC.md` and an
executable AGM BDD feature. The matrix is intentionally narrower than the full
legacy repository: it enforces the harness/model parity surfaces first while
the larger repo-wide SPEC backlog is burned down incrementally.

## EARS Requirements

**SCB-01** When AGM defines a parity-critical implementation surface, the system shall register that surface in the SPEC/BDD coverage matrix.

**SCB-02** When a parity-critical surface is registered, the system shall require its `SPEC.md` file to exist.

**SCB-03** When a parity-critical surface is registered, the system shall require its executable BDD feature file to exist under `agm/test/bdd/features/`.

**SCB-04** When a parity-critical `SPEC.md` file is present, the system shall require an `## EARS Requirements` section.

**SCB-05** When a parity-critical BDD feature is present, the system shall require a `Feature:` declaration.

**SCB-06** When a new `*_parity.feature` file is added, the system shall fail unless that feature is represented in the SPEC/BDD coverage matrix.

**SCB-07** When a pull request changes a production Go file outside test-only directories, the system shall require a co-located `SPEC.md` in that file's package directory.

**SCB-08** When a pull request changes only Go test files or files under test-only directories, the system shall not require a new package `SPEC.md` solely for that test-only change.

**SCB-09** When a parity-critical `SPEC.md` file is registered in the coverage matrix, the system shall require a completed `Last audited at` marker and shall reject the placeholder value `NEEDS-AUDIT`.

**SCB-10** When AGM defines an executable BDD feature file, the system shall require that feature to be listed in `agm/docs/BDD-CATALOG.md`.

**SCB-11** When `agm/docs/BDD-CATALOG.md` lists a BDD feature file, the system shall require that referenced feature file to exist under `agm/test/bdd/features/`.

**SCB-12** When a parity-critical `SPEC.md` file is registered in the coverage matrix, the system shall require every candidate requirement to pass strict EARS lint.

**SCB-13** When a pull request changes a production Go package with a co-located `SPEC.md`, the system shall require that co-located `SPEC.md` to pass strict EARS lint.

**SCB-14** When a parity-critical `SPEC.md` file is registered in the coverage matrix, the system shall require that `SPEC.md` to reference its executable BDD feature path.

**SCB-15** When a parity-critical BDD feature is registered in the coverage matrix, the system shall require that feature to reference its governing `SPEC.md` path.
