# SPEC and BDD Coverage Specification

<!-- Last audited at: 2026-08-08 -->

**Version:** 1.1
**Status:** Enforced
**Scope:** Repository-wide SPEC.md and executable BDD feature coverage.

## Overview

`internal/speccoverage` owns the repository governance matrix that links
parity-critical implementation surfaces to one governed `SPEC.md` owner and an
executable AGM BDD feature. An implementation may declare that owner locally or
through a checked `SPEC.owner` edge to a canonical shared contract. The module
also discovers every implementation directory in the checkout and enforces
strict EARS and reciprocal BDD traceability across production, test, support,
configuration, service, and executable sources.

## EARS Requirements

**SCB-01** When AGM defines a parity-critical implementation surface, the system shall register that surface in the SPEC/BDD coverage matrix.

**SCB-02** When a parity-critical surface is registered, the system shall require its `SPEC.md` file to exist.

**SCB-03** When a parity-critical surface is registered, the system shall require its executable BDD feature file to exist as a canonically named direct child of `agm/test/bdd/features/` so the runner, catalog, parity registry, and coverage scanners observe the same flat parseable suite.

**SCB-04** When a parity-critical `SPEC.md` file is present, the system shall require an `## EARS Requirements` section.

**SCB-05** When a parity-critical BDD feature is present, the system shall require a `Feature:` declaration.

**SCB-06** When a new `*_parity.feature` file is added, the system shall fail unless that feature is represented in the SPEC/BDD coverage matrix.

**SCB-07** When a pull request changes a production Go file outside test-only directories, the system shall require that file's package directory to declare exactly one governed contract through a co-located `SPEC.md` or `SPEC.owner` reference.

**SCB-08** When a pull request changes only Go test files or files under test-only directories, the system shall not require a new package `SPEC.md` solely for that test-only change.

**SCB-09** When a parity-critical `SPEC.md` file is registered in the coverage matrix, the system shall require a completed `Last audited at` marker and shall reject the placeholder value `NEEDS-AUDIT`.

**SCB-10** When AGM defines an executable BDD feature file, the system shall require that feature to be listed in `agm/docs/BDD-CATALOG.md`.

**SCB-11** When `agm/docs/BDD-CATALOG.md` lists a BDD feature file, the system shall require that referenced feature file to exist under `agm/test/bdd/features/`.

**SCB-12** When a parity-critical `SPEC.md` file is registered in the coverage matrix, the system shall require every candidate requirement to pass strict EARS lint.

**SCB-13** When a pull request changes a production Go package, the system shall resolve its one local or shared governed `SPEC.md` owner and require that canonical contract to pass strict EARS lint.

**SCB-14** When a parity-critical `SPEC.md` file is registered in the coverage matrix, the system shall require that `SPEC.md` to reference its executable BDD feature path.

**SCB-15** When a parity-critical BDD feature is registered in the coverage matrix, the system shall require that feature to reference its governing `SPEC.md` path.

**SCB-16** When AGM defines an executable BDD feature file, the system shall require that feature to declare a governing `SPEC.md` path.

**SCB-17** When an executable BDD feature declares a governing `SPEC.md` path, the system shall require that `SPEC.md` file to exist and reference the executable BDD feature path.

**SCB-18** When repository-wide implementation coverage is validated, the system shall include production, test-only, and support directories while excluding dependency, build-output, and nested-worktree directories.

**SCB-19** When a repository implementation directory is discovered, the system shall require exactly one governed contract through a co-located `SPEC.md` or a `SPEC.owner` reference to a canonical shared `SPEC.md` with strict EARS requirements.

**SCB-20** When a repository implementation's governed `SPEC.md` owner is validated, the system shall require at least one referenced executable BDD feature.

**SCB-21** When a repository implementation's governed contract references an executable BDD feature, the system shall require the feature to exist, declare a `Feature`, and identify the canonical `SPEC.md` owner through `SPEC` or `RELATED-SPEC` traceability.

**SCB-22** When repository-wide implementation coverage is validated, the system shall include directories containing executable source, canonical extensionless `Dockerfile` and `Makefile` build files, or runtime configuration in the repository's supported programming, shell, infrastructure, data, service, and configuration formats.

**SCB-23** When an executable BDD feature declares a `RELATED-SPEC`, the system shall require that specification to exist and reference the executable feature.

**SCB-24** When the repository test suite runs, the system shall validate repository-wide implementation coverage against the actual checkout rather than only synthetic validator fixtures.

**SCB-25** When repository-wide coverage is validated, the system shall require every SPEC.md artifact, including doc-only and hidden policy contracts, to retain strict EARS and reciprocal executable BDD traceability.

**SCB-26** When repository-wide coverage discovers source or specification files, the system shall use the shared repository inventory so ignored dependencies, generated output, embedded databases, and nested worktrees do not create governance findings.

**SCB-27** When a caller selects a class of SPEC or BDD coverage findings, the system shall use the finding's stable typed kind rather than matching human-readable paths or messages.

**SCB-28** When an implementation directory uses `SPEC.owner`, the system shall require exactly one bounded canonical repository-relative `SPEC.md` path outside dotted or bare harness configuration, registration, plugin, and grouped harness roots and shall reject traversal, ambiguous local ownership, malformed content, missing targets, pointer chains, ignored targets, and nonregular declaration or target files.

**SCB-29** When an implementation directory reuses a canonical contract through `SPEC.owner`, the system shall apply the same strict EARS and reciprocal executable BDD checks to that shared owner as it applies to a co-located contract.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_coverage.feature`
