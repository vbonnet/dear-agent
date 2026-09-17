# Project Resource Surface Specification

<!-- Last audited at: 2026-09-17 -->

**Version:** 1.0
**Status:** Enforced
**Scope:** Registry coverage for repository-provided instructions, skills, configuration, and hooks across active AGM harnesses.

## Overview

Project resources are one harness-neutral inventory even though harnesses
consume them through different mechanisms. This module records the final
supported, adapted, unsupported, or not-applicable disposition for each
capability without claiming uniform parity. It is a registry and directory
ownership seam, not a second behavioral owner: instruction, marketplace,
configuration-directory, permission, launch, and hook policy stays in those
modules' canonical contracts.

## EARS Requirements

**PRS-01** When an active harness consumes project resources, the system shall record its instruction, skill, configuration, and hook capabilities as supported, adapted, unsupported, or not-applicable.

**PRS-02** When the active harness registry changes, the system shall reject a missing or implicit project-resource capability disposition.

**PRS-03** When the repository's Pi instruction projection is validated, the system shall require the root `AGENTS.md` and reject a divergent `.pi/AGENTS.md` copy.

## Canonical Ownership Redirects

The deleted `.pi/SPEC.md` used the following IDs for behavior already owned by
deeper modules. These rows preserve migration provenance without restating the
normative requirements.

| Former ID | Canonical owner |
| --- | --- |
| `PI-DIR-01` | `AGP-43` owns native Pi loading; `PRS-03` owns static repository conformance |
| `PI-DIR-02` | `MKT-03`, `MKT-05`, `DEAR-MARKET-06`, and Pi-specific Wayfinder behavior `WFP-09` |
| `PI-DIR-03` | `APP-11`; `LAUNCH-PARITY-10` remains builder-level conformance evidence |
| `PI-DIR-04` | `APP-15` |
| `PI-DIR-05` | `CDP-09` |
| `PI-DIR-06` | `HHP-08`; `APP-16` owns hook-failure semantics |

## BDD Traceability

- Feature: `agm/test/bdd/features/project_resource_parity.feature`

## Package Test Traceability

- `agm/internal/projectresourceparity/surfaces_test.go`
