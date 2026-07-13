# Engram Validation Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**ENGRAM-VALIDATION-01** When an Engram file is validated, the system shall require parseable frontmatter with documented field types and values.

**ENGRAM-VALIDATION-02** When title or description metadata violates documented bounds, the system shall report a line-addressable validation error.

**ENGRAM-VALIDATION-03** When content contains unresolved context references or vague task verbs, the system shall report the corresponding evidence.

**ENGRAM-VALIDATION-04** When task-like content lacks examples or constraints, the system shall report actionable validation findings.

**ENGRAM-VALIDATION-05** When a directory is validated, the system shall return per-file findings and preserve filesystem errors.

**ENGRAM-VALIDATION-06** When content is valid Unicode, the system shall validate it without corrupting text or line positions.

**ENGRAM-VALIDATION-07** While Engrams are authored by any supported harness and model family, the system shall apply the same metadata and content rules.

## BDD Traceability

- Feature: `agm/test/bdd/features/validation_workspace_parity.feature`

## Test Traceability

- Unit package: `pkg/validation/engram`
