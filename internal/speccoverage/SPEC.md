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

