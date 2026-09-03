# Deepsec Configuration Specification

<!-- Last audited at: 2026-07-10 -->
<!-- DEEPSEC-03 verified 2026-08-29 against the tracked-path boundary and
     agm/test/bdd/steps/deepsec_output_contract_test.go. The file-wide stamp is
     deliberately NOT advanced: DEEPSEC-01/02 and the scanner runtime were not
     rechecked, and moving the date would claim they were. -->

## EARS Requirements

**DEEPSEC-01** When the repository security scanner loads its project configuration, the system shall select only declared source roots and matcher settings.

**DEEPSEC-02** If scanner configuration is invalid, the system shall fail before reporting a successful security scan.

**DEEPSEC-03** When a documented Deepsec scan writes generated output, the Git boundary shall leave the worktree clean while keeping user-maintained Deepsec inputs trackable. The exact generated and tracked path inventory is implementation detail owned by `README.md` and `agm/test/bdd/steps/deepsec_output_contract_test.go`.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
- Package test: `agm/test/bdd/steps/deepsec_output_contract_test.go`
