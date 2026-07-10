# Phase Engram Registry Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**PHASEENGRAM-01** When a known Wayfinder phase is resolved, the system shall return its canonical Engram path case-insensitively.

**PHASEENGRAM-02** When an unknown phase is resolved, the system shall return an error listing the known phases.

**PHASEENGRAM-03** When a phase Engram hash is requested, the system shall derive a deterministic hash from canonical file content.

**PHASEENGRAM-04** When the configured Engram repository root is valid, the system shall prefer it over workspace discovery.

**PHASEENGRAM-05** When repository discovery starts below a worktree, the system shall locate the containing Engram repository root.

**PHASEENGRAM-06** When the resolved phase file is absent, the system shall return a contextual file error.

**PHASEENGRAM-07** While phases are resolved from any supported harness and model family, the system shall preserve the same canonical paths and hashes.

## BDD Traceability

- Feature: `agm/test/bdd/features/agent_utility_parity.feature`

## Test Traceability

- Unit package: `pkg/phaseengram`
